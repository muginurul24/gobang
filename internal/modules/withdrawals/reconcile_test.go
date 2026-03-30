package withdrawals

import (
	"context"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/modules/ledger"
	"github.com/mugiew/onixggr/internal/platform/qris"
)

func TestHandleTransferWebhookSuccessCommitsReservation(t *testing.T) {
	now := time.Date(2026, time.March, 30, 15, 0, 0, 0, time.UTC)
	repository := newStubRepository(now)
	threshold := "900000.00"
	repository.store.LowBalanceThreshold = &threshold
	withdrawal := seedPendingWithdrawal(now)
	repository.byID[withdrawal.ID] = withdrawal
	repository.withdrawals[withdrawal.StoreID+":"+withdrawal.IdempotencyKey] = withdrawal
	ledgerService := &stubLedger{
		balance: ledger.BalanceSnapshot{
			AvailableBalance: "878200.00",
		},
	}
	notifier := &stubNotificationEmitter{}
	service := NewService(Options{
		Repository:    repository,
		Provider:      &stubProvider{},
		Ledger:        ledgerService,
		Notifications: notifier,
		Clock:         fixedClock{now: now},
	})

	result, err := service.HandleTransferWebhook(context.Background(), qris.TransferWebhook{
		PartnerRefNo: "partner-200",
		Status:       "success",
		MerchantID:   "merchant-1",
	}, auth.RequestMetadata{IPAddress: "127.0.0.1", UserAgent: "webhook-test"})
	if err != nil {
		t.Fatalf("HandleTransferWebhook error = %v", err)
	}

	updated := repository.byID[withdrawal.ID]
	if !result.Processed {
		t.Fatal("Processed = false, want true")
	}
	if updated.Status != WithdrawalStatusSuccess {
		t.Fatalf("status = %q, want success", updated.Status)
	}
	if !ledgerService.committed {
		t.Fatal("committed = false, want true")
	}
	if ledgerService.commitCount != 1 {
		t.Fatalf("commitCount = %d, want 1", ledgerService.commitCount)
	}
	if len(repository.audits) == 0 || repository.audits[len(repository.audits)-1] != "withdraw.success" {
		t.Fatalf("audits = %#v, want withdraw.success", repository.audits)
	}
	if len(notifier.calls) != 2 {
		t.Fatalf("notification calls = %d, want 2", len(notifier.calls))
	}
	if notifier.calls[1].eventType != "store.low_balance" {
		t.Fatalf("notification event = %q, want store.low_balance", notifier.calls[1].eventType)
	}
}

func TestHandleTransferWebhookFailureReleasesReservation(t *testing.T) {
	now := time.Date(2026, time.March, 30, 15, 5, 0, 0, time.UTC)
	repository := newStubRepository(now)
	withdrawal := seedPendingWithdrawal(now)
	repository.byID[withdrawal.ID] = withdrawal
	repository.withdrawals[withdrawal.StoreID+":"+withdrawal.IdempotencyKey] = withdrawal
	ledgerService := &stubLedger{}
	service := NewService(Options{
		Repository: repository,
		Provider:   &stubProvider{},
		Ledger:     ledgerService,
		Clock:      fixedClock{now: now},
	})

	result, err := service.HandleTransferWebhook(context.Background(), qris.TransferWebhook{
		PartnerRefNo: "partner-200",
		Status:       "failed",
		MerchantID:   "merchant-1",
	}, auth.RequestMetadata{IPAddress: "127.0.0.1", UserAgent: "webhook-test"})
	if err != nil {
		t.Fatalf("HandleTransferWebhook error = %v", err)
	}

	updated := repository.byID[withdrawal.ID]
	if !result.Processed {
		t.Fatal("Processed = false, want true")
	}
	if updated.Status != WithdrawalStatusFailed {
		t.Fatalf("status = %q, want failed", updated.Status)
	}
	if !ledgerService.released {
		t.Fatal("released = false, want true")
	}
	if len(repository.audits) == 0 || repository.audits[len(repository.audits)-1] != "withdraw.failed" {
		t.Fatalf("audits = %#v, want withdraw.failed", repository.audits)
	}
}

