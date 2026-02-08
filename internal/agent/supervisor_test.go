package agent

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/pkg/events"
)

func TestRunResultExitCode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		state RunState
		want  int
	}{
		{RunStateStopped, 0},
		{RunStateInterrupted, 130},
		{RunStateError, 1},
	}

	for _, tc := range cases {
		if got := (RunResult{State: tc.state}).ExitCode(); got != tc.want {
			t.Fatalf("unexpected exit code for %s: got %d want %d", tc.state, got, tc.want)
		}
	}
}

func TestSupervisorRunWritesArtifactsAndEmitsEvents(t *testing.T) {
	t.Parallel()

	repoDir := setupGitRepo(t)
	runtime := DefaultRuntimePaths(repoDir)

	cfg := SupervisorConfig{
		SpriteName: "bramble",
		RepoDir:    repoDir,
		Agent: AgentConfig{
			Kind:    AgentCodex,
			Command: "sh",
			Flags: []string{"-c", "echo go test ./...; echo build failed 1>&2; exit 1"},
			Assignment: TaskAssignment{
				Prompt: "Fix auth",
				Repo:   "cerberus",
				Branch: "feature/auth",
			},
		},
		Runtime:             runtime,
		HeartbeatInterval:   20 * time.Millisecond,
		ProgressInterval:    20 * time.Millisecond,
		StallTimeout:        time.Minute,
		RestartDelay:        10 * time.Millisecond,
		ShutdownGracePeriod: 20 * time.Millisecond,
		Stdout:              ioDiscard{},
		Stderr:              ioDiscard{},
	}

	supervisor := NewSupervisor(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result := supervisor.Run(ctx)
	if result.State != RunStateInterrupted {
		t.Fatalf("expected interrupted result, got %s (%v)", result.State, result.Err)
	}
	if result.Restarts == 0 {
		t.Fatalf("expected at least one restart")
	}

	if _, err := os.Stat(runtime.PIDFile); err == nil {
		t.Fatalf("pid file should be removed on shutdown")
	}

	state, err := ReadSupervisorState(runtime.StateFile)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if state.Sprite != "bramble" {
		t.Fatalf("unexpected state sprite %s", state.Sprite)
	}
	if state.Restarts == 0 {
		t.Fatalf("expected restart count in state")
	}

	eventKinds := readEventKinds(t, runtime.EventLog)
	if !contains(eventKinds, string(events.KindDispatch)) {
		t.Fatalf("expected dispatch event")
	}
	if !contains(eventKinds, string(events.KindError)) {
		t.Fatalf("expected error event from crash restarts")
	}
	if !contains(eventKinds, string(events.KindHeartbeat)) {
		t.Fatalf("expected heartbeat event")
	}

	outputBytes, err := os.ReadFile(runtime.OutputLog)
	if err != nil {
		t.Fatalf("read output log: %v", err)
	}
	output := string(outputBytes)
	if !strings.Contains(output, "go test ./...") {
		t.Fatalf("missing stdout output log entry")
	}
	if !strings.Contains(output, "build failed") {
		t.Fatalf("missing stderr output log entry")
	}
}

func TestReadPIDFileAndProcessRunning(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "pid")
	if err := writePIDFile(path, os.Getpid()); err != nil {
		t.Fatalf("write pid file: %v", err)
	}

	pid, err := ReadPIDFile(path)
	if err != nil {
		t.Fatalf("read pid file: %v", err)
	}
	if pid != os.Getpid() {
		t.Fatalf("unexpected pid %d", pid)
	}
	if !ProcessRunning(pid) {
		t.Fatalf("expected current process to be running")
	}
}

func TestWriteAndReadSupervisorState(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	state := SupervisorState{
		Sprite:        "bramble",
		Status:        "running",
		SupervisorPID: 1,
		AgentPID:      2,
		Restarts:      3,
		StartedAt:     time.Date(2026, 2, 7, 21, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 2, 7, 21, 1, 0, 0, time.UTC),
		Task: TaskAssignment{Prompt: "Fix auth", Repo: "cerberus"},
	}

	if err := writeStateFile(path, state); err != nil {
		t.Fatalf("write state file: %v", err)
	}

	decoded, err := ReadSupervisorState(path)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	if decoded.Status != state.Status || decoded.AgentPID != state.AgentPID {
		t.Fatalf("unexpected decoded state: %+v", decoded)
	}
}

func TestSleepOrDone(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sleepOrDone(ctx, time.Second) {
		t.Fatalf("expected canceled context to stop sleep")
	}

	if !sleepOrDone(context.Background(), time.Millisecond) {
		t.Fatalf("expected timer to complete")
	}
}

func readEventKinds(t *testing.T, path string) []string {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer file.Close()

	reader, err := events.NewReader(file)
	if err != nil {
		t.Fatalf("new event reader: %v", err)
	}

	kinds := make([]string, 0)
	for {
		event, err := reader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("read event: %v", err)
		}
		if event == nil {
			break
		}
		kinds = append(kinds, string(event.Kind()))
	}
	return kinds
}

type ioDiscard struct{}

func (ioDiscard) Write(data []byte) (int, error) {
	return len(data), nil
}

func TestOutputLoggerWriteLine(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "output.log")
	var stderr bytes.Buffer
	logger, err := newOutputLogger(&stderr, filePath)
	if err != nil {
		t.Fatalf("new output logger: %v", err)
	}
	defer logger.Close()

	logger.WriteLine(time.Date(2026, 2, 7, 21, 0, 0, 0, time.UTC), "stdout", "hello")

	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("open output log: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatalf("expected one output line")
	}
	if got := scanner.Text(); !strings.Contains(got, "hello") {
		t.Fatalf("unexpected output line: %s", got)
	}
}
