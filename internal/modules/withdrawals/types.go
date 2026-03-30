package withdrawals

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

type WithdrawalStatus string

const (
	WithdrawalStatusPending WithdrawalStatus = "pending"
	WithdrawalStatusSuccess WithdrawalStatus = "success"
	WithdrawalStatusFailed  WithdrawalStatus = "failed"
)

type StoreScope struct {
	ID          string
	OwnerUserID string
	Name        string
	Slug        string
	Status      StoreStatus
	DeletedAt   *time.Time
}

type StoreBankAccount struct {
	ID                     string
	StoreID                string
	BankCode               string
	BankName               string
	AccountName            string
	AccountNumberMasked    string
	AccountNumberEncrypted string
	IsActive               bool
}

type StoreWithdrawal struct {
	ID                   string           `json:"id"`
	StoreID              string           `json:"store_id"`
	StoreBankAccountID   string           `json:"store_bank_account_id"`
	IdempotencyKey       string           `json:"idempotency_key"`
	BankCode             string           `json:"bank_code"`
	BankName             string           `json:"bank_name"`
	AccountName          string           `json:"account_name"`
	AccountNumberMasked  string           `json:"account_number_masked"`
	NetRequestedAmount   string           `json:"net_requested_amount"`
	PlatformFeeAmount    string           `json:"platform_fee_amount"`
	ExternalFeeAmount    string           `json:"external_fee_amount"`
	TotalStoreDebit      string           `json:"total_store_debit"`
	ProviderPartnerRefNo *string          `json:"provider_partner_ref_no,omitempty"`
	ProviderInquiryID    *string          `json:"provider_inquiry_id,omitempty"`
	Status               WithdrawalStatus `json:"status"`
	CreatedAt            time.Time        `json:"created_at"`
	UpdatedAt            time.Time        `json:"updated_at"`
}

type ListFilter struct {
	StoreID string
}

type CreateWithdrawInput struct {
	BankAccountID  string      `json:"bank_account_id"`
	Amount         json.Number `json:"amount"`
	IdempotencyKey string      `json:"idempotency_key"`
}

type CreateStoreWithdrawalParams struct {
	StoreID              string
	StoreBankAccountID   string
	IdempotencyKey       string
	NetRequestedAmount   string
	PlatformFeeAmount    string
	ExternalFeeAmount    string
	TotalStoreDebit      string
	ProviderPartnerRefNo *string
	ProviderInquiryID    *string
	Status               WithdrawalStatus
	RequestPayload       map[string]any
	ProviderPayload      map[string]any
	OccurredAt           time.Time
}

type UpdateStoreWithdrawalParams struct {
	WithdrawalID         string
	PlatformFeeAmount    *string
	ExternalFeeAmount    *string
	TotalStoreDebit      *string
	ProviderPartnerRefNo *string
	ProviderInquiryID    *string
	Status               *WithdrawalStatus
	ProviderPayload      map[string]any
	OccurredAt           time.Time
}

type TransferWebhookResult struct {
	Processed    bool              `json:"processed"`
	Reference    string            `json:"reference"`
	WithdrawalID *string           `json:"withdrawal_id,omitempty"`
	Status       *WithdrawalStatus `json:"status,omitempty"`
}

type StatusCheckCandidate struct {
	Withdrawal    StoreWithdrawal
	AttemptNo     int
	LastAttemptAt *time.Time
}

type RecordStatusCheckParams struct {
	WithdrawalID   string
	AttemptNo      int
	Status         string
	ResponseMasked map[string]any
	OccurredAt     time.Time
}

type StatusCheckOutcome string

const (
	StatusCheckOutcomeFinalizedSuccess StatusCheckOutcome = "finalized_success"
	StatusCheckOutcomeFinalizedFailed  StatusCheckOutcome = "finalized_failed"
	StatusCheckOutcomeStillPending     StatusCheckOutcome = "still_pending"
	StatusCheckOutcomeSkipped          StatusCheckOutcome = "skipped"
)

type StatusCheckRunSummary struct {
	Scanned          int `json:"scanned"`
	FinalizedSuccess int `json:"finalized_success"`
	FinalizedFailed  int `json:"finalized_failed"`
	StillPending     int `json:"still_pending"`
	Skipped          int `json:"skipped"`
}

type ProviderInquiryInput struct {
	Amount         int64
	BankCode       string
	AccountNumber  string
	IdempotencyKey string
}

type ProviderInquiryResult struct {
	AccountName  string
	BankCode     string
	BankName     string
	PartnerRefNo string
	InquiryID    string
	ExternalFee  int64
}

type ProviderTransferInput struct {
	Amount        int64
	BankCode      string
	AccountNumber string
	InquiryID     string
}

type ProviderTransferResult struct {
	Accepted bool
}

type ProviderStatusCheckInput struct {
	PartnerRefNo string
}

type ProviderStatusCheckResult struct {
	Amount       int64
	ExternalFee  int64
	PartnerRefNo string
	MerchantID   string
	Status       string
}
