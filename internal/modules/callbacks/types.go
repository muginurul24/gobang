package callbacks

import (
	"encoding/json"
	"time"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusSuccess  Status = "success"
	StatusFailed   Status = "failed"
	StatusRetrying Status = "retrying"
)

type AttemptStatus string

const (
	AttemptStatusSuccess AttemptStatus = "success"
	AttemptStatusFailed  AttemptStatus = "failed"
)

type MemberPaymentCallbackSource struct {
	QRISTransactionID   string
	StoreID             string
	StoreMemberID       *string
	RealUsername        string
	CustomRef           string
	ProviderTrxID       *string
	AmountGross         string
	PlatformFeeAmount   string
	StoreCreditAmount   string
	TransactionStatus   string
	TransactionUpdateAt time.Time
}

type OutboundCallback struct {
	ID            string          `json:"id"`
	StoreID       string          `json:"store_id"`
	EventType     string          `json:"event_type"`
	ReferenceType string          `json:"reference_type"`
	ReferenceID   string          `json:"reference_id"`
	PayloadJSON   json.RawMessage `json:"payload_json"`
	Signature     string          `json:"signature"`
	Status        Status          `json:"status"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type DueOutboundCallback struct {
	OutboundCallback
	CallbackURL string `json:"callback_url"`
	AttemptNo   int    `json:"attempt_no"`
}

type ListQueueFilter struct {
	Query       string
	Status      *Status
	StoreID     *string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	Limit       int
	Offset      int
}

type QueueItem struct {
	ID                       string         `json:"id"`
	StoreID                  string         `json:"store_id"`
	StoreName                string         `json:"store_name"`
	StoreSlug                string         `json:"store_slug"`
	CallbackURL              string         `json:"callback_url"`
	EventType                string         `json:"event_type"`
	ReferenceType            string         `json:"reference_type"`
	ReferenceID              string         `json:"reference_id"`
	Status                   Status         `json:"status"`
	CreatedAt                time.Time      `json:"created_at"`
	UpdatedAt                time.Time      `json:"updated_at"`
	LatestAttemptNo          int            `json:"latest_attempt_no"`
	LatestHTTPStatus         *int           `json:"latest_http_status,omitempty"`
	LatestAttemptStatus      *AttemptStatus `json:"latest_attempt_status,omitempty"`
	LatestResponseBodyMasked string         `json:"latest_response_body_masked,omitempty"`
	LatestNextRetryAt        *time.Time     `json:"latest_next_retry_at,omitempty"`
	LatestAttemptAt          *time.Time     `json:"latest_attempt_at,omitempty"`
}

type QueueSummary struct {
	TotalCount    int `json:"total_count"`
	PendingCount  int `json:"pending_count"`
	RetryingCount int `json:"retrying_count"`
	SuccessCount  int `json:"success_count"`
	FailedCount   int `json:"failed_count"`
}

type QueuePage struct {
	Items   []QueueItem  `json:"items"`
	Summary QueueSummary `json:"summary"`
	Limit   int          `json:"limit"`
	Offset  int          `json:"offset"`
}

type AttemptRecord struct {
	ID                 string        `json:"id"`
	OutboundCallbackID string        `json:"outbound_callback_id"`
	AttemptNo          int           `json:"attempt_no"`
	HTTPStatus         *int          `json:"http_status,omitempty"`
	Status             AttemptStatus `json:"status"`
	ResponseBodyMasked string        `json:"response_body_masked"`
	NextRetryAt        *time.Time    `json:"next_retry_at,omitempty"`
	CreatedAt          time.Time     `json:"created_at"`
}

type AttemptPage struct {
	CallbackID string          `json:"callback_id"`
	Items      []AttemptRecord `json:"items"`
	TotalCount int             `json:"total_count"`
	Limit      int             `json:"limit"`
	Offset     int             `json:"offset"`
}

type MemberPaymentSuccessPayload struct {
	EventType     string                          `json:"event_type"`
	OccurredAt    time.Time                       `json:"occurred_at"`
	ReferenceType string                          `json:"reference_type"`
	ReferenceID   string                          `json:"reference_id"`
	Data          MemberPaymentSuccessPayloadData `json:"data"`
}

type MemberPaymentSuccessPayloadData struct {
	QRISTransactionID   string    `json:"qris_transaction_id"`
	StoreID             string    `json:"store_id"`
	StoreMemberID       *string   `json:"store_member_id,omitempty"`
	RealUsername        string    `json:"real_username"`
	Status              string    `json:"status"`
	CustomRef           string    `json:"custom_ref"`
	ProviderTrxID       *string   `json:"provider_trx_id,omitempty"`
	AmountGross         string    `json:"amount_gross"`
	PlatformFeeAmount   string    `json:"platform_fee_amount"`
	StoreCreditAmount   string    `json:"store_credit_amount"`
	TransactionUpdateAt time.Time `json:"paid_at"`
}

type EnqueueOutboundCallbackParams struct {
	StoreID       string
	EventType     string
	ReferenceType string
	ReferenceID   string
	PayloadJSON   json.RawMessage
	Signature     string
	OccurredAt    time.Time
}

type RecordAttemptParams struct {
	OutboundCallbackID string
	AttemptNo          int
	HTTPStatus         *int
	Status             AttemptStatus
	ResponseBodyMasked string
	NextRetryAt        *time.Time
	CallbackStatus     Status
	OccurredAt         time.Time
}

type DispatchResult struct {
	HTTPStatus         *int
	ResponseBodyMasked string
	Success            bool
}

type RunSummary struct {
	Scanned   int `json:"scanned"`
	Delivered int `json:"delivered"`
	Retrying  int `json:"retrying"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}
