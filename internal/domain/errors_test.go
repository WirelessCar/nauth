package domain

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Error(t *testing.T) {
	// When
	err := ErrBadRequest

	// Then
	require.Error(t, err)
	require.ErrorIs(t, err, ErrBadRequest)
	require.EqualError(t, err, "BadRequest")
	require.Nil(t, errors.Unwrap(err))

	var domainErr Error
	require.ErrorAs(t, err, &domainErr)
	require.Equal(t, ErrBadRequest, domainErr)
}

func Test_Error_WithCause(t *testing.T) {
	// Given
	cause := errors.New("the cause")

	// When
	err := ErrBadRequest.WithCause(cause)

	// Then
	require.Error(t, err)
	require.ErrorIs(t, err, ErrBadRequest)
	require.ErrorIs(t, err, cause)
	require.EqualError(t, err, "BadRequest: the cause")

	var domainErr Error
	require.ErrorAs(t, err, &domainErr)
	require.Equal(t, ErrBadRequest, domainErr)
}
