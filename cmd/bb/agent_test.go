package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/internal/agent"
	"github.com/misty-step/bitterblossom/internal/contracts"
	"github.com/spf13/cobra"
)

func TestAgentStartValidation(t *testing.T) {
	t.Parallel()

	root := newRootCommand()
	root.SetArgs([]string{"agent", "start", "--foreground", "--task-repo", "cerberus"})

	err := root.Execute()
	if err == nil {
		t.Fatalf("expected validation error")
	}

	var coded *exitError
	if !errors.As(err, &coded) {
		t.Fatalf("expected exitError, got %T", err)
	}
	if coded.Code != 1 {
		t.Fatalf("unexpected code: %d", coded.Code)
	}
}

func TestAgentStartInvalidAgent(t *testing.T) {
	t.Parallel()

	root := newRootCommand()
	root.SetArgs([]string{"agent", "start", "--foreground", "--agent", "invalid", "--task-prompt", "Fix auth", "--task-repo", "cerberus"})

	err := root.Execute()
	if err == nil {
		t.Fatalf("expected invalid agent error")
	}
	var coded *exitError
	if !errors.As(err, &coded) {
		t.Fatalf("expected exitError, got %T", err)
	}
}

func TestAgentStatusHumanAndJSON(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	statePath := filepath.Join(tempDir, "state.json")
	pidPath := filepath.Join(tempDir, "pid")

	state := agent.SupervisorState{
		Sprite:          "bramble",
		Status:          "running",
		SupervisorPID:   os.Getpid(),
		AgentPID:        os.Getpid(),
		Restarts:        4,
		StartedAt:       time.Date(2026, 2, 7, 20, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2026, 2, 7, 20, 1, 0, 0, time.UTC),
		LastProgressAt:  time.Date(2026, 2, 7, 20, 1, 0, 0, time.UTC),
		LastHeartbeatAt: time.Date(2026, 2, 7, 20, 1, 30, 0, time.UTC),
		Task:            agent.TaskAssignment{Prompt: "Fix auth", Repo: "cerberus", Branch: "feature/auth"},
	}

	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(statePath, encoded, 0o644); err != nil {
		t.Fatalf("write state file: %v", err)
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}

	t.Run("human", func(t *testing.T) {
		root := newRootCommand()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs([]string{"agent", "status", "--state-file", statePath, "--pid-file", pidPath})
		if err := root.Execute(); err != nil {
			t.Fatalf("status command: %v", err)
		}
		text := out.String()
		for _, want := range []string{"status:", "sprite: bramble", "task: Fix auth", "repo: cerberus"} {
			if !strings.Contains(text, want) {
				t.Fatalf("missing %q in %q", want, text)
			}
		}
	})

	t.Run("json", func(t *testing.T) {
		root := newRootCommand()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs([]string{"agent", "status", "--json", "--state-file", statePath, "--pid-file", pidPath})
		if err := root.Execute(); err != nil {
			t.Fatalf("status json command: %v", err)
		}
		var payload struct {
			Version string `json:"version"`
			Command string `json:"command"`
			Data    struct {
				State   agent.SupervisorState `json:"state"`
				Running bool                  `json:"running"`
			} `json:"data"`
		}
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal json output: %v", err)
		}
		if payload.Version != contracts.SchemaVersion {
			t.Fatalf("version = %q, want %q", payload.Version, contracts.SchemaVersion)
		}
		if payload.Command != "agent.status" {
			t.Fatalf("command = %q, want agent.status", payload.Command)
		}
		if payload.Data.State.Sprite != "bramble" {
			t.Fatalf("unexpected sprite in payload: %+v", payload)
		}
	})
}

func TestAgentLogsCommand(t *testing.T) {
	t.Parallel()

	logPath := filepath.Join(t.TempDir(), "agent.log")
	content := "one\ntwo\nthree\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"agent", "logs", "--output-log", logPath, "--lines", "2"})

	if err := root.Execute(); err != nil {
		t.Fatalf("logs command: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "two\nthree" {
		t.Fatalf("unexpected tail output: %q", got)
	}
}

func TestAgentLogsNoFile(t *testing.T) {
	t.Parallel()

	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"agent", "logs", "--output-log", filepath.Join(t.TempDir(), "missing.log")})

	if err := root.Execute(); err != nil {
		t.Fatalf("logs command: %v", err)
	}
	if !strings.Contains(out.String(), "no logs yet") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestAgentStopNotRunning(t *testing.T) {
	t.Parallel()

	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"agent", "stop", "--pid-file", filepath.Join(t.TempDir(), "missing.pid")})

	if err := root.Execute(); err != nil {
		t.Fatalf("stop command: %v", err)
	}
	if !strings.Contains(out.String(), "not running") {
		t.Fatalf("unexpected stop output: %q", out.String())
	}
}

func TestAgentStopStalePIDFile(t *testing.T) {
	t.Parallel()

	pidPath := filepath.Join(t.TempDir(), "stale.pid")
	if err := os.WriteFile(pidPath, []byte("999999\n"), 0o644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}

	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"agent", "stop", "--pid-file", pidPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("stop command: %v", err)
	}
	if !strings.Contains(out.String(), "stale") {
		t.Fatalf("expected stale pid message, got %q", out.String())
	}
}

