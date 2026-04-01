package storemembers

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
)

func TestCreateStoreMemberGeneratesCodeAndAudit(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 11, 0, 0, 0, time.UTC))
	service := NewService(repository, fixedClock{now: repository.now}).(*service)
	service.codeFactory = func() (string, error) {
		return "ABCDEF123456", nil
	}

	member, err := service.CreateStoreMember(context.Background(), auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	}, "store-1", CreateStoreMemberInput{
		RealUsername: "member-alpha",
	}, auth.RequestMetadata{
		IPAddress: "127.0.0.1",
		UserAgent: "storemembers-test",
	})
	if err != nil {
		t.Fatalf("CreateStoreMember returned error: %v", err)
	}

	if member.UpstreamUserCode != "ABCDEF123456" {
		t.Fatalf("UpstreamUserCode = %s, want ABCDEF123456", member.UpstreamUserCode)
	}

	if repository.auditActions[0] != "store.member_created" {
		t.Fatalf("audit action = %q, want store.member_created", repository.auditActions[0])
	}
}

func TestCreateStoreMemberRetriesOnUpstreamCodeCollision(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 11, 15, 0, 0, time.UTC))
	service := NewService(repository, fixedClock{now: repository.now}).(*service)

	codes := []string{"COLLIDE00001", "UNIQUE000001"}
	service.codeFactory = func() (string, error) {
		value := codes[0]
		codes = codes[1:]
		return value, nil
	}

	repository.members["existing"] = StoreMember{
		ID:               "existing",
		StoreID:          "store-1",
		RealUsername:     "member-existing",
		UpstreamUserCode: "COLLIDE00001",
		Status:           StatusActive,
		CreatedAt:        repository.now,
		UpdatedAt:        repository.now,
	}

	member, err := service.CreateStoreMember(context.Background(), auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	}, "store-1", CreateStoreMemberInput{
		RealUsername: "member-new",
	}, auth.RequestMetadata{})
	if err != nil {
		t.Fatalf("CreateStoreMember returned error: %v", err)
	}

	if member.UpstreamUserCode != "UNIQUE000001" {
		t.Fatalf("UpstreamUserCode = %s, want UNIQUE000001", member.UpstreamUserCode)
	}
}

func TestCreateStoreMemberRejectsDuplicateUsername(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 11, 30, 0, 0, time.UTC))
	service := NewService(repository, fixedClock{now: repository.now}).(*service)
	service.codeFactory = func() (string, error) {
		return "ZXCVBN123456", nil
	}

	repository.members["existing"] = StoreMember{
		ID:               "existing",
		StoreID:          "store-1",
		RealUsername:     "member-demo",
		UpstreamUserCode: "MEMBER000001",
		Status:           StatusActive,
		CreatedAt:        repository.now,
		UpdatedAt:        repository.now,
	}

	_, err := service.CreateStoreMember(context.Background(), auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	}, "store-1", CreateStoreMemberInput{
		RealUsername: "member-demo",
	}, auth.RequestMetadata{})
	if err != ErrDuplicateRealUsername {
		t.Fatalf("CreateStoreMember error = %v, want ErrDuplicateRealUsername", err)
	}
}

func TestListStoreMembersAllowsAssignedStaff(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 11, 45, 0, 0, time.UTC))
	repository.staff["staff-1"] = true
	repository.members["member-1"] = StoreMember{
		ID:               "member-1",
		StoreID:          "store-1",
		RealUsername:     "member-demo",
		UpstreamUserCode: "MEMBER000001",
		Status:           StatusActive,
		CreatedAt:        repository.now,
		UpdatedAt:        repository.now,
	}
	service := NewService(repository, fixedClock{now: repository.now})

	page, err := service.ListStoreMembers(context.Background(), auth.Subject{
		UserID: "staff-1",
		Role:   auth.RoleKaryawan,
	}, ListStoreMembersFilter{StoreID: "store-1"})
	if err != nil {
		t.Fatalf("ListStoreMembers returned error: %v", err)
	}

	if len(page.Items) != 1 {
		t.Fatalf("len(page.Items) = %d, want 1", len(page.Items))
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
	staff        map[string]bool
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
		},
		staff:   map[string]bool{},
		members: map[string]StoreMember{},
	}
}

func (r *fakeRepository) GetStoreScope(_ context.Context, storeID string) (StoreScope, error) {
	if storeID != r.store.ID {
		return StoreScope{}, ErrNotFound
	}

	return r.store, nil
}

func (r *fakeRepository) IsStoreStaff(_ context.Context, storeID string, userID string) (bool, error) {
	if storeID != r.store.ID {
		return false, ErrNotFound
	}

	return r.staff[userID], nil
}

func (r *fakeRepository) ListStoreMembers(_ context.Context, filter ListStoreMembersFilter) (StoreMemberPage, error) {
	if filter.StoreID != r.store.ID {
		return StoreMemberPage{}, ErrNotFound
	}

	var members []StoreMember
	for _, member := range r.members {
		if member.StoreID == filter.StoreID {
			members = append(members, member)
		}
	}

	activeCount := 0
	for _, member := range members {
		if member.Status == StatusActive {
			activeCount++
		}
	}

	return StoreMemberPage{
		Items: members,
		Summary: StoreMemberSummary{
			TotalCount:    len(members),
			ActiveCount:   activeCount,
			InactiveCount: len(members) - activeCount,
		},
		Limit:  filter.Limit,
		Offset: filter.Offset,
	}, nil
}

func (r *fakeRepository) CreateStoreMember(_ context.Context, params CreateStoreMemberParams) (StoreMember, error) {
	for _, member := range r.members {
		if member.StoreID == params.StoreID && strings.EqualFold(member.RealUsername, params.RealUsername) {
			return StoreMember{}, ErrDuplicateRealUsername
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

func (r *fakeRepository) InsertAuditLog(_ context.Context, _ *string, _ string, _ *string, action string, _ string, _ *string, _ map[string]any, _ string, _ string, _ time.Time) error {
	r.auditActions = append(r.auditActions, action)
	return nil
}
