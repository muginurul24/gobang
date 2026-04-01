package notifications

import (
	"context"
	"errors"
	"testing"
	"time"

	platformrealtime "github.com/mugiew/onixggr/internal/platform/realtime"
)

func TestCreateNotificationRequiresFields(t *testing.T) {
	svc := NewService(Options{
		Repository: &stubRepository{},
	})

	tests := []struct {
		name   string
		params CreateParams
	}{
		{"empty scope_type", CreateParams{ScopeID: "store-1", EventType: "test"}},
		{"empty scope_id", CreateParams{ScopeType: ScopeStore, EventType: "test"}},
		{"empty event_type", CreateParams{ScopeType: ScopeStore, ScopeID: "store-1"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(context.Background(), tc.params)
			if err == nil {
				t.Fatal("expected error for missing field")
			}
		})
	}
}

func TestCreateNotificationSuccessAndPushesToRealtime(t *testing.T) {
	hub := &stubHub{}
	repo := &stubRepository{}
	svc := NewService(Options{
		Repository: repo,
		Hub:        hub,
	})

	notification, err := svc.Create(context.Background(), CreateParams{
		ScopeType: ScopeStore,
		ScopeID:   "store-1",
		EventType: "game.deposit.success",
		Title:     "Deposit berhasil",
		Body:      "Deposit 100000 berhasil",
	})
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}

	if notification.ID == "" {
		t.Fatal("expected notification ID")
	}
	if notification.ScopeType != ScopeStore {
		t.Fatalf("ScopeType = %s, want store", notification.ScopeType)
	}
	if notification.EventType != "game.deposit.success" {
		t.Fatalf("EventType = %s, want game.deposit.success", notification.EventType)
	}

	if hub.publishCount != 1 {
		t.Fatalf("hub publish count = %d, want 1", hub.publishCount)
	}
	if hub.lastChannel != "store:store-1" {
		t.Fatalf("hub channel = %s, want store:store-1", hub.lastChannel)
	}
	if hub.lastType != "game.deposit.success" {
		t.Fatalf("hub event type = %s, want game.deposit.success", hub.lastType)
	}
}

func TestCreateNotificationSkipsRealtimeWhenNoHub(t *testing.T) {
	repo := &stubRepository{}
	svc := NewService(Options{
		Repository: repo,
	})

	_, err := svc.Create(context.Background(), CreateParams{
		ScopeType: ScopeStore,
		ScopeID:   "store-1",
		EventType: "test.event",
		Title:     "Test",
		Body:      "Test body",
	})
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
}

func TestMarkReadNotFoundReturnsError(t *testing.T) {
	repo := &stubRepository{markReadErr: ErrNotFound}
	svc := NewService(Options{
		Repository: repo,
	})

	err := svc.MarkRead(context.Background(), MarkReadParams{
		ID:        "nonexistent",
		ScopeType: ScopeStore,
		ScopeID:   "store-1",
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestResolveChannelMapsCorrectly(t *testing.T) {
	tests := []struct {
		scopeType ScopeType
		scopeID   string
		want      string
	}{
		{ScopeStore, "store-1", "store:store-1"},
		{ScopeUser, "user-1", "user:user-1"},
		{ScopeRole, "dev", "role:dev"},
		{ScopeGlobal, "any", "global_chat"},
	}

	for _, tc := range tests {
		got := resolveChannel(tc.scopeType, tc.scopeID)
		if got != tc.want {
			t.Fatalf("resolveChannel(%s, %s) = %s, want %s", tc.scopeType, tc.scopeID, got, tc.want)
		}
	}
}

func TestStoreEmitterDelegatesToInner(t *testing.T) {
	inner := &stubEmitter{}
	emitter := NewStoreEmitter(inner)

	emitter.Emit("store-1", "game.deposit.success", "Title", "Body")

	if inner.lastParams.ScopeType != ScopeStore {
		t.Fatalf("ScopeType = %s, want store", inner.lastParams.ScopeType)
	}
	if inner.lastParams.ScopeID != "store-1" {
		t.Fatalf("ScopeID = %s, want store-1", inner.lastParams.ScopeID)
	}
	if inner.lastParams.EventType != "game.deposit.success" {
		t.Fatalf("EventType = %s, want game.deposit.success", inner.lastParams.EventType)
	}
}

func TestPlatformRoleEmitterDelegatesToDevAndSuperadmin(t *testing.T) {
	inner := &stubEmitter{}
	emitter := NewPlatformRoleEmitter(inner)

	emitter.Emit("callback.delivery_failed", "Title", "Body")

	if len(inner.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(inner.calls))
	}
	if inner.calls[0].ScopeType != ScopeRole || inner.calls[0].ScopeID != "dev" {
		t.Fatalf("first call = %#v, want role dev", inner.calls[0])
	}
	if inner.calls[1].ScopeType != ScopeRole || inner.calls[1].ScopeID != "superadmin" {
		t.Fatalf("second call = %#v, want role superadmin", inner.calls[1])
	}
}

// --- stubs ---

type stubRepository struct {
	created     Notification
	markReadErr error
}

func (r *stubRepository) Create(_ context.Context, params CreateParams, createdAt time.Time) (Notification, error) {
	r.created = Notification{
		ID:        "notif-1",
		ScopeType: params.ScopeType,
		ScopeID:   params.ScopeID,
		EventType: params.EventType,
		Title:     params.Title,
		Body:      params.Body,
		CreatedAt: createdAt,
	}

	return r.created, nil
}

func (r *stubRepository) ListByScope(_ context.Context, _ ListParams) (ListResult, error) {
	return ListResult{Items: []Notification{}}, nil
}

func (r *stubRepository) MarkRead(_ context.Context, _ MarkReadParams, _ time.Time) error {
	return r.markReadErr
}

func (r *stubRepository) CountUnread(_ context.Context, _ ScopeType, _ string) (int, error) {
	return 0, nil
}

type stubHub struct {
	publishCount int
	lastChannel  string
	lastType     string
}

func (h *stubHub) Publish(_ context.Context, event platformrealtime.Event) error {
	h.publishCount++
	h.lastChannel = event.Channel
	h.lastType = event.Type
	return nil
}

type stubEmitter struct {
	lastParams CreateParams
	calls      []CreateParams
}

func (e *stubEmitter) Emit(params CreateParams) error {
	e.lastParams = params
	e.calls = append(e.calls, params)
	return nil
}
