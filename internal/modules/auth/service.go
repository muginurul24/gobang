package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
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
	ListActiveRecoveryCodes(ctx context.Context, userID string) ([]RecoveryCodeRecord, error)
	UseRecoveryCode(ctx context.Context, recoveryCodeID string, occurredAt time.Time) error
	EnableTOTP(ctx context.Context, params EnableTOTPParams) error
	DisableTOTP(ctx context.Context, params DisableTOTPParams) error
	UpdateIPAllowlist(ctx context.Context, params UpdateIPAllowlistParams) error
	InsertAuditLog(ctx context.Context, params AuditLogParams) error
}

type PasswordManager interface {
	Hash(password string) (string, error)
	Compare(hash string, password string) error
}

type TokenManager interface {
	Issue(userID string, role string, sessionJTI string) (string, time.Time, error)
	Parse(raw string) (security.AccessTokenClaims, error)
}

type SecretSealer interface {
	Seal(plain string) (string, error)
	Open(cipherText string) (string, error)
}

type TOTPProvider interface {
	GenerateSecret() (string, error)
	Verify(secret string, code string, at time.Time) bool
	OtpauthURL(accountName string, secret string) string
}

type Service interface {
	Login(ctx context.Context, input LoginInput) (AuthSession, error)
	Refresh(ctx context.Context, refreshToken string, metadata RequestMetadata) (AuthSession, error)
	Logout(ctx context.Context, subject Subject, metadata RequestMetadata) error
	LogoutAll(ctx context.Context, subject Subject, metadata RequestMetadata) (int, error)
	Me(ctx context.Context, subject Subject) (UserProfile, error)
	GetSecuritySettings(ctx context.Context, subject Subject) (SecuritySettings, error)
	BeginTOTPEnrollment(ctx context.Context, subject Subject) (TOTPEnrollment, error)
	EnableTOTP(ctx context.Context, subject Subject, input EnableTOTPInput) (RecoveryCodesPayload, error)
	DisableTOTP(ctx context.Context, subject Subject, input DisableTOTPInput) error
	UpdateIPAllowlist(ctx context.Context, subject Subject, input UpdateIPAllowlistInput) (SecuritySettings, error)
	AuthenticateAccessToken(ctx context.Context, rawToken string) (Subject, error)
}

type Options struct {
	Repository        RepositoryStore
	Sessions          SessionStore
	Enrollments       EnrollmentStore
	Limiter           LoginLimiter
	Passwords         PasswordManager
	Tokens            TokenManager
	Sealer            SecretSealer
	TwoFactor         TOTPProvider
	Clock             clock.Clock
	SessionTTL        time.Duration
	TOTPEnrollmentTTL time.Duration
}

type service struct {
	repository        RepositoryStore
	sessions          SessionStore
	enrollments       EnrollmentStore
	limiter           LoginLimiter
	passwords         PasswordManager
	tokens            TokenManager
	sealer            SecretSealer
	twoFactor         TOTPProvider
	clock             clock.Clock
	sessionTTL        time.Duration
	totpEnrollmentTTL time.Duration
}

func NewService(options Options) Service {
	now := options.Clock
	if now == nil {
		now = clock.SystemClock{}
	}

	return &service{
		repository:        options.Repository,
		sessions:          options.Sessions,
		enrollments:       options.Enrollments,
		limiter:           options.Limiter,
		passwords:         options.Passwords,
		tokens:            options.Tokens,
		sealer:            options.Sealer,
		twoFactor:         options.TwoFactor,
		clock:             now,
		sessionTTL:        options.SessionTTL,
		totpEnrollmentTTL: options.TOTPEnrollmentTTL,
	}
}

