package withdrawals

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/modules/ledger"
)

func TestCreateStoreWithdrawalSuccess(t *testing.T) {
	now := time.Date(2026, time.March, 30, 14, 0, 0, 0, time.UTC)
	repository := newStubRepository(now)
	ledgerService := &stubLedger{
		balance: ledger.BalanceSnapshot{AvailableBalance: "2000000.00"},
	}
	provider := &stubProvider{
		inquiryResult: ProviderInquiryResult{
			AccountName:  "DEMO OWNER",
			BankCode:     "014",
			BankName:     "PT. BANK CENTRAL ASIA, TBK.",
			PartnerRefNo: "partner-1",
			InquiryID:    "99",
			ExternalFee:  180000,
		},
		transferResult: ProviderTransferResult{Accepted: true},
	}

	service := NewService(Options{
		Repository:         repository,
		Provider:           provider,
		Ledger:             ledgerService,
		AccountOpener:      stubOpener{value: "1234567890"},
		Clock:              fixedClock{now: now},
		PlatformFeePercent: 12,
	})

	withdrawal, created, err := service.CreateStoreWithdrawal(context.Background(), ownerSubject(), "store-1", CreateWithdrawInput{
		BankAccountID:  "bank-1",
		Amount:         json.Number("1000000"),
		IdempotencyKey: "withdraw-key-001",
	}, auth.RequestMetadata{IPAddress: "127.0.0.1", UserAgent: "test"})
	if err != nil {
		t.Fatalf("CreateStoreWithdrawal error = %v", err)
	}

	if !created {
		t.Fatalf("created = false, want true")
	}
	if withdrawal.Status != WithdrawalStatusPending {
		t.Fatalf("status = %q, want pending", withdrawal.Status)
	}
	if withdrawal.PlatformFeeAmount != "120000.00" {
		t.Fatalf("platform fee = %q, want 120000.00", withdrawal.PlatformFeeAmount)
	}
	if withdrawal.ExternalFeeAmount != "1800.00" {
		t.Fatalf("external fee = %q, want 1800.00", withdrawal.ExternalFeeAmount)
	}
	if withdrawal.TotalStoreDebit != "1121800.00" {
		t.Fatalf("total debit = %q, want 1121800.00", withdrawal.TotalStoreDebit)
	}
	if ledgerService.reservedAmount != "1121800.00" {
		t.Fatalf("reserved amount = %q, want 1121800.00", ledgerService.reservedAmount)
	}
	if len(provider.calls) != 2 || provider.calls[0] != "inquiry" || provider.calls[1] != "transfer" {
		t.Fatalf("provider calls = %#v, want inquiry then transfer", provider.calls)
	}
	if len(ledgerService.calls) == 0 || ledgerService.calls[0] != "balance" || ledgerService.calls[1] != "reserve" {
		t.Fatalf("ledger calls = %#v, want balance then reserve", ledgerService.calls)
	}
	if repository.lastCreated.RequestPayload["account_number_masked"] != "******7890" {
		t.Fatalf("request payload masked account = %v, want ******7890", repository.lastCreated.RequestPayload["account_number_masked"])
	}
	if _, ok := repository.lastCreated.RequestPayload["account_number"]; ok {
		t.Fatalf("request payload unexpectedly contains raw account number: %#v", repository.lastCreated.RequestPayload)
	}
}

