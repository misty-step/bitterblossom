package agent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/internal/clients"
)

type mockRunner struct {
	out      string
	exitCode int
	err      error
	calls    []string
}

func (m *mockRunner) Run(_ context.Context, name string, args ...string) (string, int, error) {
	m.calls = append(m.calls, name+" "+join(args))
	return m.out, m.exitCode, m.err
}

func join(args []string) string {
	if len(args) == 0 {
		return ""
	}
	s := args[0]
	for i := 1; i < len(args); i++ {
		s += " " + args[i]
	}
	return s
}

type mockGit struct{}

func (mockGit) ListRepos(context.Context, string) ([]string, error) { return nil, nil }
func (mockGit) CurrentBranch(context.Context, string) (string, error) {
	return "", nil
}
func (mockGit) CommitsAhead(context.Context, string, string) (int, error) { return 0, nil }
func (mockGit) HasUncommittedChanges(context.Context, string) (bool, error) {
	return false, nil
}
func (mockGit) LastCommitEpoch(context.Context, string) (int64, error) { return 0, nil }
func (mockGit) Push(context.Context, string, string, string) error     { return nil }
func (mockGit) CollectProgress(context.Context, string) ([]clients.RepoProgress, error) {
	return nil, nil
}

type activeGit struct {
	progress []clients.RepoProgress
	pushes   int
}

func (a *activeGit) ListRepos(context.Context, string) ([]string, error) { return nil, nil }
func (a *activeGit) CurrentBranch(context.Context, string) (string, error) {
	return "", nil
}
func (a *activeGit) CommitsAhead(context.Context, string, string) (int, error) { return 0, nil }
func (a *activeGit) HasUncommittedChanges(context.Context, string) (bool, error) {
	return false, nil
}
func (a *activeGit) LastCommitEpoch(context.Context, string) (int64, error) { return 0, nil }
func (a *activeGit) Push(context.Context, string, string, string) error {
	a.pushes++
	return nil
}
func (a *activeGit) CollectProgress(context.Context, string) ([]clients.RepoProgress, error) {
	return a.progress, nil
}

type mockHealth struct{}

func (mockHealth) Collect(context.Context, int, bool) HealthSnapshot {
	return HealthSnapshot{}
}

func TestRunClaudeIterationDryRun(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "PROMPT.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &mockRunner{}
	s := &Supervisor{
		cfg: Config{
			Workspace:     tmp,
			PromptFile:    "PROMPT.md",
			LogFile:       filepath.Join(tmp, "ralph.log"),
			ClaudeCommand: "claude -p",
			DryRun:        true,
		},
		dep:  Dependencies{Runner: r},
		sink: NewNDJSONSink(&bytes.Buffer{}, "sprite"),
	}

	code, err := s.runClaudeIteration(context.Background(), 1)
	if err != nil {
		t.Fatalf("runClaudeIteration returned error: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code mismatch: %d", code)
	}
	if len(r.calls) != 0 {
		t.Fatalf("expected no runner calls in dry-run, got %d", len(r.calls))
	}
}

func TestRunClaudeIterationMissingPrompt(t *testing.T) {
	tmp := t.TempDir()
	s := &Supervisor{
		cfg:  Config{Workspace: tmp, PromptFile: "PROMPT.md"},
		dep:  Dependencies{},
		sink: NewNDJSONSink(&bytes.Buffer{}, "sprite"),
	}
	_, err := s.runClaudeIteration(context.Background(), 1)
	if !errors.Is(err, ErrPromptMissing) {
		t.Fatalf("expected ErrPromptMissing, got %v", err)
	}
}