func (s *service) Login(ctx context.Context, input LoginInput) (AuthSession, error) {
	identifier := normalizeLogin(input.Login)
	limit, err := s.limiter.Check(ctx, input.IPAddress, identifier)
	if err != nil {
		return AuthSession{}, fmt.Errorf("check login limiter: %w", err)
	}

	if limit.Limited {
		_ = s.repository.InsertAuditLog(ctx, AuditLogParams{
			ActorRole:  "guest",
			TargetType: "auth",
			Action:     "auth.login_rate_limited",
			Payload: map[string]any{
				"scope":      limit.Scope,
				"identifier": maskIdentifier(identifier),
			},
			IPAddress:  input.IPAddress,
			UserAgent:  input.UserAgent,
			OccurredAt: s.clock.Now().UTC(),
		})

		return AuthSession{}, &RateLimitError{
			Scope:      limit.Scope,
			RetryAfter: int(limit.RetryAfter.Seconds()),
		}
	}

	user, err := s.repository.FindUserByLogin(ctx, identifier)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			if err := s.failedLogin(ctx, nil, identifier, input.RequestMetadata, "invalid_credentials"); err != nil {
				return AuthSession{}, err
			}

			return AuthSession{}, ErrInvalidCredentials
		}

		return AuthSession{}, fmt.Errorf("find login user: %w", err)
	}

	if !user.IsActive {
		if err := s.failedLogin(ctx, &user, identifier, input.RequestMetadata, "inactive_user"); err != nil {
			return AuthSession{}, err
		}

		return AuthSession{}, ErrInvalidCredentials
	}

	if user.IPAllowlist != nil && *user.IPAllowlist != "" && *user.IPAllowlist != input.IPAddress {
		if err := s.failedLogin(ctx, &user, identifier, input.RequestMetadata, "ip_not_allowed"); err != nil {
			return AuthSession{}, err
		}

		return AuthSession{}, ErrIPNotAllowed
	}

	if err := s.passwords.Compare(user.PasswordHash, input.Password); err != nil {
		if err := s.failedLogin(ctx, &user, identifier, input.RequestMetadata, "invalid_credentials"); err != nil {
			return AuthSession{}, err
		}

		return AuthSession{}, ErrInvalidCredentials
	}

	now := s.clock.Now().UTC()
	if user.TOTPEnabled {
		if err := s.verifySecondFactor(ctx, user, input, now); err != nil {
			if errors.Is(err, ErrSecondFactorRequired) {
				_ = s.repository.InsertAuditLog(ctx, AuditLogParams{
					ActorUserID: &user.ID,
					ActorRole:   string(user.Role),
					TargetType:  "user",
					TargetID:    &user.ID,
					Action:      "auth.login_2fa_required",
					Payload: map[string]any{
						"identifier": maskIdentifier(identifier),
					},
					IPAddress:  input.IPAddress,
					UserAgent:  input.UserAgent,
					OccurredAt: now,
				})

				return AuthSession{}, ErrSecondFactorRequired
			}

			if errors.Is(err, ErrInvalidSecondFactor) {
				if err := s.failedLogin(ctx, &user, identifier, input.RequestMetadata, "invalid_second_factor"); err != nil {
					return AuthSession{}, err
				}

				return AuthSession{}, ErrInvalidSecondFactor
			}

			return AuthSession{}, err
		}
	}

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

	if err := s.limiter.Reset(ctx, input.IPAddress, identifier); err != nil {
		return AuthSession{}, fmt.Errorf("reset login limiter: %w", err)
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

func (s *service) GetSecuritySettings(ctx context.Context, subject Subject) (SecuritySettings, error) {
	user, err := s.repository.FindUserByID(ctx, subject.UserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return SecuritySettings{}, ErrUnauthorized
		}

		return SecuritySettings{}, fmt.Errorf("find current user for security settings: %w", err)
	}

	if !user.IsActive {
		return SecuritySettings{}, ErrUnauthorized
	}

	return securitySettings(user), nil
}

func (s *service) BeginTOTPEnrollment(ctx context.Context, subject Subject) (TOTPEnrollment, error) {
	user, err := s.repository.FindUserByID(ctx, subject.UserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return TOTPEnrollment{}, ErrUnauthorized
		}

		return TOTPEnrollment{}, fmt.Errorf("find user for totp enrollment: %w", err)
	}

	if !user.IsActive {
		return TOTPEnrollment{}, ErrUnauthorized
	}

	if user.TOTPEnabled {
		return TOTPEnrollment{}, ErrTOTPAlreadyEnabled
	}

	secret, err := s.twoFactor.GenerateSecret()
	if err != nil {
		return TOTPEnrollment{}, fmt.Errorf("generate totp secret: %w", err)
	}

	if err := s.enrollments.Save(ctx, user.ID, secret, s.totpEnrollmentTTL); err != nil {
		return TOTPEnrollment{}, fmt.Errorf("save enrollment secret: %w", err)
	}

	accountName := user.Email
	if strings.TrimSpace(accountName) == "" {
		accountName = user.Username
	}

	expiresAt := s.clock.Now().UTC().Add(s.totpEnrollmentTTL)
	return TOTPEnrollment{
		Secret:     secret,
		OtpAuthURL: s.twoFactor.OtpauthURL(accountName, secret),
		ExpiresAt:  expiresAt,
	}, nil
}

