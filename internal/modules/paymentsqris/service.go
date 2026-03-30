package paymentsqris

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/modules/ledger"
	"github.com/mugiew/onixggr/internal/platform/clock"
	"github.com/mugiew/onixggr/internal/platform/qris"
	"github.com/mugiew/onixggr/internal/platform/security"
)

const qrisLedgerReferenceType = "qris_transaction"

type RepositoryContract interface {
	AuthenticateStore(ctx context.Context, tokenHash string) (StoreScope, error)
	GetStoreScope(ctx context.Context, storeID string) (StoreScope, error)
	FindStoreMemberByUsername(ctx context.Context, storeID string, username string) (StoreMember, error)
	FindQRISTransactionForWebhook(ctx context.Context, providerTrxID string, customRef string) (QRISTransaction, error)
	CreateQRISTransaction(ctx context.Context, params CreateQRISTransactionParams) (QRISTransaction, error)
	UpdateGeneratedTransaction(ctx context.Context, params UpdateGeneratedTransactionParams) (QRISTransaction, error)
	UpdateTransactionStatus(ctx context.Context, params UpdateTransactionStatusParams) (QRISTransaction, error)
	FinalizeQRISTransaction(ctx context.Context, params FinalizeQRISTransactionParams) (QRISTransaction, error)
	ListQRISTransactions(ctx context.Context, storeID string, transactionType TransactionType) ([]QRISTransaction, error)
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

type UpstreamClient interface {
	Generate(ctx context.Context, input qris.GenerateInput) (qris.GenerateResult, error)
}

type LedgerContract interface {
	Credit(ctx context.Context, storeID string, input ledger.PostEntryInput) (ledger.PostingResult, error)
	HasReferenceEntries(ctx context.Context, referenceType string, referenceID string) (bool, error)
}

type CallbackContract interface {
	EnqueueMemberPaymentSuccess(ctx context.Context, qrisTransactionID string) error
}

type TransferWebhookHandler interface {
	HandleTransferWebhook(ctx context.Context, payload qris.TransferWebhook, metadata auth.RequestMetadata) (WebhookDispatchResult, error)
}

type Service interface {
	ListStoreTopups(ctx context.Context, subject auth.Subject, storeID string) ([]QRISTransaction, error)
	CreateStoreTopup(ctx context.Context, subject auth.Subject, storeID string, input CreateStoreTopupInput, metadata auth.RequestMetadata) (QRISTransaction, error)
	CreateMemberPayment(ctx context.Context, storeToken string, input CreateMemberPaymentInput, metadata auth.RequestMetadata) (QRISTransaction, error)
	HandlePaymentWebhook(ctx context.Context, payload qris.PaymentWebhook, metadata auth.RequestMetadata) (WebhookDispatchResult, error)
	HandleTransferWebhook(ctx context.Context, payload qris.TransferWebhook, metadata auth.RequestMetadata) (WebhookDispatchResult, error)
}

type Options struct {
	Repository           RepositoryContract
	Upstream             UpstreamClient
	Ledger               LedgerContract
	Callbacks            CallbackContract
	Clock                clock.Clock
	DefaultExpireSeconds int
	MemberPaymentFeePct  float64
	TransferWebhooks     TransferWebhookHandler
}

type service struct {
	repository           RepositoryContract
	upstream             UpstreamClient
	ledger               LedgerContract
	callbacks            CallbackContract
	clock                clock.Clock
	topupRefFactory      func() (string, error)
	memberPaymentFactory func() (string, error)
	defaultExpireSeconds int
	memberPaymentFeePct  float64
	transferWebhooks     TransferWebhookHandler
}

func NewService(options Options) Service {
	now := options.Clock
	if now == nil {
		now = clock.SystemClock{}
	}

	upstream := options.Upstream
	if upstream == nil {
		upstream = noopUpstream{}
	}
	ledgerService := options.Ledger
	if ledgerService == nil {
		ledgerService = noopLedger{}
	}
	callbackService := options.Callbacks
	if callbackService == nil {
		callbackService = noopCallbacks{}
	}
	transferWebhooks := options.TransferWebhooks
	if transferWebhooks == nil {
		transferWebhooks = noopTransferWebhookHandler{}
	}
	memberPaymentFeePct := options.MemberPaymentFeePct
	if memberPaymentFeePct <= 0 {
		memberPaymentFeePct = 3
	}

	return &service{
		repository:           options.Repository,
		upstream:             upstream,
		ledger:               ledgerService,
		callbacks:            callbackService,
		clock:                now,
		topupRefFactory:      newCustomRef,
		memberPaymentFactory: newMemberPaymentRef,
		defaultExpireSeconds: options.DefaultExpireSeconds,
		memberPaymentFeePct:  memberPaymentFeePct,
		transferWebhooks:     transferWebhooks,
	}
}

func (s *service) ListStoreTopups(ctx context.Context, subject auth.Subject, storeID string) ([]QRISTransaction, error) {
	store, err := s.repository.GetStoreScope(ctx, storeID)
	if err != nil {
		return nil, err
	}
	if store.DeletedAt != nil {
		return nil, ErrNotFound
	}
	if !canAccessStore(subject, store) {
		return nil, ErrForbidden
	}

	return s.repository.ListQRISTransactions(ctx, store.ID, TransactionTypeStoreTopup)
}

func (s *service) CreateStoreTopup(ctx context.Context, subject auth.Subject, storeID string, input CreateStoreTopupInput, metadata auth.RequestMetadata) (QRISTransaction, error) {
	store, err := s.repository.GetStoreScope(ctx, storeID)
	if err != nil {
		return QRISTransaction{}, err
	}
	if store.DeletedAt != nil {
		return QRISTransaction{}, ErrNotFound
	}
	if !canAccessStore(subject, store) {
		return QRISTransaction{}, ErrForbidden
	}
	if store.Status != StoreStatusActive {
		return QRISTransaction{}, ErrStoreInactive
	}

	amount, err := parseAmount(input.Amount)
	if err != nil {
		return QRISTransaction{}, err
	}

	now := s.clock.Now().UTC()
	var transaction QRISTransaction
	for attempt := 0; attempt < 5; attempt++ {
		customRef, err := s.topupRefFactory()
		if err != nil {
			return QRISTransaction{}, fmt.Errorf("generate topup custom ref: %w", err)
		}

		transaction, err = s.repository.CreateQRISTransaction(ctx, CreateQRISTransactionParams{
			StoreID:           store.ID,
			Type:              TransactionTypeStoreTopup,
			CustomRef:         customRef,
			ExternalUsername:  store.OwnerUsername,
			AmountGross:       formatAmount(amount),
			PlatformFeeAmount: formatAmount(0),
			StoreCreditAmount: formatAmount(amount),
			Status:            TransactionStatusPending,
			ProviderPayload: map[string]any{
				"provider_state": string(ProviderStatePendingGenerate),
			},
			OccurredAt: now,
		})
		if err == nil {
			break
		}
		if !errors.Is(err, ErrDuplicateCustomRef) {
			return QRISTransaction{}, err
		}
	}
	if transaction.ID == "" {
		return QRISTransaction{}, fmt.Errorf("create qris transaction: custom ref exhausted")
	}

	result, err := s.upstream.Generate(ctx, qris.GenerateInput{
		Username:  store.OwnerUsername,
		Amount:    amount,
		CustomRef: transaction.CustomRef,
	})
	if err != nil {
		updated, updateErr := s.resolveGenerateError(ctx, transaction, err, now)
		if updateErr != nil {
			return QRISTransaction{}, updateErr
		}

		if auditErr := s.insertDashboardCreateAudit(ctx, subject, updated, metadata, "paymentsqris.store_topup_create"); auditErr != nil {
			return QRISTransaction{}, auditErr
		}

		var businessErr *qris.BusinessError
		switch {
		case errors.Is(err, qris.ErrTimeout), errors.Is(err, qris.ErrUpstreamUnavailable), errors.Is(err, qris.ErrUnexpectedHTTP):
			return updated, nil
		case errors.As(err, &businessErr), errors.Is(err, qris.ErrNotConfigured), errors.Is(err, qris.ErrInvalidResponse), errors.Is(err, qris.ErrInvalidRequest):
			return QRISTransaction{}, err
		default:
			return QRISTransaction{}, err
		}
	}

	expiresAt := resolveExpiresAt(now, result.ExpiredAt, s.defaultExpireSeconds)
	transaction, err = s.repository.UpdateGeneratedTransaction(ctx, UpdateGeneratedTransactionParams{
		TransactionID: transaction.ID,
		ProviderTrxID: result.TrxID,
		ExpiresAt:     expiresAt,
		ProviderPayload: map[string]any{
			"provider_state": string(ProviderStateGenerated),
			"qr_code_value":  result.RawValue,
			"is_va":          result.IsVA,
		},
		OccurredAt: now,
	})
	if err != nil {
		return QRISTransaction{}, err
	}

	if err := s.insertDashboardCreateAudit(ctx, subject, transaction, metadata, "paymentsqris.store_topup_create"); err != nil {
		return QRISTransaction{}, err
	}

	return transaction, nil
}

func (s *service) CreateMemberPayment(ctx context.Context, storeToken string, input CreateMemberPaymentInput, metadata auth.RequestMetadata) (QRISTransaction, error) {
	store, err := s.authenticateStoreToken(ctx, storeToken)
	if err != nil {
		return QRISTransaction{}, err
	}
	if store.DeletedAt != nil {
		return QRISTransaction{}, ErrUnauthorized
	}
	if store.Status != StoreStatusActive {
		return QRISTransaction{}, ErrStoreInactive
	}

	username := normalizeUsername(input.Username)
	if username == "" {
		return QRISTransaction{}, ErrInvalidUsername
	}

	amount, err := parseAmount(input.Amount)
	if err != nil {
		return QRISTransaction{}, err
	}

	member, err := s.repository.FindStoreMemberByUsername(ctx, store.ID, username)
	if err != nil {
		return QRISTransaction{}, err
	}
	if member.Status != MemberStatusActive {
		return QRISTransaction{}, ErrMemberInactive
	}

	customRef, err := s.memberPaymentFactory()
	if err != nil {
		return QRISTransaction{}, fmt.Errorf("generate member payment ref: %w", err)
	}

	now := s.clock.Now().UTC()
	result, err := s.upstream.Generate(ctx, qris.GenerateInput{
		Username:  member.UpstreamUserCode,
		Amount:    amount,
		CustomRef: customRef,
	})
	if err != nil {
		transaction, createErr := s.resolveMemberPaymentGenerateError(ctx, store, member, customRef, amount, err, now)
		if createErr != nil {
			return QRISTransaction{}, createErr
		}

		if transaction.ID != "" {
			if auditErr := s.insertStoreAPICreateAudit(ctx, store, transaction, metadata, "paymentsqris.member_payment_create"); auditErr != nil {
				return QRISTransaction{}, auditErr
			}
			return transaction, nil
		}

		if auditErr := s.insertStoreAPIFailureAudit(ctx, store, member, customRef, amount, err, metadata); auditErr != nil {
			return QRISTransaction{}, auditErr
		}

		return QRISTransaction{}, err
	}

	transaction, err := s.repository.CreateQRISTransaction(ctx, CreateQRISTransactionParams{
		StoreID:           store.ID,
		StoreMemberID:     &member.ID,
		Type:              TransactionTypeMemberPayment,
		CustomRef:         customRef,
		ExternalUsername:  member.UpstreamUserCode,
		AmountGross:       formatAmount(amount),
		PlatformFeeAmount: formatAmount(0),
		StoreCreditAmount: formatAmount(0),
		Status:            TransactionStatusPending,
		ProviderPayload: map[string]any{
			"provider_state": string(ProviderStatePendingGenerate),
		},
		OccurredAt: now,
	})
	if err != nil {
		if auditErr := s.insertStoreAPIFailureAudit(ctx, store, member, customRef, amount, err, metadata); auditErr != nil {
			return QRISTransaction{}, auditErr
		}
		return QRISTransaction{}, err
	}

	expiresAt := resolveExpiresAt(now, result.ExpiredAt, s.defaultExpireSeconds)
	transaction, err = s.repository.UpdateGeneratedTransaction(ctx, UpdateGeneratedTransactionParams{
		TransactionID: transaction.ID,
		ProviderTrxID: result.TrxID,
		ExpiresAt:     expiresAt,
		ProviderPayload: map[string]any{
			"provider_state": string(ProviderStateGenerated),
			"qr_code_value":  result.RawValue,
			"is_va":          result.IsVA,
		},
		OccurredAt: now,
	})
	if err != nil {
		return QRISTransaction{}, err
	}

	if err := s.insertStoreAPICreateAudit(ctx, store, transaction, metadata, "paymentsqris.member_payment_create"); err != nil {
		return QRISTransaction{}, err
	}

	return transaction, nil
}

func (s *service) HandlePaymentWebhook(ctx context.Context, payload qris.PaymentWebhook, metadata auth.RequestMetadata) (WebhookDispatchResult, error) {
	reference := strings.TrimSpace(payload.TrxID)
	if reference == "" {
		reference = strings.TrimSpace(payload.CustomRef)
	}

	result := WebhookDispatchResult{
		Kind:      WebhookKindPayment,
		Reference: reference,
	}

	transaction, err := s.repository.FindQRISTransactionForWebhook(ctx, payload.TrxID, payload.CustomRef)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return result, nil
		}

		return WebhookDispatchResult{}, err
	}

	result.TransactionID = &transaction.ID
	result.TransactionType = &transaction.Type

	if transaction.ProviderTrxID != nil && strings.TrimSpace(payload.TrxID) != "" && *transaction.ProviderTrxID != strings.TrimSpace(payload.TrxID) {
		return result, nil
	}

	resolvedStatus, ok := resolvePaymentStatus(payload.Status)
	if !ok {
		return result, nil
	}

	if shouldIgnoreFinalStatus(transaction.Status, resolvedStatus) {
		if transaction.Type == TransactionTypeMemberPayment && resolvedStatus == TransactionStatusSuccess {
			if err := s.callbacks.EnqueueMemberPaymentSuccess(ctx, transaction.ID); err != nil {
				return WebhookDispatchResult{}, err
			}
		}
		result.Processed = true
		result.Status = &transaction.Status
		return result, nil
	}

	platformFeeAmount := formatAmount(0)
	storeCreditAmount := formatAmount(0)

	if resolvedStatus == TransactionStatusSuccess {
		switch transaction.Type {
		case TransactionTypeStoreTopup:
			storeCreditAmount = transaction.AmountGross
		case TransactionTypeMemberPayment:
			grossAmount, parseErr := parseAmountString(transaction.AmountGross)
			if parseErr != nil {
				return WebhookDispatchResult{}, parseErr
			}
			platformFeeAmount, storeCreditAmount = computeMemberPaymentAmounts(grossAmount, s.memberPaymentFeePct)
		}

		alreadyPosted, checkErr := s.ledger.HasReferenceEntries(ctx, qrisLedgerReferenceType, transaction.ID)
		if checkErr != nil {
			return WebhookDispatchResult{}, checkErr
		}

		if !alreadyPosted {
			if _, creditErr := s.ledger.Credit(ctx, transaction.StoreID, ledger.PostEntryInput{
				EntryType:     entryTypeForTransaction(transaction.Type),
				Amount:        storeCreditAmount,
				ReferenceType: qrisLedgerReferenceType,
				ReferenceID:   transaction.ID,
				Metadata: map[string]any{
					"qris_transaction_id": transaction.ID,
					"qris_type":           transaction.Type,
					"provider_trx_id":     strings.TrimSpace(payload.TrxID),
					"custom_ref":          transaction.CustomRef,
					"amount_gross":        transaction.AmountGross,
					"platform_fee_amount": platformFeeAmount,
					"store_credit_amount": storeCreditAmount,
					"rrn":                 strings.TrimSpace(payload.RRN),
					"vendor":              strings.TrimSpace(payload.Vendor),
				},
			}); creditErr != nil && !errors.Is(creditErr, ledger.ErrDuplicateReference) {
				return WebhookDispatchResult{}, creditErr
			}
		}
	}

	updated, err := s.repository.FinalizeQRISTransaction(ctx, FinalizeQRISTransactionParams{
		TransactionID:     transaction.ID,
		ProviderTrxID:     payload.TrxID,
		Status:            resolvedStatus,
		PlatformFeeAmount: platformFeeAmount,
		StoreCreditAmount: storeCreditAmount,
		ProviderPayload: map[string]any{
			"provider_state":  providerStateForStatus(resolvedStatus),
			"provider_status": strings.ToLower(strings.TrimSpace(payload.Status)),
			"terminal_id":     strings.TrimSpace(payload.TerminalID),
			"rrn":             strings.TrimSpace(payload.RRN),
			"vendor":          strings.TrimSpace(payload.Vendor),
			"custom_ref":      strings.TrimSpace(payload.CustomRef),
			"finish_at":       formatOptionalTime(payload.FinishAt),
			"created_at":      formatOptionalTime(payload.CreatedAt),
		},
		OccurredAt: s.clock.Now().UTC(),
	})
	if err != nil {
		return WebhookDispatchResult{}, err
	}

	if auditErr := s.insertWebhookAudit(ctx, updated, metadata, string(payload.Status)); auditErr != nil {
		return WebhookDispatchResult{}, auditErr
	}
	if updated.Type == TransactionTypeMemberPayment && updated.Status == TransactionStatusSuccess {
		if err := s.callbacks.EnqueueMemberPaymentSuccess(ctx, updated.ID); err != nil {
			return WebhookDispatchResult{}, err
		}
	}

	result.Processed = true
	result.Status = &updated.Status
	return result, nil
}

