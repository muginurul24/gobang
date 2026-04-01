package callbacks

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/platform/clock"
)

func TestEnqueueMemberPaymentSuccess(t *testing.T) {
	now := time.Date(2026, time.March, 30, 14, 0, 0, 0, time.UTC)
	repository := &stubRepository{
		source: MemberPaymentCallbackSource{
			QRISTransactionID:   "qris-1",
			StoreID:             "store-1",
			StoreMemberID:       stringPtr("member-1"),
			RealUsername:        "member-alpha",
			CustomRef:           "MPAY123",
			ProviderTrxID:       stringPtr("provider-1"),
			AmountGross:         "25000.00",
			PlatformFeeAmount:   "750.00",
			StoreCreditAmount:   "24250.00",
			TransactionStatus:   "success",
			TransactionUpdateAt: now,
		},
	}

	service := NewService(Options{
		Repository:    repository,
		Clock:         fixedClock{now: now},
		SigningSecret: "secret-1",
	})

	if err := service.EnqueueMemberPaymentSuccess(context.Background(), "qris-1"); err != nil {
		t.Fatalf("EnqueueMemberPaymentSuccess error = %v", err)
	}

	if repository.enqueueCalls != 1 {
		t.Fatalf("enqueueCalls = %d, want 1", repository.enqueueCalls)
	}
	if repository.lastEnqueue.EventType != memberPaymentSuccessEvent {
		t.Fatalf("eventType = %q, want member_payment.success", repository.lastEnqueue.EventType)
	}
	if repository.lastEnqueue.ReferenceType != "qris_transaction" {
		t.Fatalf("referenceType = %q, want qris_transaction", repository.lastEnqueue.ReferenceType)
	}
	if repository.lastEnqueue.Signature == "" {
		t.Fatal("signature is empty")
	}
}

func TestRunPendingMarksSuccess(t *testing.T) {
	now := time.Date(2026, time.March, 30, 14, 5, 0, 0, time.UTC)
	repository := &stubRepository{
		dueCallbacks: []DueOutboundCallback{
			{
				OutboundCallback: OutboundCallback{
					ID:            "callback-1",
					StoreID:       "store-1",
					EventType:     memberPaymentSuccessEvent,
					ReferenceType: "qris_transaction",
					ReferenceID:   "qris-1",
					PayloadJSON:   []byte(`{"ok":true}`),
					Signature:     "sha256=abc",
					Status:        StatusPending,
				},
				CallbackURL: "https://store.example/callback",
				AttemptNo:   0,
			},
		},
	}
	dispatcher := &stubDispatcher{
		result: DispatchResult{
			HTTPStatus:         intPtr(http.StatusOK),
			ResponseBodyMasked: `{"status":"ok"}`,
			Success:            true,
		},
	}
	storeNotifier := &storeNotifierStub{}
	platformNotifier := &platformNotifierStub{}

	service := NewService(Options{
		Repository:            repository,
		Dispatcher:            dispatcher,
		Notifications:         storeNotifier,
		PlatformNotifications: platformNotifier,
		Clock:                 fixedClock{now: now},
	})

	summary, err := service.RunPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunPending error = %v", err)
	}

	if summary.Delivered != 1 {
		t.Fatalf("Delivered = %d, want 1", summary.Delivered)
	}
	if repository.recordCalls != 1 {
		t.Fatalf("recordCalls = %d, want 1", repository.recordCalls)
	}
	if repository.lastRecord.CallbackStatus != StatusSuccess {
		t.Fatalf("callback status = %q, want success", repository.lastRecord.CallbackStatus)
	}
}

