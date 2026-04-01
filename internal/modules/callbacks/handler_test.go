package callbacks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mugiew/onixggr/internal/modules/auth"
)

func TestHandleListQueueRejectsInvalidStatus(t *testing.T) {
	handler := NewHandler(&stubCallbackQueryService{}, nil)
	request := httptest.NewRequest(http.MethodGet, "/v1/callbacks/queue?status=broken", nil)
	request = request.WithContext(auth.WithSubject(request.Context(), auth.Subject{
		UserID: "dev-1",
		Role:   auth.RoleDev,
	}))
	response := httptest.NewRecorder()

	handler.handleListQueue(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func TestHandleListQueuePassesFilter(t *testing.T) {
	service := &stubCallbackQueryService{}
	handler := NewHandler(service, nil)
	request := httptest.NewRequest(http.MethodGet, "/v1/callbacks/queue?status=retrying&store_id=store-1&query=member&limit=12&offset=24", nil)
	request = request.WithContext(auth.WithSubject(request.Context(), auth.Subject{
		UserID: "dev-1",
		Role:   auth.RoleDev,
	}))
	response := httptest.NewRecorder()

	handler.handleListQueue(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if service.lastQueueFilter.Status == nil || *service.lastQueueFilter.Status != StatusRetrying {
		t.Fatalf("status filter = %v, want retrying", service.lastQueueFilter.Status)
	}
	if service.lastQueueFilter.StoreID == nil || *service.lastQueueFilter.StoreID != "store-1" {
		t.Fatalf("store filter = %v, want store-1", service.lastQueueFilter.StoreID)
	}
	if service.lastQueueFilter.Query != "member" {
		t.Fatalf("query = %q, want member", service.lastQueueFilter.Query)
	}
	if service.lastQueueFilter.Limit != 12 {
		t.Fatalf("limit = %d, want 12", service.lastQueueFilter.Limit)
	}
	if service.lastQueueFilter.Offset != 24 {
		t.Fatalf("offset = %d, want 24", service.lastQueueFilter.Offset)
	}
}

func TestHandleListAttemptsPropagatesPathValue(t *testing.T) {
	service := &stubCallbackQueryService{
		attemptPage: AttemptPage{
			CallbackID: "callback-1",
		},
	}
	handler := NewHandler(service, nil)
	request := httptest.NewRequest(http.MethodGet, "/v1/callbacks/callback-1/attempts?limit=5&offset=10", nil)
	request.SetPathValue("callbackID", "callback-1")
	request = request.WithContext(auth.WithSubject(request.Context(), auth.Subject{
		UserID: "dev-1",
		Role:   auth.RoleDev,
	}))
	response := httptest.NewRecorder()

	handler.handleListAttempts(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if service.lastCallbackID != "callback-1" {
		t.Fatalf("callbackID = %q, want callback-1", service.lastCallbackID)
	}
	if service.lastAttemptLimit != 5 {
		t.Fatalf("limit = %d, want 5", service.lastAttemptLimit)
	}
	if service.lastAttemptOffset != 10 {
		t.Fatalf("offset = %d, want 10", service.lastAttemptOffset)
	}
}

type stubCallbackQueryService struct {
	queuePage         QueuePage
	attemptPage       AttemptPage
	lastQueueFilter   ListQueueFilter
	lastCallbackID    string
	lastAttemptLimit  int
	lastAttemptOffset int
}

func (s *stubCallbackQueryService) EnqueueMemberPaymentSuccess(context.Context, string) error {
	return nil
}

func (s *stubCallbackQueryService) RunPending(context.Context, int) (RunSummary, error) {
	return RunSummary{}, nil
}

func (s *stubCallbackQueryService) ListQueue(_ context.Context, _ auth.Subject, filter ListQueueFilter) (QueuePage, error) {
	s.lastQueueFilter = filter
	return s.queuePage, nil
}

func (s *stubCallbackQueryService) ListAttempts(_ context.Context, _ auth.Subject, callbackID string, limit int, offset int) (AttemptPage, error) {
	s.lastCallbackID = callbackID
	s.lastAttemptLimit = limit
	s.lastAttemptOffset = offset
	return s.attemptPage, nil
}
