package audit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
)

func TestListLogsRejectsForeignOwnerStoreFilter(t *testing.T) {
	repository := &fakeRepository{
		ownerHasStore: false,
	}
	service := NewService(Options{Repository: repository})
	storeID := "store-foreign"

	_, err := service.ListLogs(context.Background(), auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	}, Filter{StoreID: &storeID, Limit: 20})
	if !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("ListLogs error = %v, want auth.ErrUnauthorized", err)
	}
}

func TestListLogsBlocksKaryawan(t *testing.T) {
	service := NewService(Options{Repository: &fakeRepository{}})

	_, err := service.ListLogs(context.Background(), auth.Subject{
		UserID: "employee-1",
		Role:   auth.RoleKaryawan,
	}, Filter{Limit: 20})
	if !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("ListLogs error = %v, want auth.ErrUnauthorized", err)
	}
}

func TestListLogsReturnsOwnerScopedEntries(t *testing.T) {
	repository := &fakeRepository{
		ownerHasStore: true,
		logs: []LogEntry{
			{ID: "audit-1", Action: "store.create"},
		},
	}
	service := NewService(Options{Repository: repository})
	storeID := "store-1"

	logs, err := service.ListLogs(context.Background(), auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	}, Filter{StoreID: &storeID, Limit: 20})
	if err != nil {
		t.Fatalf("ListLogs returned error: %v", err)
	}

	if len(logs.Items) != 1 || logs.Items[0].Action != "store.create" {
		t.Fatalf("logs = %#v, want owner-scoped entry", logs)
	}
}

func TestListLogsNormalizesFilterValues(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(Options{Repository: repository})
	storeID := "  store-1  "
	action := "  withdraw  "
	actorRole := "  dev  "
	targetType := "  store_withdrawal  "

	_, err := service.ListLogs(context.Background(), auth.Subject{
		UserID: "dev-1",
		Role:   auth.RoleDev,
	}, Filter{
		StoreID:    &storeID,
		Action:     &action,
		ActorRole:  &actorRole,
		TargetType: &targetType,
	})
	if err != nil {
		t.Fatalf("ListLogs returned error: %v", err)
	}

	if repository.lastFilter.StoreID == nil || *repository.lastFilter.StoreID != "store-1" {
		t.Fatalf("StoreID filter = %v, want trimmed store-1", repository.lastFilter.StoreID)
	}
	if repository.lastFilter.Action == nil || *repository.lastFilter.Action != "withdraw" {
		t.Fatalf("Action filter = %v, want trimmed withdraw", repository.lastFilter.Action)
	}
	if repository.lastFilter.ActorRole == nil || *repository.lastFilter.ActorRole != "dev" {
		t.Fatalf("ActorRole filter = %v, want trimmed dev", repository.lastFilter.ActorRole)
	}
	if repository.lastFilter.TargetType == nil || *repository.lastFilter.TargetType != "store_withdrawal" {
		t.Fatalf("TargetType filter = %v, want trimmed store_withdrawal", repository.lastFilter.TargetType)
	}
}

func TestPruneExpiredUsesNinetyDayRetentionByDefault(t *testing.T) {
	repository := &fakeRepository{}
	now := time.Date(2026, 3, 31, 8, 0, 0, 0, time.UTC)
	service := NewService(Options{
		Repository: repository,
		Clock:      fixedClock{now: now},
	})

	if _, err := service.PruneExpired(context.Background()); err != nil {
		t.Fatalf("PruneExpired returned error: %v", err)
	}

	want := now.Add(-90 * 24 * time.Hour)
	if !repository.lastPruneCutoff.Equal(want) {
		t.Fatalf("cutoff = %v, want %v", repository.lastPruneCutoff, want)
	}
}

type fakeRepository struct {
	logs            []LogEntry
	ownerHasStore   bool
	lastFilter      Filter
	lastPruneCutoff time.Time
}

func (r *fakeRepository) ListGlobal(_ context.Context, filter Filter) (ListResult, error) {
	r.lastFilter = filter
	return ListResult{
		Items:      append([]LogEntry(nil), r.logs...),
		TotalCount: len(r.logs),
	}, nil
}

func (r *fakeRepository) ListOwnerScoped(_ context.Context, _ string, filter Filter) (ListResult, error) {
	r.lastFilter = filter
	return ListResult{
		Items:      append([]LogEntry(nil), r.logs...),
		TotalCount: len(r.logs),
	}, nil
}

func (r *fakeRepository) OwnerHasStore(_ context.Context, _ string, _ string) (bool, error) {
	return r.ownerHasStore, nil
}

func (r *fakeRepository) PruneBefore(_ context.Context, cutoff time.Time) (int64, error) {
	r.lastPruneCutoff = cutoff
	return 3, nil
}

type fixedClock struct {
	now time.Time
}

func (f fixedClock) Now() time.Time {
	return f.now
}
