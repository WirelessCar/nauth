package domain

import (
	"errors"
	"fmt"
)

type Error string

const (
	ErrUnknownError      Error = "UnknownError"
	ErrBadRequest        Error = "BadRequest"
	ErrAccountNotFound   Error = "AccountNotFound"
	ErrAccountNotReady   Error = "AccountNotReady"
	ErrConfigMapNotFound Error = "ConfigMapNotFound"
)

func (e Error) Error() string {
	return string(e)
}

func (e Error) Is(target error) bool {
	var targetErr Error
	ok := errors.As(target, &targetErr)
	return ok && e == targetErr
}

func (e Error) WithCause(cause error) error {
	return wrapError(e, cause)
}

func wrapError(err Error, cause error) error {
	if cause == nil {
		return err
	}
	return fmt.Errorf("%w: %w", err, cause)
}