func TestRunPendingSchedulesRetry(t *testing.T) {
	now := time.Date(2026, time.March, 30, 14, 10, 0, 0, time.UTC)
	repository := &stubRepository{
		dueCallbacks: []DueOutboundCallback{
			{
				OutboundCallback: OutboundCallback{
					ID:            "callback-2",
					StoreID:       "store-1",
					EventType:     memberPaymentSuccessEvent,
					ReferenceType: "qris_transaction",
					ReferenceID:   "qris-2",
					PayloadJSON:   []byte(`{"ok":false}`),
					Signature:     "sha256=def",
					Status:        StatusPending,
				},
				CallbackURL: "https://store.example/callback",
				AttemptNo:   0,
			},
		},
	}
	dispatcher := &stubDispatcher{
		result: DispatchResult{
			HTTPStatus:         intPtr(http.StatusBadGateway),
			ResponseBodyMasked: `{"error":"temporary"}`,
			Success:            false,
		},
	}
	storeNotifier := &storeNotifierStub{}
	platformNotifier := &platformNotifierStub{}

	service := NewService(Options{
		Repository:            repository,
		Dispatcher:            dispatcher,
		Notifications:         storeNotifier,
		PlatformNotifications: platformNotifier,
		Clock:                 fixedClock{now: now},
	})

	summary, err := service.RunPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunPending error = %v", err)
	}

	if summary.Retrying != 1 {
		t.Fatalf("Retrying = %d, want 1", summary.Retrying)
	}
	if repository.lastRecord.CallbackStatus != StatusRetrying {
		t.Fatalf("callback status = %q, want retrying", repository.lastRecord.CallbackStatus)
	}
	if repository.lastRecord.NextRetryAt == nil {
		t.Fatal("NextRetryAt = nil, want non-nil")
	}
	if want := now.Add(time.Minute); !repository.lastRecord.NextRetryAt.Equal(want) {
		t.Fatalf("NextRetryAt = %v, want %v", repository.lastRecord.NextRetryAt, want)
	}
}

func TestRunPendingFinalFailureCreatesNotification(t *testing.T) {
	now := time.Date(2026, time.March, 30, 14, 20, 0, 0, time.UTC)
	repository := &stubRepository{
		dueCallbacks: []DueOutboundCallback{
			{
				OutboundCallback: OutboundCallback{
					ID:            "callback-3",
					StoreID:       "store-1",
					EventType:     memberPaymentSuccessEvent,
					ReferenceType: "qris_transaction",
					ReferenceID:   "qris-3",
					PayloadJSON:   []byte(`{"ok":false}`),
					Signature:     "sha256=ghi",
					Status:        StatusRetrying,
				},
				CallbackURL: "https://store.example/callback",
				AttemptNo:   5,
			},
		},
	}
	dispatcher := &stubDispatcher{err: errors.New("timeout")}
	storeNotifier := &storeNotifierStub{}
	platformNotifier := &platformNotifierStub{}

	service := NewService(Options{
		Repository:            repository,
		Dispatcher:            dispatcher,
		Notifications:         storeNotifier,
		PlatformNotifications: platformNotifier,
		Clock:                 fixedClock{now: now},
	})

	summary, err := service.RunPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunPending error = %v", err)
	}

	if summary.Failed != 1 {
		t.Fatalf("Failed = %d, want 1", summary.Failed)
	}
	if repository.lastRecord.CallbackStatus != StatusFailed {
		t.Fatalf("callback status = %q, want failed", repository.lastRecord.CallbackStatus)
	}
	if storeNotifier.calls != 1 {
		t.Fatalf("store notifier calls = %d, want 1", storeNotifier.calls)
	}
	if storeNotifier.last.eventType != "callback.delivery_failed" {
		t.Fatalf("notification event = %q, want callback.delivery_failed", storeNotifier.last.eventType)
	}
	if platformNotifier.calls != 1 {
		t.Fatalf("platformNotifierCalls = %d, want 1", platformNotifier.calls)
	}
	if platformNotifier.last.eventType != "callback.delivery_failed" {
		t.Fatalf("platform notification event = %q, want callback.delivery_failed", platformNotifier.last.eventType)
	}
	if platformNotifier.last.body != "Callback member_payment.success untuk store store-1 gagal setelah 6 percobaan." {
		t.Fatalf("platform notification body = %q", platformNotifier.last.body)
	}
}

func TestListQueueRejectsNonPlatformRole(t *testing.T) {
	service := NewService(Options{
		Repository: &stubRepository{},
	})

	_, err := service.ListQueue(context.Background(), auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	}, ListQueueFilter{})
	if !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("ListQueue error = %v, want auth.ErrUnauthorized", err)
	}
}

