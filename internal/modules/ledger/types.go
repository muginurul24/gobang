package ledger

import (
	"encoding/json"
	"time"
)

type Direction string

const (
	DirectionDebit  Direction = "debit"
	DirectionCredit Direction = "credit"
)

type EntryType string

const (
	EntryTypeGameDeposit         EntryType = "game_deposit"
	EntryTypeGameWithdraw        EntryType = "game_withdraw"
	EntryTypeStoreTopup          EntryType = "store_topup"
	EntryTypeMemberPaymentCredit EntryType = "member_payment_credit"
	EntryTypeMemberPaymentFee    EntryType = "member_payment_fee"
	EntryTypeWithdrawReserve     EntryType = "withdraw_reserve"
	EntryTypeWithdrawCommit      EntryType = "withdraw_commit"
	EntryTypeWithdrawRelease     EntryType = "withdraw_release"
	EntryTypeWithdrawPlatformFee EntryType = "withdraw_platform_fee"
	EntryTypeWithdrawExternalFee EntryType = "withdraw_external_fee"
)

type ReservationStatus string

const (
	ReservationStatusPending   ReservationStatus = "pending"
	ReservationStatusCommitted ReservationStatus = "committed"
	ReservationStatusReleased  ReservationStatus = "released"
)

type BalanceSnapshot struct {
	StoreID          string `json:"store_id"`
	LedgerAccountID  string `json:"ledger_account_id"`
	Currency         string `json:"currency"`
	CurrentBalance   string `json:"current_balance"`
	ReservedAmount   string `json:"reserved_amount"`
	AvailableBalance string `json:"available_balance"`
}

type LedgerEntry struct {
	ID              string          `json:"id"`
	StoreID         string          `json:"store_id"`
	LedgerAccountID string          `json:"ledger_account_id"`
	Direction       Direction       `json:"direction"`
	EntryType       EntryType       `json:"entry_type"`
	Amount          string          `json:"amount"`
	BalanceAfter    string          `json:"balance_after"`
	ReferenceType   string          `json:"reference_type"`
	ReferenceID     string          `json:"reference_id"`
	Metadata        json.RawMessage `json:"metadata_json"`
	CreatedAt       time.Time       `json:"created_at"`
}

type LedgerReservation struct {
	ID            string            `json:"id"`
	StoreID       string            `json:"store_id"`
	ReferenceType string            `json:"reference_type"`
	ReferenceID   string            `json:"reference_id"`
	Amount        string            `json:"amount"`
	Status        ReservationStatus `json:"status"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

type PostEntryInput struct {
	EntryType     EntryType      `json:"entry_type"`
	Amount        string         `json:"amount"`
	ReferenceType string         `json:"reference_type"`
	ReferenceID   string         `json:"reference_id"`
	Metadata      map[string]any `json:"metadata"`
}

type ReserveInput struct {
	Amount        string `json:"amount"`
	ReferenceType string `json:"reference_type"`
	ReferenceID   string `json:"reference_id"`
}

type ReservationCommitEntryInput struct {
	EntryType EntryType      `json:"entry_type"`
	Amount    string         `json:"amount"`
	Metadata  map[string]any `json:"metadata"`
}

type CommitReservationInput struct {
	ReferenceType string                        `json:"reference_type"`
	ReferenceID   string                        `json:"reference_id"`
	Entries       []ReservationCommitEntryInput `json:"entries"`
}

type ReleaseReservationInput struct {
	ReferenceType string `json:"reference_type"`
	ReferenceID   string `json:"reference_id"`
}

type PostingResult struct {
	Entry   LedgerEntry     `json:"entry"`
	Balance BalanceSnapshot `json:"balance"`
}

type BatchPostEntryInput struct {
	Direction Direction      `json:"direction"`
	EntryType EntryType      `json:"entry_type"`
	Amount    string         `json:"amount"`
	Metadata  map[string]any `json:"metadata"`
}

type PostEntriesInput struct {
	ReferenceType string                `json:"reference_type"`
	ReferenceID   string                `json:"reference_id"`
	Entries       []BatchPostEntryInput `json:"entries"`
}

type ReservationResult struct {
	Reservation LedgerReservation `json:"reservation"`
	Balance     BalanceSnapshot   `json:"balance"`
}

type BatchPostingResult struct {
	Entries []LedgerEntry   `json:"entries"`
	Balance BalanceSnapshot `json:"balance"`
}

type CommitReservationResult struct {
	Reservation LedgerReservation `json:"reservation"`
	Entries     []LedgerEntry     `json:"entries"`
	Balance     BalanceSnapshot   `json:"balance"`
}

type postEntryParams struct {
	StoreID       string
	Direction     Direction
	EntryType     EntryType
	Amount        string
	ReferenceType string
	ReferenceID   string
	Metadata      map[string]any
}

type batchPostEntryParams struct {
	Direction Direction
	EntryType EntryType
	Amount    string
	Metadata  map[string]any
}

type postEntriesParams struct {
	StoreID       string
	ReferenceType string
	ReferenceID   string
	Entries       []batchPostEntryParams
}

type reserveParams struct {
	StoreID       string
	Amount        string
	ReferenceType string
	ReferenceID   string
}

type commitEntryParams struct {
	EntryType EntryType
	Amount    string
	Metadata  map[string]any
}

type commitReservationParams struct {
	StoreID       string
	ReferenceType string
	ReferenceID   string
	Entries       []commitEntryParams
}

type releaseReservationParams struct {
	StoreID       string
	ReferenceType string
	ReferenceID   string
}
