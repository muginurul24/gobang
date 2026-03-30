package game

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/platform/nexusggr"
)

func TestRunPendingScansAndFinalizesDepositSuccess(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 12, 30, 0, 0, time.UTC))
	pending := ReconcileStatusPending
	repository.transactions["tx-deposit-pending"] = GameTransaction{
		ID:               "tx-deposit-pending",
		StoreID:          "store-1",
		StoreMemberID:    "member-1",
		Action:           GameActionDeposit,
		TrxID:            "trx-reconcile-deposit",
		UpstreamUserCode: "MEMBER000001",
		Amount:           "5000.00",
		AgentSign:        "AGTRECONCILEDEP1",
		Status:           TransactionStatusPending,
		ReconcileStatus:  &pending,
		CreatedAt:        repository.now,
		UpdatedAt:        repository.now,
	}
	upstream := &fakeUpstream{
		transferStatusResult: nexusggr.TransferStatusResult{
			Message:      "SUCCESS",
			Amount:       5000,
			AgentBalance: 245000,
			UserBalance:  5000,
			Type:         "user_deposit",
		},
	}
	ledgerService := newFakeLedger("100000.00")

	service := NewReconcileService(ReconcileOptions{
		Repository: repository,
		Upstream:   upstream,
		Ledger:     ledgerService,
		Clock:      fixedClock{now: repository.now},
	})

	summary, err := service.RunPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunPending returned error: %v", err)
	}

	if summary.Scanned != 1 || summary.FinalizedSuccess != 1 {
		t.Fatalf("summary = %#v, want scanned=1 finalized_success=1", summary)
	}

	transaction := repository.transactions["tx-deposit-pending"]
	if transaction.Status != TransactionStatusSuccess {
		t.Fatalf("Transaction.Status = %s, want success", transaction.Status)
	}
	if transaction.ReconcileStatus == nil || *transaction.ReconcileStatus != ReconcileStatusResolved {
		t.Fatalf("Transaction.ReconcileStatus = %#v, want resolved", transaction.ReconcileStatus)
	}
	if ledgerService.commitCalls != 1 {
		t.Fatalf("commit calls = %d, want 1", ledgerService.commitCalls)
	}
	if repository.auditActions[len(repository.auditActions)-1] != "game.deposit_reconciled_success" {
		t.Fatalf("last audit action = %q, want game.deposit_reconciled_success", repository.auditActions[len(repository.auditActions)-1])
	}
	if repository.notifications[len(repository.notifications)-1] != "game.deposit.reconciled_success" {
		t.Fatalf("last notification = %q, want game.deposit.reconciled_success", repository.notifications[len(repository.notifications)-1])
	}
}

func TestReconcileTransactionFinalizesWithdrawFailure(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 12, 40, 0, 0, time.UTC))
	pending := ReconcileStatusPending
	repository.transactions["tx-withdraw-pending"] = GameTransaction{
		ID:               "tx-withdraw-pending",
		StoreID:          "store-1",
		StoreMemberID:    "member-1",
		Action:           GameActionWithdraw,
		TrxID:            "trx-reconcile-withdraw",
		UpstreamUserCode: "MEMBER000001",
		Amount:           "8000.00",
		AgentSign:        "AGTRECONCILEWD01",
		Status:           TransactionStatusPending,
		ReconcileStatus:  &pending,
		CreatedAt:        repository.now,
		UpdatedAt:        repository.now,
	}
	upstream := &fakeUpstream{
		transferStatusErr: &nexusggr.BusinessError{Code: "TRANSFER_NOT_FOUND", Message: "transfer not found"},
	}
	ledgerService := newFakeLedger("100000.00")

	service := NewReconcileService(ReconcileOptions{
		Repository: repository,
		Upstream:   upstream,
		Ledger:     ledgerService,
		Clock:      fixedClock{now: repository.now},
	})

	outcome, err := service.ReconcileTransaction(context.Background(), "tx-withdraw-pending")
	if err != nil {
		t.Fatalf("ReconcileTransaction returned error: %v", err)
	}

	if outcome != ReconcileOutcomeFinalizedFailed {
		t.Fatalf("outcome = %s, want finalized_failed", outcome)
	}

	transaction := repository.transactions["tx-withdraw-pending"]
	if transaction.Status != TransactionStatusFailed {
		t.Fatalf("Transaction.Status = %s, want failed", transaction.Status)
	}
	if ledgerService.creditCalls != 0 {
		t.Fatalf("credit calls = %d, want 0", ledgerService.creditCalls)
	}
	if repository.auditActions[len(repository.auditActions)-1] != "game.withdraw_reconciled_failed" {
		t.Fatalf("last audit action = %q, want game.withdraw_reconciled_failed", repository.auditActions[len(repository.auditActions)-1])
	}
}

