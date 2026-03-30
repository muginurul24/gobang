package paymentsqris

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/platform/clock"
	"github.com/mugiew/onixggr/internal/platform/qris"
)

type RepositoryContract interface {
	GetStoreScope(ctx context.Context, storeID string) (StoreScope, error)
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
	customRefFactory     func() (string, error)
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
		customRefFactory:     newCustomRef,
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
		customRef, err := s.customRefFactory()
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

		if auditErr := s.insertCreateAudit(ctx, subject, updated, metadata); auditErr != nil {
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

	if err := s.insertCreateAudit(ctx, subject, transaction, metadata); err != nil {
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

func (s *service) insertCreateAudit(ctx context.Context, subject auth.Subject, transaction QRISTransaction, metadata auth.RequestMetadata) error {
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
		"paymentsqris.store_topup_create",
		"qris_transaction",
		&transaction.ID,
		payload,
		metadata.IPAddress,
		metadata.UserAgent,
		s.clock.Now().UTC(),
	)
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