func TestListQueuePassesNormalizedFilter(t *testing.T) {
	repository := &stubRepository{
		queuePage: QueuePage{
			Items: []QueueItem{},
		},
	}
	service := NewService(Options{
		Repository: repository,
	})

	status := Status(" pending ")
	storeID := " store-1 "
	createdFrom := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)
	createdTo := time.Date(2026, time.April, 1, 10, 0, 0, 0, time.UTC)

	_, err := service.ListQueue(context.Background(), auth.Subject{
		UserID: "dev-1",
		Role:   auth.RoleDev,
	}, ListQueueFilter{
		Query:       " member_payment.success ",
		Status:      &status,
		StoreID:     &storeID,
		CreatedFrom: &createdFrom,
		CreatedTo:   &createdTo,
		Limit:       0,
		Offset:      -5,
	})
	if err != nil {
		t.Fatalf("ListQueue error = %v", err)
	}

	if repository.lastQueueFilter.Query != "member_payment.success" {
		t.Fatalf("query = %q", repository.lastQueueFilter.Query)
	}
	if repository.lastQueueFilter.Status == nil || *repository.lastQueueFilter.Status != StatusPending {
		t.Fatalf("status = %v, want pending", repository.lastQueueFilter.Status)
	}
	if repository.lastQueueFilter.StoreID == nil || *repository.lastQueueFilter.StoreID != "store-1" {
		t.Fatalf("storeID = %v, want store-1", repository.lastQueueFilter.StoreID)
	}
	if repository.lastQueueFilter.Limit != 25 {
		t.Fatalf("limit = %d, want 25", repository.lastQueueFilter.Limit)
	}
	if repository.lastQueueFilter.Offset != 0 {
		t.Fatalf("offset = %d, want 0", repository.lastQueueFilter.Offset)
	}
}

func TestListAttemptsRejectsNonPlatformRole(t *testing.T) {
	service := NewService(Options{
		Repository: &stubRepository{},
	})

	_, err := service.ListAttempts(context.Background(), auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	}, "callback-1", 10, 0)
	if !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("ListAttempts error = %v, want auth.ErrUnauthorized", err)
	}
}

type fixedClock struct {
	now time.Time
}

func (f fixedClock) Now() time.Time {
	return f.now
}

type stubRepository struct {
	source            MemberPaymentCallbackSource
	dueCallbacks      []DueOutboundCallback
	enqueueCalls      int
	recordCalls       int
	lastEnqueue       EnqueueOutboundCallbackParams
	lastRecord        RecordAttemptParams
	queuePage         QueuePage
	attemptPage       AttemptPage
	lastQueueFilter   ListQueueFilter
	lastAttemptID     string
	lastAttemptLimit  int
	lastAttemptOffset int
}

func (s *stubRepository) FindMemberPaymentCallbackSource(context.Context, string) (MemberPaymentCallbackSource, error) {
	return s.source, nil
}

func (s *stubRepository) EnqueueOutboundCallback(_ context.Context, params EnqueueOutboundCallbackParams) (OutboundCallback, error) {
	s.enqueueCalls++
	s.lastEnqueue = params
	return OutboundCallback{}, nil
}

func (s *stubRepository) ListDueOutboundCallbacks(context.Context, time.Time, int) ([]DueOutboundCallback, error) {
	return s.dueCallbacks, nil
}

func (s *stubRepository) RecordAttempt(_ context.Context, params RecordAttemptParams) error {
	s.recordCalls++
	s.lastRecord = params
	return nil
}

func (s *stubRepository) ListQueue(_ context.Context, filter ListQueueFilter) (QueuePage, error) {
	s.lastQueueFilter = filter
	return s.queuePage, nil
}

func (s *stubRepository) ListAttempts(_ context.Context, callbackID string, limit int, offset int) (AttemptPage, error) {
	s.lastAttemptID = callbackID
	s.lastAttemptLimit = limit
	s.lastAttemptOffset = offset
	return s.attemptPage, nil
}

type stubDispatcher struct {
	result DispatchResult
	err    error
}

type platformNotificationCall struct {
	eventType string
	title     string
	body      string
}

type notificationCall struct {
	storeID   string
	eventType string
	title     string
	body      string
}

type storeNotifierStub struct {
	calls int
	last  notificationCall
}

func (s *storeNotifierStub) Emit(storeID string, eventType string, title string, body string) {
	s.calls++
	s.last = notificationCall{
		storeID:   storeID,
		eventType: eventType,
		title:     title,
		body:      body,
	}
}

type platformNotifierStub struct {
	calls int
	last  platformNotificationCall
}

func (s *platformNotifierStub) Emit(eventType string, title string, body string) {
	s.calls++
	s.last = platformNotificationCall{
		eventType: eventType,
		title:     title,
		body:      body,
	}
}

func (s *stubDispatcher) Dispatch(context.Context, DueOutboundCallback) (DispatchResult, error) {
	return s.result, s.err
}

func stringPtr(value string) *string {
	return &value
}

func intPtr(value int) *int {
	return &value
}

var _ clock.Clock = fixedClock{}
