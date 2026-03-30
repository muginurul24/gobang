package bankaccounts

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/platform/bankdirectory"
)

func TestCreateBankAccountStoresMaskedAndEncryptedValues(t *testing.T) {
	now := time.Date(2026, 3, 30, 15, 0, 0, 0, time.UTC)
	repository := newFakeRepository(now)
	directory := bankdirectory.New([]bankdirectory.Entry{
		{BankCode: "542", BankName: "PT. BANK ARTOS INDONESIA"},
	})
	service := NewService(repository, directory, fakeInquirer{
		result: InquiryResult{
			BankCode:      "542",
			BankName:      "PT. BANK ARTOS INDONESIA (Bank Jago)",
			AccountNumber: "100009689749",
			AccountName:   "SISKA DAMAYANTI",
		},
	}, fakeSealer{}, fixedClock{now: now})

	bankAccount, err := service.CreateBankAccount(context.Background(), auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	}, "store-1", CreateBankAccountInput{
		BankCode:      "542",
		AccountNumber: "100009689749",
	}, auth.RequestMetadata{
		IPAddress: "127.0.0.1",
		UserAgent: "bankaccounts-test",
	})
	if err != nil {
		t.Fatalf("CreateBankAccount returned error: %v", err)
	}

	if bankAccount.AccountName != "SISKA DAMAYANTI" {
		t.Fatalf("AccountName = %q, want SISKA DAMAYANTI", bankAccount.AccountName)
	}

	if bankAccount.AccountNumberMasked != "********9749" {
		t.Fatalf("AccountNumberMasked = %q, want ********9749", bankAccount.AccountNumberMasked)
	}

	if repository.lastCreated.AccountNumberEncrypted != "sealed:100009689749" {
		t.Fatalf("AccountNumberEncrypted = %q, want sealed value", repository.lastCreated.AccountNumberEncrypted)
	}

	if len(repository.auditActions) != 1 || repository.auditActions[0] != "store.bank_account_added" {
		t.Fatalf("auditActions = %#v, want store.bank_account_added", repository.auditActions)
	}
}

func TestCreateBankAccountRejectsInvalidBankCode(t *testing.T) {
	repository := newFakeRepository(time.Now().UTC())
	directory := bankdirectory.New([]bankdirectory.Entry{
		{BankCode: "014", BankName: "PT. BANK CENTRAL ASIA, TBK."},
	})
	service := NewService(repository, directory, fakeInquirer{}, fakeSealer{}, fixedClock{now: time.Now().UTC()})

	_, err := service.CreateBankAccount(context.Background(), auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	}, "store-1", CreateBankAccountInput{
		BankCode:      "999",
		AccountNumber: "1234567890",
	}, auth.RequestMetadata{})
	if !errors.Is(err, ErrInvalidBankCode) {
		t.Fatalf("CreateBankAccount error = %v, want ErrInvalidBankCode", err)
	}
}

func TestListBankAccountsBlocksKaryawan(t *testing.T) {
	repository := newFakeRepository(time.Now().UTC())
	directory := bankdirectory.New([]bankdirectory.Entry{
		{BankCode: "014", BankName: "PT. BANK CENTRAL ASIA, TBK."},
	})
	service := NewService(repository, directory, fakeInquirer{}, fakeSealer{}, fixedClock{now: time.Now().UTC()})

	_, err := service.ListBankAccounts(context.Background(), auth.Subject{
		UserID: "employee-1",
		Role:   auth.RoleKaryawan,
	}, "store-1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListBankAccounts error = %v, want ErrForbidden", err)
	}
}