func TestReconcileTransactionBlocksDuplicateWithdrawCredit(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 12, 50, 0, 0, time.UTC))
	pending := ReconcileStatusPending
	repository.transactions["tx-withdraw-already-posted"] = GameTransaction{
		ID:               "tx-withdraw-already-posted",
		StoreID:          "store-1",
		StoreMemberID:    "member-1",
		Action:           GameActionWithdraw,
		TrxID:            "trx-withdraw-already-posted",
		UpstreamUserCode: "MEMBER000001",
		Amount:           "9000.00",
		AgentSign:        "AGTALREADYPOSTED1",
		Status:           TransactionStatusPending,
		ReconcileStatus:  &pending,
		CreatedAt:        repository.now,
		UpdatedAt:        repository.now,
	}
	upstream := &fakeUpstream{
		transferStatusResult: nexusggr.TransferStatusResult{
			Message:      "SUCCESS",
			Amount:       9000,
			AgentBalance: 241000,
			UserBalance:  9000,
			Type:         "user_withdraw",
		},
	}
	ledgerService := newFakeLedger("100000.00")
	ledgerService.referenceEntries["game_transaction:tx-withdraw-already-posted"] = true

	service := NewReconcileService(ReconcileOptions{
		Repository: repository,
		Upstream:   upstream,
		Ledger:     ledgerService,
		Clock:      fixedClock{now: repository.now},
	})

	outcome, err := service.ReconcileTransaction(context.Background(), "tx-withdraw-already-posted")
	if err != nil {
		t.Fatalf("ReconcileTransaction returned error: %v", err)
	}

	if outcome != ReconcileOutcomeFinalizedSuccess {
		t.Fatalf("outcome = %s, want finalized_success", outcome)
	}
	if ledgerService.creditCalls != 0 {
		t.Fatalf("credit calls = %d, want 0", ledgerService.creditCalls)
	}
	if repository.transactions["tx-withdraw-already-posted"].Status != TransactionStatusSuccess {
		t.Fatalf("Transaction.Status = %s, want success", repository.transactions["tx-withdraw-already-posted"].Status)
	}
}

func TestReconcileTransactionSkipsResolvedTransaction(t *testing.T) {
	repository := newFakeRepository(time.Date(2026, 3, 30, 13, 0, 0, 0, time.UTC))
	resolved := ReconcileStatusResolved
	repository.transactions["tx-resolved"] = GameTransaction{
		ID:               "tx-resolved",
		StoreID:          "store-1",
		StoreMemberID:    "member-1",
		Action:           GameActionDeposit,
		TrxID:            "trx-resolved",
		UpstreamUserCode: "MEMBER000001",
		Amount:           "5000.00",
		AgentSign:        "AGTRESOLVED00001",
		Status:           TransactionStatusSuccess,
		ReconcileStatus:  &resolved,
		CreatedAt:        repository.now,
		UpdatedAt:        repository.now,
	}
	upstream := &fakeUpstream{
		transferStatusErr: errors.New("should not be called"),
	}

	service := NewReconcileService(ReconcileOptions{
		Repository: repository,
		Upstream:   upstream,
		Ledger:     newFakeLedger("100000.00"),
		Clock:      fixedClock{now: repository.now},
	})

	outcome, err := service.ReconcileTransaction(context.Background(), "tx-resolved")
	if err != nil {
		t.Fatalf("ReconcileTransaction returned error: %v", err)
	}

	if outcome != ReconcileOutcomeSkipped {
		t.Fatalf("outcome = %s, want skipped", outcome)
	}
	if upstream.transferStatusCalls != 0 {
		t.Fatalf("transfer status calls = %d, want 0", upstream.transferStatusCalls)
	}
}
