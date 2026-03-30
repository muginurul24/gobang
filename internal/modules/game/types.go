package game

import (
	"encoding/json"
	"time"
)

type StoreStatus string

const (
	StoreStatusActive StoreStatus = "active"
)

type MemberStatus string

const (
	MemberStatusActive   MemberStatus = "active"
	MemberStatusInactive MemberStatus = "inactive"
)

type GameAction string

const (
	GameActionDeposit  GameAction = "deposit"
	GameActionWithdraw GameAction = "withdraw"
)

type TransactionStatus string

const (
	TransactionStatusPending TransactionStatus = "pending"
	TransactionStatusSuccess TransactionStatus = "success"
	TransactionStatusFailed  TransactionStatus = "failed"
)

type ReconcileStatus string

const (
	ReconcileStatusPending  ReconcileStatus = "pending_reconcile"
	ReconcileStatusResolved ReconcileStatus = "resolved"
)

type RequestMetadata struct {
	IPAddress string
	UserAgent string
}

type StoreScope struct {
	ID          string
	OwnerUserID string
	Name        string
	Slug        string
	Status      StoreStatus
	DeletedAt   *time.Time
}

type StoreMember struct {
	ID               string       `json:"id"`
	StoreID          string       `json:"store_id"`
	RealUsername     string       `json:"real_username"`
	UpstreamUserCode string       `json:"upstream_user_code"`
	Status           MemberStatus `json:"status"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
}

type CreateUserInput struct {
	Username string `json:"username"`
}

type CreateDepositInput struct {
	Username string      `json:"username"`
	Amount   json.Number `json:"amount"`
	TrxID    string      `json:"trx_id"`
}

type CreateStoreMemberParams struct {
	StoreID          string
	RealUsername     string
	UpstreamUserCode string
	Status           MemberStatus
	OccurredAt       time.Time
}

type GameTransaction struct {
	ID                string            `json:"id"`
	StoreID           string            `json:"store_id"`
	StoreMemberID     string            `json:"store_member_id"`
	Action            GameAction        `json:"action"`
	TrxID             string            `json:"trx_id"`
	UpstreamUserCode  string            `json:"upstream_user_code"`
	Amount            string            `json:"amount"`
	Status            TransactionStatus `json:"status"`
	ReconcileStatus   *ReconcileStatus  `json:"reconcile_status,omitempty"`
	UpstreamErrorCode *string           `json:"upstream_error_code,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	AgentSign         string            `json:"-"`
	UpstreamResponse  json.RawMessage   `json:"-"`
}

type CreateGameTransactionParams struct {
	StoreID          string
	StoreMemberID    string
	Action           GameAction
	TrxID            string
	UpstreamUserCode string
	Amount           string
	AgentSign        string
	Status           TransactionStatus
	OccurredAt       time.Time
}

type UpdateGameTransactionParams struct {
	GameTransactionID      string
	Status                 TransactionStatus
	ReconcileStatus        *ReconcileStatus
	UpstreamErrorCode      *string
	UpstreamResponseMasked map[string]any
	OccurredAt             time.Time
}

type BalanceSnapshot struct {
	StoreID          string `json:"store_id"`
	LedgerAccountID  string `json:"ledger_account_id"`
	Currency         string `json:"currency"`
	CurrentBalance   string `json:"current_balance"`
	ReservedAmount   string `json:"reserved_amount"`
	AvailableBalance string `json:"available_balance"`
}

type DepositResult struct {
	Transaction GameTransaction  `json:"transaction"`
	Balance     *BalanceSnapshot `json:"balance,omitempty"`
}