func TestCreateStoreWithdrawalReturnsExistingByIdempotencyKey(t *testing.T) {
	now := time.Date(2026, time.March, 30, 14, 5, 0, 0, time.UTC)
	repository := newStubRepository(now)
	repository.withdrawals["store-1:withdraw-key-dup"] = StoreWithdrawal{
		ID:                  "withdrawal-dup",
		StoreID:             "store-1",
		StoreBankAccountID:  "bank-1",
		IdempotencyKey:      "withdraw-key-dup",
		BankCode:            "014",
		BankName:            "PT. BANK CENTRAL ASIA, TBK.",
		AccountName:         "DEMO OWNER",
		AccountNumberMasked: "******7890",
		NetRequestedAmount:  "1000000.00",
		PlatformFeeAmount:   "120000.00",
		ExternalFeeAmount:   "1800.00",
		TotalStoreDebit:     "1121800.00",
		Status:              WithdrawalStatusPending,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	provider := &stubProvider{}
	ledgerService := &stubLedger{}
	service := NewService(Options{
		Repository:         repository,
		Provider:           provider,
		Ledger:             ledgerService,
		AccountOpener:      stubOpener{value: "1234567890"},
		Clock:              fixedClock{now: now},
		PlatformFeePercent: 12,
	})

	withdrawal, created, err := service.CreateStoreWithdrawal(context.Background(), ownerSubject(), "store-1", CreateWithdrawInput{
		BankAccountID:  "bank-1",
		Amount:         json.Number("1000000"),
		IdempotencyKey: "withdraw-key-dup",
	}, auth.RequestMetadata{})
	if err != nil {
		t.Fatalf("CreateStoreWithdrawal error = %v", err)
	}

	if created {
		t.Fatalf("created = true, want false")
	}
	if withdrawal.ID != "withdrawal-dup" {
		t.Fatalf("withdrawal.ID = %q, want withdrawal-dup", withdrawal.ID)
	}
	if len(provider.calls) != 0 {
		t.Fatalf("provider calls = %#v, want none", provider.calls)
	}
}

func TestCreateStoreWithdrawalRejectsIdempotencyConflict(t *testing.T) {
	now := time.Date(2026, time.March, 30, 14, 10, 0, 0, time.UTC)
	repository := newStubRepository(now)
	repository.withdrawals["store-1:withdraw-key-conflict"] = StoreWithdrawal{
		ID:                 "withdrawal-1",
		StoreID:            "store-1",
		StoreBankAccountID: "bank-1",
		IdempotencyKey:     "withdraw-key-conflict",
		NetRequestedAmount: "1000000.00",
		Status:             WithdrawalStatusPending,
	}

	service := NewService(Options{
		Repository:         repository,
		Provider:           &stubProvider{},
		Ledger:             &stubLedger{},
		AccountOpener:      stubOpener{value: "1234567890"},
		Clock:              fixedClock{now: now},
		PlatformFeePercent: 12,
	})

	_, _, err := service.CreateStoreWithdrawal(context.Background(), ownerSubject(), "store-1", CreateWithdrawInput{
		BankAccountID:  "bank-2",
		Amount:         json.Number("1000000"),
		IdempotencyKey: "withdraw-key-conflict",
	}, auth.RequestMetadata{})
	if !errors.Is(err, ErrIdempotencyKeyConflict) {
		t.Fatalf("error = %v, want ErrIdempotencyKeyConflict", err)
	}
}

func TestCreateStoreWithdrawalInsufficientBalance(t *testing.T) {
	now := time.Date(2026, time.March, 30, 14, 15, 0, 0, time.UTC)
	repository := newStubRepository(now)
	service := NewService(Options{
		Repository: repository,
		Provider: &stubProvider{
			inquiryResult: ProviderInquiryResult{
				AccountName:  "DEMO OWNER",
				BankCode:     "014",
				BankName:     "PT. BANK CENTRAL ASIA, TBK.",
				PartnerRefNo: "partner-2",
				InquiryID:    "98",
				ExternalFee:  180000,
			},
		},
		Ledger: &stubLedger{
			balance: ledger.BalanceSnapshot{AvailableBalance: "1000000.00"},
		},
		AccountOpener:      stubOpener{value: "1234567890"},
		Clock:              fixedClock{now: now},
		PlatformFeePercent: 12,
	})

	_, _, err := service.CreateStoreWithdrawal(context.Background(), ownerSubject(), "store-1", CreateWithdrawInput{
		BankAccountID:  "bank-1",
		Amount:         json.Number("1000000"),
		IdempotencyKey: "withdraw-key-low",
	}, auth.RequestMetadata{})
	if !errors.Is(err, ErrInsufficientStoreBalance) {
		t.Fatalf("error = %v, want ErrInsufficientStoreBalance", err)
	}

	var failure *CreateFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error = %v, want CreateFailure", err)
	}
	if failure.Withdrawal.Status != WithdrawalStatusFailed {
		t.Fatalf("status = %q, want failed", failure.Withdrawal.Status)
	}
}

