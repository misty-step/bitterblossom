package agent

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/pkg/events"
)

type staticGitClient struct {
	snapshot GitSnapshot
	err      error
}

func (s staticGitClient) Snapshot(context.Context) (GitSnapshot, error) {
	if s.err != nil {
		return GitSnapshot{}, s.err
	}
	return s.snapshot, nil
}

type staticSampler struct {
	usage ProcessUsage
	err   error
}

func (s staticSampler) Sample(context.Context, int) (ProcessUsage, error) {
	if s.err != nil {
		return ProcessUsage{}, s.err
	}
	return s.usage, nil
}

func TestAgentConfigValidateInvalidKind(t *testing.T) {
	t.Parallel()

	cfg := AgentConfig{Kind: AgentKind("bogus"), Assignment: TaskAssignment{Prompt: "x", Repo: "r"}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected invalid kind error")
	}
}

func TestAgentConfigDefaultCommandsForKinds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind AgentKind
		want string
	}{
		{AgentKimi, "kimi-code"},
		{AgentClaude, "claude"},
	}

	for _, tc := range cases {
		cmd, _, err := (AgentConfig{Kind: tc.kind, Assignment: TaskAssignment{Prompt: "x", Repo: "r"}}).CommandAndArgs()
		if err != nil {
			t.Fatalf("command and args: %v", err)
		}
		if cmd != tc.want {
			t.Fatalf("unexpected default command for %s: got %s want %s", tc.kind, cmd, tc.want)
		}
	}
}

func TestNormalizeDetailTruncates(t *testing.T) {
	t.Parallel()

	long := bytes.Repeat([]byte("a"), 300)
	if got := normalizeDetail(string(long)); len(got) != 240 {
		t.Fatalf("expected 240 chars, got %d", len(got))
	}
}

func TestProgressLastActivityTime(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 7, 20, 0, 0, 0, time.UTC)
	monitor := NewProgressMonitor(ProgressConfig{Sprite: "bramble"}, &recordingEventEmitter{})
	monitor.now = func() time.Time { return now }
	monitor.git = &sequenceGitClient{snapshots: []GitSnapshot{{Branch: "main", Head: "a", Branches: []string{"main"}, CommitCount: 1}}}

	monitor.poll(context.Background())
	first := monitor.LastActivityTime()
	if first.IsZero() {
		t.Fatalf("expected git activity timestamp")
	}

	now = now.Add(time.Second)
	monitor.ObserveOutput("go test ./...", false)
	latest := monitor.LastActivityTime()
	if !latest.After(first) {
		t.Fatalf("expected output activity to advance last activity")
	}
}

func TestGitCLIRequiresRepoDir(t *testing.T) {
	t.Parallel()

	client := &gitCLI{repoDir: "", runner: execRunner{}}
	if _, err := client.git(context.Background(), "status"); err == nil {
		t.Fatalf("expected repo-dir error")
	}
}

