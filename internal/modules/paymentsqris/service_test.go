package paymentsqris

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/modules/ledger"
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
	service.topupRefFactory = func() (string, error) {
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
	service.topupRefFactory = func() (string, error) {
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

func TestCreateMemberPaymentSuccess(t *testing.T) {
	now := time.Date(2026, time.March, 30, 8, 0, 0, 0, time.UTC)
	repository := &stubRepository{
		store: StoreScope{
			ID:            "store-1",
			OwnerUserID:   "owner-1",
			OwnerUsername: "owner-demo",
			Status:        StoreStatusActive,
		},
		storeMember: StoreMember{
			ID:               "member-1",
			StoreID:          "store-1",
			RealUsername:     "member-alpha",
			UpstreamUserCode: "MEMBER000001",
			Status:           MemberStatusActive,
		},
	}
	upstream := &stubUpstream{
		result: qris.GenerateResult{
			RawValue: "0002010102112669member",
			TrxID:    "trx-member-1",
		},
	}

	service := NewService(Options{
		Repository:           repository,
		Upstream:             upstream,
		Clock:                fixedClock{now: now},
		DefaultExpireSeconds: 300,
	}).(*service)
	service.memberPaymentFactory = func() (string, error) {
		return "MPAYFIXED000001", nil
	}

	transaction, err := service.CreateMemberPayment(context.Background(), "plain-store-token", CreateMemberPaymentInput{
		Username: "member-alpha",
		Amount:   json.Number("25000"),
	}, auth.RequestMetadata{})
	if err != nil {
		t.Fatalf("CreateMemberPayment error = %v", err)
	}

	if transaction.Type != TransactionTypeMemberPayment {
		t.Fatalf("transaction.Type = %q, want member_payment", transaction.Type)
	}
	if transaction.StoreMemberID == nil || *transaction.StoreMemberID != "member-1" {
		t.Fatalf("transaction.StoreMemberID = %v, want member-1", transaction.StoreMemberID)
	}
	if transaction.PlatformFeeAmount != "0.00" {
		t.Fatalf("transaction.PlatformFeeAmount = %q, want 0.00", transaction.PlatformFeeAmount)
	}
	if transaction.StoreCreditAmount != "0.00" {
		t.Fatalf("transaction.StoreCreditAmount = %q, want 0.00", transaction.StoreCreditAmount)
	}
	if transaction.QRCodeValue == nil || *transaction.QRCodeValue != "0002010102112669member" {
		t.Fatalf("transaction.QRCodeValue = %v, want member QR payload", transaction.QRCodeValue)
	}
	if upstream.lastInput.Username != "MEMBER000001" {
		t.Fatalf("Generate username = %q, want MEMBER000001", upstream.lastInput.Username)
	}
	if upstream.lastInput.CustomRef != "MPAYFIXED000001" {
		t.Fatalf("Generate custom_ref = %q, want MPAYFIXED000001", upstream.lastInput.CustomRef)
	}
}

func TestCreateMemberPaymentAmbiguousCreatesPendingRow(t *testing.T) {
	now := time.Date(2026, time.March, 30, 8, 0, 0, 0, time.UTC)
	repository := &stubRepository{
		store: StoreScope{
			ID:            "store-1",
			OwnerUserID:   "owner-1",
			OwnerUsername: "owner-demo",
			Status:        StoreStatusActive,
		},
		storeMember: StoreMember{
			ID:               "member-1",
			StoreID:          "store-1",
			RealUsername:     "member-alpha",
			UpstreamUserCode: "MEMBER000001",
			Status:           MemberStatusActive,
		},
	}
	upstream := &stubUpstream{err: qris.ErrTimeout}

	service := NewService(Options{
		Repository:           repository,
		Upstream:             upstream,
		Clock:                fixedClock{now: now},
		DefaultExpireSeconds: 300,
	}).(*service)
	service.memberPaymentFactory = func() (string, error) {
		return "MPAYFIXED000002", nil
	}

	transaction, err := service.CreateMemberPayment(context.Background(), "plain-store-token", CreateMemberPaymentInput{
		Username: "member-alpha",
		Amount:   json.Number("15000"),
	}, auth.RequestMetadata{})
	if err != nil {
		t.Fatalf("CreateMemberPayment error = %v", err)
	}

	if transaction.Status != TransactionStatusPending {
		t.Fatalf("transaction.Status = %q, want pending", transaction.Status)
	}
	if transaction.ProviderState == nil || *transaction.ProviderState != ProviderStatePendingProviderAnswer {
		t.Fatalf("transaction.ProviderState = %v, want pending_provider_response", transaction.ProviderState)
	}
	if transaction.ProviderTrxID != nil {
		t.Fatalf("transaction.ProviderTrxID = %v, want nil", transaction.ProviderTrxID)
	}
	if repository.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", repository.createCalls)
	}
	if repository.updateGeneratedCalls != 0 {
		t.Fatalf("updateGeneratedCalls = %d, want 0", repository.updateGeneratedCalls)
	}
}

func TestCreateMemberPaymentHardFailureDoesNotPersistRow(t *testing.T) {
	now := time.Date(2026, time.March, 30, 8, 0, 0, 0, time.UTC)
	repository := &stubRepository{
		store: StoreScope{
			ID:            "store-1",
			OwnerUserID:   "owner-1",
			OwnerUsername: "owner-demo",
			Status:        StoreStatusActive,
		},
		storeMember: StoreMember{
			ID:               "member-1",
			StoreID:          "store-1",
			RealUsername:     "member-alpha",
			UpstreamUserCode: "MEMBER000001",
			Status:           MemberStatusActive,
		},
	}
	upstream := &stubUpstream{err: qris.ErrNotConfigured}

	service := NewService(Options{
		Repository:           repository,
		Upstream:             upstream,
		Clock:                fixedClock{now: now},
		DefaultExpireSeconds: 300,
	}).(*service)
	service.memberPaymentFactory = func() (string, error) {
		return "MPAYFIXED000003", nil
	}

	_, err := service.CreateMemberPayment(context.Background(), "plain-store-token", CreateMemberPaymentInput{
		Username: "member-alpha",
		Amount:   json.Number("15000"),
	}, auth.RequestMetadata{})
	if !errors.Is(err, qris.ErrNotConfigured) {
		t.Fatalf("CreateMemberPayment error = %v, want qris.ErrNotConfigured", err)
	}
	if repository.createCalls != 0 {
		t.Fatalf("createCalls = %d, want 0", repository.createCalls)
	}
}

func TestHandlePaymentWebhookStoreTopupCreditsFullAmount(t *testing.T) {
	now := time.Date(2026, time.March, 30, 10, 0, 0, 0, time.UTC)
	transaction := QRISTransaction{
		ID:                "qris-topup-1",
		StoreID:           "store-1",
		Type:              TransactionTypeStoreTopup,
		ProviderTrxID:     stringPtr("provider-trx-1"),
		CustomRef:         "TOPUPFIXED000010",
		ExternalUsername:  "owner-demo",
		AmountGross:       "50000.00",
		PlatformFeeAmount: "0.00",
		StoreCreditAmount: "50000.00",
		Status:            TransactionStatusPending,
	}
	repository := &stubRepository{
		webhookTransaction: transaction,
	}
	ledgerService := newStubLedger()
	callbacks := &stubCallbacks{}

	service := NewService(Options{
		Repository:          repository,
		Ledger:              ledgerService,
		Callbacks:           callbacks,
		Clock:               fixedClock{now: now},
		MemberPaymentFeePct: 3,
	}).(*service)

	result, err := service.HandlePaymentWebhook(context.Background(), qris.PaymentWebhook{
		Amount:     50000,
		TerminalID: "owner-demo",
		TrxID:      "provider-trx-1",
		CustomRef:  "TOPUPFIXED000010",
		RRN:        "rrn-topup-1",
		Vendor:     "NOBU",
		Status:     "success",
	}, auth.RequestMetadata{
		IPAddress: "127.0.0.1",
		UserAgent: "unit-test",
	})
	if err != nil {
		t.Fatalf("HandlePaymentWebhook error = %v", err)
	}

	if !result.Processed {
		t.Fatal("result.Processed = false, want true")
	}
	if ledgerService.batchCalls != 1 {
		t.Fatalf("batchCalls = %d, want 1", ledgerService.batchCalls)
	}
	if len(ledgerService.lastBatch.Entries) != 1 {
		t.Fatalf("len(lastBatch.Entries) = %d, want 1", len(ledgerService.lastBatch.Entries))
	}
	if ledgerService.lastBatch.Entries[0].Amount != "50000.00" {
		t.Fatalf("ledger credit amount = %q, want 50000.00", ledgerService.lastBatch.Entries[0].Amount)
	}
	if ledgerService.lastBatch.Entries[0].EntryType != ledger.EntryTypeStoreTopup {
		t.Fatalf("ledger entry type = %q, want store_topup", ledgerService.lastBatch.Entries[0].EntryType)
	}
	if repository.finalizeCalls != 1 {
		t.Fatalf("finalizeCalls = %d, want 1", repository.finalizeCalls)
	}
	if repository.lastFinalize.Status != TransactionStatusSuccess {
		t.Fatalf("finalize status = %q, want success", repository.lastFinalize.Status)
	}
	if repository.lastFinalize.StoreCreditAmount != "50000.00" {
		t.Fatalf("store credit amount = %q, want 50000.00", repository.lastFinalize.StoreCreditAmount)
	}
	if callbacks.enqueueCalls != 0 {
		t.Fatalf("enqueueCalls = %d, want 0", callbacks.enqueueCalls)
	}
}

func TestHandlePaymentWebhookMemberPaymentCreditsNetAfterFee(t *testing.T) {
	now := time.Date(2026, time.March, 30, 10, 5, 0, 0, time.UTC)
	transaction := QRISTransaction{
		ID:                "qris-member-1",
		StoreID:           "store-1",
		StoreMemberID:     stringPtr("member-1"),
		Type:              TransactionTypeMemberPayment,
		ProviderTrxID:     stringPtr("provider-trx-2"),
		CustomRef:         "MPAYFIXED000010",
		ExternalUsername:  "MEMBER000001",
		AmountGross:       "25000.00",
		PlatformFeeAmount: "0.00",
		StoreCreditAmount: "0.00",
		Status:            TransactionStatusPending,
	}
	repository := &stubRepository{
		webhookTransaction: transaction,
	}
	ledgerService := newStubLedger()
	callbacks := &stubCallbacks{}

	service := NewService(Options{
		Repository:          repository,
		Ledger:              ledgerService,
		Callbacks:           callbacks,
		Clock:               fixedClock{now: now},
		MemberPaymentFeePct: 3,
	}).(*service)

	result, err := service.HandlePaymentWebhook(context.Background(), qris.PaymentWebhook{
		Amount:     25000,
		TerminalID: "MEMBER000001",
		TrxID:      "provider-trx-2",
		CustomRef:  "MPAYFIXED000010",
		RRN:        "rrn-member-1",
		Vendor:     "NOBU",
		Status:     "success",
	}, auth.RequestMetadata{
		IPAddress: "127.0.0.1",
		UserAgent: "unit-test",
	})
	if err != nil {
		t.Fatalf("HandlePaymentWebhook error = %v", err)
	}

	if !result.Processed {
		t.Fatal("result.Processed = false, want true")
	}
	if len(ledgerService.lastBatch.Entries) != 2 {
		t.Fatalf("len(lastBatch.Entries) = %d, want 2", len(ledgerService.lastBatch.Entries))
	}
	if ledgerService.lastBatch.Entries[0].Direction != ledger.DirectionCredit {
		t.Fatalf("first direction = %q, want credit", ledgerService.lastBatch.Entries[0].Direction)
	}
	if ledgerService.lastBatch.Entries[0].Amount != "25000.00" {
		t.Fatalf("first amount = %q, want 25000.00", ledgerService.lastBatch.Entries[0].Amount)
	}
	if ledgerService.lastBatch.Entries[0].EntryType != ledger.EntryTypeMemberPaymentCredit {
		t.Fatalf("first entry type = %q, want member_payment_credit", ledgerService.lastBatch.Entries[0].EntryType)
	}
	if ledgerService.lastBatch.Entries[1].Direction != ledger.DirectionDebit {
		t.Fatalf("second direction = %q, want debit", ledgerService.lastBatch.Entries[1].Direction)
	}
	if ledgerService.lastBatch.Entries[1].Amount != "750.00" {
		t.Fatalf("second amount = %q, want 750.00", ledgerService.lastBatch.Entries[1].Amount)
	}
	if ledgerService.lastBatch.Entries[1].EntryType != ledger.EntryTypeMemberPaymentFee {
		t.Fatalf("second entry type = %q, want member_payment_fee", ledgerService.lastBatch.Entries[1].EntryType)
	}
	if repository.lastFinalize.PlatformFeeAmount != "750.00" {
		t.Fatalf("platform fee amount = %q, want 750.00", repository.lastFinalize.PlatformFeeAmount)
	}
	if repository.lastFinalize.StoreCreditAmount != "24250.00" {
		t.Fatalf("store credit amount = %q, want 24250.00", repository.lastFinalize.StoreCreditAmount)
	}
	if callbacks.enqueueCalls != 1 {
		t.Fatalf("enqueueCalls = %d, want 1", callbacks.enqueueCalls)
	}
	if callbacks.lastTransactionID != "qris-member-1" {
		t.Fatalf("lastTransactionID = %q, want qris-member-1", callbacks.lastTransactionID)
	}
}

func TestHandlePaymentWebhookDuplicateSkipsSecondLedgerPost(t *testing.T) {
	now := time.Date(2026, time.March, 30, 10, 10, 0, 0, time.UTC)
	transaction := QRISTransaction{
		ID:                "qris-topup-dup",
		StoreID:           "store-1",
		Type:              TransactionTypeStoreTopup,
		ProviderTrxID:     stringPtr("provider-trx-dup"),
		CustomRef:         "TOPUPFIXED000011",
		ExternalUsername:  "owner-demo",
		AmountGross:       "10000.00",
		PlatformFeeAmount: "0.00",
		StoreCreditAmount: "10000.00",
		Status:            TransactionStatusPending,
	}
	repository := &stubRepository{
		webhookTransaction: transaction,
	}
	ledgerService := newStubLedger()
	ledgerService.duplicateReferences["qris_transaction:qris-topup-dup"] = true

	service := NewService(Options{
		Repository: repository,
		Ledger:     ledgerService,
		Clock:      fixedClock{now: now},
	}).(*service)

	_, err := service.HandlePaymentWebhook(context.Background(), qris.PaymentWebhook{
		Amount:    10000,
		TrxID:     "provider-trx-dup",
		CustomRef: "TOPUPFIXED000011",
		Status:    "success",
	}, auth.RequestMetadata{})
	if err != nil {
		t.Fatalf("HandlePaymentWebhook error = %v", err)
	}

	if ledgerService.batchCalls != 1 {
		t.Fatalf("batchCalls = %d, want 1", ledgerService.batchCalls)
	}
	if repository.finalizeCalls != 1 {
		t.Fatalf("finalizeCalls = %d, want 1", repository.finalizeCalls)
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
	storeMember          StoreMember
	transactions         []QRISTransaction
	webhookTransaction   QRISTransaction
	createCalls          int
	updateGeneratedCalls int
	updateStatusCalls    int
	finalizeCalls        int
	lastFinalize         FinalizeQRISTransactionParams
}

func (s *stubRepository) AuthenticateStore(context.Context, string) (StoreScope, error) {
	return s.store, nil
}

func (s *stubRepository) GetStoreScope(context.Context, string) (StoreScope, error) {
	return s.store, nil
}

func (s *stubRepository) FindStoreMemberByUsername(context.Context, string, string) (StoreMember, error) {
	return s.storeMember, nil
}

func (s *stubRepository) FindQRISTransactionForWebhook(context.Context, string, string) (QRISTransaction, error) {
	if s.webhookTransaction.ID == "" {
		return QRISTransaction{}, ErrNotFound
	}

	return s.webhookTransaction, nil
}

func (s *stubRepository) CreateQRISTransaction(_ context.Context, params CreateQRISTransactionParams) (QRISTransaction, error) {
	s.createCalls++
	state := payloadFieldProviderState(params.ProviderPayload)
	transaction := QRISTransaction{
		ID:                "qris-1",
		StoreID:           params.StoreID,
		StoreMemberID:     params.StoreMemberID,
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

func (s *stubRepository) FinalizeQRISTransaction(_ context.Context, params FinalizeQRISTransactionParams) (QRISTransaction, error) {
	s.finalizeCalls++
	s.lastFinalize = params

	transaction := s.webhookTransaction
	transaction.ProviderTrxID = stringPtr(params.ProviderTrxID)
	transaction.Status = params.Status
	transaction.PlatformFeeAmount = params.PlatformFeeAmount
	transaction.StoreCreditAmount = params.StoreCreditAmount
	transaction.ProviderState = payloadFieldProviderState(params.ProviderPayload)
	transaction.UpdatedAt = params.OccurredAt
	s.webhookTransaction = transaction

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

type stubLedger struct {
	batchCalls          int
	lastBatch           ledger.PostEntriesInput
	duplicateReferences map[string]bool
}

func newStubLedger() *stubLedger {
	return &stubLedger{
		duplicateReferences: map[string]bool{},
	}
}

func (s *stubLedger) PostEntries(_ context.Context, _ string, input ledger.PostEntriesInput) (ledger.BatchPostingResult, error) {
	s.batchCalls++
	s.lastBatch = input

	key := input.ReferenceType + ":" + input.ReferenceID
	if s.duplicateReferences[key] {
		return ledger.BatchPostingResult{}, ledger.ErrDuplicateReference
	}

	return ledger.BatchPostingResult{}, nil
}

type stubCallbacks struct {
	enqueueCalls      int
	lastTransactionID string
}

func (s *stubCallbacks) EnqueueMemberPaymentSuccess(_ context.Context, qrisTransactionID string) error {
	s.enqueueCalls++
	s.lastTransactionID = qrisTransactionID
	return nil
}
