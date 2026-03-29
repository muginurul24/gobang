package auth

import "errors"

var (
	ErrNotFound            = errors.New("auth: not found")
	ErrInvalidCredentials  = errors.New("auth: invalid credentials")
	ErrUnauthorized        = errors.New("auth: unauthorized")
	ErrInvalidRefreshToken = errors.New("auth: invalid refresh token")
)
