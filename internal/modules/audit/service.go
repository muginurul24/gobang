package audit

import (
	"context"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/platform/clock"
)

type RepositoryContract interface {
	ListGlobal(ctx context.Context, filter Filter) ([]LogEntry, error)
	ListOwnerScoped(ctx context.Context, ownerUserID string, filter Filter) ([]LogEntry, error)
	OwnerHasStore(ctx context.Context, ownerUserID string, storeID string) (bool, error)
	PruneBefore(ctx context.Context, cutoff time.Time) (int64, error)
}

type Service interface {
	ListLogs(ctx context.Context, subject auth.Subject, filter Filter) ([]LogEntry, error)
	PruneExpired(ctx context.Context) (int64, error)
}

type service struct {
	repository      RepositoryContract
	clock           clock.Clock
	retentionPeriod time.Duration
}

type Options struct {
	Repository      RepositoryContract
	Clock           clock.Clock
	RetentionPeriod time.Duration
}

func NewService(options Options) Service {
	now := options.Clock
	if now == nil {
		now = clock.SystemClock{}
	}

	retention := options.RetentionPeriod
	if retention <= 0 {
		retention = 90 * 24 * time.Hour
	}

	return &service{
		repository:      options.Repository,
		clock:           now,
		retentionPeriod: retention,
	}
}

func (s *service) ListLogs(ctx context.Context, subject auth.Subject, filter Filter) ([]LogEntry, error) {
	filter = normalizeFilter(filter)

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

func (s *service) PruneExpired(ctx context.Context) (int64, error) {
	return s.repository.PruneBefore(ctx, s.clock.Now().UTC().Add(-s.retentionPeriod))
}

func normalizeFilter(filter Filter) Filter {
	filter.StoreID = normalizeOptional(filter.StoreID)
	filter.Action = normalizeOptional(filter.Action)
	filter.ActorRole = normalizeOptional(filter.ActorRole)
	filter.TargetType = normalizeOptional(filter.TargetType)
	return filter
}

func normalizeOptional(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}
