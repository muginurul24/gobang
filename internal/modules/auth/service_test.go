package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/platform/security"
)

func TestLoginCreatesSessionAndReturnsTokens(t *testing.T) {
	service, repository, sessions, _, _ := newTestService(t)

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

func TestLoginRequiresSecondFactorWhenTOTPEnabled(t *testing.T) {
	service, repository, _, _, _ := newTestService(t)
	user := repository.usersByID["user-owner"]
	user.TOTPEnabled = true
	user.TOTPSecretEncrypted = "sealed:totp-owner-secret"
	repository.usersByID["user-owner"] = user

	_, err := service.Login(context.Background(), LoginInput{
		Login:    "owner@example.com",
		Password: "OwnerDemo123!",
		RequestMetadata: RequestMetadata{
			IPAddress: "127.0.0.1",
			UserAgent: "test-agent",
		},
	})
	if !errors.Is(err, ErrSecondFactorRequired) {
		t.Fatalf("Login error = %v, want second factor required", err)
	}
}

func TestLoginReplacesPreviousSessionForSameUser(t *testing.T) {
	service, repository, sessions, _, _ := newTestService(t)

	first, err := service.Login(context.Background(), LoginInput{
		Login:    "owner@example.com",
		Password: "OwnerDemo123!",
		RequestMetadata: RequestMetadata{
			IPAddress: "127.0.0.1",
			UserAgent: "test-agent-1",
		},
	})
	if err != nil {
		t.Fatalf("first Login returned error: %v", err)
	}

	second, err := service.Login(context.Background(), LoginInput{
		Login:    "owner@example.com",
		Password: "OwnerDemo123!",
		RequestMetadata: RequestMetadata{
			IPAddress: "127.0.0.2",
			UserAgent: "test-agent-2",
		},
	})
	if err != nil {
		t.Fatalf("second Login returned error: %v", err)
	}

	if first.SessionJTI == second.SessionJTI {
		t.Fatalf("SessionJTI = %q, want rotated session on second login", second.SessionJTI)
	}
	if len(repository.sessions) != 1 {
		t.Fatalf("active repository sessions = %d, want 1", len(repository.sessions))
	}
	if len(sessions.states) != 1 {
		t.Fatalf("active redis sessions = %d, want 1", len(sessions.states))
	}
	if _, ok := repository.sessions[first.SessionJTI]; ok {
		t.Fatalf("old session %q still active in repository", first.SessionJTI)
	}
	if _, ok := sessions.states[first.SessionJTI]; ok {
		t.Fatalf("old session %q still active in redis store", first.SessionJTI)
	}
	if archived, ok := repository.archived[first.SessionJTI]; !ok || archived.RevokedAt == nil {
		t.Fatalf("archived session = %#v, want revoked old session", archived)
	}
	if _, ok := repository.sessions[second.SessionJTI]; !ok {
		t.Fatalf("new session %q missing from repository", second.SessionJTI)
	}
	if _, ok := sessions.states[second.SessionJTI]; !ok {
		t.Fatalf("new session %q missing from redis store", second.SessionJTI)
	}
}

func TestLoginConsumesRecoveryCodeOnce(t *testing.T) {
	service, repository, _, _, _ := newTestService(t)
	user := repository.usersByID["user-owner"]
	user.TOTPEnabled = true
	user.TOTPSecretEncrypted = "sealed:totp-owner-secret"
	repository.usersByID["user-owner"] = user

	hasher := security.NewPasswordHasher(4)
	hash, err := hasher.Hash(normalizeRecoveryCode("ABCD3-EFGH4"))
	if err != nil {
		t.Fatalf("Hash returned error: %v", err)
	}

	repository.recoveryCodes["user-owner"] = []fakeRecoveryCode{
		{ID: "recovery-1", CodeHash: hash},
	}

	result, err := service.Login(context.Background(), LoginInput{
		Login:        "owner-demo",
		Password:     "OwnerDemo123!",
		RecoveryCode: "ABCD3-EFGH4",
		RequestMetadata: RequestMetadata{
			IPAddress: "127.0.0.1",
			UserAgent: "test-agent",
		},
	})
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}

	if result.SessionJTI == "" {
		t.Fatal("expected login to return session jti")
	}

	if !repository.recoveryCodes["user-owner"][0].Used {
		t.Fatal("expected recovery code to be marked as used")
	}

	_, err = service.Login(context.Background(), LoginInput{
		Login:        "owner-demo",
		Password:     "OwnerDemo123!",
		RecoveryCode: "ABCD3-EFGH4",
		RequestMetadata: RequestMetadata{
			IPAddress: "127.0.0.1",
			UserAgent: "test-agent",
		},
	})
	if !errors.Is(err, ErrInvalidSecondFactor) {
		t.Fatalf("second login error = %v, want invalid second factor", err)
	}
}