func (s *service) HandleTransferWebhook(ctx context.Context, payload qris.TransferWebhook, metadata auth.RequestMetadata) (WebhookDispatchResult, error) {
	return s.transferWebhooks.HandleTransferWebhook(ctx, payload, metadata)
}

func (s *service) resolveGenerateError(ctx context.Context, transaction QRISTransaction, err error, occurredAt time.Time) (QRISTransaction, error) {
	payload := map[string]any{
		"error": err.Error(),
	}

	switch {
	case errors.Is(err, qris.ErrTimeout), errors.Is(err, qris.ErrUpstreamUnavailable), errors.Is(err, qris.ErrUnexpectedHTTP):
		payload["provider_state"] = string(ProviderStatePendingProviderAnswer)
		return s.repository.UpdateTransactionStatus(ctx, UpdateTransactionStatusParams{
			TransactionID:   transaction.ID,
			Status:          TransactionStatusPending,
			ExpiresAt:       transaction.ExpiresAt,
			ProviderPayload: payload,
			OccurredAt:      occurredAt,
		})
	default:
		payload["provider_state"] = string(ProviderStateGenerateFailed)
		return s.repository.UpdateTransactionStatus(ctx, UpdateTransactionStatusParams{
			TransactionID:   transaction.ID,
			Status:          TransactionStatusFailed,
			ExpiresAt:       transaction.ExpiresAt,
			ProviderPayload: payload,
			OccurredAt:      occurredAt,
		})
	}
}

