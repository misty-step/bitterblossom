package main

import (
	"testing"
)

func TestBuildEnvArgs_SingleFlag(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"OPENROUTER_API_KEY":  "sk-or-test",
		"ANTHROPIC_AUTH_TOKEN": "tok-test",
	}

	args := buildEnvArgs(env)

	// Should produce exactly two args: "-env" and a comma-joined string
	if len(args) != 2 {
		t.Fatalf("expected 2 args (-env <pairs>), got %d: %v", len(args), args)
	}
	if args[0] != "-env" {
		t.Fatalf("expected first arg '-env', got %q", args[0])
	}

	// Keys are sorted, so ANTHROPIC_AUTH_TOKEN comes first
	want := "ANTHROPIC_AUTH_TOKEN=tok-test,OPENROUTER_API_KEY=sk-or-test"
	if args[1] != want {
		t.Fatalf("expected %q, got %q", want, args[1])
	}
}

func TestBuildEnvArgs_Empty(t *testing.T) {
	t.Parallel()

	args := buildEnvArgs(nil)
	if len(args) != 0 {
		t.Fatalf("expected 0 args for nil env, got %d: %v", len(args), args)
	}

	args = buildEnvArgs(map[string]string{})
	if len(args) != 0 {
		t.Fatalf("expected 0 args for empty env, got %d: %v", len(args), args)
	}
}

func TestBuildEnvArgs_SingleVar(t *testing.T) {
	t.Parallel()

	env := map[string]string{"KEY": "value"}
	args := buildEnvArgs(env)

	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
	if args[1] != "KEY=value" {
		t.Fatalf("expected %q, got %q", "KEY=value", args[1])
	}
}
