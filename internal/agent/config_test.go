package agent

import (
	"strings"
	"testing"
)

func TestTaskAssignmentValidate(t *testing.T) {
	t.Parallel()

	if err := (TaskAssignment{}).Validate(); err == nil {
		t.Fatalf("expected validation error")
	}

	if err := (TaskAssignment{Prompt: "Fix auth", Repo: "cerberus"}).Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestAgentKindValid(t *testing.T) {
	t.Parallel()

	if !AgentCodex.Valid() || !AgentKimi.Valid() || !AgentClaude.Valid() {
		t.Fatalf("expected built-in agent kinds to be valid")
	}
	if AgentKind("unknown").Valid() {
		t.Fatalf("unexpected valid unknown agent kind")
	}
}

func TestAgentConfigCommandAndArgs(t *testing.T) {
	t.Parallel()

	cfg := AgentConfig{
		Kind:     AgentCodex,
		Model:    "gpt-5-codex",
		Yolo:     true,
		FullAuto: true,
		Flags:    []string{"--json"},
		Assignment: TaskAssignment{
			Prompt: "Fix tests",
			Repo:   "cerberus",
		},
	}

	cmd, args, err := cfg.CommandAndArgs()
	if err != nil {
		t.Fatalf("command and args: %v", err)
	}
	if cmd != "codex" {
		t.Fatalf("unexpected command: %s", cmd)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"--yolo", "--full-auto", "--model", "gpt-5-codex", "--json", "Fix tests"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in args %q", want, joined)
		}
	}
}

func TestAgentConfigBuildEnvironment(t *testing.T) {
	t.Setenv("BB_PASSTHROUGH", "from-env")

	cfg := AgentConfig{
		Environment: map[string]string{
			"CUSTOM_FLAG": "1",
		},
		PassThroughEnv: []string{"BB_PASSTHROUGH"},
	}

	env := cfg.BuildEnvironment()
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "CUSTOM_FLAG=1") {
		t.Fatalf("expected custom env override")
	}
	if !strings.Contains(joined, "BB_PASSTHROUGH=from-env") {
		t.Fatalf("expected pass-through env value")
	}
}

func TestDefaultRuntimePaths(t *testing.T) {
	t.Parallel()

	paths := DefaultRuntimePaths("/workspace/repo")
	if !strings.Contains(paths.EventLog, "/workspace/repo/.bb-agent/events.jsonl") {
		t.Fatalf("unexpected event log path %s", paths.EventLog)
	}
	if !strings.Contains(paths.StateFile, "/workspace/repo/.bb-agent/state.json") {
		t.Fatalf("unexpected state path %s", paths.StateFile)
	}
}
