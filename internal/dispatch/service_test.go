package dispatch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func newServiceForTest(t *testing.T, runner lib.Runner) *Service {
	t.Helper()
	root := t.TempDir()
	scriptsDir := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	template := "task={{TASK_DESCRIPTION}} repo={{REPO}} sprite={{SPRITE_NAME}}"
	if err := os.WriteFile(filepath.Join(scriptsDir, "ralph-prompt-template.md"), []byte(template), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	paths, err := lib.NewPaths(root)
	if err != nil {
		t.Fatalf("new paths: %v", err)
	}
	sprite := lib.NewSpriteCLI(runner, "sprite", "misty-step")
	return NewService(nil, sprite, paths, 7)
}

func TestGenerateRalphPrompt(t *testing.T) {
	runner := &mockRunner{}
	svc := newServiceForTest(t, runner)
	out, err := svc.GenerateRalphPrompt("do thing", "misty-step/cerberus", "thorn")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "task=do thing") || !strings.Contains(out, "repo=misty-step/cerberus") || !strings.Contains(out, "sprite=thorn") {
		t.Fatalf("unexpected rendered prompt: %s", out)
	}
}

func TestSetupRepoRejectsInvalidRepo(t *testing.T) {
	runner := &mockRunner{}
	svc := newServiceForTest(t, runner)
	if err := svc.SetupRepo(context.Background(), "thorn", "bad repo"); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestDispatchOneShotRunsUploadAndCommand(t *testing.T) {
	runner := &mockRunner{results: []lib.RunResult{{}, {Stdout: "work done"}}}
	svc := newServiceForTest(t, runner)

	out, err := svc.DispatchOneShot(context.Background(), "thorn", "fix bug", "")
	if err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}
	if strings.TrimSpace(out) != "work done" {
		t.Fatalf("expected command output, got %q", out)
	}
	if len(runner.requests) != 2 {
		t.Fatalf("expected 2 commands (upload + run), got %d", len(runner.requests))
	}
	if !runner.requests[0].Mutating || !runner.requests[1].Mutating {
		t.Fatalf("expected mutating commands for one-shot flow")
	}
}

func TestStartRalphBuildsScriptAndStarts(t *testing.T) {
	runner := &mockRunner{}
	svc := newServiceForTest(t, runner)
	if err := svc.StartRalph(context.Background(), "thorn", "implement", ""); err != nil {
		t.Fatalf("start ralph failed: %v", err)
	}
	if len(runner.requests) < 3 {
		t.Fatalf("expected at least 3 commands, got %d", len(runner.requests))
	}
}

func TestCheckStatusReturnsStructuredData(t *testing.T) {
	runner := &mockRunner{results: []lib.RunResult{
		{Stdout: "RUNNING (PID 123)"},
		{Stdout: "STATUS: Working"},
		{Stdout: "log line"},
		{Stdout: "memory line"},
	}}
	svc := newServiceForTest(t, runner)
	status, err := svc.CheckStatus(context.Background(), "thorn")
	if err != nil {
		t.Fatalf("check status failed: %v", err)
	}
	if status.RalphStatus != "RUNNING (PID 123)" {
		t.Fatalf("unexpected Ralph status: %q", status.RalphStatus)
	}
	if status.RecentLog != "log line" {
		t.Fatalf("unexpected log: %q", status.RecentLog)
	}
}

func TestDispatchPropagatesRunnerError(t *testing.T) {
	runner := &mockRunner{errors: []error{nil, errors.New("boom")}}
	svc := newServiceForTest(t, runner)
	_, err := svc.DispatchOneShot(context.Background(), "thorn", "fix", "")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestGenerateRalphPromptMissingTemplate(t *testing.T) {
	root := t.TempDir()
	paths, err := lib.NewPaths(root)
	if err != nil {
		t.Fatalf("new paths: %v", err)
	}
	runner := &mockRunner{}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"), paths, 5)
	if _, err := svc.GenerateRalphPrompt("x", "", ""); err == nil {
		t.Fatalf("expected error when template missing")
	}
}

func TestSetupRepoSuccess(t *testing.T) {
	runner := &mockRunner{}
	svc := newServiceForTest(t, runner)
	if err := svc.SetupRepo(context.Background(), "thorn", "misty-step/cerberus"); err != nil {
		t.Fatalf("setup repo failed: %v", err)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("expected one request, got %d", len(runner.requests))
	}
}

func TestBuildRalphLoopScriptValidation(t *testing.T) {
	runner := &mockRunner{}
	svc := newServiceForTest(t, runner)
	svc.MaxRalphIterations = 0
	if _, err := svc.buildRalphLoopScript(); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestStopRalph(t *testing.T) {
	runner := &mockRunner{}
	svc := newServiceForTest(t, runner)
	if err := svc.StopRalph(context.Background(), "thorn"); err != nil {
		t.Fatalf("stop ralph failed: %v", err)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(runner.requests))
	}
}