func TestAgentStopRunningProcess(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("sh", "-c", "sleep 5")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start process: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	pidPath := filepath.Join(t.TempDir(), "running.pid")
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}

	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"agent", "stop", "--pid-file", pidPath, "--timeout", "2s"})

	if err := root.Execute(); err != nil {
		t.Fatalf("stop command: %v", err)
	}
	if !strings.Contains(out.String(), "stopped") && !strings.Contains(out.String(), "force-killed") {
		t.Fatalf("unexpected stop output: %q", out.String())
	}
}

func TestStartAgentDaemonError(t *testing.T) {
	t.Parallel()

	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"/definitely/missing/bb"}

	command := &cobra.Command{}
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)

	opts := defaultAgentStartOptions()
	opts.taskPrompt = "Fix auth"
	opts.taskRepo = "cerberus"

	err := startAgentDaemon(command, opts)
	if err == nil {
		t.Fatalf("expected daemon start error")
	}
}

func TestRunAgentForegroundViaStartCommand(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	runtime := agent.DefaultRuntimePaths(repoDir)

	opts := agentStartOptions{
		sprite:            "bramble",
		repoDir:           repoDir,
		agentKind:         string(agent.AgentCodex),
		agentCommand:      "sh",
		agentFlags:        "-c,exit 0",
		taskPrompt:        "Fix auth",
		taskRepo:          "cerberus",
		taskBranch:        "feature/auth",
		eventLog:          runtime.EventLog,
		outputLog:         runtime.OutputLog,
		pidFile:           runtime.PIDFile,
		stateFile:         runtime.StateFile,
		heartbeatInterval: 20 * time.Millisecond,
		progressInterval:  20 * time.Millisecond,
		stallTimeout:      time.Minute,
		restartDelay:      10 * time.Millisecond,
		shutdownGrace:     20 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	root := newRootCommand()
	root.SetArgs([]string{
		"agent", "start",
		"--foreground",
		"--sprite", opts.sprite,
		"--repo-dir", opts.repoDir,
		"--agent", opts.agentKind,
		"--agent-command", opts.agentCommand,
		"--agent-flags", opts.agentFlags,
		"--task-prompt", opts.taskPrompt,
		"--task-repo", opts.taskRepo,
		"--task-branch", opts.taskBranch,
		"--event-log", opts.eventLog,
		"--output-log", opts.outputLog,
		"--pid-file", opts.pidFile,
		"--state-file", opts.stateFile,
		"--heartbeat-interval", opts.heartbeatInterval.String(),
		"--progress-interval", opts.progressInterval.String(),
		"--stall-timeout", opts.stallTimeout.String(),
		"--restart-delay", opts.restartDelay.String(),
		"--shutdown-grace", opts.shutdownGrace.String(),
	})

	err := root.ExecuteContext(ctx)
	if err == nil {
		t.Fatalf("expected interrupted foreground run")
	}
}

func TestHelpers(t *testing.T) {
	t.Parallel()

	if got := parseCSV("a, b,,c"); len(got) != 3 {
		t.Fatalf("unexpected csv parse: %#v", got)
	}
	env := parseEnvAssignments("A=1, B=2, malformed")
	if env["A"] != "1" || env["B"] != "2" {
		t.Fatalf("unexpected env parse: %#v", env)
	}

	if got := resolveRepoDir("."); got == "" {
		t.Fatalf("expected resolved repo dir")
	}
}

func TestEnvParsersFallback(t *testing.T) {
	t.Setenv("TEST_DURATION", "bad")
	t.Setenv("TEST_INT", "bad")
	t.Setenv("TEST_BOOL", "bad")

	if got := envDurationOrDefault("TEST_DURATION", time.Minute); got != time.Minute {
		t.Fatalf("expected duration fallback")
	}
	if got := envIntOrDefault("TEST_INT", 7); got != 7 {
		t.Fatalf("expected int fallback")
	}
	if got := envBoolOrDefault("TEST_BOOL", true); !got {
		t.Fatalf("expected bool fallback")
	}
}

func TestReadTailLines(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "log")
	if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	lines, err := readTailLines(path, 2)
	if err != nil {
		t.Fatalf("read tail: %v", err)
	}
	if len(lines) != 2 || lines[0] != "b" || lines[1] != "c" {
		t.Fatalf("unexpected tail lines: %#v", lines)
	}
}

func TestFollowLogContextCancel(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "log")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var out bytes.Buffer
	if err := followLog(ctx, path, &out); err != nil {
		t.Fatalf("follow log: %v", err)
	}
}

func TestFollowLogReceivesAppendedLine(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "log")
	if err := os.WriteFile(path, []byte("existing\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- followLog(ctx, path, &out)
	}()

	time.Sleep(100 * time.Millisecond)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open log for append: %v", err)
	}
	_, _ = file.WriteString("new-line\n")
	_ = file.Close()

	time.Sleep(250 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("followLog error: %v", err)
	}
	if !strings.Contains(out.String(), "new-line") {
		t.Fatalf("expected followed line, got %q", out.String())
	}
}

func TestResolveRepoDirVariants(t *testing.T) {
	t.Parallel()

	if got := resolveRepoDir("/tmp/repo"); got != "/tmp/repo" {
		t.Fatalf("absolute repo dir should pass through")
	}
	if got := resolveRepoDir(""); got == "" {
		t.Fatalf("empty repo dir should resolve to cwd")
	}
}
