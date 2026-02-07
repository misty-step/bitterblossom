package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/misty-step/bitterblossom/internal/agent"
	"github.com/misty-step/bitterblossom/internal/contracts"
	"github.com/spf13/cobra"
)

var newSupervisor = func(cfg agent.SupervisorConfig, opts ...agent.SupervisorOption) *agent.Supervisor {
	return agent.NewSupervisor(cfg, opts...)
}

type agentStartOptions struct {
	sprite            string
	repoDir           string
	agentKind         string
	agentCommand      string
	agentFlags        string
	model             string
	yolo              bool
	fullAuto          bool
	envAssignments    string
	passThroughEnv    string
	issueURL          string
	taskPrompt        string
	taskRepo          string
	taskBranch        string
	eventLog          string
	outputLog         string
	pidFile           string
	stateFile         string
	heartbeatInterval time.Duration
	progressInterval  time.Duration
	stallTimeout      time.Duration
	restartDelay      time.Duration
	shutdownGrace     time.Duration
	foreground        bool
	daemonChild       bool
}

type agentStopOptions struct {
	pidFile string
	timeout time.Duration
}

type agentStatusOptions struct {
	stateFile string
	pidFile   string
	json      bool
}

type agentLogsOptions struct {
	outputLog string
	lines     int
	follow    bool
}

func newAgentCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "agent",
		Short: "Manage the on-sprite coding-agent supervisor",
	}
	command.AddCommand(newAgentStartCommand())
	command.AddCommand(newAgentStopCommand())
	command.AddCommand(newAgentStatusCommand())
	command.AddCommand(newAgentLogsCommand())
	return command
}

func newAgentStartCommand() *cobra.Command {
	defaults := defaultAgentStartOptions()
	opts := defaults

	command := &cobra.Command{
		Use:   "start",
		Short: "Start the sprite-local agent supervisor daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(opts.taskPrompt) == "" {
				return &exitError{Code: 1, Err: errors.New("--task-prompt is required")}
			}
			if strings.TrimSpace(opts.taskRepo) == "" {
				return &exitError{Code: 1, Err: errors.New("--task-repo is required")}
			}

			if !opts.foreground && !opts.daemonChild {
				return startAgentDaemon(cmd, opts)
			}

			return runAgentForeground(cmd, opts)
		},
	}

	command.Flags().StringVar(&opts.sprite, "sprite", defaults.sprite, "Sprite name for event metadata")
	command.Flags().StringVar(&opts.repoDir, "repo-dir", defaults.repoDir, "Repository directory to supervise")
	command.Flags().StringVar(&opts.agentKind, "agent", defaults.agentKind, "Agent kind: codex|kimi-code|claude")
	command.Flags().StringVar(&opts.agentCommand, "agent-command", defaults.agentCommand, "Explicit agent executable (optional override)")
	command.Flags().StringVar(&opts.agentFlags, "agent-flags", defaults.agentFlags, "Comma-separated agent flags")
	command.Flags().StringVar(&opts.model, "model", defaults.model, "Agent model selection")
	command.Flags().BoolVar(&opts.yolo, "yolo", defaults.yolo, "Enable --yolo agent mode")
	command.Flags().BoolVar(&opts.fullAuto, "full-auto", defaults.fullAuto, "Enable --full-auto agent mode")
	command.Flags().StringVar(&opts.envAssignments, "env", defaults.envAssignments, "Comma-separated KEY=VALUE environment overrides")
	command.Flags().StringVar(&opts.passThroughEnv, "pass-env", defaults.passThroughEnv, "Comma-separated environment variables to pass through")
	command.Flags().StringVar(&opts.issueURL, "issue-url", defaults.issueURL, "Task issue URL")
	command.Flags().StringVar(&opts.taskPrompt, "task-prompt", defaults.taskPrompt, "Task prompt assigned to the agent")
	command.Flags().StringVar(&opts.taskRepo, "task-repo", defaults.taskRepo, "Task repository name")
	command.Flags().StringVar(&opts.taskBranch, "task-branch", defaults.taskBranch, "Task branch")
	command.Flags().StringVar(&opts.eventLog, "event-log", defaults.eventLog, "JSONL event log path")
	command.Flags().StringVar(&opts.outputLog, "output-log", defaults.outputLog, "Agent output log path")
	command.Flags().StringVar(&opts.pidFile, "pid-file", defaults.pidFile, "Supervisor pid file path")
	command.Flags().StringVar(&opts.stateFile, "state-file", defaults.stateFile, "Supervisor state file path")
	command.Flags().DurationVar(&opts.heartbeatInterval, "heartbeat-interval", defaults.heartbeatInterval, "Heartbeat interval")
	command.Flags().DurationVar(&opts.progressInterval, "progress-interval", defaults.progressInterval, "Progress polling interval")
	command.Flags().DurationVar(&opts.stallTimeout, "stall-timeout", defaults.stallTimeout, "Stall timeout")
	command.Flags().DurationVar(&opts.restartDelay, "restart-delay", defaults.restartDelay, "Delay before process restart")
	command.Flags().DurationVar(&opts.shutdownGrace, "shutdown-grace", defaults.shutdownGrace, "Grace period before force-kill on shutdown")
	command.Flags().BoolVar(&opts.foreground, "foreground", defaults.foreground, "Run supervisor in foreground")
	command.Flags().BoolVar(&opts.daemonChild, "daemon-child", defaults.daemonChild, "internal: daemonized child invocation")

	return command
}

