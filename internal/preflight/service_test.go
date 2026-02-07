package preflight

import (
	"context"
	"errors"
	"testing"

	"github.com/misty-step/bitterblossom/internal/lib"
)

type mockRunner struct {
	requests []lib.RunRequest
	results  []lib.RunResult
	errors   []error
}

func (m *mockRunner) Run(_ context.Context, req lib.RunRequest) (lib.RunResult, error) {
	m.requests = append(m.requests, req)
	idx := len(m.requests) - 1
	if idx < len(m.errors) && m.errors[idx] != nil {
		var result lib.RunResult
		if idx < len(m.results) {
			result = m.results[idx]
		}
		return result, m.errors[idx]
	}
	if idx < len(m.results) {
		return m.results[idx], nil
	}
	return lib.RunResult{}, nil
}

func TestCheckSpritePassAndWarn(t *testing.T) {
	runner := &mockRunner{results: []lib.RunResult{
		{Stdout: "thorn\n"},       // list
		{Stdout: "alive\n"},       // responsive
		{Stdout: "claude 1.0\n"},  // claude
		{Stdout: "store\n"},       // cred helper
		{Stdout: "EXISTS\n"},      // cred file
		{Stdout: "PASS\n"},        // git access
		{Stdout: "NO\n"},          // CLAUDE.md missing -> warn
		{Stdout: "10G\n"},         // disk
		{Stdout: "sprite user\n"}, // git user
		{Stdout: "0\n"},           // stale processes
	}}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"))
	report, err := svc.CheckSprite(context.Background(), "thorn")
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if report.Failures != 0 {
		t.Fatalf("expected no failures, got %d", report.Failures)
	}
	if report.Warnings != 1 {
		t.Fatalf("expected one warning, got %d", report.Warnings)
	}
}

func TestCheckSpriteUnreachableStopsEarly(t *testing.T) {
	runner := &mockRunner{results: []lib.RunResult{{Stdout: "thorn\n"}}, errors: []error{nil, errors.New("timeout")}}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"))
	report, err := svc.CheckSprite(context.Background(), "thorn")
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	if report.Failures == 0 {
		t.Fatalf("expected failure when unreachable")
	}
	if len(report.Checks) < 2 {
		t.Fatalf("expected at least 2 checks, got %d", len(report.Checks))
	}
}

func TestCheckAllNoSprites(t *testing.T) {
	runner := &mockRunner{results: []lib.RunResult{{Stdout: ""}}}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"))
	_, err := svc.CheckAll(context.Background())
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestParseCount(t *testing.T) {
	if got := parseCount("5\n"); got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}
	if got := parseCount("bad"); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestCheckSpriteFailsWhenMissing(t *testing.T) {
	runner := &mockRunner{results: []lib.RunResult{{Stdout: ""}}}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"))
	report, err := svc.CheckSprite(context.Background(), "thorn")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Failures == 0 {
		t.Fatalf("expected failure for missing sprite")
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("a\nb"); got != "a" {
		t.Fatalf("unexpected first line: %q", got)
	}
	if got := firstLine("single"); got != "single" {
		t.Fatalf("unexpected first line: %q", got)
	}
}

func TestCheckSpriteInvalidName(t *testing.T) {
	runner := &mockRunner{}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"))
	if _, err := svc.CheckSprite(context.Background(), "BAD"); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestCheckSpriteGitCredentialFailure(t *testing.T) {
	runner := &mockRunner{results: []lib.RunResult{
		{Stdout: "thorn\n"},      // list
		{Stdout: "alive\n"},      // responsive
		{Stdout: "claude 1.0\n"}, // claude
		{Stdout: "MISSING\n"},    // helper -> failure
		{Stdout: "EXISTS\n"},
		{Stdout: "PASS\n"},
		{Stdout: "YES\n"},
		{Stdout: "10G\n"},
		{Stdout: "sprite\n"},
		{Stdout: "0\n"},
	}}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"))
	report, err := svc.CheckSprite(context.Background(), "thorn")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Failures == 0 {
		t.Fatalf("expected credential helper failure")
	}
}