func (s *service) resolveMemberPaymentGenerateError(ctx context.Context, store StoreScope, member StoreMember, customRef string, amount int64, err error, occurredAt time.Time) (QRISTransaction, error) {
	switch {
	case errors.Is(err, qris.ErrTimeout), errors.Is(err, qris.ErrUpstreamUnavailable), errors.Is(err, qris.ErrUnexpectedHTTP):
		transaction, createErr := s.repository.CreateQRISTransaction(ctx, CreateQRISTransactionParams{
			StoreID:           store.ID,
			StoreMemberID:     &member.ID,
			Type:              TransactionTypeMemberPayment,
			CustomRef:         customRef,
			ExternalUsername:  member.UpstreamUserCode,
			AmountGross:       formatAmount(amount),
			PlatformFeeAmount: formatAmount(0),
			StoreCreditAmount: formatAmount(0),
			Status:            TransactionStatusPending,
			ProviderPayload: map[string]any{
				"provider_state": string(ProviderStatePendingProviderAnswer),
				"error":          err.Error(),
			},
			OccurredAt: occurredAt,
		})
		if createErr != nil {
			return QRISTransaction{}, createErr
		}

		return transaction, nil
	default:
		return QRISTransaction{}, nil
	}
}

func (s *service) insertDashboardCreateAudit(ctx context.Context, subject auth.Subject, transaction QRISTransaction, metadata auth.RequestMetadata, action string) error {
	payload := map[string]any{
		"type":              transaction.Type,
		"custom_ref":        transaction.CustomRef,
		"amount_gross":      transaction.AmountGross,
		"status":            transaction.Status,
		"provider_state":    transaction.ProviderState,
		"provider_trx_id":   transaction.ProviderTrxID,
		"external_username": transaction.ExternalUsername,
	}

	return s.repository.InsertAuditLog(
		ctx,
		&subject.UserID,
		string(subject.Role),
		&transaction.StoreID,
		action,
		"qris_transaction",
		&transaction.ID,
		payload,
		metadata.IPAddress,
		metadata.UserAgent,
		s.clock.Now().UTC(),
	)
}

