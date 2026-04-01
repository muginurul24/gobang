package users

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
)

func TestCreateUserAllowsDevProvisionOwner(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository, stubHasher{}, stubClock{now: time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)})

	user, err := service.CreateUser(context.Background(), auth.Subject{
		UserID: "dev-1",
		Role:   auth.RoleDev,
	}, CreateUserInput{
		Email:    "owner@example.com",
		Username: "owner-alpha",
		Password: "OwnerDemo123!",
		Role:     "owner",
	}, auth.RequestMetadata{
		IPAddress: "127.0.0.1",
		UserAgent: "test",
	})
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}

	if user.Role != auth.RoleOwner {
		t.Fatalf("CreateUser role = %s, want owner", user.Role)
	}
	if repository.lastCreated.Role != auth.RoleOwner {
		t.Fatalf("repository created role = %s, want owner", repository.lastCreated.Role)
	}
	if repository.lastAuditAction != "user.created" {
		t.Fatalf("audit action = %s, want user.created", repository.lastAuditAction)
	}
}

func TestCreateUserRejectsSuperadminProvisionBySuperadmin(t *testing.T) {
	service := NewService(&fakeRepository{}, stubHasher{}, stubClock{now: time.Now().UTC()})

	_, err := service.CreateUser(context.Background(), auth.Subject{
		UserID: "superadmin-1",
		Role:   auth.RoleSuperadmin,
	}, CreateUserInput{
		Email:    "other-superadmin@example.com",
		Username: "other-superadmin",
		Password: "OwnerDemo123!",
		Role:     "superadmin",
	}, auth.RequestMetadata{})
	if !errors.Is(err, ErrRoleProvisionForbidden) {
		t.Fatalf("CreateUser error = %v, want ErrRoleProvisionForbidden", err)
	}
}

func TestUpdateUserStatusRejectsSelfDeactivation(t *testing.T) {
	repository := &fakeRepository{
		userByID: map[string]User{
			"dev-1": {
				ID:       "dev-1",
				Email:    "dev@example.com",
				Username: "dev",
				Role:     auth.RoleDev,
				IsActive: true,
			},
		},
	}
	service := NewService(repository, stubHasher{}, stubClock{now: time.Now().UTC()})
	isActive := false

	_, err := service.UpdateUserStatus(context.Background(), auth.Subject{
		UserID: "dev-1",
		Role:   auth.RoleDev,
	}, "dev-1", UpdateUserStatusInput{IsActive: &isActive}, auth.RequestMetadata{})
	if !errors.Is(err, ErrCannotDeactivateSelf) {
		t.Fatalf("UpdateUserStatus error = %v, want ErrCannotDeactivateSelf", err)
	}
}

func TestUpdateUserStatusAllowsSuperadminManagingOwner(t *testing.T) {
	repository := &fakeRepository{
		userByID: map[string]User{
			"owner-1": {
				ID:       "owner-1",
				Email:    "owner@example.com",
				Username: "owner",
				Role:     auth.RoleOwner,
				IsActive: true,
			},
		},
	}
	service := NewService(repository, stubHasher{}, stubClock{now: time.Now().UTC()})
	isActive := false

	user, err := service.UpdateUserStatus(context.Background(), auth.Subject{
		UserID: "superadmin-1",
		Role:   auth.RoleSuperadmin,
	}, "owner-1", UpdateUserStatusInput{IsActive: &isActive}, auth.RequestMetadata{})
	if err != nil {
		t.Fatalf("UpdateUserStatus returned error: %v", err)
	}
	if user.IsActive {
		t.Fatalf("updated user is_active = true, want false")
	}
}

type fakeRepository struct {
	lastCreated     CreateUserParams
	lastAuditAction string
	userByID        map[string]User
}

func (r *fakeRepository) ListDirectory(_ context.Context, _ ListFilter) (Page, error) {
	return Page{}, nil
}

func (r *fakeRepository) CreateUser(_ context.Context, params CreateUserParams) (User, error) {
	r.lastCreated = params
	return User{
		ID:        "user-1",
		Email:     params.Email,
		Username:  params.Username,
		Role:      params.Role,
		IsActive:  true,
		CreatedAt: params.OccurredAt,
		UpdatedAt: params.OccurredAt,
	}, nil
}

func (r *fakeRepository) GetUserByID(_ context.Context, userID string) (User, error) {
	user, ok := r.userByID[userID]
	if !ok {
		return User{}, ErrNotFound
	}

	return user, nil
}

func (r *fakeRepository) UpdateUserStatus(_ context.Context, params UpdateUserStatusParams) (User, error) {
	user, ok := r.userByID[params.UserID]
	if !ok {
		return User{}, ErrNotFound
	}

	user.IsActive = params.IsActive
	user.UpdatedAt = params.OccurredAt
	r.userByID[params.UserID] = user
	return user, nil
}

func (r *fakeRepository) InsertAuditLog(_ context.Context, _ *string, _ string, action string, _ *string, _ map[string]any, _ string, _ string, _ time.Time) error {
	r.lastAuditAction = action
	return nil
}

type stubHasher struct{}

func (stubHasher) Hash(password string) (string, error) {
	return "hashed:" + password, nil
}

type stubClock struct {
	now time.Time
}

func (c stubClock) Now() time.Time {
	return c.now
}