func TestUpdateBankAccountStatusRequiresMatchingStoreScope(t *testing.T) {
	now := time.Date(2026, 3, 30, 16, 0, 0, 0, time.UTC)
	repository := newFakeRepository(now)
	repository.bankAccounts["bank-1"] = BankAccount{
		ID:                  "bank-1",
		StoreID:             "store-2",
		BankCode:            "542",
		BankName:            "PT. BANK ARTOS INDONESIA",
		AccountNumberMasked: "********9749",
		AccountName:         "SISKA DAMAYANTI",
		IsActive:            true,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	directory := bankdirectory.New([]bankdirectory.Entry{
		{BankCode: "542", BankName: "PT. BANK ARTOS INDONESIA"},
	})
	service := NewService(repository, directory, fakeInquirer{}, fakeSealer{}, fixedClock{now: now})

	_, err := service.UpdateBankAccountStatus(context.Background(), auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	}, "store-1", "bank-1", UpdateBankAccountStatusInput{IsActive: false}, auth.RequestMetadata{})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateBankAccountStatus error = %v, want ErrNotFound", err)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

type fakeInquirer struct {
	result InquiryResult
	err    error
}

func (f fakeInquirer) Verify(_ context.Context, _ InquiryRequest) (InquiryResult, error) {
	return f.result, f.err
}

type fakeSealer struct{}

func (fakeSealer) Seal(plain string) (string, error) {
	return "sealed:" + plain, nil
}

type fakeRepository struct {
	store        StoreScope
	bankAccounts map[string]BankAccount
	lastCreated  CreateBankAccountParams
	auditActions []string
}

func newFakeRepository(now time.Time) *fakeRepository {
	return &fakeRepository{
		store: StoreScope{
			ID:          "store-1",
			OwnerUserID: "owner-1",
			Name:        "Demo Store",
			Slug:        "demo-store",
		},
		bankAccounts: map[string]BankAccount{},
	}
}

func (r *fakeRepository) GetStoreScope(_ context.Context, storeID string) (StoreScope, error) {
	if storeID != r.store.ID {
		return StoreScope{}, ErrNotFound
	}

	return r.store, nil
}

func (r *fakeRepository) ListBankAccounts(_ context.Context, storeID string) ([]BankAccount, error) {
	var bankAccounts []BankAccount
	for _, bankAccount := range r.bankAccounts {
		if bankAccount.StoreID == storeID {
			bankAccounts = append(bankAccounts, bankAccount)
		}
	}

	return bankAccounts, nil
}

func (r *fakeRepository) CreateBankAccount(_ context.Context, params CreateBankAccountParams) (BankAccount, error) {
	r.lastCreated = params
	bankAccount := BankAccount{
		ID:                  "bank-created",
		StoreID:             params.StoreID,
		BankCode:            params.BankCode,
		BankName:            params.BankName,
		AccountNumberMasked: params.AccountNumberMasked,
		AccountName:         params.AccountName,
		VerifiedAt:          &params.VerifiedAt,
		IsActive:            params.IsActive,
		CreatedAt:           params.OccurredAt,
		UpdatedAt:           params.OccurredAt,
	}
	r.bankAccounts[bankAccount.ID] = bankAccount
	return bankAccount, nil
}

func (r *fakeRepository) GetBankAccountByID(_ context.Context, bankAccountID string) (BankAccount, error) {
	bankAccount, ok := r.bankAccounts[bankAccountID]
	if !ok {
		return BankAccount{}, ErrNotFound
	}

	return bankAccount, nil
}

func (r *fakeRepository) UpdateBankAccountStatus(_ context.Context, params UpdateBankAccountStatusParams) (BankAccount, error) {
	bankAccount, ok := r.bankAccounts[params.BankAccountID]
	if !ok {
		return BankAccount{}, ErrNotFound
	}

	bankAccount.IsActive = params.IsActive
	bankAccount.UpdatedAt = params.OccurredAt
	r.bankAccounts[bankAccount.ID] = bankAccount
	return bankAccount, nil
}

func (r *fakeRepository) InsertAuditLog(_ context.Context, _ *string, _ string, _ *string, action string, _ string, _ *string, _ map[string]any, _ string, _ string, _ time.Time) error {
	r.auditActions = append(r.auditActions, action)
	return nil
}
