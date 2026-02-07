package lib

import (
	"errors"
	"fmt"
)

// ValidationError reports invalid user input.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("invalid %s: %s", e.Field, e.Message)
}

// CommandError wraps failures from external CLIs (sprite, fly, gh, etc.).
type CommandError struct {
	Command  string
	Args     []string
	ExitCode int
	Stdout   string
	Stderr   string
	Err      error
}

func (e *CommandError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err != nil {
		return fmt.Sprintf("command failed: %s: %v", e.Command, e.Err)
	}
	return fmt.Sprintf("command failed: %s (exit=%d)", e.Command, e.ExitCode)
}

func (e *CommandError) Unwrap() error {
	return e.Err
}

// IsValidationError reports whether err (or wrapped cause) is a ValidationError.
func IsValidationError(err error) bool {
	var target *ValidationError
	return errors.As(err, &target)
}
