package main

import (
	"bytes"
	"errors"
	"testing"
)

func TestRootCommandVersionOutput(t *testing.T) {
	t.Parallel()

	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root command: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected version output")
	}
}

func TestVersionSubcommandOutput(t *testing.T) {
	t.Parallel()

	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute version command: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected version output")
	}
}

func TestExitErrorMethods(t *testing.T) {
	t.Parallel()

	if got := (&exitError{Code: 2}).Error(); got == "" {
		t.Fatalf("expected non-empty default error string")
	}

	wrapped := errors.New("wrapped")
	err := &exitError{Code: 1, Err: wrapped}
	if err.Unwrap() != wrapped {
		t.Fatalf("expected unwrap to return wrapped error")
	}
	if got := err.Error(); got != wrapped.Error() {
		t.Fatalf("unexpected error string: %s", got)
	}
}
