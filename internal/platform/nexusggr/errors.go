package nexusggr

import (
	"errors"
	"fmt"
)

var (
	ErrNotConfigured       = errors.New("nexusggr: not configured")
	ErrInvalidRequest      = errors.New("nexusggr: invalid request")
	ErrUnexpectedHTTP      = errors.New("nexusggr: unexpected http status")
	ErrInvalidResponse     = errors.New("nexusggr: invalid response")
	ErrTimeout             = errors.New("nexusggr: timeout")
	ErrUpstreamUnavailable = errors.New("nexusggr: upstream unavailable")
)

type BusinessError struct {
	Method  string
	Code    string
	Message string
}

func (e *BusinessError) Error() string {
	if e == nil {
		return ""
	}

	if e.Code == "" {
		return fmt.Sprintf("nexusggr: %s business failure", e.Method)
	}

	return fmt.Sprintf("nexusggr: %s %s", e.Method, e.Code)
}
