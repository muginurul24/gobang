package game

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/platform/nexusggr"
)

func TestCreateUserRejectsDuplicateUsernameBeforeUpstream(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 20, 0, 0, 0, time.UTC))
	repository.members["existing"] = StoreMember{
		ID:               "existing",
		StoreID:          "store-1",
		RealUsername:     "member-alpha",
		UpstreamUserCode: "ABCDEF123456",
		Status:           MemberStatusActive,
		CreatedAt:        repository.now,
		UpdatedAt:        repository.now,
	}
	upstream := &fakeUpstream{}

	service := NewService(repository, upstream, fixedClock{now: repository.now}).(*service)
	service.codeFactory = func() (string, error) {
		return "ZXCVBN123456", nil
	}

	_, err := service.CreateUser(context.Background(), "store_live_demo", CreateUserInput{
		Username: "member-alpha",
	}, RequestMetadata{})
	if !errors.Is(err, ErrDuplicateUsername) {
		t.Fatalf("CreateUser error = %v, want ErrDuplicateUsername", err)
	}

	if upstream.calls != 0 {
		t.Fatalf("upstream calls = %d, want 0", upstream.calls)
	}
}

func TestCreateUserSavesMappingAfterUpstreamSuccess(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 20, 15, 0, 0, time.UTC))
	upstream := &fakeUpstream{}

	service := NewService(repository, upstream, fixedClock{now: repository.now}).(*service)
	service.codeFactory = func() (string, error) {
		return "Q3RKJ1PC0ZMK", nil
	}

	member, err := service.CreateUser(context.Background(), "store_live_demo", CreateUserInput{
		Username: "member-beta",
	}, RequestMetadata{
		IPAddress: "127.0.0.1",
		UserAgent: "game-test",
	})
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}

	if member.RealUsername != "member-beta" {
		t.Fatalf("RealUsername = %s, want member-beta", member.RealUsername)
	}

	if member.UpstreamUserCode != "Q3RKJ1PC0ZMK" {
		t.Fatalf("UpstreamUserCode = %s, want Q3RKJ1PC0ZMK", member.UpstreamUserCode)
	}

	if upstream.calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", upstream.calls)
	}

	if repository.auditActions[0] != "game.user_created" {
		t.Fatalf("audit action = %q, want game.user_created", repository.auditActions[0])
	}
}

func TestCreateUserRejectsInactiveStore(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 20, 30, 0, 0, time.UTC))
	repository.store.Status = "inactive"

	service := NewService(repository, &fakeUpstream{}, fixedClock{now: repository.now})

	_, err := service.CreateUser(context.Background(), "store_live_demo", CreateUserInput{
		Username: "member-gamma",
	}, RequestMetadata{})
	if !errors.Is(err, ErrStoreInactive) {
		t.Fatalf("CreateUser error = %v, want ErrStoreInactive", err)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

type fakeRepository struct {
	now          time.Time
	store        StoreScope
	members      map[string]StoreMember
	auditActions []string
	sequence     int
}

func newFakeRepository(now time.Time) *fakeRepository {
	return &fakeRepository{
		now: now,
		store: StoreScope{
			ID:          "store-1",
			OwnerUserID: "owner-1",
			Name:        "Demo Store",
			Slug:        "demo-store",
			Status:      StoreStatusActive,
		},
		members: map[string]StoreMember{},
	}
}

func (r *fakeRepository) AuthenticateStore(_ context.Context, tokenHash string) (StoreScope, error) {
	if tokenHash == "" {
		return StoreScope{}, ErrUnauthorized
	}

	return r.store, nil
}

func (r *fakeRepository) FindStoreMemberByUsername(_ context.Context, storeID string, username string) (StoreMember, error) {
	for _, member := range r.members {
		if member.StoreID == storeID && member.RealUsername == username {
			return member, nil
		}
	}

	return StoreMember{}, ErrNotFound
}

func (r *fakeRepository) HasUpstreamUserCode(_ context.Context, upstreamUserCode string) (bool, error) {
	for _, member := range r.members {
		if member.UpstreamUserCode == upstreamUserCode {
			return true, nil
		}
	}

	return false, nil
}

func (r *fakeRepository) CreateStoreMember(_ context.Context, params CreateStoreMemberParams) (StoreMember, error) {
	for _, member := range r.members {
		if member.StoreID == params.StoreID && member.RealUsername == params.RealUsername {
			return StoreMember{}, ErrDuplicateUsername
		}

		if member.UpstreamUserCode == params.UpstreamUserCode {
			return StoreMember{}, ErrDuplicateUpstreamUserCode
		}
	}

	r.sequence++
	member := StoreMember{
		ID:               "member-created",
		StoreID:          params.StoreID,
		RealUsername:     params.RealUsername,
		UpstreamUserCode: params.UpstreamUserCode,
		Status:           params.Status,
		CreatedAt:        params.OccurredAt,
		UpdatedAt:        params.OccurredAt,
	}
	r.members[member.ID] = member
	return member, nil
}

func (r *fakeRepository) InsertAuditLog(_ context.Context, _ string, action string, _ *string, _ map[string]any, _ string, _ string, _ time.Time) error {
	r.auditActions = append(r.auditActions, action)
	return nil
}

type fakeUpstream struct {
	calls int
	err   error
}

func (u *fakeUpstream) UserCreate(_ context.Context, input nexusggr.UserCreateInput) (nexusggr.UserCreateResult, error) {
	u.calls++
	if u.err != nil {
		return nexusggr.UserCreateResult{}, u.err
	}

	return nexusggr.UserCreateResult{
		Message:  "SUCCESS",
		UserCode: input.UserCode,
	}, nil
}
