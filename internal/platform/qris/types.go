package qris

import "time"

type Config struct {
	BaseURL              string
	Client               string
	ClientKey            string
	GlobalUUID           string
	DefaultExpireSeconds int
	Timeout              time.Duration
}

type GenerateInput struct {
	Username      string
	Amount        int64
	UUID          string
	ExpireSeconds int
	CustomRef     string
}

type GenerateResult struct {
	RawValue  string `json:"raw_value"`
	TrxID     string `json:"trx_id"`
	ExpiredAt *int   `json:"expired_at,omitempty"`
	IsVA      bool   `json:"is_va"`
}

type CheckStatusInput struct {
	TrxID string
	UUID  string
}

type PaymentStatusResult struct {
	Amount     int64      `json:"amount"`
	MerchantID string     `json:"merchant_id"`
	TrxID      string     `json:"trx_id"`
	RRN        string     `json:"rrn,omitempty"`
	Status     string     `json:"status"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
	FinishAt   *time.Time `json:"finish_at,omitempty"`
	TerminalID string     `json:"terminal_id,omitempty"`
	CustomRef  string     `json:"custom_ref,omitempty"`
	Vendor     string     `json:"vendor,omitempty"`
}

type InquiryTransferInput struct {
	UUID          string
	Amount        int64
	BankCode      string
	AccountNumber string
	TransferType  int
	Note          string
	ClientRefID   string
}

type InquiryTransferResult struct {
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"`
	BankCode      string `json:"bank_code"`
	BankName      string `json:"bank_name"`
	PartnerRefNo  string `json:"partner_ref_no"`
	VendorRefNo   string `json:"vendor_ref_no"`
	Amount        int64  `json:"amount"`
	Fee           int64  `json:"fee"`
	InquiryID     int64  `json:"inquiry_id"`
}

type TransferInput struct {
	UUID          string
	Amount        int64
	BankCode      string
	AccountNumber string
	TransferType  int
	InquiryID     int64
}

type TransferResult struct {
	Accepted bool `json:"accepted"`
}

type CheckDisbursementStatusInput struct {
	PartnerRefNo string
	UUID         string
}

type DisbursementStatusResult struct {
	Amount       int64  `json:"amount"`
	Fee          int64  `json:"fee"`
	PartnerRefNo string `json:"partner_ref_no"`
	MerchantUUID string `json:"merchant_uuid"`
	Status       string `json:"status"`
}

type PaymentWebhook struct {
	Amount     int64      `json:"amount"`
	TerminalID string     `json:"terminal_id"`
	TrxID      string     `json:"trx_id"`
	RRN        string     `json:"rrn"`
	CustomRef  string     `json:"custom_ref,omitempty"`
	Vendor     string     `json:"vendor,omitempty"`
	Status     string     `json:"status"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
	FinishAt   *time.Time `json:"finish_at,omitempty"`
}

type TransferWebhook struct {
	Amount          int64      `json:"amount"`
	PartnerRefNo    string     `json:"partner_ref_no"`
	Status          string     `json:"status"`
	TransactionDate *time.Time `json:"transaction_date,omitempty"`
	MerchantID      string     `json:"merchant_id,omitempty"`
}