func newAgentStopCommand() *cobra.Command {
	defaults := defaultAgentStopOptions()
	opts := defaults

	command := &cobra.Command{
		Use:   "stop",
		Short: "Gracefully stop the sprite-local supervisor",
		RunE: func(cmd *cobra.Command, args []string) error {
			pid, err := agent.ReadPIDFile(opts.pidFile)
			if err != nil {
				if os.IsNotExist(err) {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "supervisor not running")
					return nil
				}
				return &exitError{Code: 1, Err: fmt.Errorf("read pid file: %w", err)}
			}

			if !agent.ProcessRunning(pid) {
				_ = os.Remove(opts.pidFile)
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "supervisor pid file was stale; cleaned up")
				return nil
			}

			if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
				return &exitError{Code: 1, Err: fmt.Errorf("send SIGTERM to %d: %w", pid, err)}
			}

			deadline := time.Now().Add(opts.timeout)
			for time.Now().Before(deadline) {
				if !agent.ProcessRunning(pid) {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "supervisor %d stopped\n", pid)
					return nil
				}
				time.Sleep(100 * time.Millisecond)
			}

			_ = syscall.Kill(pid, syscall.SIGKILL)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "supervisor %d force-killed after timeout\n", pid)
			return nil
		},
	}

	command.Flags().StringVar(&opts.pidFile, "pid-file", defaults.pidFile, "Supervisor pid file path")
	command.Flags().DurationVar(&opts.timeout, "timeout", defaults.timeout, "Wait time before SIGKILL")

	return command
}

func newAgentStatusCommand() *cobra.Command {
	defaults := defaultAgentStatusOptions()
	opts := defaults

	command := &cobra.Command{
		Use:   "status",
		Short: "Show current supervisor state, task, and progress",
		RunE: func(cmd *cobra.Command, args []string) error {
			state, err := agent.ReadSupervisorState(opts.stateFile)
			if err != nil {
				if os.IsNotExist(err) {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "status: stopped")
					return nil
				}
				return &exitError{Code: 1, Err: fmt.Errorf("read state file: %w", err)}
			}

			pid, _ := agent.ReadPIDFile(opts.pidFile)
			running := agent.ProcessRunning(pid)

			if opts.json {
				payload := struct {
					State   agent.SupervisorState `json:"state"`
					Running bool                  `json:"running"`
				}{State: state, Running: running}
				if err := contracts.WriteJSON(cmd.OutOrStdout(), "agent.status", payload); err != nil {
					return &exitError{Code: 1, Err: err}
				}
				return nil
			}

			runtimeStatus := "stopped"
			if running {
				runtimeStatus = state.Status
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "status: %s\n", runtimeStatus)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "sprite: %s\n", state.Sprite)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "task: %s\n", state.Task.Prompt)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "repo: %s\n", state.Task.Repo)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "branch: %s\n", state.Task.Branch)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "supervisor_pid: %d\n", state.SupervisorPID)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "agent_pid: %d\n", state.AgentPID)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "restarts: %d\n", state.Restarts)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "last_activity: %s\n", state.LastActivity)
			if !state.LastProgressAt.IsZero() {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "last_progress: %s\n", state.LastProgressAt.Format(time.RFC3339))
			}
			if !state.LastHeartbeatAt.IsZero() {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "last_heartbeat: %s\n", state.LastHeartbeatAt.Format(time.RFC3339))
			}
			if state.LastError != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "last_error: %s\n", state.LastError)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "stalled: %t\n", state.Stalled)

			return nil
		},
	}

	command.Flags().StringVar(&opts.stateFile, "state-file", defaults.stateFile, "Supervisor state file path")
	command.Flags().StringVar(&opts.pidFile, "pid-file", defaults.pidFile, "Supervisor pid file path")
	command.Flags().BoolVar(&opts.json, "json", defaults.json, "Output status as JSON")

	return command
}

