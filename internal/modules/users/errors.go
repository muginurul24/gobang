package users

import "errors"

var (
	ErrForbidden               = errors.New("users: forbidden")
	ErrNotFound                = errors.New("users: not found")
	ErrInvalidInput            = errors.New("users: invalid input")
	ErrInvalidRole             = errors.New("users: invalid role")
	ErrRoleProvisionForbidden  = errors.New("users: role provision forbidden")
	ErrDuplicateIdentity       = errors.New("users: duplicate identity")
	ErrStatusUpdateForbidden   = errors.New("users: status update forbidden")
	ErrCannotDeactivateSelf    = errors.New("users: cannot deactivate self")
	ErrProtectedPlatformTarget = errors.New("users: protected platform target")
)
