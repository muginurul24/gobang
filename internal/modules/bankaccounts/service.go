package bankaccounts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/platform/bankdirectory"
	"github.com/mugiew/onixggr/internal/platform/clock"
)

type RepositoryContract interface {
	GetStoreScope(ctx context.Context, storeID string) (StoreScope, error)
	ListBankAccounts(ctx context.Context, storeID string) ([]BankAccount, error)
	CreateBankAccount(ctx context.Context, params CreateBankAccountParams) (BankAccount, error)
	GetBankAccountByID(ctx context.Context, bankAccountID string) (BankAccount, error)
	UpdateBankAccountStatus(ctx context.Context, params UpdateBankAccountStatusParams) (BankAccount, error)
	InsertAuditLog(
		ctx context.Context,
		actorUserID *string,
		actorRole string,
		storeID *string,
		action string,
		targetType string,
		targetID *string,
		payload map[string]any,
		ipAddress string,
		userAgent string,
		occurredAt time.Time,
	) error
}

type Service interface {
	SearchBanks(ctx context.Context, subject auth.Subject, filter SearchFilter) ([]BankDirectoryEntry, error)
	ListBankAccounts(ctx context.Context, subject auth.Subject, storeID string) ([]BankAccount, error)
	CreateBankAccount(ctx context.Context, subject auth.Subject, storeID string, input CreateBankAccountInput, metadata auth.RequestMetadata) (BankAccount, error)
	UpdateBankAccountStatus(ctx context.Context, subject auth.Subject, storeID string, bankAccountID string, input UpdateBankAccountStatusInput, metadata auth.RequestMetadata) (BankAccount, error)
}

type service struct {
	repository RepositoryContract
	directory  BankDirectory
	inquirer   InquiryVerifier
	sealer     AccountSealer
	clock      clock.Clock
}

func NewService(repository RepositoryContract, directory BankDirectory, inquirer InquiryVerifier, sealer AccountSealer, now clock.Clock) Service {
	if now == nil {
		now = clock.SystemClock{}
	}

	return &service{
		repository: repository,
		directory:  directory,
		inquirer:   inquirer,
		sealer:     sealer,
		clock:      now,
	}
}

func (s *service) SearchBanks(_ context.Context, subject auth.Subject, filter SearchFilter) ([]BankDirectoryEntry, error) {
	if !canUseBankAccounts(subject.Role) {
		return nil, ErrForbidden
	}

	results := s.directory.Search(strings.TrimSpace(filter.Query), filter.Limit)
	response := make([]BankDirectoryEntry, 0, len(results))
	for _, result := range results {
		response = append(response, BankDirectoryEntry{
			BankCode:      result.BankCode,
			BankName:      result.BankName,
			BankSwiftCode: result.BankSwiftCode,
		})
	}

	return response, nil
}

func (s *service) ListBankAccounts(ctx context.Context, subject auth.Subject, storeID string) ([]BankAccount, error) {
	store, err := s.loadStoreScope(ctx, strings.TrimSpace(storeID))
	if err != nil {
		return nil, err
	}

	if !s.canAccessStore(subject, store) {
		return nil, ErrForbidden
	}

	return s.repository.ListBankAccounts(ctx, store.ID)
}