func (s *service) insertStoreAPICreateAudit(ctx context.Context, store StoreScope, transaction QRISTransaction, metadata auth.RequestMetadata, action string) error {
	payload := map[string]any{
		"type":              transaction.Type,
		"custom_ref":        transaction.CustomRef,
		"amount_gross":      transaction.AmountGross,
		"status":            transaction.Status,
		"provider_state":    transaction.ProviderState,
		"provider_trx_id":   transaction.ProviderTrxID,
		"external_username": transaction.ExternalUsername,
	}

	return s.repository.InsertAuditLog(
		ctx,
		nil,
		"store_api",
		&transaction.StoreID,
		action,
		"qris_transaction",
		&transaction.ID,
		payload,
		metadata.IPAddress,
		metadata.UserAgent,
		s.clock.Now().UTC(),
	)
}

func (s *service) insertStoreAPIFailureAudit(ctx context.Context, store StoreScope, member StoreMember, customRef string, amount int64, cause error, metadata auth.RequestMetadata) error {
	payload := map[string]any{
		"type":              TransactionTypeMemberPayment,
		"custom_ref":        customRef,
		"amount_gross":      formatAmount(amount),
		"external_username": member.UpstreamUserCode,
		"error":             cause.Error(),
	}

	return s.repository.InsertAuditLog(
		ctx,
		nil,
		"store_api",
		&store.ID,
		"paymentsqris.member_payment_create_failed",
		"qris_transaction",
		nil,
		payload,
		metadata.IPAddress,
		metadata.UserAgent,
		s.clock.Now().UTC(),
	)
}

