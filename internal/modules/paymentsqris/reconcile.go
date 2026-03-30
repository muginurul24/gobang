package paymentsqris

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/platform/clock"
	"github.com/mugiew/onixggr/internal/platform/qris"
)

type ReconcileLock interface {
	Unlock(ctx context.Context) error
}

type ReconcileRepositoryContract interface {
	AcquireReconcileLock(ctx context.Context, transactionID string) (ReconcileLock, bool, error)
	FindQRISTransactionByID(ctx context.Context, transactionID string) (QRISTransaction, error)
	NextReconcileAttemptNo(ctx context.Context, transactionID string) (int, error)
	ListDueReconcileTransactions(ctx context.Context, now time.Time, limit int) ([]ReconcileCandidate, error)
	RecordReconcileAttempt(ctx context.Context, params RecordReconcileAttemptParams) error
}

type ReconcileUpstreamClient interface {
	CheckStatus(ctx context.Context, input qris.CheckStatusInput) (qris.PaymentStatusResult, error)
}

type ReconcileFinalizer interface {
	HandlePaymentWebhook(ctx context.Context, payload qris.PaymentWebhook, metadata auth.RequestMetadata) (WebhookDispatchResult, error)
}

type ReconcileService interface {
	RunPending(ctx context.Context, limit int) (ReconcileRunSummary, error)
	ReconcileTransaction(ctx context.Context, transactionID string) (ReconcileOutcome, error)
}

type ReconcileOptions struct {
	Repository ReconcileRepositoryContract
	Upstream   ReconcileUpstreamClient
	Finalizer  ReconcileFinalizer
	Clock      clock.Clock
}

type reconcileService struct {
	repository ReconcileRepositoryContract
	upstream   ReconcileUpstreamClient
	finalizer  ReconcileFinalizer
	clock      clock.Clock
}

func NewReconcileService(options ReconcileOptions) ReconcileService {
	now := options.Clock
	if now == nil {
		now = clock.SystemClock{}
	}

	upstream := options.Upstream
	if upstream == nil {
		upstream = noopReconcileUpstream{}
	}

	finalizer := options.Finalizer
	if finalizer == nil {
		finalizer = noopReconcileFinalizer{}
	}

	return &reconcileService{
		repository: options.Repository,
		upstream:   upstream,
		finalizer:  finalizer,
		clock:      now,
	}
}

func (s *reconcileService) RunPending(ctx context.Context, limit int) (ReconcileRunSummary, error) {
	if limit <= 0 {
		limit = 50
	}

	candidates, err := s.repository.ListDueReconcileTransactions(ctx, s.clock.Now().UTC(), limit)
	if err != nil {
		return ReconcileRunSummary{}, err
	}

	summary := ReconcileRunSummary{
		Scanned: len(candidates),
	}

	var runErr error
	for _, candidate := range candidates {
		outcome, err := s.ReconcileTransaction(ctx, candidate.Transaction.ID)
		if err != nil {
			summary.StillPending++
			if runErr == nil {
				runErr = fmt.Errorf("reconcile qris transaction %s: %w", candidate.Transaction.ID, err)
			}
			continue
		}

		switch outcome {
		case ReconcileOutcomeFinalizedSuccess:
			summary.FinalizedSuccess++
		case ReconcileOutcomeFinalizedExpired:
			summary.FinalizedExpired++
		case ReconcileOutcomeFinalizedFailed:
			summary.FinalizedFailed++
		case ReconcileOutcomeStillPending:
			summary.StillPending++
		default:
			summary.Skipped++
		}
	}

	return summary, runErr
}

