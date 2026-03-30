package withdrawals

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/modules/ledger"
	"github.com/mugiew/onixggr/internal/platform/clock"
	"github.com/mugiew/onixggr/internal/platform/qris"
)

const ledgerReferenceType = "store_withdrawal"

type RepositoryContract interface {
	GetStoreScope(ctx context.Context, storeID string) (StoreScope, error)
	GetStoreBankAccount(ctx context.Context, storeID string, bankAccountID string) (StoreBankAccount, error)
	FindByIdempotencyKey(ctx context.Context, storeID string, idempotencyKey string) (StoreWithdrawal, error)
	FindByPartnerRefNo(ctx context.Context, partnerRefNo string) (StoreWithdrawal, error)
	GetByID(ctx context.Context, withdrawalID string) (StoreWithdrawal, error)
	ListStoreWithdrawals(ctx context.Context, storeID string) ([]StoreWithdrawal, error)
	CreateStoreWithdrawal(ctx context.Context, params CreateStoreWithdrawalParams) (StoreWithdrawal, error)
	UpdateStoreWithdrawal(ctx context.Context, params UpdateStoreWithdrawalParams) (StoreWithdrawal, error)
	AcquireProcessingLock(ctx context.Context, withdrawalID string) (ProcessingLock, bool, error)
	NextStatusCheckAttemptNo(ctx context.Context, withdrawalID string) (int, error)
	ListDueStatusCheckWithdrawals(ctx context.Context, cutoff time.Time, limit int) ([]StatusCheckCandidate, error)
	RecordStatusCheck(ctx context.Context, params RecordStatusCheckParams) error
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

type LedgerContract interface {
	GetBalance(ctx context.Context, storeID string) (ledger.BalanceSnapshot, error)
	HasReferenceEntries(ctx context.Context, referenceType string, referenceID string) (bool, error)
	Reserve(ctx context.Context, storeID string, input ledger.ReserveInput) (ledger.ReservationResult, error)
	CommitReservation(ctx context.Context, storeID string, input ledger.CommitReservationInput) (ledger.CommitReservationResult, error)
	ReleaseReservation(ctx context.Context, storeID string, input ledger.ReleaseReservationInput) (ledger.ReservationResult, error)
}

type AccountOpener interface {
	Open(cipherText string) (string, error)
}

type NotificationEmitter interface {
	Emit(storeID string, eventType string, title string, body string)
}

type Service interface {
	ListStoreWithdrawals(ctx context.Context, subject auth.Subject, storeID string) ([]StoreWithdrawal, error)
	CreateStoreWithdrawal(ctx context.Context, subject auth.Subject, storeID string, input CreateWithdrawInput, metadata auth.RequestMetadata) (StoreWithdrawal, bool, error)
	HandleTransferWebhook(ctx context.Context, payload qris.TransferWebhook, metadata auth.RequestMetadata) (TransferWebhookResult, error)
	RunPendingChecks(ctx context.Context, limit int) (StatusCheckRunSummary, error)
}

type Options struct {
	Repository          RepositoryContract
	Provider            Provider
	Ledger              LedgerContract
	AccountOpener       AccountOpener
	Notifications       NotificationEmitter
	Clock               clock.Clock
	PlatformFeePercent  float64
	StatusCheckInterval time.Duration
}

type service struct {
	repository          RepositoryContract
	provider            Provider
	ledger              LedgerContract
	opener              AccountOpener
	notifications       NotificationEmitter
	clock               clock.Clock
	platformFeePercent  float64
	statusCheckInterval time.Duration
}

func NewService(options Options) Service {
	now := options.Clock
	if now == nil {
		now = clock.SystemClock{}
	}

	feePercent := options.PlatformFeePercent
	if feePercent <= 0 {
		feePercent = 12
	}

	statusCheckInterval := options.StatusCheckInterval
	if statusCheckInterval <= 0 {
		statusCheckInterval = 30 * time.Second
	}

	notifs := options.Notifications
	if notifs == nil {
		notifs = noopNotificationEmitter{}
	}

	return &service{
		repository:          options.Repository,
		provider:            options.Provider,
		ledger:              options.Ledger,
		opener:              options.AccountOpener,
		notifications:       notifs,
		clock:               now,
		platformFeePercent:  feePercent,
		statusCheckInterval: statusCheckInterval,
	}
}

func (s *service) ListStoreWithdrawals(ctx context.Context, subject auth.Subject, storeID string) ([]StoreWithdrawal, error) {
	store, err := s.loadStore(ctx, storeID)
	if err != nil {
		return nil, err
	}
	if !canViewWithdrawals(subject, store) {
		return nil, ErrForbidden
	}

	return s.repository.ListStoreWithdrawals(ctx, store.ID)
}

func (s *service) CreateStoreWithdrawal(ctx context.Context, subject auth.Subject, storeID string, input CreateWithdrawInput, metadata auth.RequestMetadata) (StoreWithdrawal, bool, error) {
	store, err := s.loadStore(ctx, storeID)
	if err != nil {
		return StoreWithdrawal{}, false, err
	}
	if !canCreateWithdrawals(subject, store) {
		return StoreWithdrawal{}, false, ErrForbidden
	}
	if store.Status != StoreStatusActive {
		return StoreWithdrawal{}, false, ErrStoreInactive
	}

	amount, err := parseAmount(input.Amount)
	if err != nil {
		return StoreWithdrawal{}, false, err
	}

	idempotencyKey := normalizeIdempotencyKey(input.IdempotencyKey)
	if !validIdempotencyKey(idempotencyKey) {
		return StoreWithdrawal{}, false, ErrInvalidIdempotencyKey
	}

	existing, err := s.repository.FindByIdempotencyKey(ctx, store.ID, idempotencyKey)
	switch {
	case err == nil:
		if !sameIntent(existing, input.BankAccountID, amount) {
			return StoreWithdrawal{}, false, ErrIdempotencyKeyConflict
		}

		return existing, false, nil
	case errors.Is(err, ErrNotFound):
	default:
		return StoreWithdrawal{}, false, err
	}

	account, err := s.repository.GetStoreBankAccount(ctx, store.ID, input.BankAccountID)
	if err != nil {
		return StoreWithdrawal{}, false, err
	}
	if !account.IsActive {
		return StoreWithdrawal{}, false, ErrBankAccountInactive
	}

	now := s.clock.Now().UTC()
	withdrawal, err := s.repository.CreateStoreWithdrawal(ctx, CreateStoreWithdrawalParams{
		StoreID:            store.ID,
		StoreBankAccountID: account.ID,
		IdempotencyKey:     idempotencyKey,
		NetRequestedAmount: formatAmount(amount),
		PlatformFeeAmount:  formatAmount(0),
		ExternalFeeAmount:  formatAmount(0),
		TotalStoreDebit:    formatAmount(0),
		Status:             WithdrawalStatusPending,
		RequestPayload: map[string]any{
			"idempotency_key":       idempotencyKey,
			"bank_account_id":       account.ID,
			"bank_code":             account.BankCode,
			"bank_name":             account.BankName,
			"account_name":          account.AccountName,
			"account_number_masked": account.AccountNumberMasked,
			"net_requested_amount":  formatAmount(amount),
		},
		ProviderPayload: map[string]any{
			"provider_state": "pending_inquiry",
		},
		OccurredAt: now,
	})
	if err != nil {
		return StoreWithdrawal{}, false, err
	}

	accountNumber, err := s.opener.Open(account.AccountNumberEncrypted)
	if err != nil {
		return StoreWithdrawal{}, true, fmt.Errorf("open bank account number: %w", err)
	}

	inquiryResult, err := s.provider.Inquiry(ctx, ProviderInquiryInput{
		Amount:         int64(amount / 100),
		BankCode:       account.BankCode,
		AccountNumber:  accountNumber,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		failureCode := classifyInquiryError(err)
		failed, updateErr := s.repository.UpdateStoreWithdrawal(ctx, UpdateStoreWithdrawalParams{
			WithdrawalID: withdrawal.ID,
			Status:       statusPtr(WithdrawalStatusFailed),
			ProviderPayload: map[string]any{
				"provider_state": "inquiry_failed",
				"error":          err.Error(),
			},
			OccurredAt: now,
		})
		if updateErr != nil {
			return StoreWithdrawal{}, true, updateErr
		}

		if auditErr := s.insertAudit(ctx, subject, failed, metadata, "withdraw.failed", map[string]any{
			"reason": "inquiry_failed",
		}); auditErr != nil {
			return StoreWithdrawal{}, true, auditErr
		}

		return StoreWithdrawal{}, true, &CreateFailure{Withdrawal: failed, Cause: failureCode}
	}

	platformFee := computePlatformFee(amount, s.platformFeePercent)
	externalFee := money(inquiryResult.ExternalFee)
	totalStoreDebit := amount + platformFee + externalFee
	inquiryID := stringPtr(inquiryResult.InquiryID)
	partnerRefNo := stringPtr(inquiryResult.PartnerRefNo)

	withdrawal, err = s.repository.UpdateStoreWithdrawal(ctx, UpdateStoreWithdrawalParams{
		WithdrawalID:         withdrawal.ID,
		PlatformFeeAmount:    stringPtr(formatAmount(platformFee)),
		ExternalFeeAmount:    stringPtr(formatAmount(externalFee)),
		TotalStoreDebit:      stringPtr(formatAmount(totalStoreDebit)),
		ProviderPartnerRefNo: partnerRefNo,
		ProviderInquiryID:    inquiryID,
		ProviderPayload: map[string]any{
			"provider_state":       "inquiry_success",
			"account_name":         inquiryResult.AccountName,
			"bank_code":            inquiryResult.BankCode,
			"bank_name":            inquiryResult.BankName,
			"partner_ref_no":       inquiryResult.PartnerRefNo,
			"inquiry_id":           inquiryResult.InquiryID,
			"external_fee":         formatAmount(externalFee),
			"net_requested_amount": formatAmount(amount),
		},
		OccurredAt: now,
	})
	if err != nil {
		return StoreWithdrawal{}, true, err
	}

	balance, err := s.ledger.GetBalance(ctx, store.ID)
	if err != nil {
		return StoreWithdrawal{}, true, err
	}

	availableBalance, err := parseMoneyString(balance.AvailableBalance)
	if err != nil {
		return StoreWithdrawal{}, true, err
	}

	if availableBalance.LessThan(totalStoreDebit) {
		failed, updateErr := s.repository.UpdateStoreWithdrawal(ctx, UpdateStoreWithdrawalParams{
			WithdrawalID: withdrawal.ID,
			Status:       statusPtr(WithdrawalStatusFailed),
			ProviderPayload: map[string]any{
				"provider_state":    "insufficient_balance",
				"available_balance": balance.AvailableBalance,
				"total_store_debit": formatAmount(totalStoreDebit),
				"partner_ref_no":    inquiryResult.PartnerRefNo,
				"inquiry_id":        inquiryResult.InquiryID,
			},
			OccurredAt: now,
		})
		if updateErr != nil {
			return StoreWithdrawal{}, true, updateErr
		}

		if auditErr := s.insertAudit(ctx, subject, failed, metadata, "withdraw.failed", map[string]any{
			"reason":            "insufficient_balance",
			"available_balance": balance.AvailableBalance,
			"total_store_debit": formatAmount(totalStoreDebit),
		}); auditErr != nil {
			return StoreWithdrawal{}, true, auditErr
		}

		return StoreWithdrawal{}, true, &CreateFailure{Withdrawal: failed, Cause: ErrInsufficientStoreBalance}
	}

	if _, err := s.ledger.Reserve(ctx, store.ID, ledger.ReserveInput{
		Amount:        formatAmount(totalStoreDebit),
		ReferenceType: ledgerReferenceType,
		ReferenceID:   withdrawal.ID,
	}); err != nil {
		failed, updateErr := s.repository.UpdateStoreWithdrawal(ctx, UpdateStoreWithdrawalParams{
			WithdrawalID: withdrawal.ID,
			Status:       statusPtr(WithdrawalStatusFailed),
			ProviderPayload: map[string]any{
				"provider_state": "reserve_failed",
				"error":          err.Error(),
			},
			OccurredAt: now,
		})
		if updateErr != nil {
			return StoreWithdrawal{}, true, updateErr
		}

		if auditErr := s.insertAudit(ctx, subject, failed, metadata, "withdraw.failed", map[string]any{
			"reason": "reserve_failed",
		}); auditErr != nil {
			return StoreWithdrawal{}, true, auditErr
		}

		if errors.Is(err, ledger.ErrInsufficientFunds) {
			return StoreWithdrawal{}, true, &CreateFailure{Withdrawal: failed, Cause: ErrInsufficientStoreBalance}
		}

		return StoreWithdrawal{}, true, err
	}

	transferResult, err := s.provider.Transfer(ctx, ProviderTransferInput{
		Amount:        int64(amount / 100),
		BankCode:      account.BankCode,
		AccountNumber: accountNumber,
		InquiryID:     inquiryResult.InquiryID,
	})
	if err != nil {
		classified, ambiguous := classifyTransferError(err)
		if ambiguous {
			pending, updateErr := s.repository.UpdateStoreWithdrawal(ctx, UpdateStoreWithdrawalParams{
				WithdrawalID: withdrawal.ID,
				ProviderPayload: map[string]any{
					"provider_state": "pending_transfer_confirmation",
					"error":          err.Error(),
					"partner_ref_no": inquiryResult.PartnerRefNo,
					"inquiry_id":     inquiryResult.InquiryID,
				},
				OccurredAt: now,
			})
			if updateErr != nil {
				return StoreWithdrawal{}, true, updateErr
			}

			if auditErr := s.insertAudit(ctx, subject, pending, metadata, "withdraw.pending", map[string]any{
				"reason": "transfer_ambiguous",
			}); auditErr != nil {
				return StoreWithdrawal{}, true, auditErr
			}

			return pending, true, nil
		}

		if _, releaseErr := s.ledger.ReleaseReservation(ctx, store.ID, ledger.ReleaseReservationInput{
			ReferenceType: ledgerReferenceType,
			ReferenceID:   withdrawal.ID,
		}); releaseErr != nil {
			return StoreWithdrawal{}, true, releaseErr
		}

		failed, updateErr := s.repository.UpdateStoreWithdrawal(ctx, UpdateStoreWithdrawalParams{
			WithdrawalID: withdrawal.ID,
			Status:       statusPtr(WithdrawalStatusFailed),
			ProviderPayload: map[string]any{
				"provider_state": "transfer_failed",
				"error":          err.Error(),
				"partner_ref_no": inquiryResult.PartnerRefNo,
				"inquiry_id":     inquiryResult.InquiryID,
			},
			OccurredAt: now,
		})
		if updateErr != nil {
			return StoreWithdrawal{}, true, updateErr
		}

		if auditErr := s.insertAudit(ctx, subject, failed, metadata, "withdraw.failed", map[string]any{
			"reason": "transfer_failed",
		}); auditErr != nil {
			return StoreWithdrawal{}, true, auditErr
		}

		return StoreWithdrawal{}, true, &CreateFailure{Withdrawal: failed, Cause: classified}
	}

	withdrawal, err = s.repository.UpdateStoreWithdrawal(ctx, UpdateStoreWithdrawalParams{
		WithdrawalID: withdrawal.ID,
		ProviderPayload: map[string]any{
			"provider_state":    "transfer_accepted",
			"transfer_accepted": transferResult.Accepted,
			"partner_ref_no":    inquiryResult.PartnerRefNo,
			"inquiry_id":        inquiryResult.InquiryID,
		},
		OccurredAt: now,
	})
	if err != nil {
		return StoreWithdrawal{}, true, err
	}

	if auditErr := s.insertAudit(ctx, subject, withdrawal, metadata, "withdraw.pending", map[string]any{
		"total_store_debit": withdrawal.TotalStoreDebit,
	}); auditErr != nil {
		return StoreWithdrawal{}, true, auditErr
	}

	return withdrawal, true, nil
}

func (s *service) loadStore(ctx context.Context, storeID string) (StoreScope, error) {
	store, err := s.repository.GetStoreScope(ctx, strings.TrimSpace(storeID))
	if err != nil {
		return StoreScope{}, err
	}
	if store.DeletedAt != nil {
		return StoreScope{}, ErrNotFound
	}

	return store, nil
}

func (s *service) insertAudit(ctx context.Context, subject auth.Subject, withdrawal StoreWithdrawal, metadata auth.RequestMetadata, action string, payload map[string]any) error {
	masked := map[string]any{
		"idempotency_key":       withdrawal.IdempotencyKey,
		"store_bank_account_id": withdrawal.StoreBankAccountID,
		"bank_code":             withdrawal.BankCode,
		"bank_name":             withdrawal.BankName,
		"account_name":          withdrawal.AccountName,
		"account_number_masked": withdrawal.AccountNumberMasked,
		"net_requested_amount":  withdrawal.NetRequestedAmount,
		"platform_fee_amount":   withdrawal.PlatformFeeAmount,
		"external_fee_amount":   withdrawal.ExternalFeeAmount,
		"total_store_debit":     withdrawal.TotalStoreDebit,
		"status":                withdrawal.Status,
	}
	for key, value := range payload {
		masked[key] = value
	}

	return s.repository.InsertAuditLog(
		ctx,
		&subject.UserID,
		string(subject.Role),
		&withdrawal.StoreID,
		action,
		"store_withdrawal",
		&withdrawal.ID,
		masked,
		metadata.IPAddress,
		metadata.UserAgent,
		s.clock.Now().UTC(),
	)
}

func canViewWithdrawals(subject auth.Subject, store StoreScope) bool {
	switch subject.Role {
	case auth.RoleOwner:
		return store.OwnerUserID == subject.UserID
	case auth.RoleDev, auth.RoleSuperadmin:
		return true
	default:
		return false
	}
}

func canCreateWithdrawals(subject auth.Subject, store StoreScope) bool {
	return canViewWithdrawals(subject, store)
}

func statusPtr(value WithdrawalStatus) *WithdrawalStatus {
	if value == "" {
		return nil
	}

	result := value
	return &result
}

type noopNotificationEmitter struct{}

func (noopNotificationEmitter) Emit(string, string, string, string) {}