func (s *service) insertWebhookAudit(ctx context.Context, transaction QRISTransaction, metadata auth.RequestMetadata, providerStatus string) error {
	action := "paymentsqris.store_topup_webhook"
	if transaction.Type == TransactionTypeMemberPayment {
		action = "paymentsqris.member_payment_webhook"
	}

	payload := map[string]any{
		"type":                transaction.Type,
		"custom_ref":          transaction.CustomRef,
		"provider_trx_id":     transaction.ProviderTrxID,
		"status":              transaction.Status,
		"provider_status":     strings.ToLower(strings.TrimSpace(providerStatus)),
		"amount_gross":        transaction.AmountGross,
		"platform_fee_amount": transaction.PlatformFeeAmount,
		"store_credit_amount": transaction.StoreCreditAmount,
	}

	return s.repository.InsertAuditLog(
		ctx,
		nil,
		"provider_webhook",
		&transaction.StoreID,
		action,
		"qris_transaction",
		&transaction.ID,
		payload,
		metadata.IPAddress,
		metadata.UserAgent,
		s.clock.Now().UTC(),
	)
}

func (s *service) authenticateStoreToken(ctx context.Context, rawToken string) (StoreScope, error) {
	token := strings.TrimSpace(rawToken)
	if token == "" {
		return StoreScope{}, ErrUnauthorized
	}

	return s.repository.AuthenticateStore(ctx, security.HashStoreToken(token))
}

