package audit

import (
	"context"
	"strings"

	"github.com/mugiew/onixggr/internal/modules/auth"
)

type RepositoryContract interface {
	ListGlobal(ctx context.Context, filter Filter) ([]LogEntry, error)
	ListOwnerScoped(ctx context.Context, ownerUserID string, filter Filter) ([]LogEntry, error)
	OwnerHasStore(ctx context.Context, ownerUserID string, storeID string) (bool, error)
}

type Service interface {
	ListLogs(ctx context.Context, subject auth.Subject, filter Filter) ([]LogEntry, error)
}

type service struct {
	repository RepositoryContract
}

func NewService(repository RepositoryContract) Service {
	return &service{repository: repository}
}

func (s *service) ListLogs(ctx context.Context, subject auth.Subject, filter Filter) ([]LogEntry, error) {
	if filter.StoreID != nil {
		trimmed := strings.TrimSpace(*filter.StoreID)
		if trimmed == "" {
			filter.StoreID = nil
		} else {
			filter.StoreID = &trimmed
		}
	}

	switch subject.Role {
	case auth.RoleDev, auth.RoleSuperadmin:
		return s.repository.ListGlobal(ctx, filter)
	case auth.RoleOwner:
		if filter.StoreID != nil {
			allowed, err := s.repository.OwnerHasStore(ctx, subject.UserID, *filter.StoreID)
			if err != nil {
				return nil, err
			}

			if !allowed {
				return nil, auth.ErrUnauthorized
			}
		}

		return s.repository.ListOwnerScoped(ctx, subject.UserID, filter)
	default:
		return nil, auth.ErrUnauthorized
	}
}