func TestHandleTransferWebhookSuccessIsIdempotent(t *testing.T) {
	now := time.Date(2026, time.March, 30, 15, 10, 0, 0, time.UTC)
	repository := newStubRepository(now)
	withdrawal := seedPendingWithdrawal(now)
	repository.byID[withdrawal.ID] = withdrawal
	repository.withdrawals[withdrawal.StoreID+":"+withdrawal.IdempotencyKey] = withdrawal
	ledgerService := &stubLedger{}
	service := NewService(Options{
		Repository: repository,
		Provider:   &stubProvider{},
		Ledger:     ledgerService,
		Clock:      fixedClock{now: now},
	})

	if _, err := service.HandleTransferWebhook(context.Background(), qris.TransferWebhook{
		PartnerRefNo: "partner-200",
		Status:       "success",
	}, auth.RequestMetadata{UserAgent: "webhook-test"}); err != nil {
		t.Fatalf("first HandleTransferWebhook error = %v", err)
	}
	if _, err := service.HandleTransferWebhook(context.Background(), qris.TransferWebhook{
		PartnerRefNo: "partner-200",
		Status:       "success",
	}, auth.RequestMetadata{UserAgent: "webhook-test"}); err != nil {
		t.Fatalf("second HandleTransferWebhook error = %v", err)
	}

	if ledgerService.commitCount != 1 {
		t.Fatalf("commitCount = %d, want 1", ledgerService.commitCount)
	}
	if repository.byID[withdrawal.ID].Status != WithdrawalStatusSuccess {
		t.Fatalf("status = %q, want success", repository.byID[withdrawal.ID].Status)
	}
}

func TestRunPendingChecksFinalizesSuccessWithoutWebhook(t *testing.T) {
	now := time.Date(2026, time.March, 30, 15, 15, 0, 0, time.UTC)
	repository := newStubRepository(now)
	withdrawal := seedPendingWithdrawal(now)
	repository.byID[withdrawal.ID] = withdrawal
	repository.withdrawals[withdrawal.StoreID+":"+withdrawal.IdempotencyKey] = withdrawal
	ledgerService := &stubLedger{}
	service := NewService(Options{
		Repository: repository,
		Provider: &stubProvider{
			checkStatusResult: ProviderStatusCheckResult{
				Amount:       1000000,
				ExternalFee:  180000,
				PartnerRefNo: "partner-200",
				MerchantID:   "merchant-1",
				Status:       "success",
			},
		},
		Ledger:              ledgerService,
		Clock:               fixedClock{now: now},
		StatusCheckInterval: 30 * time.Second,
	})

	summary, err := service.RunPendingChecks(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunPendingChecks error = %v", err)
	}

	if summary.FinalizedSuccess != 1 {
		t.Fatalf("FinalizedSuccess = %d, want 1", summary.FinalizedSuccess)
	}
	if repository.byID[withdrawal.ID].Status != WithdrawalStatusSuccess {
		t.Fatalf("status = %q, want success", repository.byID[withdrawal.ID].Status)
	}
	if len(repository.checks) != 1 {
		t.Fatalf("checks = %d, want 1", len(repository.checks))
	}
	if repository.checks[0].Status != "success" {
		t.Fatalf("check status = %q, want success", repository.checks[0].Status)
	}
	if !ledgerService.committed {
		t.Fatal("committed = false, want true")
	}
}

func TestRunPendingChecksKeepsPendingOnProviderError(t *testing.T) {
	now := time.Date(2026, time.March, 30, 15, 20, 0, 0, time.UTC)
	repository := newStubRepository(now)
	withdrawal := seedPendingWithdrawal(now)
	repository.byID[withdrawal.ID] = withdrawal
	repository.withdrawals[withdrawal.StoreID+":"+withdrawal.IdempotencyKey] = withdrawal
	service := NewService(Options{
		Repository: repository,
		Provider: &stubProvider{
			checkStatusErr: qris.ErrUpstreamUnavailable,
		},
		Ledger:              &stubLedger{},
		Clock:               fixedClock{now: now},
		StatusCheckInterval: 30 * time.Second,
	})

	summary, err := service.RunPendingChecks(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunPendingChecks error = %v", err)
	}

	if summary.StillPending != 1 {
		t.Fatalf("StillPending = %d, want 1", summary.StillPending)
	}
	if repository.byID[withdrawal.ID].Status != WithdrawalStatusPending {
		t.Fatalf("status = %q, want pending", repository.byID[withdrawal.ID].Status)
	}
	if len(repository.checks) != 1 {
		t.Fatalf("checks = %d, want 1", len(repository.checks))
	}
	if repository.checks[0].Status != "upstream_error" {
		t.Fatalf("check status = %q, want upstream_error", repository.checks[0].Status)
	}
}

func seedPendingWithdrawal(now time.Time) StoreWithdrawal {
	partnerRefNo := "partner-200"
	inquiryID := "inquiry-77"

	return StoreWithdrawal{
		ID:                   "withdrawal-pending",
		StoreID:              "store-1",
		StoreBankAccountID:   "bank-1",
		IdempotencyKey:       "withdraw-key-pending",
		BankCode:             "014",
		BankName:             "PT. BANK CENTRAL ASIA, TBK.",
		AccountName:          "DEMO OWNER",
		AccountNumberMasked:  "******7890",
		NetRequestedAmount:   "1000000.00",
		PlatformFeeAmount:    "120000.00",
		ExternalFeeAmount:    "1800.00",
		TotalStoreDebit:      "1121800.00",
		ProviderPartnerRefNo: &partnerRefNo,
		ProviderInquiryID:    &inquiryID,
		Status:               WithdrawalStatusPending,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}
