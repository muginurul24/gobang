package storemembers

import "errors"

var (
	ErrNotFound                  = errors.New("storemembers: not found")
	ErrForbidden                 = errors.New("storemembers: forbidden")
	ErrInvalidRealUsername       = errors.New("storemembers: invalid real username")
	ErrDuplicateRealUsername     = errors.New("storemembers: duplicate real username")
	ErrDuplicateUpstreamUserCode = errors.New("storemembers: duplicate upstream user code")
	ErrCodeGenerationExhausted   = errors.New("storemembers: code generation exhausted")
)
