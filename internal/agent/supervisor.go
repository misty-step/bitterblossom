package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/misty-step/bitterblossom/pkg/events"
)

const (
	DefaultRestartDelay        = 2 * time.Second
	DefaultShutdownGracePeriod = 15 * time.Second
)

// RunState reports terminal supervisor outcomes.
type RunState string

const (
	RunStateStopped     RunState = "stopped"
	RunStateInterrupted RunState = "interrupted"
	RunStateError       RunState = "error"
)

// RunResult is returned when the supervisor exits.
type RunResult struct {
	State    RunState
	Restarts int
	Err      error
}

// ExitCode maps runtime state to process exit status.
func (r RunResult) ExitCode() int {
	switch r.State {
	case RunStateStopped:
		return 0
	case RunStateInterrupted:
		return 130
	default:
		return 1
	}
}

// SupervisorConfig controls daemon behavior on the sprite VM.
type SupervisorConfig struct {
	SpriteName          string
	RepoDir             string
	Agent               AgentConfig
	Runtime             RuntimePaths
	HeartbeatInterval   time.Duration
	ProgressInterval    time.Duration
	StallTimeout        time.Duration
	RestartDelay        time.Duration
	ShutdownGracePeriod time.Duration
	Stdout              io.Writer
	Stderr              io.Writer
}

