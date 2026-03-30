package paymentsqris

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/platform/qris"
)

func TestCreateStoreTopupSuccess(t *testing.T) {
	now := time.Date(2026, time.March, 30, 8, 0, 0, 0, time.UTC)
	repository := &stubRepository{
		store: StoreScope{
			ID:            "store-1",
			OwnerUserID:   "owner-1",
			OwnerUsername: "owner-demo",
			Status:        StoreStatusActive,
		},
	}
	upstream := &stubUpstream{
		result: qris.GenerateResult{
			RawValue: "0002010102112669demo",
			TrxID:    "trx-provider-1",
		},
	}

	service := NewService(Options{
		Repository:           repository,
		Upstream:             upstream,
		Clock:                fixedClock{now: now},
		DefaultExpireSeconds: 300,
	}).(*service)
	service.customRefFactory = func() (string, error) {
		return "TOPUPFIXED000001", nil
	}

	transaction, err := service.CreateStoreTopup(context.Background(), auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	}, "store-1", CreateStoreTopupInput{
		Amount: json.Number("50000"),
	}, auth.RequestMetadata{
		IPAddress: "127.0.0.1",
		UserAgent: "unit-test",
	})
	if err != nil {
		t.Fatalf("CreateStoreTopup error = %v", err)
	}

	if transaction.Status != TransactionStatusPending {
		t.Fatalf("transaction.Status = %q, want pending", transaction.Status)
	}
	if transaction.ProviderTrxID == nil || *transaction.ProviderTrxID != "trx-provider-1" {
		t.Fatalf("transaction.ProviderTrxID = %v, want trx-provider-1", transaction.ProviderTrxID)
	}
	if transaction.QRCodeValue == nil || *transaction.QRCodeValue != "0002010102112669demo" {
		t.Fatalf("transaction.QRCodeValue = %v, want qr payload", transaction.QRCodeValue)
	}
	if repository.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", repository.createCalls)
	}
	if repository.updateGeneratedCalls != 1 {
		t.Fatalf("updateGeneratedCalls = %d, want 1", repository.updateGeneratedCalls)
	}
	if upstream.lastInput.Username != "owner-demo" {
		t.Fatalf("Generate username = %q, want owner-demo", upstream.lastInput.Username)
	}
	if upstream.lastInput.CustomRef != "TOPUPFIXED000001" {
		t.Fatalf("Generate custom_ref = %q, want TOPUPFIXED000001", upstream.lastInput.CustomRef)
	}
}

func TestCreateStoreTopupTimeoutKeepsPending(t *testing.T) {
	now := time.Date(2026, time.March, 30, 8, 0, 0, 0, time.UTC)
	repository := &stubRepository{
		store: StoreScope{
			ID:            "store-1",
			OwnerUserID:   "owner-1",
			OwnerUsername: "owner-demo",
			Status:        StoreStatusActive,
		},
	}
	upstream := &stubUpstream{err: qris.ErrTimeout}

	service := NewService(Options{
		Repository:           repository,
		Upstream:             upstream,
		Clock:                fixedClock{now: now},
		DefaultExpireSeconds: 300,
	}).(*service)
	service.customRefFactory = func() (string, error) {
		return "TOPUPFIXED000002", nil
	}

	transaction, err := service.CreateStoreTopup(context.Background(), auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	}, "store-1", CreateStoreTopupInput{
		Amount: json.Number("10000"),
	}, auth.RequestMetadata{})
	if err != nil {
		t.Fatalf("CreateStoreTopup error = %v", err)
	}

	if transaction.Status != TransactionStatusPending {
		t.Fatalf("transaction.Status = %q, want pending", transaction.Status)
	}
	if transaction.ProviderState == nil || *transaction.ProviderState != ProviderStatePendingProviderAnswer {
		t.Fatalf("transaction.ProviderState = %v, want pending_provider_response", transaction.ProviderState)
	}
	if repository.updateStatusCalls != 1 {
		t.Fatalf("updateStatusCalls = %d, want 1", repository.updateStatusCalls)
	}
}

