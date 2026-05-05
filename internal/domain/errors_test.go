package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestCodeOf(t *testing.T) {
	t.Run("returns empty reason for nil", func(t *testing.T) {
		if got := CodeOf(nil); got != "" {
			t.Fatalf("expected empty reason, got %q", got)
		}
	})

	t.Run("returns domain error reason", func(t *testing.T) {
		err := ErrAccountNotFound()

		if got := CodeOf(err); got != CodeAccountNotFound {
			t.Fatalf("expected %q, got %q", CodeAccountNotFound, got)
		}
	})

	t.Run("unwraps wrapped domain errors", func(t *testing.T) {
		err := fmt.Errorf("lookup account: %w", ErrAccountNotReady())

		if got := CodeOf(err); got != CodeAccountNotReady {
			t.Fatalf("expected %q, got %q", CodeAccountNotReady, got)
		}
	})

	t.Run("classifies non-domain errors as unhandled", func(t *testing.T) {
		err := errors.New("plain failure")

		if got := CodeOf(err); got != CodeUnknownError {
			t.Fatalf("expected %q, got %q", CodeUnknownError, got)
		}
	})
}

func TestNAuthError(t *testing.T) {
	cause := errors.New("invalid account reference")
	err := ErrBadRequest(cause)

	if got := err.Code(); got != CodeBadRequest {
		t.Fatalf("expected %q, got %q", CodeBadRequest, got)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("expected errors.Is to match wrapped cause")
	}
	if !errors.Is(err, ErrBadRequest(nil)) {
		t.Fatalf("expected errors.Is to match domain error code")
	}

	var domainErr NAuthError
	if !errors.As(fmt.Errorf("wrapped: %w", err), &domainErr) {
		t.Fatalf("expected errors.As to find NAuthError")
	}
	if got := domainErr.Code(); got != CodeBadRequest {
		t.Fatalf("expected %q, got %q", CodeBadRequest, got)
	}
}

func TestNAuthError_Is(t *testing.T) {
	t.Run("matches same code", func(t *testing.T) {
		err := fmt.Errorf("wrapped: %w", ErrAccountNotReady())

		if !errors.Is(err, ErrAccountNotReady()) {
			t.Fatalf("expected errors.Is to match same domain error code")
		}
	})

	t.Run("does not match different code", func(t *testing.T) {
		err := ErrAccountNotReady()

		if errors.Is(err, ErrAccountNotFound()) {
			t.Fatalf("expected errors.Is not to match a different domain error code")
		}
	})
}