func TestCreateStoreWithdrawalTransferFailureReleasesReservation(t *testing.T) {
	now := time.Date(2026, time.March, 30, 14, 20, 0, 0, time.UTC)
	repository := newStubRepository(now)
	ledgerService := &stubLedger{
		balance: ledger.BalanceSnapshot{AvailableBalance: "2000000.00"},
	}
	service := NewService(Options{
		Repository: repository,
		Provider: &stubProvider{
			inquiryResult: ProviderInquiryResult{
				AccountName:  "DEMO OWNER",
				BankCode:     "014",
				BankName:     "PT. BANK CENTRAL ASIA, TBK.",
				PartnerRefNo: "partner-3",
				InquiryID:    "97",
				ExternalFee:  180000,
			},
			transferErr: errors.New("business failure"),
		},
		Ledger:             ledgerService,
		AccountOpener:      stubOpener{value: "1234567890"},
		Clock:              fixedClock{now: now},
		PlatformFeePercent: 12,
	})

	_, _, err := service.CreateStoreWithdrawal(context.Background(), ownerSubject(), "store-1", CreateWithdrawInput{
		BankAccountID:  "bank-1",
		Amount:         json.Number("1000000"),
		IdempotencyKey: "withdraw-key-transfer-fail",
	}, auth.RequestMetadata{})
	if !errors.Is(err, ErrTransferFailed) {
		t.Fatalf("error = %v, want ErrTransferFailed", err)
	}
	if !ledgerService.released {
		t.Fatalf("released = false, want true")
	}
}

func TestListStoreWithdrawalsBlocksKaryawan(t *testing.T) {
	repository := newStubRepository(time.Now().UTC())
	service := NewService(Options{
		Repository:    repository,
		Provider:      &stubProvider{},
		Ledger:        &stubLedger{},
		AccountOpener: stubOpener{value: "1234567890"},
	})

	_, err := service.ListStoreWithdrawals(context.Background(), auth.Subject{
		UserID: "employee-1",
		Role:   auth.RoleKaryawan,
	}, "store-1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("error = %v, want ErrForbidden", err)
	}
}

type fixedClock struct {
	now time.Time
}

func (f fixedClock) Now() time.Time {
	return f.now
}

type stubRepository struct {
	store       StoreScope
	bankAccount StoreBankAccount
	withdrawals map[string]StoreWithdrawal
	byID        map[string]StoreWithdrawal
	audits      []string
	checks      []RecordStatusCheckParams
	lastCreated CreateStoreWithdrawalParams
	now         time.Time
}

type stubNotificationEmitter struct {
	calls []notificationCall
}

func (s *stubNotificationEmitter) Emit(storeID string, eventType string, title string, body string) {
	s.calls = append(s.calls, notificationCall{
		storeID:   storeID,
		eventType: eventType,
		title:     title,
		body:      body,
	})
}

type notificationCall struct {
	storeID   string
	eventType string
	title     string
	body      string
}

func newStubRepository(now time.Time) *stubRepository {
	return &stubRepository{
		store: StoreScope{
			ID:          "store-1",
			OwnerUserID: "user-1",
			Name:        "Demo Store",
			Slug:        "demo-store",
			Status:      StoreStatusActive,
		},
		bankAccount: StoreBankAccount{
			ID:                     "bank-1",
			StoreID:                "store-1",
			BankCode:               "014",
			BankName:               "PT. BANK CENTRAL ASIA, TBK.",
			AccountName:            "DEMO OWNER",
			AccountNumberMasked:    "******7890",
			AccountNumberEncrypted: "cipher",
			IsActive:               true,
		},
		withdrawals: map[string]StoreWithdrawal{},
		byID:        map[string]StoreWithdrawal{},
		now:         now,
	}
}

func (r *stubRepository) GetStoreScope(context.Context, string) (StoreScope, error) {
	return r.store, nil
}

func (r *stubRepository) GetStoreBankAccount(_ context.Context, _ string, bankAccountID string) (StoreBankAccount, error) {
	if bankAccountID != r.bankAccount.ID {
		return StoreBankAccount{}, ErrNotFound
	}

	return r.bankAccount, nil
}

func (r *stubRepository) FindByIdempotencyKey(_ context.Context, storeID string, idempotencyKey string) (StoreWithdrawal, error) {
	withdrawal, ok := r.withdrawals[storeID+":"+idempotencyKey]
	if !ok {
		return StoreWithdrawal{}, ErrNotFound
	}

	return withdrawal, nil
}

