package errs

import "errors"

var (
	ErrNoOperatorFound = errors.New("no operator found")
	ErrNoAccountFound  = errors.New("no account found")
	ErrAccountNotReady = errors.New("account is not ready")
	ErrNotFound        = errors.New("not found")
	ErrUpdateFailed    = errors.New("failed to update resource")
)
