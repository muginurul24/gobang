package paymentsqris

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/platform/qris"
)

func TestReconcileTransactionSuccessFinalizesViaWebhook(t *testing.T) {
	now := time.Date(2026, time.March, 30, 15, 0, 0, 0, time.UTC)
	repository := &fakeReconcileRepository{
		transaction: QRISTransaction{
			ID:            "qris-1",
			StoreID:       "store-1",
			Type:          TransactionTypeStoreTopup,
			ProviderTrxID: stringPtr("provider-1"),
			CustomRef:     "TOPUP001",
			Status:        TransactionStatusPending,
		},
		nextAttemptNo: 1,
		lock:          &fakeReconcileLock{},
	}
	finalizer := &fakeReconcileFinalizer{}

	service := NewReconcileService(ReconcileOptions{
		Repository: repository,
		Upstream: &fakeReconcileUpstream{
			result: qris.PaymentStatusResult{
				Amount: 10000,
				TrxID:  "provider-1",
				Status: "success",
			},
		},
		Finalizer: finalizer,
		Clock:     fixedReconcileClock{now: now},
	})

	outcome, err := service.ReconcileTransaction(context.Background(), "qris-1")
	if err != nil {
		t.Fatalf("ReconcileTransaction error = %v", err)
	}

	if outcome != ReconcileOutcomeFinalizedSuccess {
		t.Fatalf("outcome = %q, want finalized_success", outcome)
	}
	if repository.lastAttempt.Status != "success" {
		t.Fatalf("attempt status = %q, want success", repository.lastAttempt.Status)
	}
	if finalizer.calls != 1 {
		t.Fatalf("finalizer calls = %d, want 1", finalizer.calls)
	}
	if finalizer.lastPayload.Status != "success" {
		t.Fatalf("finalizer status = %q, want success", finalizer.lastPayload.Status)
	}
}

func TestReconcileTransactionPendingKeepsPending(t *testing.T) {
	now := time.Date(2026, time.March, 30, 15, 5, 0, 0, time.UTC)
	expiresAt := now.Add(5 * time.Minute)
	repository := &fakeReconcileRepository{
		transaction: QRISTransaction{
			ID:            "qris-2",
			StoreID:       "store-1",
			Type:          TransactionTypeStoreTopup,
			ProviderTrxID: stringPtr("provider-2"),
			CustomRef:     "TOPUP002",
			Status:        TransactionStatusPending,
			ExpiresAt:     &expiresAt,
		},
		nextAttemptNo: 2,
		lock:          &fakeReconcileLock{},
	}
	finalizer := &fakeReconcileFinalizer{}

	service := NewReconcileService(ReconcileOptions{
		Repository: repository,
		Upstream: &fakeReconcileUpstream{
			result: qris.PaymentStatusResult{
				Amount: 10000,
				TrxID:  "provider-2",
				Status: "pending",
			},
		},
		Finalizer: finalizer,
		Clock:     fixedReconcileClock{now: now},
	})

	outcome, err := service.ReconcileTransaction(context.Background(), "qris-2")
	if err != nil {
		t.Fatalf("ReconcileTransaction error = %v", err)
	}

	if outcome != ReconcileOutcomeStillPending {
		t.Fatalf("outcome = %q, want still_pending", outcome)
	}
	if repository.lastAttempt.Status != "pending" {
		t.Fatalf("attempt status = %q, want pending", repository.lastAttempt.Status)
	}
	if finalizer.calls != 0 {
		t.Fatalf("finalizer calls = %d, want 0", finalizer.calls)
	}
}

func TestReconcileTransactionExpiredFinalizesAfterProviderPending(t *testing.T) {
	now := time.Date(2026, time.March, 30, 15, 10, 0, 0, time.UTC)
	expiresAt := now.Add(-time.Minute)
	repository := &fakeReconcileRepository{
		transaction: QRISTransaction{
			ID:            "qris-3",
			StoreID:       "store-1",
			Type:          TransactionTypeMemberPayment,
			ProviderTrxID: stringPtr("provider-3"),
			CustomRef:     "MPAY003",
			Status:        TransactionStatusPending,
			ExpiresAt:     &expiresAt,
		},
		nextAttemptNo: 3,
		lock:          &fakeReconcileLock{},
	}
	finalizer := &fakeReconcileFinalizer{}

	service := NewReconcileService(ReconcileOptions{
		Repository: repository,
		Upstream: &fakeReconcileUpstream{
			result: qris.PaymentStatusResult{
				Amount: 25000,
				TrxID:  "provider-3",
				Status: "pending",
			},
		},
		Finalizer: finalizer,
		Clock:     fixedReconcileClock{now: now},
	})

	outcome, err := service.ReconcileTransaction(context.Background(), "qris-3")
	if err != nil {
		t.Fatalf("ReconcileTransaction error = %v", err)
	}

	if outcome != ReconcileOutcomeFinalizedExpired {
		t.Fatalf("outcome = %q, want finalized_expired", outcome)
	}
	if repository.lastAttempt.Status != "expired" {
		t.Fatalf("attempt status = %q, want expired", repository.lastAttempt.Status)
	}
	if finalizer.calls != 1 {
		t.Fatalf("finalizer calls = %d, want 1", finalizer.calls)
	}
	if finalizer.lastPayload.Status != "expired" {
		t.Fatalf("finalizer status = %q, want expired", finalizer.lastPayload.Status)
	}
}

