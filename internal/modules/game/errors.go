package game

import "errors"

var (
	ErrUnauthorized              = errors.New("game: unauthorized")
	ErrStoreInactive             = errors.New("game: store inactive")
	ErrInvalidUsername           = errors.New("game: invalid username")
	ErrDuplicateUsername         = errors.New("game: duplicate username")
	ErrDuplicateUpstreamUserCode = errors.New("game: duplicate upstream user code")
	ErrCodeGenerationExhausted   = errors.New("game: code generation exhausted")
	ErrNotFound                  = errors.New("game: not found")
)
