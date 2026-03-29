package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/platform/security"
)

func TestLoginCreatesSessionAndReturnsTokens(t *testing.T) {
	service, repository, sessions := newTestService(t)

	result, err := service.Login(context.Background(), LoginInput{
		Login:    "dev@example.com",
		Password: "DevDemo123!",
		RequestMetadata: RequestMetadata{
			IPAddress: "127.0.0.1",
			UserAgent: "test-agent",
		},
	})
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}

	if result.AccessToken == "" || result.RefreshToken == "" {
		t.Fatal("expected login to return access and refresh tokens")
	}

	if result.User.Role != RoleDev {
		t.Fatalf("Role = %s, want %s", result.User.Role, RoleDev)
	}

	if _, found := sessions.states[result.SessionJTI]; !found {
		t.Fatal("expected redis session state to be stored")
	}

	stored, ok := repository.sessions[result.SessionJTI]
	if !ok {
		t.Fatal("expected session to exist in repository")
	}

	if stored.RevokedAt != nil {
		t.Fatal("new session should not be revoked")
	}
}

func TestLoginRevokesPreviousSession(t *testing.T) {
	service, repository, _ := newTestService(t)

	first, err := service.Login(context.Background(), LoginInput{
		Login:    "owner-demo",
		Password: "OwnerDemo123!",
	})
	if err != nil {
		t.Fatalf("first login returned error: %v", err)
	}

	second, err := service.Login(context.Background(), LoginInput{
		Login:    "owner@example.com",
		Password: "OwnerDemo123!",
	})
	if err != nil {
		t.Fatalf("second login returned error: %v", err)
	}

	if first.SessionJTI == second.SessionJTI {
		t.Fatal("expected second login to rotate session jti")
	}

	firstSession := repository.archived[first.SessionJTI]
	if firstSession.RevokedAt == nil {
		t.Fatal("expected first session to be revoked after second login")
	}

	if _, err := service.AuthenticateAccessToken(context.Background(), first.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("AuthenticateAccessToken(first) error = %v, want unauthorized", err)
	}

	subject, err := service.AuthenticateAccessToken(context.Background(), second.AccessToken)
	if err != nil {
		t.Fatalf("AuthenticateAccessToken(second) returned error: %v", err)
	}

	if subject.UserID != "user-owner" {
		t.Fatalf("Subject.UserID = %s, want user-owner", subject.UserID)
	}
}

func TestRefreshRotatesSessionAndInvalidatesOldTokens(t *testing.T) {
	service, repository, _ := newTestService(t)

	loginResult, err := service.Login(context.Background(), LoginInput{
		Login:    "owner@example.com",
		Password: "OwnerDemo123!",
	})
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}

	refreshResult, err := service.Refresh(context.Background(), loginResult.RefreshToken, RequestMetadata{
		IPAddress: "127.0.0.1",
		UserAgent: "refresh-agent",
	})
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}

	if refreshResult.SessionJTI == loginResult.SessionJTI {
		t.Fatal("expected refresh to rotate session jti")
	}

	if _, exists := repository.sessions[loginResult.SessionJTI]; exists {
		t.Fatal("expected old session key to be removed from repository after refresh")
	}

	if _, err := service.AuthenticateAccessToken(context.Background(), loginResult.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("AuthenticateAccessToken(old) error = %v, want unauthorized", err)
	}

	if _, err := service.Refresh(context.Background(), loginResult.RefreshToken, RequestMetadata{}); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("Refresh(old) error = %v, want invalid refresh token", err)
	}

	subject, err := service.AuthenticateAccessToken(context.Background(), refreshResult.AccessToken)
	if err != nil {
		t.Fatalf("AuthenticateAccessToken(new) returned error: %v", err)
	}

	if subject.SessionJTI != refreshResult.SessionJTI {
		t.Fatalf("Subject.SessionJTI = %s, want %s", subject.SessionJTI, refreshResult.SessionJTI)
	}
}

func TestLogoutRevokesCurrentSession(t *testing.T) {
	service, _, _ := newTestService(t)

	loginResult, err := service.Login(context.Background(), LoginInput{
		Login:    "dev-demo",
		Password: "DevDemo123!",
	})
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}

	subject, err := service.AuthenticateAccessToken(context.Background(), loginResult.AccessToken)
	if err != nil {
		t.Fatalf("AuthenticateAccessToken returned error: %v", err)
	}

	if err := service.Logout(context.Background(), subject, RequestMetadata{
		IPAddress: "127.0.0.1",
		UserAgent: "logout-agent",
	}); err != nil {
		t.Fatalf("Logout returned error: %v", err)
	}

	if _, err := service.AuthenticateAccessToken(context.Background(), loginResult.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("AuthenticateAccessToken(after logout) error = %v, want unauthorized", err)
	}
}

func newTestService(t *testing.T) (Service, *fakeRepository, *fakeSessionStore) {
	t.Helper()

	hasher := security.NewPasswordHasher(4)
	devHash, err := hasher.Hash("DevDemo123!")
	if err != nil {
		t.Fatalf("Hash(dev) returned error: %v", err)
	}

	ownerHash, err := hasher.Hash("OwnerDemo123!")
	if err != nil {
		t.Fatalf("Hash(owner) returned error: %v", err)
	}

	now := time.Now().UTC()
	repository := &fakeRepository{
		usersByID: map[string]User{
			"user-dev": {
				ID:           "user-dev",
				Email:        "dev@example.com",
				Username:     "dev-demo",
				PasswordHash: devHash,
				Role:         RoleDev,
				IsActive:     true,
			},
			"user-owner": {
				ID:           "user-owner",
				Email:        "owner@example.com",
				Username:     "owner-demo",
				PasswordHash: ownerHash,
				Role:         RoleOwner,
				IsActive:     true,
			},
		},
		sessions: map[string]SessionRecord{},
		archived: map[string]SessionRecord{},
	}

	sessions := &fakeSessionStore{
		states: map[string]SessionState{},
	}

	return NewService(Options{
		Repository: repository,
		Sessions:   sessions,
		Passwords:  hasher,
		Tokens:     security.NewAccessTokenManager("test-secret", time.Hour, "onixggr-test", testClock{now: now}),
		Clock:      testClock{now: now},
		SessionTTL: 7 * 24 * time.Hour,
	}), repository, sessions
}

