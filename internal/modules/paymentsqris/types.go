package paymentsqris

import (
	"encoding/json"
	"time"
)

type StoreStatus string

const (
	StoreStatusActive   StoreStatus = "active"
	StoreStatusInactive StoreStatus = "inactive"
	StoreStatusBanned   StoreStatus = "banned"
	StoreStatusDeleted  StoreStatus = "deleted"
)

type MemberStatus string

const (
	MemberStatusActive   MemberStatus = "active"
	MemberStatusInactive MemberStatus = "inactive"
)

type TransactionType string

const (
	TransactionTypeStoreTopup    TransactionType = "store_topup"
	TransactionTypeMemberPayment TransactionType = "member_payment"
)

type TransactionStatus string

const (
	TransactionStatusPending TransactionStatus = "pending"
	TransactionStatusSuccess TransactionStatus = "success"
	TransactionStatusExpired TransactionStatus = "expired"
	TransactionStatusFailed  TransactionStatus = "failed"
)

type ProviderState string

const (
	ProviderStatePendingGenerate       ProviderState = "pending_generate"
	ProviderStateGenerated             ProviderState = "generated"
	ProviderStatePendingProviderAnswer ProviderState = "pending_provider_response"
	ProviderStateGenerateFailed        ProviderState = "generate_failed"
	ProviderStateWebhookSuccess        ProviderState = "webhook_success"
	ProviderStateWebhookFailed         ProviderState = "webhook_failed"
	ProviderStateWebhookExpired        ProviderState = "webhook_expired"
)

type WebhookKind string

const (
	WebhookKindPayment          WebhookKind = "payment"
	WebhookKindWithdrawalStatus WebhookKind = "withdrawal_status"
)

type StoreScope struct {
	ID            string
	OwnerUserID   string
	OwnerUsername string
	Name          string
	Slug          string
	Status        StoreStatus
	DeletedAt     *time.Time
}

type StoreMember struct {
	ID               string
	StoreID          string
	RealUsername     string
	UpstreamUserCode string
	Status           MemberStatus
}

type QRISTransaction struct {
	ID                string            `json:"id"`
	StoreID           string            `json:"store_id"`
	StoreMemberID     *string           `json:"store_member_id"`
	Type              TransactionType   `json:"type"`
	ProviderTrxID     *string           `json:"provider_trx_id"`
	CustomRef         string            `json:"custom_ref"`
	ExternalUsername  string            `json:"external_username"`
	AmountGross       string            `json:"amount_gross"`
	PlatformFeeAmount string            `json:"platform_fee_amount"`
	StoreCreditAmount string            `json:"store_credit_amount"`
	Status            TransactionStatus `json:"status"`
	ExpiresAt         *time.Time        `json:"expires_at"`
	ProviderState     *ProviderState    `json:"provider_state,omitempty"`
	QRCodeValue       *string           `json:"qr_code_value,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

type CreateStoreTopupInput struct {
	Amount json.Number `json:"amount"`
}

type CreateMemberPaymentInput struct {
	Username string      `json:"username"`
	Amount   json.Number `json:"amount"`
}

type ListTransactionsFilter struct {
	StoreID     string
	Type        TransactionType
	Status      *TransactionStatus
	Query       string
	Limit       int
	Offset      int
	CreatedFrom *time.Time
	CreatedTo   *time.Time
}

type QRISTransactionSummary struct {
	TotalCount   int    `json:"total_count"`
	PendingCount int    `json:"pending_count"`
	SuccessCount int    `json:"success_count"`
	ExpiredCount int    `json:"expired_count"`
	FailedCount  int    `json:"failed_count"`
	TotalGross   string `json:"total_gross"`
	PendingGross string `json:"pending_gross"`
}

type QRISTransactionPage struct {
	Items   []QRISTransaction      `json:"items"`
	Summary QRISTransactionSummary `json:"summary"`
	Limit   int                    `json:"limit"`
	Offset  int                    `json:"offset"`
}

type CreateQRISTransactionParams struct {
	StoreID           string
	StoreMemberID     *string
	Type              TransactionType
	CustomRef         string
	ExternalUsername  string
	AmountGross       string
	PlatformFeeAmount string
	StoreCreditAmount string
	Status            TransactionStatus
	ExpiresAt         *time.Time
	ProviderPayload   map[string]any
	OccurredAt        time.Time
}

type UpdateGeneratedTransactionParams struct {
	TransactionID   string
	ProviderTrxID   string
	ExpiresAt       *time.Time
	ProviderPayload map[string]any
	OccurredAt      time.Time
}

type UpdateTransactionStatusParams struct {
	TransactionID   string
	Status          TransactionStatus
	ExpiresAt       *time.Time
	ProviderPayload map[string]any
	OccurredAt      time.Time
}

type FinalizeQRISTransactionParams struct {
	TransactionID     string
	ProviderTrxID     string
	Status            TransactionStatus
	PlatformFeeAmount string
	StoreCreditAmount string
	ProviderPayload   map[string]any
	OccurredAt        time.Time
}

type WebhookDispatchResult struct {
	Kind            WebhookKind        `json:"kind"`
	Processed       bool               `json:"processed"`
	Reference       string             `json:"reference"`
	TransactionID   *string            `json:"transaction_id,omitempty"`
	TransactionType *TransactionType   `json:"transaction_type,omitempty"`
	Status          *TransactionStatus `json:"status,omitempty"`
}

type ReconcileCandidate struct {
	Transaction   QRISTransaction
	AttemptNo     int
	LastAttemptAt *time.Time
}

type RecordReconcileAttemptParams struct {
	QRISTransactionID string
	AttemptNo         int
	Status            string
	ResponseMasked    map[string]any
	OccurredAt        time.Time
}

type ReconcileOutcome string

const (
	ReconcileOutcomeFinalizedSuccess ReconcileOutcome = "finalized_success"
	ReconcileOutcomeFinalizedExpired ReconcileOutcome = "finalized_expired"
	ReconcileOutcomeFinalizedFailed  ReconcileOutcome = "finalized_failed"
	ReconcileOutcomeStillPending     ReconcileOutcome = "still_pending"
	ReconcileOutcomeSkipped          ReconcileOutcome = "skipped"
)

type ReconcileRunSummary struct {
	Scanned          int `json:"scanned"`
	FinalizedSuccess int `json:"finalized_success"`
	FinalizedExpired int `json:"finalized_expired"`
	FinalizedFailed  int `json:"finalized_failed"`
	StillPending     int `json:"still_pending"`
	Skipped          int `json:"skipped"`
}