func (s *service) EnableTOTP(ctx context.Context, subject Subject, input EnableTOTPInput) (RecoveryCodesPayload, error) {
	user, err := s.repository.FindUserByID(ctx, subject.UserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return RecoveryCodesPayload{}, ErrUnauthorized
		}

		return RecoveryCodesPayload{}, fmt.Errorf("find user for enable totp: %w", err)
	}

	if !user.IsActive {
		return RecoveryCodesPayload{}, ErrUnauthorized
	}

	if user.TOTPEnabled {
		return RecoveryCodesPayload{}, ErrTOTPAlreadyEnabled
	}

	secret, err := s.enrollments.Get(ctx, user.ID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return RecoveryCodesPayload{}, ErrNoPendingEnrollment
		}

		return RecoveryCodesPayload{}, fmt.Errorf("get pending enrollment secret: %w", err)
	}

	now := s.clock.Now().UTC()
	if !s.twoFactor.Verify(secret, input.Code, now) {
		return RecoveryCodesPayload{}, ErrInvalidSecondFactor
	}

	encryptedSecret, err := s.sealer.Seal(secret)
	if err != nil {
		return RecoveryCodesPayload{}, fmt.Errorf("seal totp secret: %w", err)
	}

	plainRecoveryCodes, recoveryCodeHashes, err := s.generateRecoveryCodes()
	if err != nil {
		return RecoveryCodesPayload{}, fmt.Errorf("generate recovery codes: %w", err)
	}

	if err := s.repository.EnableTOTP(ctx, EnableTOTPParams{
		UserID:             user.ID,
		ActorRole:          subject.Role,
		EncryptedSecret:    encryptedSecret,
		RecoveryCodeHashes: recoveryCodeHashes,
		IPAddress:          input.IPAddress,
		UserAgent:          input.UserAgent,
		OccurredAt:         now,
	}); err != nil {
		return RecoveryCodesPayload{}, fmt.Errorf("persist totp enablement: %w", err)
	}

	if err := s.enrollments.Delete(ctx, user.ID); err != nil {
		return RecoveryCodesPayload{}, fmt.Errorf("delete pending enrollment secret: %w", err)
	}

	return RecoveryCodesPayload{Codes: plainRecoveryCodes}, nil
}

func (s *service) DisableTOTP(ctx context.Context, subject Subject, input DisableTOTPInput) error {
	user, err := s.repository.FindUserByID(ctx, subject.UserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrUnauthorized
		}

		return fmt.Errorf("find user for disable totp: %w", err)
	}

	if !user.IsActive {
		return ErrUnauthorized
	}

	if !user.TOTPEnabled {
		return ErrTOTPNotEnabled
	}

	now := s.clock.Now().UTC()
	if err := s.verifyTOTPOrRecoveryCode(ctx, user, input.TOTPCode, input.RecoveryCode, now); err != nil {
		return err
	}

	if err := s.repository.DisableTOTP(ctx, DisableTOTPParams{
		UserID:     user.ID,
		ActorRole:  subject.Role,
		IPAddress:  input.IPAddress,
		UserAgent:  input.UserAgent,
		OccurredAt: now,
	}); err != nil {
		return fmt.Errorf("persist totp disablement: %w", err)
	}

	return nil
}

func (s *service) UpdateIPAllowlist(ctx context.Context, subject Subject, input UpdateIPAllowlistInput) (SecuritySettings, error) {
	user, err := s.repository.FindUserByID(ctx, subject.UserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return SecuritySettings{}, ErrUnauthorized
		}

		return SecuritySettings{}, fmt.Errorf("find user for ip allowlist: %w", err)
	}

	if !user.IsActive {
		return SecuritySettings{}, ErrUnauthorized
	}

	var normalized *string
	if input.IPAllowlist != nil {
		trimmed := strings.TrimSpace(*input.IPAllowlist)
		if trimmed != "" {
			parsed := net.ParseIP(trimmed)
			if parsed == nil {
				return SecuritySettings{}, ErrInvalidIPAllowlist
			}

			value := parsed.String()
			normalized = &value
		}
	}

	if err := s.repository.UpdateIPAllowlist(ctx, UpdateIPAllowlistParams{
		UserID:      user.ID,
		ActorRole:   subject.Role,
		IPAllowlist: normalized,
		IPAddress:   input.IPAddress,
		UserAgent:   input.UserAgent,
		OccurredAt:  s.clock.Now().UTC(),
	}); err != nil {
		return SecuritySettings{}, fmt.Errorf("update ip allowlist: %w", err)
	}

	user.IPAllowlist = normalized
	return securitySettings(user), nil
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

