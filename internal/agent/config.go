package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AgentKind identifies the coding CLI to run on the sprite.
type AgentKind string

const (
	AgentCodex    AgentKind = "codex"
	AgentKimi     AgentKind = "kimi-code"
	AgentClaude   AgentKind = "claude"
	AgentOpenCode AgentKind = "opencode"
)

const defaultOpenCodeModel = "openrouter/moonshotai/kimi-k2.5"

// TaskAssignment is the active work item supervised on the sprite.
type TaskAssignment struct {
	IssueURL string `json:"issue_url,omitempty"`
	Prompt   string `json:"prompt"`
	Repo     string `json:"repo"`
	Branch   string `json:"branch,omitempty"`
}

// Validate checks task requirements required by the supervisor.
func (t TaskAssignment) Validate() error {
	if strings.TrimSpace(t.Prompt) == "" {
		return errors.New("task prompt is required")
	}
	if strings.TrimSpace(t.Repo) == "" {
		return errors.New("task repo is required")
	}
	return nil
}

// AgentConfig defines which coding agent to run and how to invoke it.
type AgentConfig struct {
	Kind           AgentKind         `json:"kind"`
	Command        string            `json:"command,omitempty"`
	Flags          []string          `json:"flags,omitempty"`
	Model          string            `json:"model,omitempty"`
	Yolo           bool              `json:"yolo,omitempty"`
	FullAuto       bool              `json:"full_auto,omitempty"`
	Environment    map[string]string `json:"environment,omitempty"`
	PassThroughEnv []string          `json:"pass_through_env,omitempty"`
	Assignment     TaskAssignment    `json:"assignment"`
}

// RuntimePaths stores pid/state/log file locations for supervisor runtime artifacts.
type RuntimePaths struct {
	EventLog  string `json:"event_log"`
	OutputLog string `json:"output_log"`
	PIDFile   string `json:"pid_file"`
	StateFile string `json:"state_file"`
}

const defaultRuntimeDirName = ".bb-agent"

// DefaultRuntimePaths returns default runtime file locations rooted in repoDir.
func DefaultRuntimePaths(repoDir string) RuntimePaths {
	repoDir = strings.TrimSpace(repoDir)
	if repoDir == "" {
		repoDir = "."
	}
	runtimeDir := filepath.Join(repoDir, defaultRuntimeDirName)
	return RuntimePaths{
		EventLog:  filepath.Join(runtimeDir, "events.jsonl"),
		OutputLog: filepath.Join(runtimeDir, "agent.log"),
		PIDFile:   filepath.Join(runtimeDir, "supervisor.pid"),
		StateFile: filepath.Join(runtimeDir, "state.json"),
	}
}

// Validate ensures the agent config can be launched.
func (c AgentConfig) Validate() error {
	if !c.Kind.Valid() {
		return fmt.Errorf("unsupported agent kind %q", c.Kind)
	}
	if err := c.Assignment.Validate(); err != nil {
		return err
	}
	return nil
}

// Valid reports whether the agent kind is supported.
func (k AgentKind) Valid() bool {
	switch k {
	case AgentCodex, AgentKimi, AgentClaude, AgentOpenCode:
		return true
	default:
		return false
	}
}

func (k AgentKind) defaultCommand() string {
	switch k {
	case AgentCodex:
		return "codex"
	case AgentKimi:
		return "kimi-code"
	case AgentClaude:
		return "claude"
	case AgentOpenCode:
		return "opencode"
	default:
		return ""
	}
}

// CommandAndArgs builds the process command-line for the configured agent.
func (c AgentConfig) CommandAndArgs() (string, []string, error) {
	if err := c.Validate(); err != nil {
		return "", nil, err
	}

	command := strings.TrimSpace(c.Command)
	if command == "" {
		command = c.Kind.defaultCommand()
	}
	if command == "" {
		return "", nil, fmt.Errorf("no command configured for agent kind %q", c.Kind)
	}

	args := make([]string, 0, len(c.Flags)+8)
	if c.Kind == AgentOpenCode {
		model := strings.TrimSpace(c.Model)
		if model == "" {
			model = defaultOpenCodeModel
		}
		args = append(args, "run", "-m", model, "--agent", "coder")
		for _, flag := range c.Flags {
			flag = strings.TrimSpace(flag)
			if flag == "" {
				continue
			}
			args = append(args, flag)
		}
		args = append(args, c.Assignment.Prompt)

		return command, args, nil
	}

	if c.Yolo {
		switch c.Kind {
		case AgentClaude:
			args = append(args, "--dangerously-skip-permissions")
		case AgentCodex, AgentKimi:
			args = append(args, "--yolo")
		}
	}
	if c.FullAuto {
		args = append(args, "--full-auto")
	}
	if model := strings.TrimSpace(c.Model); model != "" {
		args = append(args, "--model", model)
	}
	for _, flag := range c.Flags {
		flag = strings.TrimSpace(flag)
		if flag == "" {
			continue
		}
		args = append(args, flag)
	}
	args = append(args, c.Assignment.Prompt)

	return command, args, nil
}

// BuildEnvironment returns a full environment with pass-through and overrides.
func (c AgentConfig) BuildEnvironment() []string {
	envMap := make(map[string]string, len(os.Environ()))
	for _, pair := range os.Environ() {
		parts := strings.SplitN(pair, "=", 2)
		key := parts[0]
		value := ""
		if len(parts) == 2 {
			value = parts[1]
		}
		envMap[key] = value
	}

	for _, key := range c.PassThroughEnv {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if value, ok := os.LookupEnv(key); ok {
			envMap[key] = value
		}
	}

	for key, value := range c.Environment {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		envMap[trimmed] = value
	}

	keys := make([]string, 0, len(envMap))
	for key := range envMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+envMap[key])
	}

	return result
}
