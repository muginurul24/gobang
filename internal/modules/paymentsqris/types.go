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