func (s *service) CreateBankAccount(ctx context.Context, subject auth.Subject, storeID string, input CreateBankAccountInput, metadata auth.RequestMetadata) (BankAccount, error) {
	store, err := s.loadStoreScope(ctx, strings.TrimSpace(storeID))
	if err != nil {
		return BankAccount{}, err
	}

	if !s.canAccessStore(subject, store) {
		return BankAccount{}, ErrForbidden
	}

	bankCode := normalizeBankCode(input.BankCode)
	if !s.directory.HasCode(bankCode) {
		return BankAccount{}, ErrInvalidBankCode
	}

	accountNumber := normalizeAccountNumber(input.AccountNumber)
	if !validAccountNumber(accountNumber) {
		return BankAccount{}, ErrInvalidAccountNumber
	}

	verified, err := s.inquirer.Verify(ctx, InquiryRequest{
		BankCode:      bankCode,
		AccountNumber: accountNumber,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrInquiryUnavailable):
			return BankAccount{}, ErrInquiryUnavailable
		case errors.Is(err, ErrInvalidBankCode):
			return BankAccount{}, ErrInvalidBankCode
		default:
			return BankAccount{}, ErrInquiryFailed
		}
	}

	bankName := strings.TrimSpace(verified.BankName)
	if bankName == "" {
		entry, ok := s.directory.PrimaryByCode(bankCode)
		if !ok {
			return BankAccount{}, ErrInvalidBankCode
		}
		bankName = entry.BankName
	}

	accountName := strings.TrimSpace(verified.AccountName)
	if accountName == "" {
		return BankAccount{}, ErrInquiryFailed
	}

	encryptedAccountNumber, err := s.sealer.Seal(accountNumber)
	if err != nil {
		return BankAccount{}, fmt.Errorf("seal account number: %w", err)
	}

	now := s.clock.Now().UTC()
	bankAccount, err := s.repository.CreateBankAccount(ctx, CreateBankAccountParams{
		StoreID:                store.ID,
		BankCode:               bankCode,
		BankName:               bankName,
		AccountNumberEncrypted: encryptedAccountNumber,
		AccountNumberMasked:    maskAccountNumber(accountNumber),
		AccountName:            accountName,
		VerifiedAt:             now,
		IsActive:               true,
		OccurredAt:             now,
	})
	if err != nil {
		return BankAccount{}, err
	}

	if err := s.repository.InsertAuditLog(
		ctx,
		&subject.UserID,
		string(subject.Role),
		&store.ID,
		"store.bank_account_added",
		"store_bank_account",
		&bankAccount.ID,
		map[string]any{
			"bank_code":             bankCode,
			"bank_name":             bankName,
			"account_number_masked": bankAccount.AccountNumberMasked,
		},
		metadata.IPAddress,
		metadata.UserAgent,
		now,
	); err != nil {
		return BankAccount{}, err
	}

	return bankAccount, nil
}

func (s *service) UpdateBankAccountStatus(ctx context.Context, subject auth.Subject, storeID string, bankAccountID string, input UpdateBankAccountStatusInput, metadata auth.RequestMetadata) (BankAccount, error) {
	store, err := s.loadStoreScope(ctx, strings.TrimSpace(storeID))
	if err != nil {
		return BankAccount{}, err
	}

	if !s.canAccessStore(subject, store) {
		return BankAccount{}, ErrForbidden
	}

	bankAccount, err := s.repository.GetBankAccountByID(ctx, strings.TrimSpace(bankAccountID))
	if err != nil {
		return BankAccount{}, err
	}

	if bankAccount.StoreID != store.ID {
		return BankAccount{}, ErrNotFound
	}

	now := s.clock.Now().UTC()
	updated, err := s.repository.UpdateBankAccountStatus(ctx, UpdateBankAccountStatusParams{
		BankAccountID: bankAccount.ID,
		IsActive:      input.IsActive,
		OccurredAt:    now,
	})
	if err != nil {
		return BankAccount{}, err
	}

	if err := s.repository.InsertAuditLog(
		ctx,
		&subject.UserID,
		string(subject.Role),
		&store.ID,
		"store.bank_account_status_updated",
		"store_bank_account",
		&updated.ID,
		map[string]any{
			"is_active": updated.IsActive,
		},
		metadata.IPAddress,
		metadata.UserAgent,
		now,
	); err != nil {
		return BankAccount{}, err
	}

	return updated, nil
}

func (s *service) loadStoreScope(ctx context.Context, storeID string) (StoreScope, error) {
	store, err := s.repository.GetStoreScope(ctx, storeID)
	if err != nil {
		return StoreScope{}, err
	}

	if store.DeletedAt != nil {
		return StoreScope{}, ErrNotFound
	}

	return store, nil
}

func (s *service) canAccessStore(subject auth.Subject, store StoreScope) bool {
	if !canUseBankAccounts(subject.Role) {
		return false
	}

	switch subject.Role {
	case auth.RoleOwner:
		return store.OwnerUserID == subject.UserID
	case auth.RoleDev, auth.RoleSuperadmin:
		return true
	default:
		return false
	}
}

func canUseBankAccounts(role auth.Role) bool {
	switch role {
	case auth.RoleOwner, auth.RoleDev, auth.RoleSuperadmin:
		return true
	default:
		return false
	}
}

var _ BankDirectory = (*bankdirectory.Directory)(nil)