func (r *stubRepository) FindByPartnerRefNo(_ context.Context, partnerRefNo string) (StoreWithdrawal, error) {
	for _, withdrawal := range r.byID {
		if withdrawal.ProviderPartnerRefNo != nil && *withdrawal.ProviderPartnerRefNo == partnerRefNo {
			return withdrawal, nil
		}
	}

	return StoreWithdrawal{}, ErrNotFound
}

func (r *stubRepository) GetByID(_ context.Context, withdrawalID string) (StoreWithdrawal, error) {
	withdrawal, ok := r.byID[withdrawalID]
	if !ok {
		return StoreWithdrawal{}, ErrNotFound
	}

	return withdrawal, nil
}

func (r *stubRepository) ListStoreWithdrawals(context.Context, string) ([]StoreWithdrawal, error) {
	return nil, nil
}

func (r *stubRepository) AcquireProcessingLock(context.Context, string) (ProcessingLock, bool, error) {
	return stubLock{}, true, nil
}

func (r *stubRepository) NextStatusCheckAttemptNo(_ context.Context, withdrawalID string) (int, error) {
	attempt := 1
	for _, item := range r.checks {
		if item.WithdrawalID == withdrawalID && item.AttemptNo >= attempt {
			attempt = item.AttemptNo + 1
		}
	}

	return attempt, nil
}

func (r *stubRepository) ListDueStatusCheckWithdrawals(_ context.Context, cutoff time.Time, limit int) ([]StatusCheckCandidate, error) {
	candidates := make([]StatusCheckCandidate, 0, len(r.byID))
	for _, withdrawal := range r.byID {
		if withdrawal.Status != WithdrawalStatusPending || withdrawal.ProviderPartnerRefNo == nil || strings.TrimSpace(*withdrawal.ProviderPartnerRefNo) == "" {
			continue
		}

		var lastAttempt *time.Time
		attemptNo := 0
		for _, item := range r.checks {
			if item.WithdrawalID != withdrawal.ID {
				continue
			}
			if attemptNo == 0 || item.AttemptNo > attemptNo {
				attemptNo = item.AttemptNo
				occurredAt := item.OccurredAt
				lastAttempt = &occurredAt
			}
		}
		if lastAttempt != nil && lastAttempt.After(cutoff) {
			continue
		}

		candidates = append(candidates, StatusCheckCandidate{
			Withdrawal:    withdrawal,
			AttemptNo:     attemptNo,
			LastAttemptAt: lastAttempt,
		})
		if len(candidates) >= limit {
			break
		}
	}

	return candidates, nil
}

func (r *stubRepository) CreateStoreWithdrawal(_ context.Context, params CreateStoreWithdrawalParams) (StoreWithdrawal, error) {
	r.lastCreated = params

	withdrawal := StoreWithdrawal{
		ID:                   "withdrawal-" + params.IdempotencyKey,
		StoreID:              params.StoreID,
		StoreBankAccountID:   params.StoreBankAccountID,
		IdempotencyKey:       params.IdempotencyKey,
		BankCode:             r.bankAccount.BankCode,
		BankName:             r.bankAccount.BankName,
		AccountName:          r.bankAccount.AccountName,
		AccountNumberMasked:  r.bankAccount.AccountNumberMasked,
		NetRequestedAmount:   params.NetRequestedAmount,
		PlatformFeeAmount:    params.PlatformFeeAmount,
		ExternalFeeAmount:    params.ExternalFeeAmount,
		TotalStoreDebit:      params.TotalStoreDebit,
		ProviderPartnerRefNo: params.ProviderPartnerRefNo,
		ProviderInquiryID:    params.ProviderInquiryID,
		Status:               params.Status,
		CreatedAt:            params.OccurredAt,
		UpdatedAt:            params.OccurredAt,
	}
	r.withdrawals[params.StoreID+":"+params.IdempotencyKey] = withdrawal
	r.byID[withdrawal.ID] = withdrawal
	return withdrawal, nil
}

