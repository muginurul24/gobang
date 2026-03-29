package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/platform/clock"
	"github.com/mugiew/onixggr/internal/platform/security"
)

type RepositoryStore interface {
	FindUserByLogin(ctx context.Context, login string) (User, error)
	FindUserByID(ctx context.Context, userID string) (User, error)
	ReplaceUserSessions(ctx context.Context, params ReplaceUserSessionsParams) (ReplaceUserSessionsResult, error)
	GetSessionForRefresh(ctx context.Context, sessionJTI string) (SessionWithUser, error)
	RotateSession(ctx context.Context, params RotateSessionParams) (SessionRecord, error)
	RevokeSession(ctx context.Context, params RevokeSessionParams) error
	RevokeAllSessions(ctx context.Context, params RevokeAllSessionsParams) ([]string, error)
}

type PasswordVerifier interface {
	Compare(hash string, password string) error
}

type TokenManager interface {
	Issue(userID string, role string, sessionJTI string) (string, time.Time, error)
	Parse(raw string) (security.AccessTokenClaims, error)
}

type Service interface {
	Login(ctx context.Context, input LoginInput) (AuthSession, error)
	Refresh(ctx context.Context, refreshToken string, metadata RequestMetadata) (AuthSession, error)
	Logout(ctx context.Context, subject Subject, metadata RequestMetadata) error
	LogoutAll(ctx context.Context, subject Subject, metadata RequestMetadata) (int, error)
	Me(ctx context.Context, subject Subject) (UserProfile, error)
	AuthenticateAccessToken(ctx context.Context, rawToken string) (Subject, error)
}

type Options struct {
	Repository RepositoryStore
	Sessions   SessionStore
	Passwords  PasswordVerifier
	Tokens     TokenManager
	Clock      clock.Clock
	SessionTTL time.Duration
}

type service struct {
	repository RepositoryStore
	sessions   SessionStore
	passwords  PasswordVerifier
	tokens     TokenManager
	clock      clock.Clock
	sessionTTL time.Duration
}

func NewService(options Options) Service {
	now := options.Clock
	if now == nil {
		now = clock.SystemClock{}
	}

	return &service{
		repository: options.Repository,
		sessions:   options.Sessions,
		passwords:  options.Passwords,
		tokens:     options.Tokens,
		clock:      now,
		sessionTTL: options.SessionTTL,
	}
}

func (s *service) Login(ctx context.Context, input LoginInput) (AuthSession, error) {
	user, err := s.repository.FindUserByLogin(ctx, strings.TrimSpace(input.Login))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return AuthSession{}, ErrInvalidCredentials
		}

		return AuthSession{}, fmt.Errorf("find login user: %w", err)
	}

	if !user.IsActive {
		return AuthSession{}, ErrInvalidCredentials
	}

	if err := s.passwords.Compare(user.PasswordHash, input.Password); err != nil {
		return AuthSession{}, ErrInvalidCredentials
	}

	now := s.clock.Now().UTC()
	sessionJTI, err := newSessionJTI()
	if err != nil {
		return AuthSession{}, fmt.Errorf("generate session jti: %w", err)
	}

	refreshToken, refreshHash, err := newRefreshToken(sessionJTI)
	if err != nil {
		return AuthSession{}, fmt.Errorf("generate refresh token: %w", err)
	}

	expiresAt := now.Add(s.sessionTTL)
	result, err := s.repository.ReplaceUserSessions(ctx, ReplaceUserSessionsParams{
		UserID:      user.ID,
		ActorRole:   user.Role,
		SessionJTI:  sessionJTI,
		RefreshHash: refreshHash,
		IPAddress:   input.IPAddress,
		UserAgent:   input.UserAgent,
		ExpiresAt:   expiresAt,
		OccurredAt:  now,
	})
	if err != nil {
		return AuthSession{}, fmt.Errorf("replace user sessions: %w", err)
	}

	if err := s.sessions.DeleteMany(ctx, result.RevokedJTIs); err != nil {
		return AuthSession{}, fmt.Errorf("delete old redis sessions: %w", err)
	}

	if err := s.sessions.Save(ctx, SessionState{
		SessionID:  result.Session.ID,
		UserID:     user.ID,
		Role:       user.Role,
		SessionJTI: result.Session.SessionJTI,
		ExpiresAt:  expiresAt,
	}, s.sessionTTL); err != nil {
		return AuthSession{}, fmt.Errorf("save redis session: %w", err)
	}

	accessToken, accessTokenExpiresAt, err := s.tokens.Issue(user.ID, string(user.Role), result.Session.SessionJTI)
	if err != nil {
		return AuthSession{}, fmt.Errorf("issue access token: %w", err)
	}

	user.LastLoginAt = &now

	return AuthSession{
		User:                  toUserProfile(user),
		TokenType:             "Bearer",
		AccessToken:           accessToken,
		AccessTokenExpiresAt:  accessTokenExpiresAt,
		RefreshToken:          refreshToken,
		RefreshTokenExpiresAt: expiresAt,
		SessionJTI:            result.Session.SessionJTI,
	}, nil
}

