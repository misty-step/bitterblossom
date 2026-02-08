package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/pkg/events"
)

type recordingEventEmitter struct {
	mu     sync.Mutex
	events []events.Event
}

func (r *recordingEventEmitter) Emit(event events.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	return nil
}

func (r *recordingEventEmitter) Events() []events.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	copyEvents := make([]events.Event, len(r.events))
	copy(copyEvents, r.events)
	return copyEvents
}

type sequenceGitClient struct {
	mu        sync.Mutex
	snapshots []GitSnapshot
	idx       int
}

func (s *sequenceGitClient) Snapshot(context.Context) (GitSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.snapshots) == 0 {
		return GitSnapshot{}, nil
	}
	if s.idx >= len(s.snapshots) {
		return s.snapshots[len(s.snapshots)-1], nil
	}
	snapshot := s.snapshots[s.idx]
	s.idx++
	return snapshot, nil
}

func TestClassifyAgentOutput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		line     string
		stderr   bool
		activity string
		meaning  bool
	}{
		{line: "go test ./...", activity: "test_run", meaning: true},
		{line: "build succeeded", activity: "build_result", meaning: true},
		{line: "build failed", activity: "error", meaning: true},
		{line: "Tool call: exec_command", activity: "tool_call", meaning: true},
		{line: "*** Update File: internal/agent/progress.go", activity: "file_edit", meaning: true},
		{line: "$ git status --short", activity: "command_run", meaning: true},
		{line: "stdout note", stderr: true, activity: "error", meaning: true},
		{line: "arbitrary log line", activity: "", meaning: false},
		{line: "", activity: "", meaning: false},
	}

	for _, tc := range cases {
		activity, _, _, meaningful := classifyAgentOutput(tc.line, tc.stderr)
		if activity != tc.activity || meaningful != tc.meaning {
			t.Fatalf("unexpected classification for %q: activity=%s meaningful=%t", tc.line, activity, meaningful)
		}
	}
}

func TestBranchesAdded(t *testing.T) {
	t.Parallel()

	added := branchesAdded([]string{"main"}, []string{"main", "feature/auth", "bugfix"})
	if len(added) != 2 {
		t.Fatalf("unexpected added branch count: %d", len(added))
	}
	if added[0] != "bugfix" || added[1] != "feature/auth" {
		t.Fatalf("unexpected branch diff ordering: %#v", added)
	}
}

func TestProgressMonitorPollAndStallSignal(t *testing.T) {
	t.Parallel()

	emitter := &recordingEventEmitter{}
	now := time.Date(2026, 2, 7, 20, 0, 0, 0, time.UTC)

	monitor := NewProgressMonitor(ProgressConfig{
		Sprite:       "bramble",
		RepoDir:      "/tmp/unused",
		PollInterval: time.Second,
		StallTimeout: 5 * time.Minute,
	}, emitter)
	monitor.now = func() time.Time { return now }
	monitor.git = &sequenceGitClient{snapshots: []GitSnapshot{
		{Branch: "main", Head: "aaaaaaaaaaaa", ChangedFiles: 0, Branches: []string{"main"}, CommitCount: 1},
		{Branch: "feature/auth", Head: "bbbbbbbbbbbb", ChangedFiles: 2, Uncommitted: true, Branches: []string{"main", "feature/auth"}, CommitCount: 2},
		{Branch: "feature/auth", Head: "bbbbbbbbbbbb", ChangedFiles: 2, Uncommitted: true, Branches: []string{"main", "feature/auth"}, CommitCount: 2},
	}}

	monitor.poll(context.Background())
	now = now.Add(2 * time.Second)
	monitor.poll(context.Background())

	activities := make([]string, 0)
	for _, event := range emitter.Events() {
		progress, ok := event.(*events.ProgressEvent)
		if !ok {
			continue
		}
		activities = append(activities, progress.Activity)
	}

	for _, want := range []string{"git_commit", "branch_created", "file_change"} {
		if !contains(activities, want) {
			t.Fatalf("missing progress activity %q in %#v", want, activities)
		}
	}

	now = now.Add(6 * time.Minute)
	monitor.poll(context.Background())

	select {
	case signal := <-monitor.Signals():
		if signal.Type != ProgressSignalStalled {
			t.Fatalf("unexpected signal type %s", signal.Type)
		}
	default:
		t.Fatalf("expected stalled signal")
	}

	blockedFound := false
	for _, event := range emitter.Events() {
		if _, ok := event.(*events.BlockedEvent); ok {
			blockedFound = true
			break
		}
	}
	if !blockedFound {
		t.Fatalf("expected blocked event after stall")
	}
}

func TestProgressMonitorObserveOutput(t *testing.T) {
	t.Parallel()

	emitter := &recordingEventEmitter{}
	var callbackActivity string

	monitor := NewProgressMonitor(ProgressConfig{
		Sprite: "bramble",
		OnActivity: func(activity string, _ time.Time, _ bool) {
			callbackActivity = activity
		},
	}, emitter)

	monitor.ObserveOutput("go test ./...", false)
	if callbackActivity != "test_run" {
		t.Fatalf("unexpected callback activity: %s", callbackActivity)
	}

	found := false
	for _, event := range emitter.Events() {
		progress, ok := event.(*events.ProgressEvent)
		if !ok {
			continue
		}
		if progress.Activity == "test_run" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected test_run progress event")
	}
}

func TestProgressMonitorObserveOutputStructuredActivities(t *testing.T) {
	t.Parallel()

	emitter := &recordingEventEmitter{}
	monitor := NewProgressMonitor(ProgressConfig{
		Sprite: "bramble",
	}, emitter)

	monitor.ObserveOutput("Tool call: exec_command", false)
	monitor.ObserveOutput("*** Update File: cmd/bb/watch.go", false)
	monitor.ObserveOutput("$ git status --short", false)

	activities := make([]string, 0, 3)
	for _, event := range emitter.Events() {
		progress, ok := event.(*events.ProgressEvent)
		if !ok {
			continue
		}
		activities = append(activities, progress.Activity)
	}

	for _, want := range []string{"tool_call", "file_edit", "command_run"} {
		if !contains(activities, want) {
			t.Fatalf("missing activity %q in %#v", want, activities)
		}
	}
}

func TestGitCLISnapshot(t *testing.T) {
	t.Parallel()

	repoDir := setupGitRepo(t)
	writeTestFile(t, filepath.Join(repoDir, "work.txt"), "draft")

	snapshot, err := newGitCLI(repoDir).Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	if snapshot.Branch != "main" {
		t.Fatalf("unexpected branch: %s", snapshot.Branch)
	}
	if snapshot.Head == "" {
		t.Fatalf("expected head commit hash")
	}
	if snapshot.CommitCount < 1 {
		t.Fatalf("expected commit count >= 1")
	}
	if !snapshot.Uncommitted {
		t.Fatalf("expected uncommitted state")
	}
}

func setupGitRepo(t *testing.T) string {
	t.Helper()

	repoDir := t.TempDir()
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "sprite@example.com")
	runGit(t, repoDir, "config", "user.name", "Sprite")
	writeTestFile(t, filepath.Join(repoDir, "README.md"), "hello")
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-m", "initial")
	runGit(t, repoDir, "branch", "-M", "main")
	return repoDir
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
	return strings.TrimSpace(string(output))
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
