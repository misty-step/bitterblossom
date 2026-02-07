package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/misty-step/bitterblossom/internal/clients"
)

var (
	// ErrPromptMissing is returned when PROMPT.md is missing.
	ErrPromptMissing = errors.New("prompt file missing")
)

// Config controls supervisor behavior.
type Config struct {
	SpriteName       string
	Workspace        string
	PromptFile       string
	LogFile          string
	EventFile        string
	ClaudeCommand    string
	MaxIterations    int
	RestartDelay     time.Duration
	HeartbeatEvery   time.Duration
	GitScanEvery     time.Duration
	AutoPushEvery    time.Duration
	AutoPush         bool
	DryRun           bool
	PushRemote       string
	StopOnTaskSignal bool
}

// Dependencies are external integrations.
type Dependencies struct {
	Runner  clients.Runner
	Git     clients.GitClient
	Health  HealthCollector
	Logger  *slog.Logger
	Stdout  io.Writer
	NowFunc func() time.Time
}

// Supervisor coordinates the long-running agent loop.
type Supervisor struct {
	cfg  Config
	dep  Dependencies
	sink EventSink
}

// RunDaemon starts the supervisor with SIGINT/SIGTERM handling.
func RunDaemon(parent context.Context, cfg Config, dep Dependencies) error {
	cfg = withDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		return err
	}
	if dep.Runner == nil {
		dep.Runner = clients.ExecRunner{}
	}
	if dep.Git == nil {
		dep.Git = clients.NewGitCLI(dep.Runner, "git")
	}
	if dep.Logger == nil {
		dep.Logger = slog.New(slog.NewJSONHandler(os.Stderr, nil))
	}
	if dep.Stdout == nil {
		dep.Stdout = os.Stdout
	}
	if dep.NowFunc == nil {
		dep.NowFunc = time.Now
	}
	if dep.Health == nil {
		dep.Health = &SystemHealthCollector{
			Workspace: cfg.Workspace,
			Runner:    dep.Runner,
			CPUCores:  runtime.NumCPU(),
		}
	}

	ctx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := os.MkdirAll(filepath.Dir(cfg.EventFile), 0o755); err != nil {
		return fmt.Errorf("create event dir: %w", err)
	}
	fh, err := os.OpenFile(cfg.EventFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open event file: %w", err)
	}
	defer func() { _ = fh.Close() }()

	supervisor := &Supervisor{
		cfg:  cfg,
		dep:  dep,
		sink: NewNDJSONSink(io.MultiWriter(fh, dep.Stdout), cfg.SpriteName),
	}
	return supervisor.run(ctx)
}

func (s *Supervisor) run(ctx context.Context) error {
	log := s.dep.Logger.With("component", "agent", "sprite", s.cfg.SpriteName)
	_ = s.sink.Emit("agent_start", map[string]any{
		"workspace": s.cfg.Workspace,
		"dry_run":   s.cfg.DryRun,
	})
	log.Info("agent supervisor started", "workspace", s.cfg.Workspace, "dry_run", s.cfg.DryRun)

	bgCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		s.heartbeatLoop(bgCtx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		s.gitProgressLoop(bgCtx)
	}()

	if s.cfg.AutoPush {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.autoPushLoop(bgCtx)
		}()
	}

	var runErr error
	iteration := 0
	for {
		if ctx.Err() != nil {
			runErr = ctx.Err()
			break
		}
		iteration++
		if s.cfg.MaxIterations > 0 && iteration > s.cfg.MaxIterations {
			_ = s.sink.Emit("max_iterations_reached", map[string]any{"max": s.cfg.MaxIterations})
			break
		}

		if s.cfg.StopOnTaskSignal {
			signalState, msg := s.controlSignal(ctx)
			if signalState != "working" {
				_ = s.sink.Emit(signalState, map[string]any{"message": msg, "iteration": iteration})
				break
			}
		}

		exitCode, iterErr := s.runClaudeIteration(ctx, iteration)
		if iterErr != nil {
			if ctx.Err() != nil {
				runErr = ctx.Err()
				break
			}
			log.Error("claude iteration failed", "iteration", iteration, "exit_code", exitCode, "error", iterErr)
		}

		if !sleepWithContext(ctx, s.cfg.RestartDelay) {
			runErr = ctx.Err()
			break
		}
	}

	cancel()
	wg.Wait()

	shutdownReason := "completed"
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		shutdownReason = runErr.Error()
	}
	_ = s.sink.Emit("agent_shutdown", map[string]any{"reason": shutdownReason})
	log.Info("agent supervisor stopped", "reason", shutdownReason)

	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		return runErr
	}
	return nil
}

func (s *Supervisor) runClaudeIteration(ctx context.Context, iteration int) (int, error) {
	promptPath := filepath.Join(s.cfg.Workspace, s.cfg.PromptFile)
	if _, err := os.Stat(promptPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			_ = s.sink.Emit("prompt_missing", map[string]any{"path": promptPath})
			return 0, fmt.Errorf("%w: %s", ErrPromptMissing, promptPath)
		}
		return 0, err
	}

	_ = s.sink.Emit("iteration_start", map[string]any{"iteration": iteration})
	if s.cfg.DryRun {
		_ = s.sink.Emit("iteration_dry_run", map[string]any{"iteration": iteration, "command": s.cfg.ClaudeCommand})
		return 0, nil
	}

	shell := fmt.Sprintf("cd %q && cat %q | %s >> %q 2>&1", s.cfg.Workspace, s.cfg.PromptFile, s.cfg.ClaudeCommand, s.cfg.LogFile)
	out, code, err := s.dep.Runner.Run(ctx, "bash", "-lc", shell)
	metadata := map[string]any{
		"iteration": iteration,
		"exit_code": code,
	}
	if trimmed := strings.TrimSpace(out); trimmed != "" {
		metadata["output"] = trimmed
	}
	_ = s.sink.Emit("iteration_end", metadata)
	if err != nil {
		return code, err
	}
	return code, nil
}