func newAgentLogsCommand() *cobra.Command {
	defaults := defaultAgentLogsOptions()
	opts := defaults

	command := &cobra.Command{
		Use:   "logs",
		Short: "Show agent output log (tail or follow)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.lines <= 0 {
				opts.lines = 100
			}

			lines, err := readTailLines(opts.outputLog, opts.lines)
			if err != nil {
				if os.IsNotExist(err) {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no logs yet")
					return nil
				}
				return &exitError{Code: 1, Err: err}
			}
			for _, line := range lines {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
			}

			if !opts.follow {
				return nil
			}

			return followLog(cmd.Context(), opts.outputLog, cmd.OutOrStdout())
		},
	}

	command.Flags().StringVar(&opts.outputLog, "output-log", defaults.outputLog, "Agent output log path")
	command.Flags().IntVar(&opts.lines, "lines", defaults.lines, "Number of tail lines")
	command.Flags().BoolVar(&opts.follow, "follow", defaults.follow, "Follow appended log output")

	return command
}

func runAgentForeground(cmd *cobra.Command, opts agentStartOptions) error {
	kind := agent.AgentKind(strings.TrimSpace(opts.agentKind))
	if !kind.Valid() {
		return &exitError{Code: 1, Err: fmt.Errorf("unsupported --agent %q", opts.agentKind)}
	}

	supervisor := newSupervisor(agent.SupervisorConfig{
		SpriteName: opts.sprite,
		RepoDir:    opts.repoDir,
		Agent: agent.AgentConfig{
			Kind:           kind,
			Command:        strings.TrimSpace(opts.agentCommand),
			Flags:          parseCSV(opts.agentFlags),
			Model:          strings.TrimSpace(opts.model),
			Yolo:           opts.yolo,
			FullAuto:       opts.fullAuto,
			Environment:    parseEnvAssignments(opts.envAssignments),
			PassThroughEnv: parseCSV(opts.passThroughEnv),
			Assignment: agent.TaskAssignment{
				IssueURL: strings.TrimSpace(opts.issueURL),
				Prompt:   strings.TrimSpace(opts.taskPrompt),
				Repo:     strings.TrimSpace(opts.taskRepo),
				Branch:   strings.TrimSpace(opts.taskBranch),
			},
		},
		Runtime: agent.RuntimePaths{
			EventLog:  opts.eventLog,
			OutputLog: opts.outputLog,
			PIDFile:   opts.pidFile,
			StateFile: opts.stateFile,
		},
		HeartbeatInterval:   opts.heartbeatInterval,
		ProgressInterval:    opts.progressInterval,
		StallTimeout:        opts.stallTimeout,
		RestartDelay:        opts.restartDelay,
		ShutdownGracePeriod: opts.shutdownGrace,
		Stdout:              cmd.OutOrStdout(),
		Stderr:              cmd.ErrOrStderr(),
	})

	result := supervisor.Run(cmd.Context())
	if result.State == agent.RunStateStopped {
		return nil
	}
	if result.Err == nil {
		result.Err = fmt.Errorf("supervisor exited with state %s", result.State)
	}
	return &exitError{Code: result.ExitCode(), Err: result.Err}
}

func startAgentDaemon(cmd *cobra.Command, opts agentStartOptions) error {
	if pid, err := agent.ReadPIDFile(opts.pidFile); err == nil && agent.ProcessRunning(pid) {
		return &exitError{Code: 1, Err: fmt.Errorf("supervisor already running with pid %d", pid)}
	}

	args := []string{
		"agent", "start",
		"--foreground",
		"--daemon-child",
		"--sprite", opts.sprite,
		"--repo-dir", opts.repoDir,
		"--agent", opts.agentKind,
		"--agent-command", opts.agentCommand,
		"--agent-flags", opts.agentFlags,
		"--model", opts.model,
		"--env", opts.envAssignments,
		"--pass-env", opts.passThroughEnv,
		"--issue-url", opts.issueURL,
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
	}
	if opts.yolo {
		args = append(args, "--yolo")
	}
	if opts.fullAuto {
		args = append(args, "--full-auto")
	}

	child := exec.Command(os.Args[0], args...)
	child.Stdout = cmd.OutOrStdout()
	child.Stderr = cmd.ErrOrStderr()
	child.Stdin = nil
	if err := child.Start(); err != nil {
		return &exitError{Code: 1, Err: fmt.Errorf("start daemon child: %w", err)}
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "supervisor daemon started (pid=%d)\n", child.Process.Pid)
	return nil
}