func TestControlSignal(t *testing.T) {
	tmp := t.TempDir()
	complete := filepath.Join(tmp, "TASK_COMPLETE")
	if err := os.WriteFile(complete, []byte("done"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Supervisor{cfg: Config{Workspace: tmp}}
	state, msg := s.controlSignal(context.Background())
	if state != "task_complete" {
		t.Fatalf("state mismatch: %s", state)
	}
	if msg != "done" {
		t.Fatalf("message mismatch: %s", msg)
	}
}

func TestSleepWithContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sleepWithContext(ctx, time.Second) {
		t.Fatal("expected false when context canceled")
	}
}

func TestValidateConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want bool
	}{
		{name: "missing workspace", cfg: Config{SpriteName: "thorn"}, want: false},
		{name: "missing sprite", cfg: Config{Workspace: "/tmp"}, want: false},
		{name: "valid", cfg: Config{Workspace: "/tmp", SpriteName: "thorn"}, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConfig(tc.cfg)
			if tc.want && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
			if !tc.want && err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestWithDefaults(t *testing.T) {
	cfg := withDefaults(Config{Workspace: "/tmp/w", SpriteName: "thorn"})
	if cfg.PromptFile != "PROMPT.md" {
		t.Fatalf("prompt default mismatch: %s", cfg.PromptFile)
	}
	if cfg.ClaudeCommand == "" {
		t.Fatal("expected default claude command")
	}
}

func TestSupervisorRun(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "PROMPT.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &mockRunner{out: "1", exitCode: 0}
	git := &activeGit{
		progress: []clients.RepoProgress{{
			Path:   filepath.Join(tmp, "repo"),
			Name:   "repo",
			Branch: "main",
			Ahead:  1,
		}},
	}
	supervisor := &Supervisor{
		cfg: withDefaults(Config{
			SpriteName:       "thorn",
			Workspace:        tmp,
			PromptFile:       "PROMPT.md",
			LogFile:          filepath.Join(tmp, "ralph.log"),
			EventFile:        filepath.Join(tmp, "events.ndjson"),
			DryRun:           true,
			AutoPush:         true,
			MaxIterations:    2,
			HeartbeatEvery:   1 * time.Millisecond,
			GitScanEvery:     1 * time.Millisecond,
			AutoPushEvery:    1 * time.Millisecond,
			RestartDelay:     2 * time.Millisecond,
			StopOnTaskSignal: false,
		}),
		dep: Dependencies{
			Runner: r,
			Git:    git,
			Health: mockHealth{},
			Logger: newDiscardLogger(),
		},
		sink: NewNDJSONSink(&bytes.Buffer{}, "thorn"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := supervisor.run(ctx); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
}

func TestRunClaudeIterationCommandPath(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "PROMPT.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &mockRunner{out: "ok", exitCode: 0}
	s := &Supervisor{
		cfg: Config{
			Workspace:     tmp,
			PromptFile:    "PROMPT.md",
			LogFile:       filepath.Join(tmp, "ralph.log"),
			ClaudeCommand: "cat",
			DryRun:        false,
		},
		dep:  Dependencies{Runner: r},
		sink: NewNDJSONSink(&bytes.Buffer{}, "sprite"),
	}
	code, err := s.runClaudeIteration(context.Background(), 2)
	if err != nil {
		t.Fatalf("runClaudeIteration returned error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected code 0 got %d", code)
	}
	if len(r.calls) == 0 {
		t.Fatal("expected runner call for iteration")
	}
}

func TestRunDaemon(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "PROMPT.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := RunDaemon(ctx, Config{
		SpriteName:       "thorn",
		Workspace:        tmp,
		PromptFile:       "PROMPT.md",
		EventFile:        filepath.Join(tmp, "events.ndjson"),
		DryRun:           true,
		AutoPush:         false,
		MaxIterations:    1,
		HeartbeatEvery:   2 * time.Millisecond,
		GitScanEvery:     2 * time.Millisecond,
		RestartDelay:     1 * time.Millisecond,
		StopOnTaskSignal: false,
	}, Dependencies{
		Runner: &mockRunner{out: "1", exitCode: 0},
		Git:    &activeGit{},
		Health: mockHealth{},
		Logger: newDiscardLogger(),
		Stdout: io.Discard,
	})
	if err != nil {
		t.Fatalf("RunDaemon returned error: %v", err)
	}
}

func TestAutoPushLoop(t *testing.T) {
	tmp := t.TempDir()
	git := &activeGit{
		progress: []clients.RepoProgress{{
			Path:   filepath.Join(tmp, "repo"),
			Name:   "repo",
			Branch: "main",
			Ahead:  2,
		}},
	}
	s := &Supervisor{
		cfg: withDefaults(Config{
			Workspace:     tmp,
			AutoPushEvery: 1 * time.Millisecond,
			AutoPush:      true,
			DryRun:        false,
		}),
		dep: Dependencies{
			Git:    git,
			Runner: &mockRunner{out: "1", exitCode: 0},
			Health: mockHealth{},
			Logger: newDiscardLogger(),
		},
		sink: NewNDJSONSink(&bytes.Buffer{}, "thorn"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Millisecond)
	defer cancel()
	s.autoPushLoop(ctx)
	if git.pushes == 0 {
		t.Fatal("expected pushes to be attempted")
	}
}

var _ clients.Runner = (*mockRunner)(nil)
var _ clients.GitClient = mockGit{}

func newDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
