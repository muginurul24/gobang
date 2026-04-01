package storemembers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/platform/clock"
)

type RepositoryContract interface {
	GetStoreScope(ctx context.Context, storeID string) (StoreScope, error)
	IsStoreStaff(ctx context.Context, storeID string, userID string) (bool, error)
	ListStoreMembers(ctx context.Context, filter ListStoreMembersFilter) (StoreMemberPage, error)
	CreateStoreMember(ctx context.Context, params CreateStoreMemberParams) (StoreMember, error)
	InsertAuditLog(
		ctx context.Context,
		actorUserID *string,
		actorRole string,
		storeID *string,
		action string,
		targetType string,
		targetID *string,
		payload map[string]any,
		ipAddress string,
		userAgent string,
		occurredAt time.Time,
	) error
}

type Service interface {
	ListStoreMembers(ctx context.Context, subject auth.Subject, filter ListStoreMembersFilter) (StoreMemberPage, error)
	CreateStoreMember(ctx context.Context, subject auth.Subject, storeID string, input CreateStoreMemberInput, metadata auth.RequestMetadata) (StoreMember, error)
}

type service struct {
	repository  RepositoryContract
	clock       clock.Clock
	codeFactory func() (string, error)
}

func NewService(repository RepositoryContract, now clock.Clock) Service {
	if now == nil {
		now = clock.SystemClock{}
	}

	return &service{
		repository:  repository,
		clock:       now,
		codeFactory: NewUpstreamUserCode,
	}
}

func (s *service) ListStoreMembers(ctx context.Context, subject auth.Subject, filter ListStoreMembersFilter) (StoreMemberPage, error) {
	filter.StoreID = strings.TrimSpace(filter.StoreID)
	store, err := s.loadStoreScope(ctx, filter.StoreID)
	if err != nil {
		return StoreMemberPage{}, err
	}

	allowed, err := s.canViewStore(ctx, subject, store)
	if err != nil {
		return StoreMemberPage{}, err
	}

	if !allowed {
		return StoreMemberPage{}, ErrForbidden
	}

	filter.Query = strings.TrimSpace(filter.Query)
	filter.Limit = normalizeLimit(filter.Limit, 25, 100)
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	filter.StoreID = store.ID

	return s.repository.ListStoreMembers(ctx, filter)
}

func (s *service) CreateStoreMember(ctx context.Context, subject auth.Subject, storeID string, input CreateStoreMemberInput, metadata auth.RequestMetadata) (StoreMember, error) {
	store, err := s.loadStoreScope(ctx, strings.TrimSpace(storeID))
	if err != nil {
		return StoreMember{}, err
	}

	if !s.canManageStore(subject, store) {
		return StoreMember{}, ErrForbidden
	}

	realUsername := normalizeRealUsername(input.RealUsername)
	if !validRealUsername(realUsername) {
		return StoreMember{}, ErrInvalidRealUsername
	}

	now := s.clock.Now().UTC()
	for range 8 {
		upstreamUserCode, err := s.codeFactory()
		if err != nil {
			return StoreMember{}, fmt.Errorf("generate upstream user code: %w", err)
		}

		member, createErr := s.repository.CreateStoreMember(ctx, CreateStoreMemberParams{
			StoreID:          store.ID,
			RealUsername:     realUsername,
			UpstreamUserCode: upstreamUserCode,
			Status:           StatusActive,
			OccurredAt:       now,
		})
		if createErr == nil {
			if err := s.repository.InsertAuditLog(
				ctx,
				&subject.UserID,
				string(subject.Role),
				&store.ID,
				"store.member_created",
				"store_member",
				&member.ID,
				map[string]any{
					"real_username":      member.RealUsername,
					"upstream_user_code": member.UpstreamUserCode,
				},
				metadata.IPAddress,
				metadata.UserAgent,
				now,
			); err != nil {
				return StoreMember{}, err
			}

			return member, nil
		}

		if createErr == ErrDuplicateRealUsername {
			return StoreMember{}, createErr
		}

		if createErr == ErrDuplicateUpstreamUserCode {
			continue
		}

		return StoreMember{}, createErr
	}

	return StoreMember{}, ErrCodeGenerationExhausted
}

func (s *service) loadStoreScope(ctx context.Context, storeID string) (StoreScope, error) {
	store, err := s.repository.GetStoreScope(ctx, storeID)
	if err != nil {
		return StoreScope{}, err
	}

	if store.DeletedAt != nil {
		return StoreScope{}, ErrNotFound
	}

	return store, nil
}

func (s *service) canViewStore(ctx context.Context, subject auth.Subject, store StoreScope) (bool, error) {
	switch subject.Role {
	case auth.RoleOwner:
		return subject.UserID == store.OwnerUserID, nil
	case auth.RoleDev, auth.RoleSuperadmin:
		return true, nil
	case auth.RoleKaryawan:
		return s.repository.IsStoreStaff(ctx, store.ID, subject.UserID)
	default:
		return false, nil
	}
}

func (s *service) canManageStore(subject auth.Subject, store StoreScope) bool {
	switch subject.Role {
	case auth.RoleOwner:
		return subject.UserID == store.OwnerUserID
	case auth.RoleDev, auth.RoleSuperadmin:
		return true
	default:
		return false
	}
}

func normalizeLimit(value int, fallback int, max int) int {
	if value <= 0 {
		return fallback
	}
	if value > max {
		return max
	}

	return value
}
