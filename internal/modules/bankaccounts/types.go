package bankaccounts

import "time"

type BankDirectoryEntry struct {
	BankCode      string `json:"bank_code"`
	BankName      string `json:"bank_name"`
	BankSwiftCode string `json:"bank_swift_code"`
}

type StoreScope struct {
	ID          string
	OwnerUserID string
	Name        string
	Slug        string
	DeletedAt   *time.Time
}

type BankAccount struct {
	ID                  string     `json:"id"`
	StoreID             string     `json:"store_id"`
	BankCode            string     `json:"bank_code"`
	BankName            string     `json:"bank_name"`
	AccountNumberMasked string     `json:"account_number_masked"`
	AccountName         string     `json:"account_name"`
	VerifiedAt          *time.Time `json:"verified_at"`
	IsActive            bool       `json:"is_active"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type InquiryRequest struct {
	BankCode      string
	AccountNumber string
}

type InquiryResult struct {
	BankCode      string
	BankName      string
	AccountNumber string
	AccountName   string
}

type SearchFilter struct {
	Query string
	Limit int
}

type CreateBankAccountInput struct {
	BankCode      string `json:"bank_code"`
	AccountNumber string `json:"account_number"`
}

type UpdateBankAccountStatusInput struct {
	IsActive bool `json:"is_active"`
}

type CreateBankAccountParams struct {
	StoreID                string
	BankCode               string
	BankName               string
	AccountNumberEncrypted string
	AccountNumberMasked    string
	AccountName            string
	VerifiedAt             time.Time
	IsActive               bool
	OccurredAt             time.Time
}

type UpdateBankAccountStatusParams struct {
	BankAccountID string
	IsActive      bool
	OccurredAt    time.Time
}
