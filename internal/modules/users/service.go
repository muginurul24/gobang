package users

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/platform/clock"
)

type PasswordHasher interface {
	Hash(password string) (string, error)
}

type RepositoryContract interface {
	ListDirectory(ctx context.Context, filter ListFilter) (Page, error)
	CreateUser(ctx context.Context, params CreateUserParams) (User, error)
	GetUserByID(ctx context.Context, userID string) (User, error)
	UpdateUserStatus(ctx context.Context, params UpdateUserStatusParams) (User, error)
	InsertAuditLog(
		ctx context.Context,
		actorUserID *string,
		actorRole string,
		action string,
		targetID *string,
		payload map[string]any,
		ipAddress string,
		userAgent string,
		occurredAt time.Time,
	) error
}

type Service interface {
	ListDirectory(ctx context.Context, subject auth.Subject, filter ListFilter) (Page, error)
	CreateUser(ctx context.Context, subject auth.Subject, input CreateUserInput, metadata auth.RequestMetadata) (User, error)
	UpdateUserStatus(ctx context.Context, subject auth.Subject, userID string, input UpdateUserStatusInput, metadata auth.RequestMetadata) (User, error)
}

type service struct {
	repository RepositoryContract
	passwords  PasswordHasher
	clock      clock.Clock
}

func NewService(repository RepositoryContract, passwords PasswordHasher, now clock.Clock) Service {
	if now == nil {
		now = clock.SystemClock{}
	}

	return &service{
		repository: repository,
		passwords:  passwords,
		clock:      now,
	}
}

func (s *service) ListDirectory(ctx context.Context, subject auth.Subject, filter ListFilter) (Page, error) {
	if !canManageUsers(subject.Role) {
		return Page{}, ErrForbidden
	}

	filter.Query = strings.TrimSpace(filter.Query)
	filter.Limit = normalizeLimit(filter.Limit, 12, 100)
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	if filter.Role != nil && !isManagedRole(*filter.Role) {
		return Page{}, ErrInvalidRole
	}

	return s.repository.ListDirectory(ctx, filter)
}

func (s *service) CreateUser(ctx context.Context, subject auth.Subject, input CreateUserInput, metadata auth.RequestMetadata) (User, error) {
	if !canManageUsers(subject.Role) {
		return User{}, ErrForbidden
	}

	email := strings.ToLower(strings.TrimSpace(input.Email))
	username := strings.ToLower(strings.TrimSpace(input.Username))
	password := strings.TrimSpace(input.Password)
	role, err := parseManagedRole(input.Role)
	if err != nil {
		return User{}, err
	}

	if !canProvisionRole(subject.Role, role) {
		return User{}, ErrRoleProvisionForbidden
	}

	if email == "" || username == "" || password == "" {
		return User{}, ErrInvalidInput
	}

	hash, err := s.passwords.Hash(password)
	if err != nil {
		return User{}, fmt.Errorf("hash user password: %w", err)
	}

	now := s.clock.Now().UTC()
	user, err := s.repository.CreateUser(ctx, CreateUserParams{
		Email:           email,
		Username:        username,
		PasswordHash:    hash,
		Role:            role,
		CreatedByUserID: nil,
		OccurredAt:      now,
	})
	if err != nil {
		return User{}, err
	}

	if err := s.repository.InsertAuditLog(
		ctx,
		&subject.UserID,
		string(subject.Role),
		"user.created",
		&user.ID,
		map[string]any{
			"email":    email,
			"username": username,
			"role":     role,
		},
		metadata.IPAddress,
		metadata.UserAgent,
		now,
	); err != nil {
		return User{}, err
	}

	return user, nil
}

func (s *service) UpdateUserStatus(ctx context.Context, subject auth.Subject, userID string, input UpdateUserStatusInput, metadata auth.RequestMetadata) (User, error) {
	if !canManageUsers(subject.Role) {
		return User{}, ErrForbidden
	}

	if input.IsActive == nil {
		return User{}, ErrInvalidInput
	}

	target, err := s.repository.GetUserByID(ctx, strings.TrimSpace(userID))
	if err != nil {
		return User{}, err
	}

	if !isManagedRole(target.Role) {
		return User{}, ErrStatusUpdateForbidden
	}

	if subject.UserID == target.ID && !*input.IsActive {
		return User{}, ErrCannotDeactivateSelf
	}

	if !canMutateTarget(subject, target) {
		return User{}, ErrStatusUpdateForbidden
	}

	now := s.clock.Now().UTC()
	updated, err := s.repository.UpdateUserStatus(ctx, UpdateUserStatusParams{
		UserID:     target.ID,
		IsActive:   *input.IsActive,
		OccurredAt: now,
	})
	if err != nil {
		return User{}, err
	}

	action := "user.deactivated"
	if updated.IsActive {
		action = "user.activated"
	}

	if err := s.repository.InsertAuditLog(
		ctx,
		&subject.UserID,
		string(subject.Role),
		action,
		&updated.ID,
		map[string]any{
			"role":      updated.Role,
			"is_active": updated.IsActive,
		},
		metadata.IPAddress,
		metadata.UserAgent,
		now,
	); err != nil {
		return User{}, err
	}

	return updated, nil
}

func canManageUsers(role auth.Role) bool {
	return role == auth.RoleDev || role == auth.RoleSuperadmin
}

func canProvisionRole(actorRole auth.Role, targetRole auth.Role) bool {
	switch targetRole {
	case auth.RoleOwner:
		return actorRole == auth.RoleDev || actorRole == auth.RoleSuperadmin
	case auth.RoleSuperadmin:
		return actorRole == auth.RoleDev
	default:
		return false
	}
}

func canMutateTarget(subject auth.Subject, target User) bool {
	switch target.Role {
	case auth.RoleOwner:
		return canManageUsers(subject.Role)
	case auth.RoleSuperadmin:
		return subject.Role == auth.RoleDev
	default:
		return false
	}
}

func parseManagedRole(raw string) (auth.Role, error) {
	role := auth.Role(strings.ToLower(strings.TrimSpace(raw)))
	if !isManagedRole(role) {
		return "", ErrInvalidRole
	}

	return role, nil
}

func isManagedRole(role auth.Role) bool {
	return role == auth.RoleOwner || role == auth.RoleSuperadmin || role == auth.RoleDev
}

func normalizeLimit(limit int, fallback int, max int) int {
	if limit <= 0 {
		return fallback
	}
	if limit > max {
		return max
	}

	return limit
}