func (s *Supervisor) controlSignal(ctx context.Context) (string, string) {
	completePath := filepath.Join(s.cfg.Workspace, "TASK_COMPLETE")
	if msg, ok := readIfExists(completePath); ok {
		return "task_complete", msg
	}
	blockedPath := filepath.Join(s.cfg.Workspace, "BLOCKED.md")
	if msg, ok := readIfExists(blockedPath); ok {
		return "blocked", msg
	}
	if ctx.Err() != nil {
		return "shutdown", ctx.Err().Error()
	}
	return "working", ""
}

func (s *Supervisor) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.HeartbeatEvery)
	defer ticker.Stop()
	iteration := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			iteration++
			running := s.claudeRunning(ctx)
			snap := s.dep.Health.Collect(ctx, iteration, running)
			_ = s.sink.Emit("heartbeat", map[string]any{
				"cpu":             snap.CPUPercent,
				"memory":          snap.MemoryPercent,
				"disk":            snap.DiskPercent,
				"claude_running":  snap.ClaudeRunning,
				"loop_iteration":  snap.LoopIteration,
				"workspace_bytes": snap.WorkspaceBytes,
			})
		}
	}
}

func (s *Supervisor) gitProgressLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.GitScanEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			progress, err := s.dep.Git.CollectProgress(ctx, s.cfg.Workspace)
			if err != nil {
				_ = s.sink.Emit("git_scan_error", map[string]any{"error": err.Error()})
				continue
			}
			for _, repo := range progress {
				_ = s.sink.Emit("git_progress", map[string]any{
					"repo":            repo.Name,
					"branch":          repo.Branch,
					"ahead":           repo.Ahead,
					"has_uncommitted": repo.HasUncommitted,
					"last_commit":     repo.LastCommitEpoch,
				})
			}
		}
	}
}

func (s *Supervisor) autoPushLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.AutoPushEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			progress, err := s.dep.Git.CollectProgress(ctx, s.cfg.Workspace)
			if err != nil {
				_ = s.sink.Emit("autopush_scan_error", map[string]any{"error": err.Error()})
				continue
			}
			for _, repo := range progress {
				if repo.Ahead <= 0 || repo.Branch == "" {
					continue
				}
				if s.cfg.DryRun {
					_ = s.sink.Emit("autopush_dry_run", map[string]any{"repo": repo.Name, "branch": repo.Branch, "ahead": repo.Ahead})
					continue
				}
				err := s.dep.Git.Push(ctx, repo.Path, s.cfg.PushRemote, repo.Branch)
				if err != nil {
					_ = s.sink.Emit("autopush_error", map[string]any{"repo": repo.Name, "branch": repo.Branch, "error": err.Error()})
					continue
				}
				_ = s.sink.Emit("autopush_success", map[string]any{"repo": repo.Name, "branch": repo.Branch, "ahead": repo.Ahead})
			}
		}
	}
}

func (s *Supervisor) claudeRunning(ctx context.Context) bool {
	out, _, err := s.dep.Runner.Run(ctx, "bash", "-lc", "pgrep -c claude 2>/dev/null || echo 0")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line != "0" {
			return true
		}
	}
	return false
}

func withDefaults(cfg Config) Config {
	if cfg.Workspace == "" {
		cfg.Workspace = "/home/sprite/workspace"
	}
	if cfg.PromptFile == "" {
		cfg.PromptFile = "PROMPT.md"
	}
	if cfg.LogFile == "" {
		cfg.LogFile = filepath.Join(cfg.Workspace, "ralph.log")
	}
	if cfg.EventFile == "" {
		cfg.EventFile = filepath.Join(cfg.Workspace, "sprite-agent-events.ndjson")
	}
	if cfg.ClaudeCommand == "" {
		cfg.ClaudeCommand = "claude -p --permission-mode bypassPermissions"
	}
	if cfg.RestartDelay <= 0 {
		cfg.RestartDelay = 5 * time.Second
	}
	if cfg.HeartbeatEvery <= 0 {
		cfg.HeartbeatEvery = 30 * time.Second
	}
	if cfg.GitScanEvery <= 0 {
		cfg.GitScanEvery = 45 * time.Second
	}
	if cfg.AutoPushEvery <= 0 {
		cfg.AutoPushEvery = 2 * time.Minute
	}
	if cfg.PushRemote == "" {
		cfg.PushRemote = "origin"
	}
	return cfg
}

func validateConfig(cfg Config) error {
	if cfg.Workspace == "" {
		return fmt.Errorf("workspace required")
	}
	if cfg.SpriteName == "" {
		return fmt.Errorf("sprite name required")
	}
	return nil
}

func readIfExists(path string) (string, bool) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	msg := strings.TrimSpace(string(payload))
	return msg, true
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
