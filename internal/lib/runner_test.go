package lib

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestExecRunnerDryRunSkipsMutating(t *testing.T) {
	r := &ExecRunner{DryRun: true}
	result, err := r.Run(context.Background(), RunRequest{Cmd: "sh", Args: []string{"-c", "echo nope"}, Mutating: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stdout != "" {
		t.Fatalf("expected empty stdout in dry-run, got %q", result.Stdout)
	}
}

func TestExecRunnerRunsCommand(t *testing.T) {
	r := &ExecRunner{}
	result, err := r.Run(context.Background(), RunRequest{Cmd: "sh", Args: []string{"-c", "echo ok"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Stdout, "ok") {
		t.Fatalf("expected stdout to include ok, got %q", result.Stdout)
	}
}

func TestFormatCommand(t *testing.T) {
	got := FormatCommand("sprite", []string{"list", "-o", "misty-step"})
	if got != "sprite list -o misty-step" {
		t.Fatalf("unexpected command format: %s", got)
	}
}

func TestExecRunnerRequiresCommand(t *testing.T) {
	r := &ExecRunner{}
	if _, err := r.Run(context.Background(), RunRequest{}); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestCommandErrorUnwrap(t *testing.T) {
	inner := errors.New("inner")
	err := &CommandError{Command: "x", Err: inner}
	if !errors.Is(err, inner) {
		t.Fatalf("expected unwrap support")
	}
}
