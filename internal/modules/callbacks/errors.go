package callbacks

import "errors"

var (
	ErrNotFound         = errors.New("callbacks: not found")
	ErrDuplicateAttempt = errors.New("callbacks: duplicate attempt")
	ErrInvalidReference = errors.New("callbacks: invalid reference")
)
