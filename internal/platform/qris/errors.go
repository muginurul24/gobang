package qris

import (
	"errors"
	"fmt"
)

var (
	ErrNotConfigured       = errors.New("qris: not configured")
	ErrInvalidRequest      = errors.New("qris: invalid request")
	ErrTimeout             = errors.New("qris: timeout")
	ErrUpstreamUnavailable = errors.New("qris: upstream unavailable")
	ErrUnexpectedHTTP      = errors.New("qris: unexpected http status")
	ErrInvalidResponse     = errors.New("qris: invalid response")
)

type BusinessError struct {
	Code    string
	Message string
}

func (e *BusinessError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == "" {
		return fmt.Sprintf("qris: business error: %s", e.Message)
	}

	return fmt.Sprintf("qris: business error %s: %s", e.Code, e.Message)
}
