package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run(context.Background(), []string{"version"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("run(version) error = %v", err)
	}
	if !strings.Contains(out.String(), "bb version") {
		t.Fatalf("version output = %q", out.String())
	}
}

func TestRunUsageAndUnknownCommand(t *testing.T) {
	t.Parallel()

	var usage bytes.Buffer
	if err := run(context.Background(), nil, &usage, &bytes.Buffer{}); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(usage.String(), "Usage:") {
		t.Fatalf("usage output = %q", usage.String())
	}

	if err := run(context.Background(), []string{"wat"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("run(unknown) expected error")
	}
}

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
		t.Fatalf("expected command output")
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

func TestRootWiresDispatchAndWatchdog(t *testing.T) {
	t.Parallel()

	root := newRootCommand()
	names := make(map[string]struct{}, len(root.Commands()))
	for _, command := range root.Commands() {
		names[command.Name()] = struct{}{}
	}
	if _, ok := names["dispatch"]; !ok {
		t.Fatal("dispatch command not wired")
	}
	if _, ok := names["watchdog"]; !ok {
		t.Fatal("watchdog command not wired")
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

	var nilErr *exitError
	if nilErr.Unwrap() != nil {
		t.Fatalf("nil receiver Unwrap() should return nil")
	}
}
