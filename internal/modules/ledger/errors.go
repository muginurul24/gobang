package ledger

import "errors"

var (
	ErrNotFound                 = errors.New("ledger: not found")
	ErrInvalidAmount            = errors.New("ledger: invalid amount")
	ErrInvalidReference         = errors.New("ledger: invalid reference")
	ErrInvalidEntryType         = errors.New("ledger: invalid entry type")
	ErrInvalidReservationCommit = errors.New("ledger: invalid reservation commit")
	ErrInsufficientFunds        = errors.New("ledger: insufficient funds")
	ErrDuplicateReference       = errors.New("ledger: duplicate reference")
	ErrReservationFinalized     = errors.New("ledger: reservation finalized")
)