func TestListStoreTopupsForKaryawanForbidden(t *testing.T) {
	service := NewService(Options{
		Repository: &stubRepository{
			store: StoreScope{
				ID:            "store-1",
				OwnerUserID:   "owner-1",
				OwnerUsername: "owner-demo",
				Status:        StoreStatusActive,
			},
		},
	})

	_, err := service.ListStoreTopups(context.Background(), auth.Subject{
		UserID: "staff-1",
		Role:   auth.RoleKaryawan,
	}, "store-1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListStoreTopups error = %v, want ErrForbidden", err)
	}
}

type fixedClock struct {
	now time.Time
}

func (f fixedClock) Now() time.Time {
	return f.now
}

type stubRepository struct {
	store                StoreScope
	transactions         []QRISTransaction
	createCalls          int
	updateGeneratedCalls int
	updateStatusCalls    int
}

func (s *stubRepository) GetStoreScope(context.Context, string) (StoreScope, error) {
	return s.store, nil
}

func (s *stubRepository) CreateQRISTransaction(_ context.Context, params CreateQRISTransactionParams) (QRISTransaction, error) {
	s.createCalls++
	state := payloadFieldProviderState(params.ProviderPayload)
	transaction := QRISTransaction{
		ID:                "qris-1",
		StoreID:           params.StoreID,
		Type:              params.Type,
		CustomRef:         params.CustomRef,
		ExternalUsername:  params.ExternalUsername,
		AmountGross:       params.AmountGross,
		PlatformFeeAmount: params.PlatformFeeAmount,
		StoreCreditAmount: params.StoreCreditAmount,
		Status:            params.Status,
		ExpiresAt:         params.ExpiresAt,
		ProviderState:     state,
		CreatedAt:         params.OccurredAt,
		UpdatedAt:         params.OccurredAt,
	}
	s.transactions = []QRISTransaction{transaction}
	return transaction, nil
}

func (s *stubRepository) UpdateGeneratedTransaction(_ context.Context, params UpdateGeneratedTransactionParams) (QRISTransaction, error) {
	s.updateGeneratedCalls++
	state := payloadFieldProviderState(params.ProviderPayload)
	qrCodeValue := payloadFieldString(params.ProviderPayload, "qr_code_value")
	transaction := s.transactions[0]
	transaction.ProviderTrxID = stringPtr(params.ProviderTrxID)
	transaction.ExpiresAt = params.ExpiresAt
	transaction.ProviderState = state
	transaction.QRCodeValue = qrCodeValue
	transaction.UpdatedAt = params.OccurredAt
	s.transactions[0] = transaction
	return transaction, nil
}

func (s *stubRepository) UpdateTransactionStatus(_ context.Context, params UpdateTransactionStatusParams) (QRISTransaction, error) {
	s.updateStatusCalls++
	state := payloadFieldProviderState(params.ProviderPayload)
	transaction := s.transactions[0]
	transaction.Status = params.Status
	transaction.ExpiresAt = params.ExpiresAt
	transaction.ProviderState = state
	transaction.UpdatedAt = params.OccurredAt
	s.transactions[0] = transaction
	return transaction, nil
}

func (s *stubRepository) ListQRISTransactions(context.Context, string, TransactionType) ([]QRISTransaction, error) {
	return s.transactions, nil
}

func (s *stubRepository) InsertAuditLog(context.Context, *string, string, *string, string, string, *string, map[string]any, string, string, time.Time) error {
	return nil
}

type stubUpstream struct {
	result    qris.GenerateResult
	err       error
	lastInput qris.GenerateInput
}

func (s *stubUpstream) Generate(_ context.Context, input qris.GenerateInput) (qris.GenerateResult, error) {
	s.lastInput = input
	if s.err != nil {
		return qris.GenerateResult{}, s.err
	}

	return s.result, nil
}