func (s *service) Refresh(ctx context.Context, refreshToken string, metadata RequestMetadata) (AuthSession, error) {
	currentJTI, secret, err := splitRefreshToken(refreshToken)
	if err != nil {
		return AuthSession{}, ErrInvalidRefreshToken
	}

	record, err := s.repository.GetSessionForRefresh(ctx, currentJTI)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return AuthSession{}, ErrInvalidRefreshToken
		}

		return AuthSession{}, fmt.Errorf("get session for refresh: %w", err)
	}

	if !record.User.IsActive {
		return AuthSession{}, ErrUnauthorized
	}

	now := s.clock.Now().UTC()
	if record.Session.RevokedAt != nil || !record.Session.ExpiresAt.After(now) {
		return AuthSession{}, ErrInvalidRefreshToken
	}

	if hashRefreshSecret(secret) != record.Session.RefreshHash {
		return AuthSession{}, ErrInvalidRefreshToken
	}

	newSessionJTI, err := newSessionJTI()
	if err != nil {
		return AuthSession{}, fmt.Errorf("generate replacement session jti: %w", err)
	}

	newRefreshToken, newRefreshHash, err := newRefreshToken(newSessionJTI)
	if err != nil {
		return AuthSession{}, fmt.Errorf("generate rotated refresh token: %w", err)
	}

	expiresAt := now.Add(s.sessionTTL)
	session, err := s.repository.RotateSession(ctx, RotateSessionParams{
		OldSessionJTI: currentJTI,
		NewSessionJTI: newSessionJTI,
		RefreshHash:   newRefreshHash,
		IPAddress:     metadata.IPAddress,
		UserAgent:     metadata.UserAgent,
		ExpiresAt:     expiresAt,
		OccurredAt:    now,
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return AuthSession{}, ErrInvalidRefreshToken
		}

		return AuthSession{}, fmt.Errorf("rotate session: %w", err)
	}

	if err := s.sessions.Delete(ctx, currentJTI); err != nil {
		return AuthSession{}, fmt.Errorf("delete old redis session: %w", err)
	}

	if err := s.sessions.Save(ctx, SessionState{
		SessionID:  session.ID,
		UserID:     record.User.ID,
		Role:       record.User.Role,
		SessionJTI: session.SessionJTI,
		ExpiresAt:  expiresAt,
	}, s.sessionTTL); err != nil {
		return AuthSession{}, fmt.Errorf("save rotated redis session: %w", err)
	}

	accessToken, accessTokenExpiresAt, err := s.tokens.Issue(record.User.ID, string(record.User.Role), session.SessionJTI)
	if err != nil {
		return AuthSession{}, fmt.Errorf("issue rotated access token: %w", err)
	}

	return AuthSession{
		User:                  toUserProfile(record.User),
		TokenType:             "Bearer",
		AccessToken:           accessToken,
		AccessTokenExpiresAt:  accessTokenExpiresAt,
		RefreshToken:          newRefreshToken,
		RefreshTokenExpiresAt: expiresAt,
		SessionJTI:            session.SessionJTI,
	}, nil
}

