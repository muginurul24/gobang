package chat

import "errors"

var (
	ErrForbidden   = errors.New("chat: forbidden")
	ErrNotFound    = errors.New("chat: not found")
	ErrInvalidBody = errors.New("chat: invalid body")
)