// SupervisorState is the persisted supervisor status read by bb agent status.
type SupervisorState struct {
	Sprite          string         `json:"sprite"`
	Status          string         `json:"status"`
	SupervisorPID   int            `json:"supervisor_pid"`
	AgentPID        int            `json:"agent_pid,omitempty"`
	Restarts        int            `json:"restarts"`
	StartedAt       time.Time      `json:"started_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	LastHeartbeatAt time.Time      `json:"last_heartbeat_at,omitempty"`
	LastProgressAt  time.Time      `json:"last_progress_at,omitempty"`
	LastActivity    string         `json:"last_activity,omitempty"`
	Stalled         bool           `json:"stalled"`
	LastError       string         `json:"last_error,omitempty"`
	Task            TaskAssignment `json:"task"`
}

// Supervisor runs a coding agent as the sprite-local process manager.
type Supervisor struct {
	cfg SupervisorConfig

	now       func() time.Time
	signalCh  <-chan os.Signal
	launch    func(command string, args []string, dir string, env []string) (*exec.Cmd, io.ReadCloser, io.ReadCloser, error)
	sampler   ProcessSampler
	gitClient GitClient

	stateMu sync.Mutex
	state   SupervisorState

	emitter *jsonlEmitter
	output  *outputLogger
}

// SupervisorOption customizes supervisor dependencies, primarily for tests.
type SupervisorOption func(*Supervisor)

// WithSignalChannel overrides SIGTERM/SIGINT input.
func WithSignalChannel(signalCh <-chan os.Signal) SupervisorOption {
	return func(s *Supervisor) {
		if signalCh != nil {
			s.signalCh = signalCh
		}
	}
}

// WithProcessLauncher overrides child process launch behavior.
func WithProcessLauncher(launcher func(command string, args []string, dir string, env []string) (*exec.Cmd, io.ReadCloser, io.ReadCloser, error)) SupervisorOption {
	return func(s *Supervisor) {
		if launcher != nil {
			s.launch = launcher
		}
	}
}

// WithGitClient overrides git status collection.
func WithGitClient(client GitClient) SupervisorOption {
	return func(s *Supervisor) {
		if client != nil {
			s.gitClient = client
		}
	}
}

// WithProcessSampler overrides process CPU/memory sampling.
func WithProcessSampler(sampler ProcessSampler) SupervisorOption {
	return func(s *Supervisor) {
		if sampler != nil {
			s.sampler = sampler
		}
	}
}

// WithClock overrides time source.
func WithClock(now func() time.Time) SupervisorOption {
	return func(s *Supervisor) {
		if now != nil {
			s.now = now
		}
	}
}

// NewSupervisor creates a daemon supervisor with sane defaults.
func NewSupervisor(cfg SupervisorConfig, opts ...SupervisorOption) *Supervisor {
	cfg = applyDefaults(cfg)

	s := &Supervisor{
		cfg:       cfg,
		now:       time.Now,
		launch:    launchAgentProcess,
		sampler:   newPSSampler(),
		gitClient: newGitCLI(cfg.RepoDir),
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(s)
	}
	if s.now == nil {
		s.now = time.Now
	}

	s.state = SupervisorState{
		Sprite:        cfg.SpriteName,
		Status:        "starting",
		SupervisorPID: os.Getpid(),
		StartedAt:     s.now().UTC(),
		UpdatedAt:     s.now().UTC(),
		Task:          cfg.Agent.Assignment,
	}

	return s
}

func applyDefaults(cfg SupervisorConfig) SupervisorConfig {
	if strings.TrimSpace(cfg.SpriteName) == "" {
		host, err := os.Hostname()
		if err != nil || strings.TrimSpace(host) == "" {
			host = "sprite"
		}
		cfg.SpriteName = host
	}
	if strings.TrimSpace(cfg.RepoDir) == "" {
		cfg.RepoDir = "."
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = DefaultHeartbeatInterval
	}
	if cfg.ProgressInterval <= 0 {
		cfg.ProgressInterval = DefaultProgressInterval
	}
	if cfg.StallTimeout <= 0 {
		cfg.StallTimeout = DefaultStallTimeout
	}
	if cfg.RestartDelay <= 0 {
		cfg.RestartDelay = DefaultRestartDelay
	}
	if cfg.ShutdownGracePeriod <= 0 {
		cfg.ShutdownGracePeriod = DefaultShutdownGracePeriod
	}
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}
	if cfg.Runtime == (RuntimePaths{}) {
		cfg.Runtime = DefaultRuntimePaths(cfg.RepoDir)
	}
	return cfg
}

// Run starts the supervisor daemon loop and blocks until termination.
func (s *Supervisor) Run(ctx context.Context) (result RunResult) {
	if err := s.cfg.Agent.Validate(); err != nil {
		return RunResult{State: RunStateError, Err: err}
	}

	if err := s.ensureRuntimeDirs(); err != nil {
		return RunResult{State: RunStateError, Err: err}
	}

	emitter, err := newJSONLEmitter(s.cfg.Stdout, s.cfg.Runtime.EventLog)
	if err != nil {
		return RunResult{State: RunStateError, Err: err}
	}
	s.emitter = emitter
	defer func() {
		if err := s.emitter.Close(); err != nil {
			result = appendRunError(result, fmt.Errorf("close event emitter: %w", err))
		}
	}()

	output, err := newOutputLogger(s.cfg.Stderr, s.cfg.Runtime.OutputLog)
	if err != nil {
		return RunResult{State: RunStateError, Err: err}
	}
	s.output = output
	defer func() {
		if err := s.output.Close(); err != nil {
			result = appendRunError(result, fmt.Errorf("close output logger: %w", err))
		}
	}()

	if err := writePIDFile(s.cfg.Runtime.PIDFile, os.Getpid()); err != nil {
		return RunResult{State: RunStateError, Err: err}
	}
	defer func() {
		if err := os.Remove(s.cfg.Runtime.PIDFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = appendRunError(result, fmt.Errorf("remove pid file %s: %w", s.cfg.Runtime.PIDFile, err))
		}
	}()

	if err := s.persistState(); err != nil {
		return RunResult{State: RunStateError, Err: err}
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	signalCh := s.signalCh
	if signalCh == nil {
		defaultSignalCh := make(chan os.Signal, 2)
		signal.Notify(defaultSignalCh, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(defaultSignalCh)
		signalCh = defaultSignalCh
	}

	_ = s.emitter.Emit(&events.DispatchEvent{
		Meta:   events.Meta{TS: s.now().UTC(), SpriteName: s.cfg.SpriteName, EventKind: events.KindDispatch},
		Task:   s.cfg.Agent.Assignment.Prompt,
		Repo:   s.cfg.Agent.Assignment.Repo,
		Branch: s.cfg.Agent.Assignment.Branch,
	})

	progress := NewProgressMonitor(ProgressConfig{
		Sprite:       s.cfg.SpriteName,
		RepoDir:      s.cfg.RepoDir,
		PollInterval: s.cfg.ProgressInterval,
		StallTimeout: s.cfg.StallTimeout,
		OnActivity: func(activity string, at time.Time, stalled bool) {
			s.updateState(func(state *SupervisorState) {
				state.LastProgressAt = at
				state.LastActivity = activity
				state.Stalled = stalled
				if stalled {
					state.Status = "stalled"
				} else if state.Status != "running" {
					state.Status = "running"
				}
			})
		},
	}, s.emitter)
	if s.gitClient != nil {
		progress.git = s.gitClient
	}

	heartbeat := NewHeartbeat(s.cfg.HeartbeatInterval, s.cfg.SpriteName, &heartbeatSource{supervisor: s, progress: progress}, s.emitter, func(at time.Time) {
		s.updateState(func(state *SupervisorState) {
			state.LastHeartbeatAt = at
		})
	})
	heartbeat.now = s.now

	var workers sync.WaitGroup
	workers.Add(2)
	go progress.Run(runCtx, &workers)
	go heartbeat.Run(runCtx, &workers)

	restarts := 0

	for {
		command, args, err := s.cfg.Agent.CommandAndArgs()
		if err != nil {
			cancel()
			workers.Wait()
			s.updateState(func(state *SupervisorState) {
				state.Status = "error"
				state.LastError = err.Error()
			})
			return RunResult{State: RunStateError, Restarts: restarts, Err: err}
		}

		cmd, stdout, stderr, err := s.launch(command, args, s.cfg.RepoDir, s.cfg.Agent.BuildEnvironment())
		if err != nil {
			restarts++
			s.updateState(func(state *SupervisorState) {
				state.Restarts = restarts
				state.LastError = fmt.Sprintf("launch agent: %v", err)
				state.Status = "error"
			})
			_ = s.emitter.Emit(&events.ErrorEvent{
				Meta:    events.Meta{TS: s.now().UTC(), SpriteName: s.cfg.SpriteName, EventKind: events.KindError},
				Code:    "launch_failed",
				Message: err.Error(),
			})
			if !sleepOrDone(runCtx, s.cfg.RestartDelay) {
				cancel()
				workers.Wait()
				return RunResult{State: RunStateInterrupted, Restarts: restarts, Err: runCtx.Err()}
			}
			continue
		}

		agentPID := cmd.Process.Pid
		s.updateState(func(state *SupervisorState) {
			state.Status = "running"
			state.AgentPID = agentPID
			state.Stalled = false
			state.Restarts = restarts
		})

		_ = s.emitter.Emit(&events.ProgressEvent{
			Meta:     events.Meta{TS: s.now().UTC(), SpriteName: s.cfg.SpriteName, EventKind: events.KindProgress},
			Branch:   s.cfg.Agent.Assignment.Branch,
			Activity: "agent_started",
			Detail:   fmt.Sprintf("pid=%d command=%s", agentPID, command),
		})

		var streamWG sync.WaitGroup
		streamWG.Add(2)
		go s.consumeOutput(runCtx, stdout, false, progress, &streamWG)
		go s.consumeOutput(runCtx, stderr, true, progress, &streamWG)

		waitCh := make(chan error, 1)
		go func() {
			waitCh <- cmd.Wait()
		}()

		processDone := false
		for !processDone {
			select {
			case sig := <-signalCh:
				cancel()
				s.updateState(func(state *SupervisorState) {
					state.Status = "stopping"
					state.LastError = fmt.Sprintf("received signal %s", sig)
				})
				_ = s.emitter.Emit(&events.ErrorEvent{
					Meta:    events.Meta{TS: s.now().UTC(), SpriteName: s.cfg.SpriteName, EventKind: events.KindError},
					Code:    "signal",
					Message: fmt.Sprintf("received signal %s", sig),
				})
				_ = s.stopAgent(cmd, waitCh)
				streamWG.Wait()
				workers.Wait()
				s.updateState(func(state *SupervisorState) {
					state.Status = "stopped"
					state.AgentPID = 0
				})
				return RunResult{State: RunStateInterrupted, Restarts: restarts, Err: fmt.Errorf("interrupted by %s", sig)}

			case <-runCtx.Done():
				s.updateState(func(state *SupervisorState) {
					state.Status = "stopping"
				})
				_ = s.stopAgent(cmd, waitCh)
				streamWG.Wait()
				workers.Wait()
				s.updateState(func(state *SupervisorState) {
					state.Status = "stopped"
					state.AgentPID = 0
				})
				if errors.Is(runCtx.Err(), context.Canceled) {
					return RunResult{State: RunStateInterrupted, Restarts: restarts, Err: runCtx.Err()}
				}
				return RunResult{State: RunStateError, Restarts: restarts, Err: runCtx.Err()}

			case stalled := <-progress.Signals():
				if stalled.Type == ProgressSignalStalled {
					s.updateState(func(state *SupervisorState) {
						state.Status = "stalled"
						state.Stalled = true
						state.LastError = stalled.Reason
					})
				}

			case waitErr := <-waitCh:
				processDone = true
				streamWG.Wait()
				s.updateState(func(state *SupervisorState) {
					state.AgentPID = 0
				})

				if runCtx.Err() != nil {
					cancel()
					workers.Wait()
					s.updateState(func(state *SupervisorState) {
						state.Status = "stopped"
					})
					return RunResult{State: RunStateInterrupted, Restarts: restarts, Err: runCtx.Err()}
				}

				restarts++
				s.updateState(func(state *SupervisorState) {
					state.Restarts = restarts
				})

				if waitErr == nil {
					_ = s.emitter.Emit(&events.ProgressEvent{
						Meta:     events.Meta{TS: s.now().UTC(), SpriteName: s.cfg.SpriteName, EventKind: events.KindProgress},
						Branch:   s.cfg.Agent.Assignment.Branch,
						Activity: "agent_exited",
						Detail:   "agent exited cleanly, restarting",
					})
					if !sleepOrDone(runCtx, s.cfg.RestartDelay) {
						cancel()
						workers.Wait()
						return RunResult{State: RunStateInterrupted, Restarts: restarts, Err: runCtx.Err()}
					}
					continue
				}

				s.updateState(func(state *SupervisorState) {
					state.Status = "error"
					state.LastError = waitErr.Error()
				})
				_ = s.emitter.Emit(&events.ErrorEvent{
					Meta:    events.Meta{TS: s.now().UTC(), SpriteName: s.cfg.SpriteName, EventKind: events.KindError},
					Code:    "agent_crash",
					Message: waitErr.Error(),
				})
				if !sleepOrDone(runCtx, s.cfg.RestartDelay) {
					cancel()
					workers.Wait()
					return RunResult{State: RunStateInterrupted, Restarts: restarts, Err: runCtx.Err()}
				}
			}
		}
	}
}

func appendRunError(result RunResult, err error) RunResult {
	if err == nil {
		return result
	}
	if result.Err == nil {
		result.Err = err
	} else {
		result.Err = errors.Join(result.Err, err)
	}
	if result.State == "" {
		result.State = RunStateError
	}
	return result
}

func (s *Supervisor) stopAgent(cmd *exec.Cmd, waitCh <-chan error) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	pid := cmd.Process.Pid
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	_ = cmd.Process.Signal(syscall.SIGTERM)

	timer := time.NewTimer(s.cfg.ShutdownGracePeriod)
	defer timer.Stop()

	select {
	case err := <-waitCh:
		return err
	case <-timer.C:
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = cmd.Process.Kill()
		return <-waitCh
	}
}

func (s *Supervisor) consumeOutput(ctx context.Context, reader io.ReadCloser, stderr bool, progress *ProgressMonitor, wg *sync.WaitGroup) {
	defer wg.Done()
	defer func() {
		if err := reader.Close(); err != nil {
			stream := "stdout"
			if stderr {
				stream = "stderr"
			}
			_ = s.emitter.Emit(&events.ErrorEvent{
				Meta:    events.Meta{TS: s.now().UTC(), SpriteName: s.cfg.SpriteName, EventKind: events.KindError},
				Code:    "output_close_failed",
				Message: fmt.Sprintf("close %s reader: %v", stream, err),
			})
		}
	}()

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()
		stream := "stdout"
		if stderr {
			stream = "stderr"
		}
		s.output.WriteLine(s.now().UTC(), stream, line)
		progress.ObserveOutput(line, stderr)
	}
}

func (s *Supervisor) ensureRuntimeDirs() error {
	paths := []string{s.cfg.Runtime.EventLog, s.cfg.Runtime.OutputLog, s.cfg.Runtime.PIDFile, s.cfg.Runtime.StateFile}
	for _, path := range paths {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create runtime directory %s: %w", dir, err)
		}
	}
	return nil
}

func (s *Supervisor) updateState(mutator func(*SupervisorState)) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	if mutator != nil {
		mutator(&s.state)
	}
	s.state.UpdatedAt = s.now().UTC()
	_ = writeStateFile(s.cfg.Runtime.StateFile, s.state)
}

func (s *Supervisor) persistState() error {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.state.UpdatedAt = s.now().UTC()
	return writeStateFile(s.cfg.Runtime.StateFile, s.state)
}

// HeartbeatSnapshot implements HeartbeatSource.
func (s *Supervisor) HeartbeatSnapshot(ctx context.Context) (HeartbeatSnapshot, error) {
	s.stateMu.Lock()
	state := s.state
	s.stateMu.Unlock()

	usage, err := s.sampler.Sample(ctx, state.AgentPID)
	if err != nil {
		return HeartbeatSnapshot{}, err
	}

	gitStatus, err := s.gitClient.Snapshot(ctx)
	if err != nil {
		return HeartbeatSnapshot{}, err
	}

	return HeartbeatSnapshot{
		UptimeSeconds:      int64(s.now().UTC().Sub(state.StartedAt).Seconds()),
		AgentPID:           state.AgentPID,
		CPUPercent:         usage.CPUPercent,
		MemoryBytes:        usage.MemoryBytes,
		Branch:             gitStatus.Branch,
		LastCommit:         shortHash(gitStatus.Head),
		UncommittedChanges: gitStatus.Uncommitted,
	}, nil
}

type heartbeatSource struct {
	supervisor *Supervisor
	progress   *ProgressMonitor
}

func (h *heartbeatSource) HeartbeatSnapshot(ctx context.Context) (HeartbeatSnapshot, error) {
	stateSnapshot, err := h.supervisor.HeartbeatSnapshot(ctx)
	if err != nil {
		return HeartbeatSnapshot{}, err
	}
	if progressSnapshot, ok := h.progress.Snapshot(); ok {
		stateSnapshot.Branch = progressSnapshot.Branch
		stateSnapshot.LastCommit = shortHash(progressSnapshot.Head)
		stateSnapshot.UncommittedChanges = progressSnapshot.Uncommitted
	}
	return stateSnapshot, nil
}

func sleepOrDone(ctx context.Context, duration time.Duration) bool {
	if duration <= 0 {
		return true
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func launchAgentProcess(command string, args []string, dir string, env []string) (*exec.Cmd, io.ReadCloser, io.ReadCloser, error) {
	cmd := exec.Command(command, args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, err
	}
	return cmd, stdout, stderr, nil
}

// ReadSupervisorState decodes supervisor state from disk.
func ReadSupervisorState(path string) (SupervisorState, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return SupervisorState{}, err
	}
	var state SupervisorState
	if err := json.Unmarshal(payload, &state); err != nil {
		return SupervisorState{}, fmt.Errorf("decode state file: %w", err)
	}
	return state, nil
}

func writeStateFile(path string, state SupervisorState) error {
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, append(payload, '\n'), 0o644); err != nil {
		return fmt.Errorf("write state tmp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename state tmp: %w", err)
	}
	return nil
}

// ReadPIDFile returns the pid stored at path.
func ReadPIDFile(path string) (int, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(payload)))
	if err != nil {
		return 0, fmt.Errorf("parse pid %q: %w", strings.TrimSpace(string(payload)), err)
	}
	return pid, nil
}

func writePIDFile(path string, pid int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o644)
}

// ProcessRunning reports whether pid responds to signal 0.
func ProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

type jsonlEmitter struct {
	mu      sync.Mutex
	emitter *events.Emitter
	file    *os.File
}

func newJSONLEmitter(stdout io.Writer, path string) (*jsonlEmitter, error) {
	if stdout == nil {
		stdout = io.Discard
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open event log: %w", err)
	}
	multi := io.MultiWriter(stdout, file)
	emitter, err := events.NewEmitter(multi)
	if err != nil {
		if closeErr := file.Close(); closeErr != nil {
			return nil, errors.Join(err, fmt.Errorf("close event log: %w", closeErr))
		}
		return nil, err
	}
	return &jsonlEmitter{emitter: emitter, file: file}, nil
}

func (e *jsonlEmitter) Emit(event events.Event) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.emitter.Emit(event)
}

func (e *jsonlEmitter) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.file == nil {
		return nil
	}
	err := e.file.Close()
	e.file = nil
	return err
}

type outputLogger struct {
	mu     sync.Mutex
	writer io.Writer
	file   *os.File
}

func newOutputLogger(stderr io.Writer, path string) (*outputLogger, error) {
	if stderr == nil {
		stderr = io.Discard
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open output log: %w", err)
	}
	return &outputLogger{writer: io.MultiWriter(stderr, file), file: file}, nil
}

func (l *outputLogger) WriteLine(ts time.Time, stream, line string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = fmt.Fprintf(l.writer, "%s [%s] %s\n", ts.Format(time.RFC3339), stream, line)
}

func (l *outputLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}
