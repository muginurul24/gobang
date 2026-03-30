package paymentsqris

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/platform/clock"
	"github.com/mugiew/onixggr/internal/platform/qris"
	"github.com/mugiew/onixggr/internal/platform/security"
)

type RepositoryContract interface {
	AuthenticateStore(ctx context.Context, tokenHash string) (StoreScope, error)
	GetStoreScope(ctx context.Context, storeID string) (StoreScope, error)
	FindStoreMemberByUsername(ctx context.Context, storeID string, username string) (StoreMember, error)
	CreateQRISTransaction(ctx context.Context, params CreateQRISTransactionParams) (QRISTransaction, error)
	UpdateGeneratedTransaction(ctx context.Context, params UpdateGeneratedTransactionParams) (QRISTransaction, error)
	UpdateTransactionStatus(ctx context.Context, params UpdateTransactionStatusParams) (QRISTransaction, error)
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

type Service interface {
	ListStoreTopups(ctx context.Context, subject auth.Subject, storeID string) ([]QRISTransaction, error)
	CreateStoreTopup(ctx context.Context, subject auth.Subject, storeID string, input CreateStoreTopupInput, metadata auth.RequestMetadata) (QRISTransaction, error)
	CreateMemberPayment(ctx context.Context, storeToken string, input CreateMemberPaymentInput, metadata auth.RequestMetadata) (QRISTransaction, error)
}

type Options struct {
	Repository           RepositoryContract
	Upstream             UpstreamClient
	Clock                clock.Clock
	DefaultExpireSeconds int
}

type service struct {
	repository           RepositoryContract
	upstream             UpstreamClient
	clock                clock.Clock
	topupRefFactory      func() (string, error)
	memberPaymentFactory func() (string, error)
	defaultExpireSeconds int
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

	return &service{
		repository:           options.Repository,
		upstream:             upstream,
		clock:                now,
		topupRefFactory:      newCustomRef,
		memberPaymentFactory: newMemberPaymentRef,
		defaultExpireSeconds: options.DefaultExpireSeconds,
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
