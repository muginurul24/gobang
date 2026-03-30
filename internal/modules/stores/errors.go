package stores

import "errors"

var (
	ErrNotFound              = errors.New("stores: not found")
	ErrForbidden             = errors.New("stores: forbidden")
	ErrInvalidStoreName      = errors.New("stores: invalid store name")
	ErrInvalidSlug           = errors.New("stores: invalid slug")
	ErrDuplicateSlug         = errors.New("stores: duplicate slug")
	ErrInvalidThreshold      = errors.New("stores: invalid threshold")
	ErrInvalidStatus         = errors.New("stores: invalid status")
	ErrInvalidCallbackURL    = errors.New("stores: invalid callback url")
	ErrInvalidEmployeeInput  = errors.New("stores: invalid employee input")
	ErrEmployeeNotFound      = errors.New("stores: employee not found")
	ErrEmployeeScopeMismatch = errors.New("stores: employee scope mismatch")
	ErrDuplicateStaff        = errors.New("stores: duplicate staff relation")
	ErrDuplicateIdentity     = errors.New("stores: duplicate identity")
)
