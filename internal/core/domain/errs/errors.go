package errs

import "errors"

var (
	ErrNoAccountFound  = errors.New("no account found")
	ErrAccountNotReady = errors.New("account is not ready")
	ErrNotFound        = errors.New("not found")
)
