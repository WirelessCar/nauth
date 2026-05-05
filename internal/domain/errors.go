package domain

import "errors"

type ErrCode string

const (
	CodeUnknownError      ErrCode = "UnknownError"
	CodeBadRequest        ErrCode = "BadRequest"
	CodeConfigMapNotFound ErrCode = "ConfigMapNotFound"
	CodeAccountNotFound   ErrCode = "AccountNotFound"
	CodeAccountNotReady   ErrCode = "AccountNotReady"
)

func ErrBadRequest(cause error) NAuthError {
	return Err(CodeBadRequest, cause)
}

func ErrConfigMapNotFound() NAuthError {
	return Err(CodeConfigMapNotFound, nil)
}

func ErrAccountNotFound() NAuthError {
	return Err(CodeAccountNotFound, nil)
}

func ErrAccountNotReady() NAuthError {
	return Err(CodeAccountNotReady, nil)
}

func ErrUnknownError(cause error) NAuthError {
	return Err(CodeUnknownError, cause)
}

func Err(code ErrCode, cause error) NAuthError {
	return &nauthError{
		code:  code,
		cause: cause,
	}
}

func CodeOf(err error) ErrCode {
	if err == nil {
		return ""
	}

	if domainErr, ok := errors.AsType[NAuthError](err); ok {
		return domainErr.Code()
	}
	return CodeUnknownError
}

func HasCode(err error, code ErrCode) bool {
	return CodeOf(err) == code
}

type NAuthError interface {
	error
	Code() ErrCode
	Unwrap() error
}

type nauthError struct {
	code  ErrCode
	cause error
}

func (e *nauthError) Error() string {
	code := e.Code()
	if code == "" {
		code = CodeUnknownError
	}

	result := string(code)
	if e.cause != nil {
		result += ": " + e.cause.Error()
	}
	return result
}

func (e *nauthError) Code() ErrCode {
	if e.code == "" {
		return CodeUnknownError
	}
	return e.code
}

func (e *nauthError) Unwrap() error {
	return e.cause
}

func (e *nauthError) Is(target error) bool {
	if target == nil {
		return false
	}

	if targetErr, ok := errors.AsType[NAuthError](target); ok {
		return e.Code() == targetErr.Code()
	}
	return false
}

var _ error = &nauthError{}
var _ NAuthError = &nauthError{}
