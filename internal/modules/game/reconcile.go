package game

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/modules/ledger"
	"github.com/mugiew/onixggr/internal/platform/clock"
	"github.com/mugiew/onixggr/internal/platform/nexusggr"
)

type ReconcileLock interface {
	Unlock(ctx context.Context) error
}

type ReconcileRepositoryContract interface {
	AcquireReconcileLock(ctx context.Context, transactionID string) (ReconcileLock, bool, error)
	FindGameTransactionByID(ctx context.Context, transactionID string) (GameTransaction, error)
	FindStoreMemberByID(ctx context.Context, memberID string) (StoreMember, error)
	ListPendingReconcileTransactions(ctx context.Context, limit int) ([]GameTransaction, error)
	FinalizeGameTransactionReconcile(ctx context.Context, params FinalizeGameTransactionReconcileParams) (GameTransaction, error)
}

type ReconcileUpstreamClient interface {
	TransferStatus(ctx context.Context, input nexusggr.TransferStatusInput) (nexusggr.TransferStatusResult, error)
}

type ReconcileLedgerService interface {
	Credit(ctx context.Context, storeID string, input ledger.PostEntryInput) (ledger.PostingResult, error)
	CommitReservation(ctx context.Context, storeID string, input ledger.CommitReservationInput) (ledger.CommitReservationResult, error)
	HasReferenceEntries(ctx context.Context, referenceType string, referenceID string) (bool, error)
	ReleaseReservation(ctx context.Context, storeID string, input ledger.ReleaseReservationInput) (ledger.ReservationResult, error)
}

type ReconcileService interface {
	RunPending(ctx context.Context, limit int) (ReconcileRunSummary, error)
	ReconcileTransaction(ctx context.Context, transactionID string) (ReconcileOutcome, error)
}

type ReconcileOptions struct {
	Repository ReconcileRepositoryContract
	Upstream   ReconcileUpstreamClient
	Ledger     ReconcileLedgerService
	Clock      clock.Clock
}

type ReconcileOutcome string

const (
	ReconcileOutcomeFinalizedSuccess ReconcileOutcome = "finalized_success"
	ReconcileOutcomeFinalizedFailed  ReconcileOutcome = "finalized_failed"
	ReconcileOutcomeStillPending     ReconcileOutcome = "still_pending"
	ReconcileOutcomeSkipped          ReconcileOutcome = "skipped"
)

type ReconcileRunSummary struct {
	Scanned          int `json:"scanned"`
	FinalizedSuccess int `json:"finalized_success"`
	FinalizedFailed  int `json:"finalized_failed"`
	StillPending     int `json:"still_pending"`
	Skipped          int `json:"skipped"`
}

type FinalizeGameTransactionReconcileParams struct {
	GameTransactionID      string
	Status                 TransactionStatus
	ReconcileStatus        ReconcileStatus
	UpstreamErrorCode      *string
	UpstreamResponseMasked map[string]any
	AuditAction            string
	AuditPayloadMasked     map[string]any
	NotificationEventType  string
	NotificationTitle      string
	NotificationBody       string
	OccurredAt             time.Time
}

type reconcileService struct {
	repository ReconcileRepositoryContract
	upstream   ReconcileUpstreamClient
	ledger     ReconcileLedgerService
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

	ledgerService := options.Ledger
	if ledgerService == nil {
		ledgerService = noopReconcileLedger{}
	}

	return &reconcileService{
		repository: options.Repository,
		upstream:   upstream,
		ledger:     ledgerService,
		clock:      now,
	}
}