func TestPSSamplerParseErrors(t *testing.T) {
	t.Parallel()

	sampler := &psSampler{runner: fakeRunner{output: "bad output"}}
	if _, err := sampler.Sample(context.Background(), 999); err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestApplyDefaults(t *testing.T) {
	t.Parallel()

	cfg := applyDefaults(SupervisorConfig{RepoDir: "/tmp/repo"})
	if cfg.SpriteName == "" {
		t.Fatalf("expected sprite default")
	}
	if cfg.HeartbeatInterval != DefaultHeartbeatInterval {
		t.Fatalf("unexpected heartbeat default")
	}
	if cfg.ProgressInterval != DefaultProgressInterval {
		t.Fatalf("unexpected progress default")
	}
	if cfg.StallTimeout != DefaultStallTimeout {
		t.Fatalf("unexpected stall default")
	}
	if cfg.RestartDelay != DefaultRestartDelay {
		t.Fatalf("unexpected restart delay default")
	}
	if cfg.ShutdownGracePeriod != DefaultShutdownGracePeriod {
		t.Fatalf("unexpected shutdown default")
	}
	if cfg.Stdout == nil || cfg.Stderr == nil {
		t.Fatalf("expected stdio defaults")
	}
}

func TestSupervisorOptionsApply(t *testing.T) {
	t.Parallel()

	signalCh := make(chan os.Signal, 1)
	now := time.Date(2026, 2, 7, 20, 0, 0, 0, time.UTC)
	git := staticGitClient{snapshot: GitSnapshot{Branch: "main", Head: "abc"}}
	sampler := staticSampler{usage: ProcessUsage{CPUPercent: 1.5, MemoryBytes: 1234}}

	supervisor := NewSupervisor(SupervisorConfig{
		Agent: AgentConfig{Kind: AgentCodex, Assignment: TaskAssignment{Prompt: "x", Repo: "r"}},
	},
		WithSignalChannel(signalCh),
		WithGitClient(git),
		WithProcessSampler(sampler),
		WithClock(func() time.Time { return now }),
	)

	if supervisor.signalCh != signalCh {
		t.Fatalf("signal channel override not applied")
	}
	if got := supervisor.now(); !got.Equal(now) {
		t.Fatalf("clock override not applied")
	}
	if _, err := supervisor.gitClient.Snapshot(context.Background()); err != nil {
		t.Fatalf("git override not applied: %v", err)
	}
	if _, err := supervisor.sampler.Sample(context.Background(), 1); err != nil {
		t.Fatalf("sampler override not applied: %v", err)
	}
}

func TestSupervisorStopAgentForceKill(t *testing.T) {
	t.Parallel()

	cmd, stdout, stderr, err := launchAgentProcess("sh", []string{"-c", "trap '' TERM; sleep 5"}, "", os.Environ())
	if err != nil {
		t.Fatalf("launch process: %v", err)
	}
	defer stdout.Close()
	defer stderr.Close()

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	supervisor := NewSupervisor(SupervisorConfig{
		Agent: AgentConfig{Kind: AgentCodex, Assignment: TaskAssignment{Prompt: "x", Repo: "r"}},
		ShutdownGracePeriod: 20 * time.Millisecond,
	})

	_ = supervisor.stopAgent(cmd, waitCh)
	time.Sleep(10 * time.Millisecond)
	if ProcessRunning(cmd.Process.Pid) {
		t.Fatalf("process should be stopped")
	}
}

func TestNewJSONLEmitterAndClose(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	logPath := filepath.Join(t.TempDir(), "events.jsonl")

	emitter, err := newJSONLEmitter(&stdout, logPath)
	if err != nil {
		t.Fatalf("new emitter: %v", err)
	}
	if err := emitter.Emit(&events.ProgressEvent{Meta: events.Meta{TS: time.Now().UTC(), SpriteName: "bramble", EventKind: events.KindProgress}}); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if err := emitter.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := emitter.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if stdout.Len() == 0 {
		t.Fatalf("expected stdout output")
	}
}

func TestLaunchAgentProcessError(t *testing.T) {
	t.Parallel()

	if _, _, _, err := launchAgentProcess("definitely-not-a-real-command", nil, "", os.Environ()); err == nil {
		t.Fatalf("expected launch error")
	}
}

func TestReadPIDFileParseError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "pid")
	if err := os.WriteFile(path, []byte("not-a-number\n"), 0o644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}
	if _, err := ReadPIDFile(path); err == nil {
		t.Fatalf("expected pid parse error")
	}
}

func TestReadSupervisorStateDecodeError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatalf("write state file: %v", err)
	}
	if _, err := ReadSupervisorState(path); err == nil {
		t.Fatalf("expected decode error")
	}
}

func TestProcessRunningFalse(t *testing.T) {
	t.Parallel()

	if ProcessRunning(-1) {
		t.Fatalf("negative pid should not be running")
	}
}

func TestSupervisorRunInvalidConfig(t *testing.T) {
	t.Parallel()

	supervisor := NewSupervisor(SupervisorConfig{})
	result := supervisor.Run(context.Background())
	if result.State != RunStateError {
		t.Fatalf("expected error run state, got %s", result.State)
	}
}

func TestNewOutputLoggerError(t *testing.T) {
	t.Parallel()

	_, err := newOutputLogger(io.Discard, t.TempDir())
	if err == nil {
		t.Fatalf("expected output logger error")
	}
}
