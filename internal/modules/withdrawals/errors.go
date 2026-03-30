package withdrawals

import "errors"

var (
	ErrForbidden                = errors.New("withdrawals: forbidden")
	ErrNotFound                 = errors.New("withdrawals: not found")
	ErrStoreInactive            = errors.New("withdrawals: store inactive")
	ErrInvalidAmount            = errors.New("withdrawals: invalid amount")
	ErrInvalidIdempotencyKey    = errors.New("withdrawals: invalid idempotency key")
	ErrIdempotencyKeyConflict   = errors.New("withdrawals: idempotency key conflict")
	ErrBankAccountInactive      = errors.New("withdrawals: bank account inactive")
	ErrInsufficientStoreBalance = errors.New("withdrawals: insufficient store balance")
	ErrInquiryFailed            = errors.New("withdrawals: inquiry failed")
	ErrInquiryUnavailable       = errors.New("withdrawals: inquiry unavailable")
	ErrTransferFailed           = errors.New("withdrawals: transfer failed")
	ErrTransferUnavailable      = errors.New("withdrawals: transfer unavailable")
)

type CreateFailure struct {
	Withdrawal StoreWithdrawal
	Cause      error
}

func (e *CreateFailure) Error() string {
	if e == nil || e.Cause == nil {
		return ""
	}

	return e.Cause.Error()
}

func (e *CreateFailure) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.Cause
}