func (s *service) verifySecondFactor(ctx context.Context, user User, input LoginInput, now time.Time) error {
	return s.verifyTOTPOrRecoveryCode(ctx, user, input.TOTPCode, input.RecoveryCode, now)
}

func (s *service) verifyTOTPOrRecoveryCode(ctx context.Context, user User, totpCode string, recoveryCode string, now time.Time) error {
	if strings.TrimSpace(totpCode) == "" && strings.TrimSpace(recoveryCode) == "" {
		return ErrSecondFactorRequired
	}

	if strings.TrimSpace(recoveryCode) != "" {
		used, err := s.consumeRecoveryCode(ctx, user.ID, recoveryCode, now)
		if err != nil {
			return err
		}

		if used {
			return nil
		}

		return ErrInvalidSecondFactor
	}

	secret, err := s.sealer.Open(user.TOTPSecretEncrypted)
	if err != nil {
		return fmt.Errorf("open totp secret: %w", err)
	}

	if !s.twoFactor.Verify(secret, totpCode, now) {
		return ErrInvalidSecondFactor
	}

	return nil
}

func (s *service) consumeRecoveryCode(ctx context.Context, userID string, recoveryCode string, occurredAt time.Time) (bool, error) {
	records, err := s.repository.ListActiveRecoveryCodes(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("list active recovery codes: %w", err)
	}

	normalized := normalizeRecoveryCode(recoveryCode)
	for _, record := range records {
		if err := s.passwords.Compare(record.CodeHash, normalized); err == nil {
			if err := s.repository.UseRecoveryCode(ctx, record.ID, occurredAt); err != nil {
				if errors.Is(err, ErrNotFound) {
					return false, ErrInvalidSecondFactor
				}

				return false, fmt.Errorf("use recovery code: %w", err)
			}

			return true, nil
		}
	}

	return false, nil
}

func (s *service) failedLogin(ctx context.Context, user *User, identifier string, input RequestMetadata, reason string) error {
	if err := s.limiter.RegisterFailure(ctx, input.IPAddress, identifier); err != nil {
		return fmt.Errorf("register login limiter failure: %w", err)
	}

	payload := map[string]any{
		"identifier": maskIdentifier(identifier),
		"reason":     reason,
	}

	params := AuditLogParams{
		ActorRole:  "guest",
		TargetType: "auth",
		Action:     "auth.login_failed",
		Payload:    payload,
		IPAddress:  input.IPAddress,
		UserAgent:  input.UserAgent,
		OccurredAt: s.clock.Now().UTC(),
	}

	if user != nil {
		params.ActorUserID = &user.ID
		params.ActorRole = string(user.Role)
		params.TargetType = "user"
		params.TargetID = &user.ID
	}

	if err := s.repository.InsertAuditLog(ctx, params); err != nil {
		return fmt.Errorf("insert failed login audit log: %w", err)
	}

	return nil
}

func (s *service) generateRecoveryCodes() ([]string, []string, error) {
	codes := make([]string, 0, 8)
	hashes := make([]string, 0, 8)

	for index := 0; index < 8; index++ {
		code, err := newRecoveryCode()
		if err != nil {
			return nil, nil, err
		}

		hash, err := s.passwords.Hash(normalizeRecoveryCode(code))
		if err != nil {
			return nil, nil, err
		}

		codes = append(codes, code)
		hashes = append(hashes, hash)
	}

	return codes, hashes, nil
}

func securitySettings(user User) SecuritySettings {
	return SecuritySettings{
		UserID:         user.ID,
		TOTPEnabled:    user.TOTPEnabled,
		IPAllowlist:    user.IPAllowlist,
		Recommended2FA: !user.TOTPEnabled,
	}
}

func toUserProfile(user User) UserProfile {
	return UserProfile{
		ID:          user.ID,
		Email:       user.Email,
		Username:    user.Username,
		Role:        user.Role,
		TOTPEnabled: user.TOTPEnabled,
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
