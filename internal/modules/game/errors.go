package game

import "errors"

var (
	ErrUnauthorized              = errors.New("game: unauthorized")
	ErrStoreInactive             = errors.New("game: store inactive")
	ErrInvalidUsername           = errors.New("game: invalid username")
	ErrDuplicateUsername         = errors.New("game: duplicate username")
	ErrDuplicateUpstreamUserCode = errors.New("game: duplicate upstream user code")
	ErrCodeGenerationExhausted   = errors.New("game: code generation exhausted")
	ErrMemberInactive            = errors.New("game: member inactive")
	ErrInvalidAmount             = errors.New("game: invalid amount")
	ErrInvalidTransactionID      = errors.New("game: invalid transaction id")
	ErrDuplicateTransactionID    = errors.New("game: duplicate transaction id")
	ErrInsufficientBalance       = errors.New("game: insufficient balance")
	ErrAgentSignExhausted        = errors.New("game: agent sign generation exhausted")
	ErrNotFound                  = errors.New("game: not found")
)
