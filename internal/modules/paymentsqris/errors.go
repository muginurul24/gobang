package paymentsqris

import "errors"

var (
	ErrUnauthorized       = errors.New("paymentsqris: unauthorized")
	ErrForbidden          = errors.New("paymentsqris: forbidden")
	ErrNotFound           = errors.New("paymentsqris: not found")
	ErrStoreInactive      = errors.New("paymentsqris: store inactive")
	ErrInvalidAmount      = errors.New("paymentsqris: invalid amount")
	ErrInvalidUsername    = errors.New("paymentsqris: invalid username")
	ErrMemberInactive     = errors.New("paymentsqris: member inactive")
	ErrDuplicateCustomRef = errors.New("paymentsqris: duplicate custom ref")
	ErrDuplicateProvider  = errors.New("paymentsqris: duplicate provider transaction")
)