func (r *stubRepository) UpdateStoreWithdrawal(_ context.Context, params UpdateStoreWithdrawalParams) (StoreWithdrawal, error) {
	withdrawal := r.byID[params.WithdrawalID]
	if params.PlatformFeeAmount != nil {
		withdrawal.PlatformFeeAmount = *params.PlatformFeeAmount
	}
	if params.ExternalFeeAmount != nil {
		withdrawal.ExternalFeeAmount = *params.ExternalFeeAmount
	}
	if params.TotalStoreDebit != nil {
		withdrawal.TotalStoreDebit = *params.TotalStoreDebit
	}
	if params.ProviderPartnerRefNo != nil {
		withdrawal.ProviderPartnerRefNo = params.ProviderPartnerRefNo
	}
	if params.ProviderInquiryID != nil {
		withdrawal.ProviderInquiryID = params.ProviderInquiryID
	}
	if params.Status != nil {
		withdrawal.Status = *params.Status
	}
	withdrawal.UpdatedAt = params.OccurredAt
	r.byID[withdrawal.ID] = withdrawal
	r.withdrawals[withdrawal.StoreID+":"+withdrawal.IdempotencyKey] = withdrawal
	return withdrawal, nil
}

func (r *stubRepository) InsertAuditLog(_ context.Context, _ *string, _ string, _ *string, action string, _ string, _ *string, _ map[string]any, _ string, _ string, _ time.Time) error {
	r.audits = append(r.audits, action)
	return nil
}

func (r *stubRepository) RecordStatusCheck(_ context.Context, params RecordStatusCheckParams) error {
	r.checks = append(r.checks, params)
	return nil
}

type stubLock struct{}

func (stubLock) Unlock(context.Context) error {
	return nil
}

type stubProvider struct {
	inquiryResult     ProviderInquiryResult
	inquiryErr        error
	transferResult    ProviderTransferResult
	transferErr       error
	checkStatusResult ProviderStatusCheckResult
	checkStatusErr    error
	calls             []string
}

func (s *stubProvider) Inquiry(context.Context, ProviderInquiryInput) (ProviderInquiryResult, error) {
	s.calls = append(s.calls, "inquiry")
	if s.inquiryErr != nil {
		return ProviderInquiryResult{}, s.inquiryErr
	}

	return s.inquiryResult, nil
}

func (s *stubProvider) Transfer(context.Context, ProviderTransferInput) (ProviderTransferResult, error) {
	s.calls = append(s.calls, "transfer")
	if s.transferErr != nil {
		return ProviderTransferResult{}, s.transferErr
	}

	return s.transferResult, nil
}

func (s *stubProvider) CheckStatus(context.Context, ProviderStatusCheckInput) (ProviderStatusCheckResult, error) {
	s.calls = append(s.calls, "check_status")
	if s.checkStatusErr != nil {
		return ProviderStatusCheckResult{}, s.checkStatusErr
	}

	return s.checkStatusResult, nil
}

type stubLedger struct {
	balance        ledger.BalanceSnapshot
	calls          []string
	reservedAmount string
	released       bool
	committed      bool
	commitCount    int
	hasEntries     bool
}

func (s *stubLedger) GetBalance(context.Context, string) (ledger.BalanceSnapshot, error) {
	s.calls = append(s.calls, "balance")
	return s.balance, nil
}

func (s *stubLedger) Reserve(_ context.Context, _ string, input ledger.ReserveInput) (ledger.ReservationResult, error) {
	s.calls = append(s.calls, "reserve")
	s.reservedAmount = input.Amount
	return ledger.ReservationResult{}, nil
}

func (s *stubLedger) HasReferenceEntries(context.Context, string, string) (bool, error) {
	s.calls = append(s.calls, "has_entries")
	return s.hasEntries, nil
}

func (s *stubLedger) CommitReservation(_ context.Context, _ string, _ ledger.CommitReservationInput) (ledger.CommitReservationResult, error) {
	s.calls = append(s.calls, "commit")
	s.committed = true
	s.commitCount++
	s.hasEntries = true
	return ledger.CommitReservationResult{}, nil
}

func (s *stubLedger) ReleaseReservation(_ context.Context, _ string, _ ledger.ReleaseReservationInput) (ledger.ReservationResult, error) {
	s.calls = append(s.calls, "release")
	s.released = true
	return ledger.ReservationResult{}, nil
}

type stubOpener struct {
	value string
}

func (s stubOpener) Open(string) (string, error) {
	return s.value, nil
}

func ownerSubject() auth.Subject {
	return auth.Subject{
		UserID: "user-1",
		Role:   auth.RoleOwner,
	}
}
