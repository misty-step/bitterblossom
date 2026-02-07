package dispatch

import (
	"context"
	"strings"
	"testing"
	"time"
)

type execCall struct {
	sprite  string
	command string
	stdin   string
}

type fakeExecutor struct {
	calls   []execCall
	outputs []string
	errs    []error
}

func (f *fakeExecutor) Exec(_ context.Context, sprite, command string, stdin []byte) (string, error) {
	f.calls = append(f.calls, execCall{
		sprite:  sprite,
		command: command,
		stdin:   string(stdin),
	})
	idx := len(f.calls) - 1
	var output string
	if idx < len(f.outputs) {
		output = f.outputs[idx]
	}
	var err error
	if idx < len(f.errs) {
		err = f.errs[idx]
	}
	return output, err
}

func TestRunIssueTask(t *testing.T) {
	fake := &fakeExecutor{
		outputs: []string{"", "1234\n"},
	}
	svc := NewService(fake)
	now := time.Date(2026, time.February, 7, 17, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	result, err := svc.RunIssueTask(context.Background(), DispatchRequest{
		Sprite:      "bramble",
		Repo:        "heartbeat",
		IssueNumber: 42,
		PersonaRole: "tester",
	})
	if err != nil {
		t.Fatalf("RunIssueTask() error = %v", err)
	}

	if len(fake.calls) != 2 {
		t.Fatalf("expected 2 executor calls, got %d", len(fake.calls))
	}

	if got := fake.calls[0].command; got != "cat > /home/sprite/workspace/TASK.md" {
		t.Fatalf("unexpected upload command: %q", got)
	}
	if !strings.Contains(fake.calls[0].stdin, "GitHub issue #42 in misty-step/heartbeat") {
		t.Fatalf("prompt does not include issue target: %q", fake.calls[0].stdin)
	}

	if !strings.Contains(fake.calls[1].command, "git clone 'https://github.com/misty-step/heartbeat.git' 'heartbeat'") {
		t.Fatalf("launch command missing clone line: %q", fake.calls[1].command)
	}

	if result.Sprite != "bramble" {
		t.Fatalf("unexpected sprite: %q", result.Sprite)
	}
	if result.Task != "misty-step/heartbeat#42" {
		t.Fatalf("unexpected task: %q", result.Task)
	}
	if result.PID != 1234 {
		t.Fatalf("unexpected pid: %d", result.PID)
	}
	if result.StartedAt != now {
		t.Fatalf("unexpected started time: %s", result.StartedAt)
	}
}

func TestRunIssueTaskInvalidRepo(t *testing.T) {
	svc := NewService(&fakeExecutor{})
	_, err := svc.RunIssueTask(context.Background(), DispatchRequest{
		Sprite:      "bramble",
		Repo:        "https://github.com/misty-step/heartbeat",
		IssueNumber: 7,
	})
	if err == nil {
		t.Fatal("expected invalid repo error")
	}
}

func TestRunIssueTaskInvalidPID(t *testing.T) {
	fake := &fakeExecutor{
		outputs: []string{"", "not-a-pid"},
	}
	svc := NewService(fake)
	_, err := svc.RunIssueTask(context.Background(), DispatchRequest{
		Sprite:      "bramble",
		Repo:        "misty-step/heartbeat",
		IssueNumber: 7,
	})
	if err == nil {
		t.Fatal("expected pid parse error")
	}
}
