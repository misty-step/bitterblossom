package lib

import (
	"errors"
	"testing"
)

func TestIsValidationError(t *testing.T) {
	err := &ValidationError{Field: "sprite", Message: "bad"}
	if !IsValidationError(err) {
		t.Fatalf("expected validation error")
	}
	wrapped := errors.New("other")
	if IsValidationError(wrapped) {
		t.Fatalf("did not expect validation error")
	}
}

func TestCommandErrorString(t *testing.T) {
	err := &CommandError{Command: "sprite", ExitCode: 1}
	if err.Error() == "" {
		t.Fatalf("expected non-empty error")
	}
}
