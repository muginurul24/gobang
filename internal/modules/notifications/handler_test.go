package notifications

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mugiew/onixggr/internal/modules/auth"
)

func TestResolveScopeRejectsOwnerWithoutStoreAccess(t *testing.T) {
	handler := NewHandler(&stubNotificationService{}, nil, stubAccessRepository{allowed: false})
	request := httptest.NewRequest(http.MethodGet, "/v1/notifications?store_id=store-2", nil)

	_, _, err := handler.resolveScope(request, auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	})
	if !errors.Is(err, errForbiddenScope) {
		t.Fatalf("resolveScope error = %v, want forbidden", err)
	}
}

func TestResolveScopeAllowsAccessibleStore(t *testing.T) {
	handler := NewHandler(&stubNotificationService{}, nil, stubAccessRepository{allowed: true})
	request := httptest.NewRequest(http.MethodGet, "/v1/notifications?store_id=store-1", nil)

	scopeType, scopeID, err := handler.resolveScope(request, auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	})
	if err != nil {
		t.Fatalf("resolveScope error = %v", err)
	}
	if scopeType != ScopeStore || scopeID != "store-1" {
		t.Fatalf("scope = (%s,%s), want (store,store-1)", scopeType, scopeID)
	}
}

func TestResolveScopeDefaultsDevToRoleScope(t *testing.T) {
	handler := NewHandler(&stubNotificationService{}, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/v1/notifications", nil)

	scopeType, scopeID, err := handler.resolveScope(request, auth.Subject{
		UserID: "dev-1",
		Role:   auth.RoleDev,
	})
	if err != nil {
		t.Fatalf("resolveScope error = %v", err)
	}
	if scopeType != ScopeRole || scopeID != "dev" {
		t.Fatalf("scope = (%s,%s), want (role,dev)", scopeType, scopeID)
	}
}

func TestHandleMarkReadPassesScopedParams(t *testing.T) {
	service := &stubNotificationService{}
	handler := NewHandler(service, nil, stubAccessRepository{allowed: true})

	request := httptest.NewRequest(http.MethodPost, "/v1/notifications/notif-1/read?store_id=store-1", nil)
	request.SetPathValue("id", "notif-1")
	request = request.WithContext(auth.WithSubject(request.Context(), auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	}))
	response := httptest.NewRecorder()

	handler.handleMarkRead(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if service.lastMarkRead.ID != "notif-1" {
		t.Fatalf("markRead ID = %q, want notif-1", service.lastMarkRead.ID)
	}
	if service.lastMarkRead.ScopeType != ScopeStore || service.lastMarkRead.ScopeID != "store-1" {
		t.Fatalf("markRead scope = (%s,%s), want (store,store-1)", service.lastMarkRead.ScopeType, service.lastMarkRead.ScopeID)
	}
}

type stubNotificationService struct {
	lastMarkRead MarkReadParams
	markReadErr  error
}

func (s *stubNotificationService) Create(context.Context, CreateParams) (Notification, error) {
	return Notification{}, nil
}

func (s *stubNotificationService) ListByScope(context.Context, ListParams) (ListResult, error) {
	return ListResult{}, nil
}

func (s *stubNotificationService) MarkRead(_ context.Context, params MarkReadParams) error {
	s.lastMarkRead = params
	return s.markReadErr
}

func (s *stubNotificationService) CountUnread(context.Context, ScopeType, string) (int, error) {
	return 0, nil
}

type stubAccessRepository struct {
	allowed bool
	err     error
}

func (s stubAccessRepository) HasStoreAccess(context.Context, string, string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}

	return s.allowed, nil
}