func TestReconcileTransactionUpstreamErrorRecordsAttempt(t *testing.T) {
	now := time.Date(2026, time.March, 30, 15, 15, 0, 0, time.UTC)
	repository := &fakeReconcileRepository{
		transaction: QRISTransaction{
			ID:            "qris-4",
			StoreID:       "store-1",
			Type:          TransactionTypeStoreTopup,
			ProviderTrxID: stringPtr("provider-4"),
			CustomRef:     "TOPUP004",
			Status:        TransactionStatusPending,
		},
		nextAttemptNo: 1,
		lock:          &fakeReconcileLock{},
	}
	finalizer := &fakeReconcileFinalizer{}

	service := NewReconcileService(ReconcileOptions{
		Repository: repository,
		Upstream:   &fakeReconcileUpstream{err: qris.ErrTimeout},
		Finalizer:  finalizer,
		Clock:      fixedReconcileClock{now: now},
	})

	outcome, err := service.ReconcileTransaction(context.Background(), "qris-4")
	if err != nil {
		t.Fatalf("ReconcileTransaction error = %v", err)
	}

	if outcome != ReconcileOutcomeStillPending {
		t.Fatalf("outcome = %q, want still_pending", outcome)
	}
	if repository.lastAttempt.Status != "upstream_error" {
		t.Fatalf("attempt status = %q, want upstream_error", repository.lastAttempt.Status)
	}
	if finalizer.calls != 0 {
		t.Fatalf("finalizer calls = %d, want 0", finalizer.calls)
	}
}

type fixedReconcileClock struct {
	now time.Time
}

func (f fixedReconcileClock) Now() time.Time {
	return f.now
}

type fakeReconcileRepository struct {
	transaction   QRISTransaction
	nextAttemptNo int
	lastAttempt   RecordReconcileAttemptParams
	lock          ReconcileLock
	lockAcquired  bool
}

func (r *fakeReconcileRepository) AcquireReconcileLock(context.Context, string) (ReconcileLock, bool, error) {
	if r.lock == nil {
		return nil, false, nil
	}

	return r.lock, true, nil
}

func (r *fakeReconcileRepository) FindQRISTransactionByID(context.Context, string) (QRISTransaction, error) {
	if r.transaction.ID == "" {
		return QRISTransaction{}, ErrNotFound
	}

	return r.transaction, nil
}

func (r *fakeReconcileRepository) NextReconcileAttemptNo(context.Context, string) (int, error) {
	return r.nextAttemptNo, nil
}

func (r *fakeReconcileRepository) ListDueReconcileTransactions(context.Context, time.Time, int) ([]ReconcileCandidate, error) {
	if r.transaction.ID == "" {
		return nil, nil
	}

	return []ReconcileCandidate{{Transaction: r.transaction}}, nil
}

func (r *fakeReconcileRepository) RecordReconcileAttempt(_ context.Context, params RecordReconcileAttemptParams) error {
	r.lastAttempt = params
	return nil
}

type fakeReconcileLock struct{}

func (f *fakeReconcileLock) Unlock(context.Context) error {
	return nil
}

type fakeReconcileUpstream struct {
	result qris.PaymentStatusResult
	err    error
}

func (f *fakeReconcileUpstream) CheckStatus(context.Context, qris.CheckStatusInput) (qris.PaymentStatusResult, error) {
	if f.err != nil {
		return qris.PaymentStatusResult{}, f.err
	}

	return f.result, nil
}

type fakeReconcileFinalizer struct {
	calls       int
	lastPayload qris.PaymentWebhook
}

func (f *fakeReconcileFinalizer) HandlePaymentWebhook(_ context.Context, payload qris.PaymentWebhook, _ auth.RequestMetadata) (WebhookDispatchResult, error) {
	f.calls++
	f.lastPayload = payload
	return WebhookDispatchResult{}, nil
}

var _ ReconcileLock = (*fakeReconcileLock)(nil)

