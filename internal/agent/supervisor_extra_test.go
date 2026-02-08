package agent

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWithProcessLauncherOptionAndStopAgentNil(t *testing.T) {
	t.Parallel()

	called := false
	launcher := func(string, []string, string, []string) (*exec.Cmd, io.ReadCloser, io.ReadCloser, error) {
		called = true
		return nil, nil, nil, errors.New("unused")
	}

	// Bridge type check via real signature.
	wrapped := launcher

	supervisor := NewSupervisor(SupervisorConfig{
		Agent: AgentConfig{Kind: AgentCodex, Assignment: TaskAssignment{Prompt: "p", Repo: "r"}},
	}, WithProcessLauncher(func(command string, args []string, dir string, env []string) (*exec.Cmd, io.ReadCloser, io.ReadCloser, error) {
		return wrapped(command, args, dir, env)
	}))

	if _, _, _, err := supervisor.launch("", nil, "", nil); err == nil {
		t.Fatal("expected custom launcher error")
	}
	if !called {
		t.Fatal("expected custom launcher to be invoked")
	}

	if err := supervisor.stopAgent(nil, nil); err != nil {
		t.Fatalf("stopAgent(nil) error = %v", err)
	}
}

func TestSupervisorRunLaunchFailureInterrupted(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	runtime := DefaultRuntimePaths(repoDir)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	supervisor := NewSupervisor(SupervisorConfig{
		SpriteName: "bramble",
		RepoDir:    repoDir,
		Agent: AgentConfig{
			Kind:       AgentCodex,
			Assignment: TaskAssignment{Prompt: "Fix auth", Repo: "cerberus"},
		},
		Runtime:             runtime,
		HeartbeatInterval:   20 * time.Millisecond,
		ProgressInterval:    20 * time.Millisecond,
		StallTimeout:        time.Minute,
		RestartDelay:        500 * time.Millisecond,
		ShutdownGracePeriod: 20 * time.Millisecond,
		Stdout:              io.Discard,
		Stderr:              io.Discard,
	}, WithProcessLauncher(func(string, []string, string, []string) (*exec.Cmd, io.ReadCloser, io.ReadCloser, error) {
		return nil, nil, nil, errors.New("launch failed")
	}))

	result := supervisor.Run(ctx)
	if result.State != RunStateInterrupted {
		t.Fatalf("Run() state = %s, want interrupted", result.State)
	}
	if result.Restarts == 0 {
		t.Fatalf("expected restarts > 0 on repeated launch failure")
	}
}

func TestSupervisorRunSignalAndCleanExitPaths(t *testing.T) {
	t.Parallel()

	t.Run("signal interrupt", func(t *testing.T) {
		t.Parallel()

		repoDir := setupGitRepo(t)
		runtime := DefaultRuntimePaths(repoDir)
		signalCh := make(chan os.Signal, 1)

		supervisor := NewSupervisor(SupervisorConfig{
			SpriteName: "bramble",
			RepoDir:    repoDir,
			Agent: AgentConfig{
				Kind:    AgentCodex,
				Command: "sh",
				Flags:   []string{"-c", "sleep 5"},
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
			Stdout:              io.Discard,
			Stderr:              io.Discard,
		}, WithSignalChannel(signalCh))

		go func() {
			time.Sleep(100 * time.Millisecond)
			signalCh <- os.Interrupt
		}()

		result := supervisor.Run(context.Background())
		if result.State != RunStateInterrupted {
			t.Fatalf("Run() state = %s, want interrupted", result.State)
		}
		if result.Err == nil || !strings.Contains(result.Err.Error(), "interrupted by") {
			t.Fatalf("Run() err = %v, want interrupted-by message", result.Err)
		}
	})

	t.Run("clean exit restart", func(t *testing.T) {
		t.Parallel()

		repoDir := setupGitRepo(t)
		runtime := DefaultRuntimePaths(repoDir)
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
		defer cancel()

		supervisor := NewSupervisor(SupervisorConfig{
			SpriteName: "bramble",
			RepoDir:    repoDir,
			Agent: AgentConfig{
				Kind:    AgentCodex,
				Command: "sh",
				Flags:   []string{"-c", "exit 0"},
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
			Stdout:              io.Discard,
			Stderr:              io.Discard,
		})

		result := supervisor.Run(ctx)
		if result.State != RunStateInterrupted {
			t.Fatalf("Run() state = %s, want interrupted", result.State)
		}
		if result.Restarts == 0 {
			t.Fatalf("expected at least one restart from clean process exits")
		}
	})
}

func TestWriteStateFileRenameError(t *testing.T) {
	t.Parallel()

	dirAsPath := filepath.Join(t.TempDir(), "state-dir")
	if err := os.MkdirAll(dirAsPath, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	err := writeStateFile(dirAsPath, SupervisorState{Sprite: "bramble"})
	if err == nil || !strings.Contains(err.Error(), "rename state tmp") {
		t.Fatalf("writeStateFile() error = %v, want rename state tmp error", err)
	}
}

func TestPSSamplerRunnerNilAndRunnerError(t *testing.T) {
	t.Parallel()

	sampler := &psSampler{}
	_, _ = sampler.Sample(context.Background(), 99999999)
	if sampler.runner == nil {
		t.Fatal("Sample() should initialize default runner when nil")
	}

	sampler = &psSampler{runner: fakeRunner{err: errors.New("ps failed")}}
	if _, err := sampler.Sample(context.Background(), 42); err == nil {
		t.Fatal("Sample() expected runner error")
	}
}

func TestProgressSnapshotBeforeFirstPoll(t *testing.T) {
	t.Parallel()

	monitor := NewProgressMonitor(ProgressConfig{Sprite: "bramble"}, &recordingEventEmitter{})
	if _, ok := monitor.Snapshot(); ok {
		t.Fatal("Snapshot() should return ok=false before first poll")
	}
}
