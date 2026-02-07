package main

import (
	"context"
	"os"
	"testing"
)

func TestRunHelp(t *testing.T) {
	if err := run(context.Background(), []string{"help"}); err != nil {
		t.Fatalf("run help returned error: %v", err)
	}
}

func TestRunUnknown(t *testing.T) {
	if err := run(context.Background(), []string{"nope"}); err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}

func TestEnvOr(t *testing.T) {
	const key = "BB_TEST_ENV"
	_ = os.Unsetenv(key)
	if got := envOr(key, "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}
	if err := os.Setenv(key, "value"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Unsetenv(key) }()
	if got := envOr(key, "fallback"); got != "value" {
		t.Fatalf("expected env value, got %q", got)
	}
}
