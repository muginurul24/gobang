package paymentsqris

import "errors"

var (
	ErrForbidden          = errors.New("paymentsqris: forbidden")
	ErrNotFound           = errors.New("paymentsqris: not found")
	ErrStoreInactive      = errors.New("paymentsqris: store inactive")
	ErrInvalidAmount      = errors.New("paymentsqris: invalid amount")
	ErrDuplicateCustomRef = errors.New("paymentsqris: duplicate custom ref")
)