func (s *reconcileService) RunPending(ctx context.Context, limit int) (ReconcileRunSummary, error) {
	if limit <= 0 {
		limit = 50
	}

	transactions, err := s.repository.ListPendingReconcileTransactions(ctx, limit)
	if err != nil {
		return ReconcileRunSummary{}, err
	}

	summary := ReconcileRunSummary{
		Scanned: len(transactions),
	}

	var runErr error
	for _, transaction := range transactions {
		outcome, err := s.ReconcileTransaction(ctx, transaction.ID)
		if err != nil {
			summary.StillPending++
			if runErr == nil {
				runErr = fmt.Errorf("reconcile game transaction %s: %w", transaction.ID, err)
			}
			continue
		}

		switch outcome {
		case ReconcileOutcomeFinalizedSuccess:
			summary.FinalizedSuccess++
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

	transaction, err := s.repository.FindGameTransactionByID(ctx, transactionID)
	if err != nil {
		return ReconcileOutcomeStillPending, err
	}

	if transaction.Status != TransactionStatusPending || transaction.ReconcileStatus == nil || *transaction.ReconcileStatus != ReconcileStatusPending {
		return ReconcileOutcomeSkipped, nil
	}

	statusResult, err := s.upstream.TransferStatus(ctx, nexusggr.TransferStatusInput{
		UserCode:  transaction.UpstreamUserCode,
		AgentSign: transaction.AgentSign,
	})
	if err == nil {
		return s.finalizeReconcileSuccess(ctx, transaction, statusResult)
	}

	var businessErr *nexusggr.BusinessError
	switch {
	case errors.As(err, &businessErr):
		return s.finalizeReconcileFailure(ctx, transaction, businessErr.Code, map[string]any{
			"message": businessErr.Message,
			"code":    businessErr.Code,
			"source":  "transfer_status",
		})
	case errors.Is(err, nexusggr.ErrNotConfigured),
		errors.Is(err, nexusggr.ErrTimeout),
		errors.Is(err, nexusggr.ErrUpstreamUnavailable),
		errors.Is(err, nexusggr.ErrUnexpectedHTTP),
		errors.Is(err, nexusggr.ErrInvalidResponse):
		return ReconcileOutcomeStillPending, nil
	default:
		return ReconcileOutcomeStillPending, err
	}
}

func (s *reconcileService) finalizeReconcileSuccess(ctx context.Context, transaction GameTransaction, statusResult nexusggr.TransferStatusResult) (ReconcileOutcome, error) {
	if err := validateTransferStatusMatch(transaction, statusResult); err != nil {
		return ReconcileOutcomeStillPending, err
	}

	member, err := s.repository.FindStoreMemberByID(ctx, transaction.StoreMemberID)
	if err != nil {
		return ReconcileOutcomeStillPending, err
	}

	alreadyPosted, err := s.ledger.HasReferenceEntries(ctx, "game_transaction", transaction.ID)
	if err != nil {
		return ReconcileOutcomeStillPending, err
	}

	switch transaction.Action {
	case GameActionDeposit:
		if !alreadyPosted {
			if _, err := s.ledger.CommitReservation(ctx, transaction.StoreID, ledger.CommitReservationInput{
				ReferenceType: "game_transaction",
				ReferenceID:   transaction.ID,
				Entries: []ledger.ReservationCommitEntryInput{
					{
						EntryType: ledger.EntryTypeGameDeposit,
						Amount:    transaction.Amount,
						Metadata: map[string]any{
							"trx_id":             transaction.TrxID,
							"real_username":      member.RealUsername,
							"upstream_user_code": member.UpstreamUserCode,
						},
					},
				},
			}); err != nil {
				return ReconcileOutcomeStillPending, err
			}
		}
	case GameActionWithdraw:
		if !alreadyPosted {
			if _, err := s.ledger.Credit(ctx, transaction.StoreID, ledger.PostEntryInput{
				EntryType:     ledger.EntryTypeGameWithdraw,
				Amount:        transaction.Amount,
				ReferenceType: "game_transaction",
				ReferenceID:   transaction.ID,
				Metadata: map[string]any{
					"trx_id":             transaction.TrxID,
					"real_username":      member.RealUsername,
					"upstream_user_code": member.UpstreamUserCode,
				},
			}); err != nil {
				return ReconcileOutcomeStillPending, err
			}
		}
	default:
		return ReconcileOutcomeStillPending, fmt.Errorf("unsupported game action %q", transaction.Action)
	}

	now := s.clock.Now().UTC()
	_, err = s.repository.FinalizeGameTransactionReconcile(ctx, FinalizeGameTransactionReconcileParams{
		GameTransactionID: transaction.ID,
		Status:            TransactionStatusSuccess,
		ReconcileStatus:   ReconcileStatusResolved,
		UpstreamResponseMasked: map[string]any{
			"message":       statusResult.Message,
			"amount":        statusResult.Amount,
			"agent_balance": statusResult.AgentBalance,
			"user_balance":  statusResult.UserBalance,
			"type":          statusResult.Type,
			"source":        "transfer_status",
		},
		AuditAction:           reconcileAuditAction(transaction.Action, TransactionStatusSuccess),
		AuditPayloadMasked:    reconcileAuditPayload(transaction, member.RealUsername, nil),
		NotificationEventType: reconcileNotificationEventType(transaction.Action, TransactionStatusSuccess),
		NotificationTitle:     reconcileNotificationTitle(transaction.Action, TransactionStatusSuccess),
		NotificationBody:      reconcileNotificationBody(transaction, member.RealUsername, TransactionStatusSuccess),
		OccurredAt:            now,
	})
	if err != nil {
		return ReconcileOutcomeStillPending, err
	}

	return ReconcileOutcomeFinalizedSuccess, nil
}

func (s *reconcileService) finalizeReconcileFailure(ctx context.Context, transaction GameTransaction, code string, response map[string]any) (ReconcileOutcome, error) {
	alreadyPosted, err := s.ledger.HasReferenceEntries(ctx, "game_transaction", transaction.ID)
	if err != nil {
		return ReconcileOutcomeStillPending, err
	}
	if alreadyPosted {
		return s.finalizeReconcileSuccess(ctx, transaction, fallbackTransferStatusResult(transaction))
	}

	if transaction.Action == GameActionDeposit {
		if _, err := s.ledger.ReleaseReservation(ctx, transaction.StoreID, ledger.ReleaseReservationInput{
			ReferenceType: "game_transaction",
			ReferenceID:   transaction.ID,
		}); err != nil {
			if !errors.Is(err, ledger.ErrReservationFinalized) {
				return ReconcileOutcomeStillPending, err
			}

			alreadyPosted, checkErr := s.ledger.HasReferenceEntries(ctx, "game_transaction", transaction.ID)
			if checkErr != nil {
				return ReconcileOutcomeStillPending, checkErr
			}
			if alreadyPosted {
				return s.finalizeReconcileSuccess(ctx, transaction, fallbackTransferStatusResult(transaction))
			}

			return ReconcileOutcomeStillPending, err
		}
	}

	now := s.clock.Now().UTC()
	_, err = s.repository.FinalizeGameTransactionReconcile(ctx, FinalizeGameTransactionReconcileParams{
		GameTransactionID:      transaction.ID,
		Status:                 TransactionStatusFailed,
		ReconcileStatus:        ReconcileStatusResolved,
		UpstreamErrorCode:      nullableString(code),
		UpstreamResponseMasked: response,
		AuditAction:            reconcileAuditAction(transaction.Action, TransactionStatusFailed),
		AuditPayloadMasked:     reconcileAuditPayload(transaction, "", nullableString(code)),
		NotificationEventType:  reconcileNotificationEventType(transaction.Action, TransactionStatusFailed),
		NotificationTitle:      reconcileNotificationTitle(transaction.Action, TransactionStatusFailed),
		NotificationBody:       reconcileNotificationBody(transaction, "", TransactionStatusFailed),
		OccurredAt:             now,
	})
	if err != nil {
		return ReconcileOutcomeStillPending, err
	}

	return ReconcileOutcomeFinalizedFailed, nil
}

func reconcileAuditAction(action GameAction, status TransactionStatus) string {
	switch {
	case action == GameActionDeposit && status == TransactionStatusSuccess:
		return "game.deposit_reconciled_success"
	case action == GameActionDeposit && status == TransactionStatusFailed:
		return "game.deposit_reconciled_failed"
	case action == GameActionWithdraw && status == TransactionStatusSuccess:
		return "game.withdraw_reconciled_success"
	default:
		return "game.withdraw_reconciled_failed"
	}
}

func reconcileNotificationEventType(action GameAction, status TransactionStatus) string {
	switch {
	case action == GameActionDeposit && status == TransactionStatusSuccess:
		return "game.deposit.reconciled_success"
	case action == GameActionDeposit && status == TransactionStatusFailed:
		return "game.deposit.reconciled_failed"
	case action == GameActionWithdraw && status == TransactionStatusSuccess:
		return "game.withdraw.reconciled_success"
	default:
		return "game.withdraw.reconciled_failed"
	}
}

func reconcileNotificationTitle(action GameAction, status TransactionStatus) string {
	switch {
	case action == GameActionDeposit && status == TransactionStatusSuccess:
		return "Game deposit berhasil direconcile"
	case action == GameActionDeposit && status == TransactionStatusFailed:
		return "Game deposit gagal direconcile"
	case action == GameActionWithdraw && status == TransactionStatusSuccess:
		return "Game withdraw berhasil direconcile"
	default:
		return "Game withdraw gagal direconcile"
	}
}

func reconcileNotificationBody(transaction GameTransaction, realUsername string, status TransactionStatus) string {
	actionLabel := "withdraw"
	if transaction.Action == GameActionDeposit {
		actionLabel = "deposit"
	}

	memberLabel := "member"
	if realUsername != "" {
		memberLabel = realUsername
	}

	if status == TransactionStatusSuccess {
		return fmt.Sprintf("Transaksi game %s %s untuk %s sudah final success.", actionLabel, transaction.TrxID, memberLabel)
	}

	return fmt.Sprintf("Transaksi game %s %s untuk %s difinalisasi sebagai gagal.", actionLabel, transaction.TrxID, memberLabel)
}

func reconcileAuditPayload(transaction GameTransaction, realUsername string, code *string) map[string]any {
	payload := map[string]any{
		"trx_id": transaction.TrxID,
		"amount": transaction.Amount,
		"source": "transfer_status",
	}
	if realUsername != "" {
		payload["real_username"] = realUsername
	}
	if code != nil && *code != "" {
		payload["code"] = *code
	}

	return payload
}

func fallbackTransferStatusResult(transaction GameTransaction) nexusggr.TransferStatusResult {
	amount, err := parseMoney(transaction.Amount)
	if err != nil {
		amount = 0
	}

	resultType := "user_withdraw"
	if transaction.Action == GameActionDeposit {
		resultType = "user_deposit"
	}

	return nexusggr.TransferStatusResult{
		Message: "SUCCESS",
		Amount:  amount.Float64(),
		Type:    resultType,
	}
}

func validateTransferStatusMatch(transaction GameTransaction, statusResult nexusggr.TransferStatusResult) error {
	expectedType := "user_withdraw"
	if transaction.Action == GameActionDeposit {
		expectedType = "user_deposit"
	}
	if strings.TrimSpace(statusResult.Type) != expectedType {
		return fmt.Errorf("transfer status type mismatch: got %q want %q", statusResult.Type, expectedType)
	}

	expectedAmount, err := parseMoney(transaction.Amount)
	if err != nil {
		return err
	}
	if expectedAmount != moneyFromFloat64(statusResult.Amount) {
		return fmt.Errorf("transfer status amount mismatch: got %.2f want %s", statusResult.Amount, transaction.Amount)
	}

	return nil
}

type noopReconcileUpstream struct{}

func (noopReconcileUpstream) TransferStatus(context.Context, nexusggr.TransferStatusInput) (nexusggr.TransferStatusResult, error) {
	return nexusggr.TransferStatusResult{}, nexusggr.ErrNotConfigured
}

type noopReconcileLedger struct{}

func (noopReconcileLedger) Credit(context.Context, string, ledger.PostEntryInput) (ledger.PostingResult, error) {
	return ledger.PostingResult{}, ledger.ErrNotFound
}

func (noopReconcileLedger) CommitReservation(context.Context, string, ledger.CommitReservationInput) (ledger.CommitReservationResult, error) {
	return ledger.CommitReservationResult{}, ledger.ErrNotFound
}

func (noopReconcileLedger) HasReferenceEntries(context.Context, string, string) (bool, error) {
	return false, ledger.ErrNotFound
}

func (noopReconcileLedger) ReleaseReservation(context.Context, string, ledger.ReleaseReservationInput) (ledger.ReservationResult, error) {
	return ledger.ReservationResult{}, ledger.ErrNotFound
}