func defaultAgentStartOptions() agentStartOptions {
	repoDir := envOrDefault("BB_REPO_DIR", ".")
	runtime := agent.DefaultRuntimePaths(repoDir)

	return agentStartOptions{
		sprite:            envOrDefault("BB_SPRITE", defaultSpriteName()),
		repoDir:           repoDir,
		agentKind:         envOrDefault("BB_AGENT", string(agent.AgentCodex)),
		agentCommand:      envOrDefault("BB_AGENT_COMMAND", ""),
		agentFlags:        envOrDefault("BB_AGENT_FLAGS", ""),
		model:             envOrDefault("BB_AGENT_MODEL", ""),
		yolo:              envBoolOrDefault("BB_AGENT_YOLO", false),
		fullAuto:          envBoolOrDefault("BB_AGENT_FULL_AUTO", false),
		envAssignments:    envOrDefault("BB_AGENT_ENV", ""),
		passThroughEnv:    envOrDefault("BB_AGENT_PASS_ENV", ""),
		issueURL:          envOrDefault("BB_TASK_ISSUE_URL", ""),
		taskPrompt:        envOrDefault("BB_TASK_PROMPT", ""),
		taskRepo:          envOrDefault("BB_TASK_REPO", ""),
		taskBranch:        envOrDefault("BB_TASK_BRANCH", ""),
		eventLog:          envOrDefault("BB_EVENT_LOG", runtime.EventLog),
		outputLog:         envOrDefault("BB_OUTPUT_LOG", runtime.OutputLog),
		pidFile:           envOrDefault("BB_PID_FILE", runtime.PIDFile),
		stateFile:         envOrDefault("BB_STATE_FILE", runtime.StateFile),
		heartbeatInterval: envDurationOrDefault("BB_HEARTBEAT_INTERVAL", agent.DefaultHeartbeatInterval),
		progressInterval:  envDurationOrDefault("BB_PROGRESS_INTERVAL", agent.DefaultProgressInterval),
		stallTimeout:      envDurationOrDefault("BB_STALL_TIMEOUT", agent.DefaultStallTimeout),
		restartDelay:      envDurationOrDefault("BB_RESTART_DELAY", agent.DefaultRestartDelay),
		shutdownGrace:     envDurationOrDefault("BB_SHUTDOWN_GRACE", agent.DefaultShutdownGracePeriod),
		foreground:        envBoolOrDefault("BB_AGENT_FOREGROUND", false),
		daemonChild:       envBoolOrDefault("BB_AGENT_DAEMON_CHILD", false),
	}
}

func defaultAgentStopOptions() agentStopOptions {
	runtime := agent.DefaultRuntimePaths(envOrDefault("BB_REPO_DIR", "."))
	return agentStopOptions{
		pidFile: envOrDefault("BB_PID_FILE", runtime.PIDFile),
		timeout: envDurationOrDefault("BB_STOP_TIMEOUT", 15*time.Second),
	}
}

func defaultAgentStatusOptions() agentStatusOptions {
	runtime := agent.DefaultRuntimePaths(envOrDefault("BB_REPO_DIR", "."))
	return agentStatusOptions{
		stateFile: envOrDefault("BB_STATE_FILE", runtime.StateFile),
		pidFile:   envOrDefault("BB_PID_FILE", runtime.PIDFile),
		json:      false,
	}
}

func defaultAgentLogsOptions() agentLogsOptions {
	runtime := agent.DefaultRuntimePaths(envOrDefault("BB_REPO_DIR", "."))
	return agentLogsOptions{
		outputLog: envOrDefault("BB_OUTPUT_LOG", runtime.OutputLog),
		lines:     envIntOrDefault("BB_LOG_LINES", 100),
		follow:    envBoolOrDefault("BB_LOG_FOLLOW", false),
	}
}

func defaultSpriteName() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return "sprite"
	}
	return host
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envIntOrDefault(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBoolOrDefault(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func parseCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		result = append(result, value)
	}
	return result
}

func parseEnvAssignments(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	result := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}
		result[key] = parts[1]
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func readTailLines(path string, lineCount int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	buffer := make([]string, 0, lineCount)
	for scanner.Scan() {
		line := scanner.Text()
		if len(buffer) < lineCount {
			buffer = append(buffer, line)
			continue
		}
		copy(buffer, buffer[1:])
		buffer[len(buffer)-1] = line
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return buffer, nil
}

func followLog(ctx context.Context, path string, out io.Writer) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return err
	}

	reader := bufio.NewReader(file)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				time.Sleep(200 * time.Millisecond)
				continue
			}
			return err
		}
		_, _ = fmt.Fprint(out, line)
	}
}

func resolveRepoDir(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "."
		}
		return cwd
	}
	if filepath.IsAbs(trimmed) {
		return trimmed
	}
	cwd, err := os.Getwd()
	if err != nil {
		return trimmed
	}
	return filepath.Join(cwd, trimmed)
}