func TestEnableAndDisableTOTP(t *testing.T) {
	service, repository, _, enrollments, _ := newTestService(t)

	subject := Subject{UserID: "user-dev", Role: RoleDev}

	enrollment, err := service.BeginTOTPEnrollment(context.Background(), subject)
	if err != nil {
		t.Fatalf("BeginTOTPEnrollment returned error: %v", err)
	}

	if enrollment.Secret != "totp-secret-1" {
		t.Fatalf("Enrollment secret = %s, want totp-secret-1", enrollment.Secret)
	}

	if stored := enrollments.values["user-dev"]; stored != "totp-secret-1" {
		t.Fatalf("stored enrollment secret = %s, want totp-secret-1", stored)
	}

	recoveryCodes, err := service.EnableTOTP(context.Background(), subject, EnableTOTPInput{
		Code: "654321",
		RequestMetadata: RequestMetadata{
			IPAddress: "127.0.0.1",
			UserAgent: "totp-enable",
		},
	})
	if err != nil {
		t.Fatalf("EnableTOTP returned error: %v", err)
	}

	if len(recoveryCodes.Codes) != 8 {
		t.Fatalf("len(recovery codes) = %d, want 8", len(recoveryCodes.Codes))
	}

	user := repository.usersByID["user-dev"]
	if !user.TOTPEnabled {
		t.Fatal("expected user to have totp enabled")
	}

	if user.TOTPSecretEncrypted == "" {
		t.Fatal("expected encrypted totp secret to be stored")
	}

	if _, exists := enrollments.values["user-dev"]; exists {
		t.Fatal("expected pending enrollment secret to be deleted")
	}

	err = service.DisableTOTP(context.Background(), subject, DisableTOTPInput{
		TOTPCode: "654321",
		RequestMetadata: RequestMetadata{
			IPAddress: "127.0.0.1",
			UserAgent: "totp-disable",
		},
	})
	if err != nil {
		t.Fatalf("DisableTOTP returned error: %v", err)
	}

	user = repository.usersByID["user-dev"]
	if user.TOTPEnabled {
		t.Fatal("expected user to have totp disabled")
	}
}

func TestUpdateIPAllowlist(t *testing.T) {
	service, repository, _, _, _ := newTestService(t)
	subject := Subject{UserID: "user-dev", Role: RoleDev}

	settings, err := service.UpdateIPAllowlist(context.Background(), subject, UpdateIPAllowlistInput{
		IPAllowlist: ptr("10.10.10.10"),
		RequestMetadata: RequestMetadata{
			IPAddress: "127.0.0.1",
			UserAgent: "allowlist",
		},
	})
	if err != nil {
		t.Fatalf("UpdateIPAllowlist returned error: %v", err)
	}

	if settings.IPAllowlist == nil || *settings.IPAllowlist != "10.10.10.10" {
		t.Fatalf("IPAllowlist = %v, want 10.10.10.10", settings.IPAllowlist)
	}

	if repository.usersByID["user-dev"].IPAllowlist == nil {
		t.Fatal("expected repository user to persist allowlist")
	}
}