type testClock struct {
	now time.Time
}

func (c testClock) Now() time.Time {
	return c.now
}

type fakeRepository struct {
	usersByID map[string]User
	sessions  map[string]SessionRecord
	archived  map[string]SessionRecord
}

func (r *fakeRepository) FindUserByLogin(_ context.Context, login string) (User, error) {
	for _, user := range r.usersByID {
		if user.Email == login || user.Username == login {
			return user, nil
		}
	}

	return User{}, ErrNotFound
}

func (r *fakeRepository) FindUserByID(_ context.Context, userID string) (User, error) {
	user, ok := r.usersByID[userID]
	if !ok {
		return User{}, ErrNotFound
	}

	return user, nil
}

func (r *fakeRepository) ReplaceUserSessions(_ context.Context, params ReplaceUserSessionsParams) (ReplaceUserSessionsResult, error) {
	var revoked []string
	for sessionJTI, session := range r.sessions {
		if session.UserID != params.UserID || session.RevokedAt != nil {
			continue
		}

		revokedAt := params.OccurredAt
		session.RevokedAt = &revokedAt
		session.UpdatedAt = params.OccurredAt
		r.archived[sessionJTI] = session
		delete(r.sessions, sessionJTI)
		revoked = append(revoked, sessionJTI)
	}

	session := SessionRecord{
		ID:          "session-" + params.SessionJTI,
		UserID:      params.UserID,
		SessionJTI:  params.SessionJTI,
		RefreshHash: params.RefreshHash,
		ExpiresAt:   params.ExpiresAt,
		CreatedAt:   params.OccurredAt,
		UpdatedAt:   params.OccurredAt,
	}
	r.sessions[session.SessionJTI] = session

	user := r.usersByID[params.UserID]
	user.LastLoginAt = &params.OccurredAt
	r.usersByID[params.UserID] = user

	return ReplaceUserSessionsResult{
		Session:     session,
		RevokedJTIs: revoked,
	}, nil
}

func (r *fakeRepository) GetSessionForRefresh(_ context.Context, sessionJTI string) (SessionWithUser, error) {
	session, ok := r.sessions[sessionJTI]
	if !ok {
		return SessionWithUser{}, ErrNotFound
	}

	user := r.usersByID[session.UserID]
	return SessionWithUser{
		Session: session,
		User:    user,
	}, nil
}

func (r *fakeRepository) RotateSession(_ context.Context, params RotateSessionParams) (SessionRecord, error) {
	session, ok := r.sessions[params.OldSessionJTI]
	if !ok || session.RevokedAt != nil {
		return SessionRecord{}, ErrNotFound
	}

	delete(r.sessions, params.OldSessionJTI)
	session.SessionJTI = params.NewSessionJTI
	session.RefreshHash = params.RefreshHash
	session.ExpiresAt = params.ExpiresAt
	session.UpdatedAt = params.OccurredAt
	r.sessions[session.SessionJTI] = session
	r.archived[params.OldSessionJTI] = session

	return session, nil
}

func (r *fakeRepository) RevokeSession(_ context.Context, params RevokeSessionParams) error {
	session, ok := r.sessions[params.SessionJTI]
	if !ok {
		return nil
	}

	revokedAt := params.OccurredAt
	session.RevokedAt = &revokedAt
	session.UpdatedAt = params.OccurredAt
	r.archived[params.SessionJTI] = session
	delete(r.sessions, params.SessionJTI)
	return nil
}

func (r *fakeRepository) RevokeAllSessions(_ context.Context, params RevokeAllSessionsParams) ([]string, error) {
	var revoked []string
	for sessionJTI, session := range r.sessions {
		if session.UserID != params.UserID || session.RevokedAt != nil {
			continue
		}

		revokedAt := params.OccurredAt
		session.RevokedAt = &revokedAt
		session.UpdatedAt = params.OccurredAt
		r.archived[sessionJTI] = session
		delete(r.sessions, sessionJTI)
		revoked = append(revoked, sessionJTI)
	}

	return revoked, nil
}

type fakeSessionStore struct {
	states map[string]SessionState
}

func (s *fakeSessionStore) Save(_ context.Context, state SessionState, _ time.Duration) error {
	s.states[state.SessionJTI] = state
	return nil
}

func (s *fakeSessionStore) Get(_ context.Context, sessionJTI string) (SessionState, error) {
	state, ok := s.states[sessionJTI]
	if !ok {
		return SessionState{}, ErrNotFound
	}

	return state, nil
}

func (s *fakeSessionStore) Delete(_ context.Context, sessionJTI string) error {
	delete(s.states, sessionJTI)
	return nil
}

func (s *fakeSessionStore) DeleteMany(_ context.Context, sessionJTIs []string) error {
	for _, sessionJTI := range sessionJTIs {
		delete(s.states, sessionJTI)
	}

	return nil
}

func (r *fakeRepository) String() string {
	return fmt.Sprintf("users=%d sessions=%d", len(r.usersByID), len(r.sessions))
}
