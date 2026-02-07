package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/internal/clients"
)

type cmdSprite struct{}

func (cmdSprite) List(context.Context, string) ([]string, error) { return []string{"thorn"}, nil }
func (cmdSprite) Exec(_ context.Context, _ string, _ string, cmd string) (string, error) {
	switch {
	case strings.Contains(cmd, "claude -p"):
		return "1", nil
	case strings.Contains(cmd, "TASK_COMPLETE"):
		return "no", nil
	case strings.Contains(cmd, "BLOCKED.md"):
		return "no", nil
	case strings.Contains(cmd, "wc -l"):
		return "1", nil
	case strings.Contains(cmd, "find"):
		return "/home/sprite/workspace/main.go", nil
	case strings.Contains(cmd, "stat -c"):
		return "1700000000 /home/sprite/workspace/main.go", nil
	case strings.Contains(cmd, "AHEAD") || strings.Contains(cmd, "COMMITS_AHEAD"):
		return "BRANCH:main AHEAD:1 UNCOMMITTED: 1 file changed", nil
	case strings.Contains(cmd, "ls -la"):
		return "workspace", nil
	case strings.Contains(cmd, "head -20"):
		return "memory", nil
	case strings.Contains(cmd, "tail -"):
		return "logline", nil
	default:
		return "RUNNING", nil
	}
}
func (cmdSprite) API(context.Context, string, string) ([]byte, error) {
	return []byte(`{"sprites":[{"name":"thorn","status":"running","url":"x"}]}`), nil
}
func (cmdSprite) SpriteAPI(context.Context, string, string, string) ([]byte, error) {
	return []byte(`{"status":"running"}`), nil
}
func (cmdSprite) ListCheckpoints(context.Context, string, string) (string, error) { return "cp1", nil }
func (cmdSprite) ListSprites(context.Context, string) ([]clients.SpriteInfo, error) {
	return []clients.SpriteInfo{{Name: "thorn", Status: "running", URL: "x"}}, nil
}

type cmdFly struct{}

func (cmdFly) SSHRun(context.Context, string, string, string) (string, error) { return "RUNNING", nil }

type cmdGH struct{}

func (cmdGH) SearchOpenPRs(context.Context, string, string, int) ([]clients.PullRequest, error) {
	return []clients.PullRequest{{
		Number:    1,
		Title:     "PR",
		Repo:      "heartbeat",
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T01:00:00Z",
		HTMLURL:   "u",
	}}, nil
}
func (cmdGH) PRChecks(context.Context, string, string, int) (string, error) { return "pass", nil }
func (cmdGH) LastReviewState(context.Context, string, string, int) (string, error) {
	return "APPROVED", nil
}

type cmdRunner struct{}

func (cmdRunner) Run(context.Context, string, ...string) (string, int, error) { return "0", 0, nil }

type cmdGit struct{}

func (cmdGit) ListRepos(context.Context, string) ([]string, error) { return nil, nil }
func (cmdGit) CurrentBranch(context.Context, string) (string, error) {
	return "", nil
}
func (cmdGit) CommitsAhead(context.Context, string, string) (int, error) { return 0, nil }
func (cmdGit) HasUncommittedChanges(context.Context, string) (bool, error) {
	return false, nil
}
func (cmdGit) LastCommitEpoch(context.Context, string) (int64, error) { return 0, nil }
func (cmdGit) Push(context.Context, string, string, string) error     { return nil }
func (cmdGit) CollectProgress(context.Context, string) ([]clients.RepoProgress, error) {
	return nil, nil
}

func TestRunSubcommands(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sprite := cmdSprite{}

	if err := runWatch(ctx, logger, sprite, "misty-step", []string{"--stale-minutes", "30"}); err != nil {
		t.Fatalf("runWatch error: %v", err)
	}
	if err := runFleet(ctx, sprite, cmdFly{}, "misty-step", []string{"--events-file", filepath.Join(t.TempDir(), "missing.ndjson"), "--json"}); err != nil {
		t.Fatalf("runFleet error: %v", err)
	}
	if err := runHealth(ctx, sprite, "misty-step", []string{"--json"}); err != nil {
		t.Fatalf("runHealth error: %v", err)
	}

	composition := filepath.Join(t.TempDir(), "v1.yaml")
	_ = os.WriteFile(composition, []byte("sprites:\n  thorn:\n    role: qa\n"), 0o644)
	if err := runStatus(ctx, sprite, "misty-step", []string{"--composition", composition}); err != nil {
		t.Fatalf("runStatus overview error: %v", err)
	}
	if err := runStatus(ctx, sprite, "misty-step", []string{"thorn"}); err != nil {
		t.Fatalf("runStatus detail error: %v", err)
	}
	if err := runPRs(ctx, cmdGH{}, []string{"--dry-run"}); err != nil {
		t.Fatalf("runPRs error: %v", err)
	}
	if err := runLogs(ctx, sprite, "misty-step", []string{"thorn", "-n", "5"}); err != nil {
		t.Fatalf("runLogs error: %v", err)
	}
}

func TestRunAgentPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "PROMPT.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runAgent(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), cmdRunner{}, cmdGit{}, []string{
		"--sprite", "thorn",
		"--workspace", workspace,
		"--event-file", filepath.Join(workspace, "events.ndjson"),
		"--dry-run",
		"--max-iterations", "1",
		"--auto-push=false",
		"--stop-on-signal=false",
		"--heartbeat", "1ms",
		"--git-scan", "1ms",
		"--restart-delay", "1ms",
	})
	if err != nil {
		t.Fatalf("runAgent returned error: %v", err)
	}
}

func TestRunLogsRequiresSprite(t *testing.T) {
	if err := runLogs(context.Background(), cmdSprite{}, "misty-step", []string{}); err == nil {
		t.Fatal("expected error for missing sprite")
	}
}

func TestRunWritesUsageOnNoArgs(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"bb"}
	if err := run(context.Background(), []string{}); err != nil {
		t.Fatalf("run empty args returned error: %v", err)
	}
}

func TestUsagePrints(t *testing.T) {
	buf := &bytes.Buffer{}
	orig := os.Stderr
	_ = orig
	_ = buf
	usage()
}

var _ clients.SpriteClient = cmdSprite{}
var _ clients.FlyClient = cmdFly{}
var _ clients.GitHubClient = cmdGH{}
var _ clients.Runner = cmdRunner{}
var _ clients.GitClient = cmdGit{}
