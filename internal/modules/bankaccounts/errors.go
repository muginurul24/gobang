package bankaccounts

import "errors"

var (
	ErrForbidden            = errors.New("bankaccounts: forbidden")
	ErrNotFound             = errors.New("bankaccounts: not found")
	ErrInvalidBankCode      = errors.New("bankaccounts: invalid bank code")
	ErrInvalidAccountNumber = errors.New("bankaccounts: invalid account number")
	ErrInquiryFailed        = errors.New("bankaccounts: inquiry failed")
	ErrInquiryUnavailable   = errors.New("bankaccounts: inquiry unavailable")
)