func TestLoginRateLimit(t *testing.T) {
	service, _, _, _, limiter := newTestService(t)

	for range 5 {
		_, err := service.Login(context.Background(), LoginInput{
			Login:    "missing-user",
			Password: "bad-password",
			RequestMetadata: RequestMetadata{
				IPAddress: "192.168.1.10",
				UserAgent: "rate-test",
			},
		})
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("Login error = %v, want invalid credentials during warmup", err)
		}
	}

	_, err := service.Login(context.Background(), LoginInput{
		Login:    "missing-user",
		Password: "bad-password",
		RequestMetadata: RequestMetadata{
			IPAddress: "192.168.1.10",
			UserAgent: "rate-test",
		},
	})

	var rateLimitError *RateLimitError
	if !errors.As(err, &rateLimitError) {
		t.Fatalf("Login error = %v, want rate limit error", err)
	}

	if limiter.identifierAttempts["missing-user"] != 5 {
		t.Fatalf("identifier limiter count = %d, want 5", limiter.identifierAttempts["missing-user"])
	}
}

func newTestService(t *testing.T) (Service, *fakeRepository, *fakeSessionStore, *fakeEnrollmentStore, *fakeLoginLimiter) {
	t.Helper()

	passwords := security.NewPasswordHasher(4)
	devHash, err := passwords.Hash("DevDemo123!")
	if err != nil {
		t.Fatalf("Hash(dev) returned error: %v", err)
	}

	ownerHash, err := passwords.Hash("OwnerDemo123!")
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
		sessions:      map[string]SessionRecord{},
		archived:      map[string]SessionRecord{},
		recoveryCodes: map[string][]fakeRecoveryCode{},
	}

	sessions := &fakeSessionStore{states: map[string]SessionState{}}
	enrollments := &fakeEnrollmentStore{values: map[string]string{}}
	limiter := &fakeLoginLimiter{
		window:             15 * time.Minute,
		maxAttemptsPerIP:   20,
		maxAttemptsPerID:   5,
		ipAttempts:         map[string]int{},
		identifierAttempts: map[string]int{},
	}

	return NewService(Options{
		Repository:        repository,
		Sessions:          sessions,
		Enrollments:       enrollments,
		Limiter:           limiter,
		Passwords:         passwords,
		Tokens:            security.NewAccessTokenManager("test-secret", time.Hour, "onixggr-test", testClock{now: now}),
		Sealer:            fakeSealer{},
		TwoFactor:         &fakeTOTPProvider{},
		Clock:             testClock{now: now},
		SessionTTL:        7 * 24 * time.Hour,
		TOTPEnrollmentTTL: 10 * time.Minute,
	}), repository, sessions, enrollments, limiter
}

type testClock struct {
	now time.Time
}

func (c testClock) Now() time.Time {
	return c.now
}

type fakeRepository struct {
	usersByID      map[string]User
	sessions       map[string]SessionRecord
	archived       map[string]SessionRecord
	recoveryCodes  map[string][]fakeRecoveryCode
	auditTrailSize int
}