func (s *service) Logout(ctx context.Context, subject Subject, metadata RequestMetadata) error {
	if err := s.repository.RevokeSession(ctx, RevokeSessionParams{
		UserID:     subject.UserID,
		SessionJTI: subject.SessionJTI,
		ActorRole:  subject.Role,
		IPAddress:  metadata.IPAddress,
		UserAgent:  metadata.UserAgent,
		OccurredAt: s.clock.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("revoke current session: %w", err)
	}

	if err := s.sessions.Delete(ctx, subject.SessionJTI); err != nil {
		return fmt.Errorf("delete current redis session: %w", err)
	}

	return nil
}

func (s *service) LogoutAll(ctx context.Context, subject Subject, metadata RequestMetadata) (int, error) {
	revokedJTIs, err := s.repository.RevokeAllSessions(ctx, RevokeAllSessionsParams{
		UserID:     subject.UserID,
		ActorRole:  subject.Role,
		IPAddress:  metadata.IPAddress,
		UserAgent:  metadata.UserAgent,
		OccurredAt: s.clock.Now().UTC(),
	})
	if err != nil {
		return 0, fmt.Errorf("revoke all sessions: %w", err)
	}

	if err := s.sessions.DeleteMany(ctx, revokedJTIs); err != nil {
		return 0, fmt.Errorf("delete redis sessions from logout all: %w", err)
	}

	return len(revokedJTIs), nil
}

func (s *service) Me(ctx context.Context, subject Subject) (UserProfile, error) {
	user, err := s.repository.FindUserByID(ctx, subject.UserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return UserProfile{}, ErrUnauthorized
		}

		return UserProfile{}, fmt.Errorf("find current user: %w", err)
	}

	if !user.IsActive {
		return UserProfile{}, ErrUnauthorized
	}

	return toUserProfile(user), nil
}

func (s *service) AuthenticateAccessToken(ctx context.Context, rawToken string) (Subject, error) {
	claims, err := s.tokens.Parse(rawToken)
	if err != nil {
		return Subject{}, ErrUnauthorized
	}

	state, err := s.sessions.Get(ctx, claims.ID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Subject{}, ErrUnauthorized
		}

		return Subject{}, fmt.Errorf("get redis session: %w", err)
	}

	now := s.clock.Now().UTC()
	if !state.ExpiresAt.After(now) {
		_ = s.sessions.Delete(ctx, state.SessionJTI)
		return Subject{}, ErrUnauthorized
	}

	if state.UserID != claims.Subject || string(state.Role) != claims.Role || state.SessionJTI != claims.ID {
		return Subject{}, ErrUnauthorized
	}

	return Subject{
		UserID:     state.UserID,
		Role:       state.Role,
		SessionJTI: state.SessionJTI,
	}, nil
}

func toUserProfile(user User) UserProfile {
	return UserProfile{
		ID:          user.ID,
		Email:       user.Email,
		Username:    user.Username,
		Role:        user.Role,
		LastLoginAt: user.LastLoginAt,
	}
}

func newSessionJTI() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}

	return "ses_" + hex.EncodeToString(buffer), nil
}

func newRefreshToken(sessionJTI string) (string, string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", "", err
	}

	secret := base64.RawURLEncoding.EncodeToString(buffer)
	return sessionJTI + "." + secret, hashRefreshSecret(secret), nil
}

func splitRefreshToken(refreshToken string) (string, string, error) {
	sessionJTI, secret, found := strings.Cut(strings.TrimSpace(refreshToken), ".")
	if !found || sessionJTI == "" || secret == "" {
		return "", "", ErrInvalidRefreshToken
	}

	return sessionJTI, secret, nil
}

func hashRefreshSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