func (s *reconcileService) ReconcileTransaction(ctx context.Context, transactionID string) (outcome ReconcileOutcome, err error) {
	lock, locked, err := s.repository.AcquireReconcileLock(ctx, transactionID)
	if err != nil {
		return ReconcileOutcomeStillPending, err
	}
	if !locked {
		return ReconcileOutcomeSkipped, nil
	}
	defer func() {
		if unlockErr := lock.Unlock(ctx); unlockErr != nil && err == nil {
			err = unlockErr
		}
	}()

	transaction, err := s.repository.FindQRISTransactionByID(ctx, transactionID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ReconcileOutcomeSkipped, nil
		}
		return ReconcileOutcomeStillPending, err
	}
	if transaction.Status != TransactionStatusPending || transaction.ProviderTrxID == nil || strings.TrimSpace(*transaction.ProviderTrxID) == "" {
		return ReconcileOutcomeSkipped, nil
	}

	attemptNo, err := s.repository.NextReconcileAttemptNo(ctx, transactionID)
	if err != nil {
		return ReconcileOutcomeStillPending, err
	}

	now := s.clock.Now().UTC()
	statusResult, err := s.upstream.CheckStatus(ctx, qris.CheckStatusInput{
		TrxID: strings.TrimSpace(*transaction.ProviderTrxID),
	})
	if err != nil {
		var businessErr *qris.BusinessError
		status := "upstream_error"
		response := map[string]any{
			"source": "check_status",
			"error":  err.Error(),
		}
		if errors.As(err, &businessErr) {
			status = "business_error"
			response["code"] = businessErr.Code
			response["message"] = businessErr.Message
		}

		if recordErr := s.repository.RecordReconcileAttempt(ctx, RecordReconcileAttemptParams{
			QRISTransactionID: transaction.ID,
			AttemptNo:         attemptNo,
			Status:            status,
			ResponseMasked:    response,
			OccurredAt:        now,
		}); recordErr != nil {
			return ReconcileOutcomeStillPending, recordErr
		}

		return ReconcileOutcomeStillPending, nil
	}

	resolvedStatus := strings.ToLower(strings.TrimSpace(statusResult.Status))
	response := map[string]any{
		"amount":      statusResult.Amount,
		"merchant_id": statusResult.MerchantID,
		"trx_id":      statusResult.TrxID,
		"rrn":         statusResult.RRN,
		"status":      resolvedStatus,
		"terminal_id": statusResult.TerminalID,
		"custom_ref":  statusResult.CustomRef,
		"vendor":      statusResult.Vendor,
		"created_at":  formatOptionalTime(statusResult.CreatedAt),
		"finish_at":   formatOptionalTime(statusResult.FinishAt),
	}

	finalStatus, isFinal := resolvePaymentStatus(resolvedStatus)
	if !isFinal && resolvedStatus == string(TransactionStatusPending) && shouldFinalizeExpired(now, transaction.ExpiresAt) {
		finalStatus = TransactionStatusExpired
		isFinal = true
		response["status"] = string(TransactionStatusExpired)
	}

	recordStatus := resolvedStatus
	if isFinal && finalStatus != TransactionStatusSuccess {
		recordStatus = string(finalStatus)
	}
	if recordStatus == "" {
		recordStatus = "unknown"
	}
	if recordErr := s.repository.RecordReconcileAttempt(ctx, RecordReconcileAttemptParams{
		QRISTransactionID: transaction.ID,
		AttemptNo:         attemptNo,
		Status:            recordStatus,
		ResponseMasked:    response,
		OccurredAt:        now,
	}); recordErr != nil {
		return ReconcileOutcomeStillPending, recordErr
	}

	if !isFinal {
		return ReconcileOutcomeStillPending, nil
	}

	if _, err := s.finalizer.HandlePaymentWebhook(ctx, qris.PaymentWebhook{
		Amount:     statusResult.Amount,
		TerminalID: statusResult.TerminalID,
		TrxID:      transactionProviderTrxID(transaction, statusResult.TrxID),
		RRN:        statusResult.RRN,
		CustomRef:  transaction.CustomRef,
		Vendor:     statusResult.Vendor,
		Status:     string(finalStatus),
		CreatedAt:  statusResult.CreatedAt,
		FinishAt:   statusResult.FinishAt,
	}, auth.RequestMetadata{
		IPAddress: "127.0.0.1",
		UserAgent: "qris-reconcile-worker",
	}); err != nil {
		return ReconcileOutcomeStillPending, err
	}

	if finalStatus == TransactionStatusSuccess {
		return ReconcileOutcomeFinalizedSuccess, nil
	}
	if finalStatus == TransactionStatusFailed {
		return ReconcileOutcomeFinalizedFailed, nil
	}

	return ReconcileOutcomeFinalizedExpired, nil
}

func shouldFinalizeExpired(now time.Time, expiresAt *time.Time) bool {
	if expiresAt == nil {
		return false
	}

	return !now.Before(expiresAt.UTC())
}

func transactionProviderTrxID(transaction QRISTransaction, statusTrxID string) string {
	if trimmed := strings.TrimSpace(statusTrxID); trimmed != "" {
		return trimmed
	}
	if transaction.ProviderTrxID == nil {
		return ""
	}

	return strings.TrimSpace(*transaction.ProviderTrxID)
}

type noopReconcileUpstream struct{}

func (noopReconcileUpstream) CheckStatus(context.Context, qris.CheckStatusInput) (qris.PaymentStatusResult, error) {
	return qris.PaymentStatusResult{}, qris.ErrNotConfigured
}

type noopReconcileFinalizer struct{}

func (noopReconcileFinalizer) HandlePaymentWebhook(context.Context, qris.PaymentWebhook, auth.RequestMetadata) (WebhookDispatchResult, error) {
	return WebhookDispatchResult{}, nil
}