type fakeRecoveryCode struct {
	ID       string
	CodeHash string
	Used     bool
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
	return SessionWithUser{Session: session, User: user}, nil
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

func (r *fakeRepository) ListActiveRecoveryCodes(_ context.Context, userID string) ([]RecoveryCodeRecord, error) {
	records := r.recoveryCodes[userID]
	response := make([]RecoveryCodeRecord, 0, len(records))
	for _, record := range records {
		if record.Used {
			continue
		}

		response = append(response, RecoveryCodeRecord{ID: record.ID, CodeHash: record.CodeHash})
	}

	return response, nil
}

func (r *fakeRepository) UseRecoveryCode(_ context.Context, recoveryCodeID string, _ time.Time) error {
	for userID, records := range r.recoveryCodes {
		for index := range records {
			if records[index].ID == recoveryCodeID && !records[index].Used {
				records[index].Used = true
				r.recoveryCodes[userID] = records
				return nil
			}
		}
	}

	return ErrNotFound
}

func (r *fakeRepository) EnableTOTP(_ context.Context, params EnableTOTPParams) error {
	user := r.usersByID[params.UserID]
	user.TOTPEnabled = true
	user.TOTPSecretEncrypted = params.EncryptedSecret
	r.usersByID[params.UserID] = user

	codes := make([]fakeRecoveryCode, 0, len(params.RecoveryCodeHashes))
	for index, hash := range params.RecoveryCodeHashes {
		codes = append(codes, fakeRecoveryCode{
			ID:       fmt.Sprintf("recovery-%d", index+1),
			CodeHash: hash,
		})
	}

	r.recoveryCodes[params.UserID] = codes
	return nil
}

func (r *fakeRepository) DisableTOTP(_ context.Context, params DisableTOTPParams) error {
	user := r.usersByID[params.UserID]
	user.TOTPEnabled = false
	user.TOTPSecretEncrypted = ""
	r.usersByID[params.UserID] = user
	delete(r.recoveryCodes, params.UserID)
	return nil
}

func (r *fakeRepository) UpdateIPAllowlist(_ context.Context, params UpdateIPAllowlistParams) error {
	user := r.usersByID[params.UserID]
	user.IPAllowlist = params.IPAllowlist
	r.usersByID[params.UserID] = user
	return nil
}

func (r *fakeRepository) InsertAuditLog(_ context.Context, _ AuditLogParams) error {
	r.auditTrailSize++
	return nil
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

type fakeEnrollmentStore struct {
	values map[string]string
}

func (s *fakeEnrollmentStore) Save(_ context.Context, userID string, secret string, _ time.Duration) error {
	s.values[userID] = secret
	return nil
}

func (s *fakeEnrollmentStore) Get(_ context.Context, userID string) (string, error) {
	secret, ok := s.values[userID]
	if !ok {
		return "", ErrNotFound
	}

	return secret, nil
}

func (s *fakeEnrollmentStore) Delete(_ context.Context, userID string) error {
	delete(s.values, userID)
	return nil
}

type fakeLoginLimiter struct {
	window             time.Duration
	maxAttemptsPerIP   int
	maxAttemptsPerID   int
	ipAttempts         map[string]int
	identifierAttempts map[string]int
}

func (l *fakeLoginLimiter) Check(_ context.Context, ip string, identifier string) (LimitStatus, error) {
	if l.identifierAttempts[identifier] >= l.maxAttemptsPerID {
		return LimitStatus{Limited: true, Scope: "identifier", RetryAfter: l.window}, nil
	}

	if l.ipAttempts[ip] >= l.maxAttemptsPerIP {
		return LimitStatus{Limited: true, Scope: "ip", RetryAfter: l.window}, nil
	}

	return LimitStatus{}, nil
}

func (l *fakeLoginLimiter) RegisterFailure(_ context.Context, ip string, identifier string) error {
	l.ipAttempts[ip]++
	l.identifierAttempts[identifier]++
	return nil
}

func (l *fakeLoginLimiter) Reset(_ context.Context, ip string, identifier string) error {
	delete(l.ipAttempts, ip)
	delete(l.identifierAttempts, identifier)
	return nil
}

type fakeSealer struct{}

func (fakeSealer) Seal(plain string) (string, error) {
	return "sealed:" + plain, nil
}

func (fakeSealer) Open(cipherText string) (string, error) {
	return strings.TrimPrefix(cipherText, "sealed:"), nil
}

type fakeTOTPProvider struct {
	sequence int
}

func (p *fakeTOTPProvider) GenerateSecret() (string, error) {
	p.sequence++
	return fmt.Sprintf("totp-secret-%d", p.sequence), nil
}

func (p *fakeTOTPProvider) Verify(secret string, code string, _ time.Time) bool {
	if code != "654321" {
		return false
	}

	return strings.HasPrefix(secret, "totp-secret-") || secret == "totp-owner-secret"
}

func (p *fakeTOTPProvider) OtpauthURL(accountName string, secret string) string {
	return "otpauth://totp/" + accountName + "?secret=" + secret
}

func ptr(value string) *string {
	return &value
}
