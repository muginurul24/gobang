package audit

import (
	"context"
	"errors"
	"testing"

	"github.com/mugiew/onixggr/internal/modules/auth"
)

func TestListLogsRejectsForeignOwnerStoreFilter(t *testing.T) {
	repository := &fakeRepository{
		ownerHasStore: false,
	}
	service := NewService(repository)
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
	service := NewService(&fakeRepository{})

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
	service := NewService(repository)
	storeID := "store-1"

	logs, err := service.ListLogs(context.Background(), auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	}, Filter{StoreID: &storeID, Limit: 20})
	if err != nil {
		t.Fatalf("ListLogs returned error: %v", err)
	}

	if len(logs) != 1 || logs[0].Action != "store.create" {
		t.Fatalf("logs = %#v, want owner-scoped entry", logs)
	}
}

type fakeRepository struct {
	logs          []LogEntry
	ownerHasStore bool
}

func (r *fakeRepository) ListGlobal(_ context.Context, _ Filter) ([]LogEntry, error) {
	return append([]LogEntry(nil), r.logs...), nil
}

func (r *fakeRepository) ListOwnerScoped(_ context.Context, _ string, _ Filter) ([]LogEntry, error) {
	return append([]LogEntry(nil), r.logs...), nil
}

func (r *fakeRepository) OwnerHasStore(_ context.Context, _ string, _ string) (bool, error) {
	return r.ownerHasStore, nil
}
