package watchdog

import (
	"encoding/base64"
	"testing"
)

func TestParseProbeOutput(t *testing.T) {
	t.Parallel()

	status := `{"repo":"misty-step/heartbeat","issue":43,"started":"2026-02-08T09:30:00Z","mode":"ralph"}`
	blocked := "needs api token"
	taskID := "bb-20260208-0930-bramble"
	branch := "feature/issue-43"

	output := "" +
		"__CLAUDE_COUNT__1\n" +
		"__AGENT_RUNNING__yes\n" +
		"__HAS_COMPLETE__no\n" +
		"__HAS_BLOCKED__yes\n" +
		"__COMMITS_LAST_2H__3\n" +
		"__DIRTY_REPOS__1\n" +
		"__AHEAD_COMMITS__2\n" +
		"__HAS_PROMPT__yes\n" +
		"__BLOCKED_B64__" + base64.StdEncoding.EncodeToString([]byte(blocked)) + "\n" +
		"__BRANCH_B64__" + base64.StdEncoding.EncodeToString([]byte(branch)) + "\n" +
		"__STATUS_B64__" + base64.StdEncoding.EncodeToString([]byte(status)) + "\n" +
		"__TASK_ID_B64__" + base64.StdEncoding.EncodeToString([]byte(taskID)) + "\n"

	parsed, err := parseProbeOutput(output)
	if err != nil {
		t.Fatalf("parseProbeOutput() error = %v", err)
	}

	if parsed.ClaudeCount != 1 {
		t.Fatalf("ClaudeCount = %d, want 1", parsed.ClaudeCount)
	}
	if !parsed.AgentRunning {
		t.Fatal("AgentRunning = false, want true")
	}
	if !parsed.HasBlocked {
		t.Fatal("HasBlocked = false, want true")
	}
	if parsed.BlockedSummary != blocked {
		t.Fatalf("BlockedSummary = %q, want %q", parsed.BlockedSummary, blocked)
	}
	if parsed.Branch != branch {
		t.Fatalf("Branch = %q, want %q", parsed.Branch, branch)
	}
	if parsed.CommitsLast2h != 3 {
		t.Fatalf("CommitsLast2h = %d, want 3", parsed.CommitsLast2h)
	}
	if parsed.DirtyRepos != 1 {
		t.Fatalf("DirtyRepos = %d, want 1", parsed.DirtyRepos)
	}
	if parsed.AheadCommits != 2 {
		t.Fatalf("AheadCommits = %d, want 2", parsed.AheadCommits)
	}
	if parsed.CurrentTaskID != taskID {
		t.Fatalf("CurrentTaskID = %q, want %q", parsed.CurrentTaskID, taskID)
	}
	if parsed.Status.Repo != "misty-step/heartbeat" {
		t.Fatalf("Status.Repo = %q", parsed.Status.Repo)
	}
	if parsed.Status.Issue != 43 {
		t.Fatalf("Status.Issue = %d, want 43", parsed.Status.Issue)
	}
}

func TestParseProbeOutputInvalidValue(t *testing.T) {
	t.Parallel()

	_, err := parseProbeOutput("__CLAUDE_COUNT__bad\n")
	if err == nil {
		t.Fatal("expected parse error")
	}
}