func canAccessStore(subject auth.Subject, store StoreScope) bool {
	switch subject.Role {
	case auth.RoleDev, auth.RoleSuperadmin:
		return true
	case auth.RoleOwner:
		return store.OwnerUserID == subject.UserID
	default:
		return false
	}
}

type noopUpstream struct{}

func (noopUpstream) Generate(context.Context, qris.GenerateInput) (qris.GenerateResult, error) {
	return qris.GenerateResult{}, qris.ErrNotConfigured
}

type noopLedger struct{}

func (noopLedger) Credit(context.Context, string, ledger.PostEntryInput) (ledger.PostingResult, error) {
	return ledger.PostingResult{}, ledger.ErrNotFound
}

func (noopLedger) HasReferenceEntries(context.Context, string, string) (bool, error) {
	return false, nil
}

type noopCallbacks struct{}

func (noopCallbacks) EnqueueMemberPaymentSuccess(context.Context, string) error {
	return nil
}

type noopTransferWebhookHandler struct{}

func (noopTransferWebhookHandler) HandleTransferWebhook(_ context.Context, payload qris.TransferWebhook, _ auth.RequestMetadata) (WebhookDispatchResult, error) {
	return WebhookDispatchResult{
		Kind:      WebhookKindWithdrawalStatus,
		Processed: false,
		Reference: strings.TrimSpace(payload.PartnerRefNo),
	}, nil
}

func entryTypeForTransaction(transactionType TransactionType) ledger.EntryType {
	switch transactionType {
	case TransactionTypeMemberPayment:
		return ledger.EntryTypeMemberPaymentCredit
	default:
		return ledger.EntryTypeStoreTopup
	}
}

func shouldIgnoreFinalStatus(current TransactionStatus, incoming TransactionStatus) bool {
	if current == TransactionStatusSuccess {
		return true
	}

	if current == incoming && current != TransactionStatusPending {
		return true
	}

	if (current == TransactionStatusFailed || current == TransactionStatusExpired) && incoming != TransactionStatusSuccess {
		return true
	}

	return false
}

func formatOptionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}

	return value.UTC().Format(time.RFC3339)
}

func parseAmountString(raw string) (int64, error) {
	whole, _, _ := strings.Cut(strings.TrimSpace(raw), ".")
	if whole == "" {
		return 0, ErrInvalidAmount
	}

	amount, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || amount <= 0 {
		return 0, ErrInvalidAmount
	}

	return amount, nil
}
