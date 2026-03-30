package auth

import "errors"

var (
	ErrNotFound             = errors.New("auth: not found")
	ErrInvalidCredentials   = errors.New("auth: invalid credentials")
	ErrUnauthorized         = errors.New("auth: unauthorized")
	ErrInvalidRefreshToken  = errors.New("auth: invalid refresh token")
	ErrSecondFactorRequired = errors.New("auth: second factor required")
	ErrInvalidSecondFactor  = errors.New("auth: invalid second factor")
	ErrIPNotAllowed         = errors.New("auth: ip not allowed")
	ErrTOTPAlreadyEnabled   = errors.New("auth: totp already enabled")
	ErrTOTPNotEnabled       = errors.New("auth: totp not enabled")
	ErrNoPendingEnrollment  = errors.New("auth: no pending enrollment")
	ErrInvalidIPAllowlist   = errors.New("auth: invalid ip allowlist")
)

type RateLimitError struct {
	Scope      string
	RetryAfter int
}

func (e *RateLimitError) Error() string {
	return "auth: rate limited"
}