func TestRunPendingCountsFinalizedAndSkipped(t *testing.T) {
	now := time.Date(2026, time.March, 30, 15, 20, 0, 0, time.UTC)
	repository := &fakeRunRepository{
		candidates: []ReconcileCandidate{
			{Transaction: QRISTransaction{ID: "qris-success", StoreID: "store-1", ProviderTrxID: stringPtr("trx-success"), Status: TransactionStatusPending}},
			{Transaction: QRISTransaction{ID: "qris-skip", StoreID: "store-1", Status: TransactionStatusSuccess}},
		},
	}
	service := NewReconcileService(ReconcileOptions{
		Repository: repository,
		Upstream: &fakeReconcileUpstream{
			result: qris.PaymentStatusResult{Amount: 10000, TrxID: "trx-success", Status: "success"},
		},
		Finalizer: &fakeReconcileFinalizer{},
		Clock:     fixedReconcileClock{now: now},
	})

	summary, err := service.RunPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunPending error = %v", err)
	}

	if summary.Scanned != 2 {
		t.Fatalf("Scanned = %d, want 2", summary.Scanned)
	}
}

type fakeRunRepository struct {
	candidates []ReconcileCandidate
}

func (r *fakeRunRepository) AcquireReconcileLock(context.Context, string) (ReconcileLock, bool, error) {
	return &fakeReconcileLock{}, true, nil
}

func (r *fakeRunRepository) FindQRISTransactionByID(_ context.Context, transactionID string) (QRISTransaction, error) {
	for _, candidate := range r.candidates {
		if candidate.Transaction.ID == transactionID {
			return candidate.Transaction, nil
		}
	}

	return QRISTransaction{}, ErrNotFound
}

func (r *fakeRunRepository) NextReconcileAttemptNo(context.Context, string) (int, error) {
	return 1, nil
}

func (r *fakeRunRepository) ListDueReconcileTransactions(context.Context, time.Time, int) ([]ReconcileCandidate, error) {
	return r.candidates, nil
}

func (r *fakeRunRepository) RecordReconcileAttempt(context.Context, RecordReconcileAttemptParams) error {
	return nil
}

var _ ReconcileRepositoryContract = (*fakeReconcileRepository)(nil)
var _ ReconcileRepositoryContract = (*fakeRunRepository)(nil)

func TestReconcileTransactionBusinessErrorKeepsPending(t *testing.T) {
	now := time.Date(2026, time.March, 30, 15, 25, 0, 0, time.UTC)
	repository := &fakeReconcileRepository{
		transaction: QRISTransaction{
			ID:            "qris-5",
			StoreID:       "store-1",
			Type:          TransactionTypeStoreTopup,
			ProviderTrxID: stringPtr("provider-5"),
			CustomRef:     "TOPUP005",
			Status:        TransactionStatusPending,
		},
		nextAttemptNo: 1,
		lock:          &fakeReconcileLock{},
	}

	service := NewReconcileService(ReconcileOptions{
		Repository: repository,
		Upstream:   &fakeReconcileUpstream{err: &qris.BusinessError{Code: "NOT_FOUND", Message: "transaction not found"}},
		Finalizer:  &fakeReconcileFinalizer{},
		Clock:      fixedReconcileClock{now: now},
	})

	outcome, err := service.ReconcileTransaction(context.Background(), "qris-5")
	if err != nil {
		t.Fatalf("ReconcileTransaction error = %v", err)
	}

	if outcome != ReconcileOutcomeStillPending {
		t.Fatalf("outcome = %q, want still_pending", outcome)
	}
	if repository.lastAttempt.Status != "business_error" {
		t.Fatalf("attempt status = %q, want business_error", repository.lastAttempt.Status)
	}
}

var _ ReconcileUpstreamClient = (*fakeReconcileUpstream)(nil)
var _ ReconcileFinalizer = (*fakeReconcileFinalizer)(nil)

func TestReconcileTransactionLockMissSkips(t *testing.T) {
	service := NewReconcileService(ReconcileOptions{
		Repository: &fakeNoLockRepository{},
		Clock:      fixedReconcileClock{now: time.Date(2026, time.March, 30, 15, 30, 0, 0, time.UTC)},
	})

	outcome, err := service.ReconcileTransaction(context.Background(), "qris-skip")
	if err != nil {
		t.Fatalf("ReconcileTransaction error = %v", err)
	}

	if outcome != ReconcileOutcomeSkipped {
		t.Fatalf("outcome = %q, want skipped", outcome)
	}
}

type fakeNoLockRepository struct{}

func (r *fakeNoLockRepository) AcquireReconcileLock(context.Context, string) (ReconcileLock, bool, error) {
	return nil, false, nil
}

func (r *fakeNoLockRepository) FindQRISTransactionByID(context.Context, string) (QRISTransaction, error) {
	return QRISTransaction{}, ErrNotFound
}

func (r *fakeNoLockRepository) NextReconcileAttemptNo(context.Context, string) (int, error) {
	return 0, errors.New("should not be called")
}

func (r *fakeNoLockRepository) ListDueReconcileTransactions(context.Context, time.Time, int) ([]ReconcileCandidate, error) {
	return nil, nil
}

func (r *fakeNoLockRepository) RecordReconcileAttempt(context.Context, RecordReconcileAttemptParams) error {
	return nil
}

var _ ReconcileRepositoryContract = (*fakeNoLockRepository)(nil)
