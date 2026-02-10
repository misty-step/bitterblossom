package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	dispatchsvc "github.com/misty-step/bitterblossom/internal/dispatch"
	"github.com/misty-step/bitterblossom/pkg/fly"
	"github.com/spf13/cobra"
)

type dispatchOptions struct {
	Repo                 string
	PromptFile           string
	Ralph                bool
	Execute              bool
	DryRun               bool
	JSON                 bool
	Wait                 bool
	Timeout              time.Duration
	App                  string
	Token                string
	APIURL               string
	Org                  string
	SpriteCLI            string
	CompositionPath      string
	MaxIterations        int
	MaxTokens            int
	MaxTime              time.Duration
	WebhookURL           string
	AllowAnthropicDirect bool
}

type dispatchDeps struct {
	readFile     func(path string) ([]byte, error)
	newFlyClient func(token, apiURL string) (fly.MachineClient, error)
	newRemote    func(binary, org string) *spriteCLIRemote
	newService   func(cfg dispatchsvc.Config) (dispatchRunner, error)
	pollSprite   func(ctx context.Context, remote *spriteCLIRemote, sprite string, timeout time.Duration, progress func(string)) (*waitResult, error)
}

type dispatchRunner interface {
	Run(ctx context.Context, req dispatchsvc.Request) (dispatchsvc.Result, error)
}

// waitResult contains the final result from waiting for a sprite task.
type waitResult struct {
	State       string `json:"state"`
	Task        string `json:"task,omitempty"`
	Repo        string `json:"repo,omitempty"`
	Started     string `json:"started,omitempty"`
	Runtime     string `json:"runtime,omitempty"`
	PRURL       string `json:"pr_url,omitempty"`
	Blocked     bool   `json:"blocked,omitempty"`
	BlockedReason string `json:"blocked_reason,omitempty"`
	Complete    bool   `json:"complete"`
	Error       string `json:"error,omitempty"`
}

func defaultDispatchDeps() dispatchDeps {
	return dispatchDeps{
		readFile: os.ReadFile,
		newFlyClient: func(token, apiURL string) (fly.MachineClient, error) {
			return fly.NewClient(token, fly.WithBaseURL(apiURL))
		},
		newRemote:  newSpriteCLIRemote,
		newService: newDispatchService,
		pollSprite: pollSpriteStatus,
	}
}

func newDispatchService(cfg dispatchsvc.Config) (dispatchRunner, error) {
	return dispatchsvc.NewService(cfg)
}

func newDispatchCmd() *cobra.Command {
	return newDispatchCmdWithDeps(defaultDispatchDeps())
}

func newDispatchCmdWithDeps(deps dispatchDeps) *cobra.Command {
	opts := dispatchOptions{
		DryRun:          true,
		Wait:            false,
		Timeout:         30 * time.Minute,
		App:             strings.TrimSpace(os.Getenv("FLY_APP")),
		Token:           defaultFlyToken(),
		APIURL:          fly.DefaultBaseURL,
		Org:             strings.TrimSpace(os.Getenv("FLY_ORG")),
		SpriteCLI:       strings.TrimSpace(os.Getenv("SPRITE_CLI")),
		CompositionPath: "compositions/v1.yaml",
		MaxIterations:   dispatchsvc.DefaultMaxRalphIterations,
		MaxTokens:       dispatchsvc.DefaultMaxTokens,
		MaxTime:         dispatchsvc.DefaultMaxTime,
		WebhookURL:      strings.TrimSpace(os.Getenv("SPRITE_WEBHOOK_URL")),
	}

	command := &cobra.Command{
		Use:   "dispatch <sprite> [prompt]",
		Short: "Dispatch a task prompt to a sprite (dry-run by default)",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("dispatch: sprite name is required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Execute {
				opts.DryRun = false
			}
			if !opts.Execute && cmd.Flags().Changed("dry-run") && !opts.DryRun {
				return errors.New("dispatch: --dry-run=false requires --execute")
			}

			// Validate --wait requires --execute
			if opts.Wait && !opts.Execute {
				return errors.New("dispatch: --wait requires --execute")
			}

			prompt, err := resolveDispatchPrompt(args, opts, deps)
			if err != nil {
				return err
			}

			appMissing := strings.TrimSpace(opts.App) == ""
			tokenMissing := strings.TrimSpace(opts.Token) == ""
			if appMissing || tokenMissing {
				return errors.New("Error: FLY_APP and FLY_API_TOKEN are required for sprite operations.\n  export FLY_APP=your-app\n  export FLY_API_TOKEN=your-token")
			}

			flyClient, err := deps.newFlyClient(opts.Token, opts.APIURL)
			if err != nil {
				return err
			}
			remote := deps.newRemote(opts.SpriteCLI, opts.Org)
			service, err := deps.newService(dispatchsvc.Config{
				Remote:             remote,
				Fly:                flyClient,
				App:                opts.App,
				Workspace:          dispatchsvc.DefaultWorkspace,
				CompositionPath:    opts.CompositionPath,
				RalphTemplatePath:  "scripts/ralph-prompt-template.md",
				MaxRalphIterations: opts.MaxIterations,
			})
			if err != nil {
				return err
			}

			result, err := service.Run(contextOrBackground(cmd.Context()), dispatchsvc.Request{
				Sprite:               args[0],
				Prompt:               prompt,
				Repo:                 opts.Repo,
				Ralph:                opts.Ralph,
				Execute:              opts.Execute,
				WebhookURL:           opts.WebhookURL,
				AllowAnthropicDirect: opts.AllowAnthropicDirect,
				MaxTokens:            opts.MaxTokens,
				MaxTime:              opts.MaxTime,
			})
			if err != nil {
				return err
			}

			// If --wait flag is set, poll for completion
			if opts.Wait && result.Executed {
				waitRes, waitErr := deps.pollSprite(cmd.Context(), remote, args[0], opts.Timeout, func(msg string) {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), msg)
				})
				if waitErr != nil {
					// Graceful degradation: return dispatch result with warning
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: polling failed: %v\n", waitErr)
					return renderDispatchResult(cmd, result, opts.JSON)
				}
				return renderWaitResult(cmd, result, waitRes, opts.JSON)
			}

			return renderDispatchResult(cmd, result, opts.JSON)
		},
	}

	command.Flags().StringVar(&opts.Repo, "repo", "", "Repo to clone/pull before dispatch (org/repo or URL)")
	command.Flags().StringVar(&opts.PromptFile, "file", "", "Read prompt from a file")
	command.Flags().BoolVar(&opts.Ralph, "ralph", false, "Start persistent Ralph loop instead of one-shot")
	command.Flags().BoolVar(&opts.Execute, "execute", false, "Execute dispatch actions (default is dry-run)")
	command.Flags().BoolVar(&opts.DryRun, "dry-run", true, "Preview dispatch plan without side effects")
	command.Flags().BoolVar(&opts.JSON, "json", false, "Emit JSON output")
	command.Flags().BoolVar(&opts.Wait, "wait", false, "Wait for task completion and stream progress")
	command.Flags().DurationVar(&opts.Timeout, "timeout", opts.Timeout, "Timeout for --wait (default: 30m)")
	command.Flags().StringVar(&opts.App, "app", opts.App, "Sprites app name")
	command.Flags().StringVar(&opts.Token, "token", opts.Token, "API token (or FLY_API_TOKEN/FLY_TOKEN)")
	command.Flags().StringVar(&opts.APIURL, "api-url", opts.APIURL, "Sprites API base URL")
	command.Flags().StringVar(&opts.Org, "org", opts.Org, "Sprite org passed to sprite CLI")
	command.Flags().StringVar(&opts.SpriteCLI, "sprite-cli", opts.SpriteCLI, "Sprite CLI binary path")
	command.Flags().StringVar(&opts.CompositionPath, "composition", opts.CompositionPath, "Composition YAML used for provisioning metadata")
	command.Flags().IntVar(&opts.MaxIterations, "max-iterations", opts.MaxIterations, "Ralph loop iteration safety cap")
	command.Flags().IntVar(&opts.MaxTokens, "max-tokens", opts.MaxTokens, "Ralph stuck-loop token safety cap (only with --ralph)")
	command.Flags().DurationVar(&opts.MaxTime, "max-time", opts.MaxTime, "Ralph stuck-loop runtime safety cap (only with --ralph)")
	command.Flags().StringVar(&opts.WebhookURL, "webhook-url", opts.WebhookURL, "Optional sprite-agent webhook URL")
	command.Flags().BoolVar(&opts.AllowAnthropicDirect, "allow-anthropic-direct", false, "Allow dispatch even if sprite has a real ANTHROPIC_API_KEY")

	return command
}

func resolveDispatchPrompt(args []string, opts dispatchOptions, deps dispatchDeps) (string, error) {
	if strings.TrimSpace(opts.PromptFile) != "" {
		content, err := deps.readFile(opts.PromptFile)
		if err != nil {
			return "", err
		}
		prompt := strings.TrimSpace(string(content))
		if prompt == "" {
			return "", errors.New("dispatch: prompt file is empty")
		}
		return prompt, nil
	}

	if len(args) < 2 {
		return "", errors.New("dispatch: prompt is required when --file is not set")
	}
	prompt := strings.TrimSpace(strings.Join(args[1:], " "))
	if prompt == "" {
		return "", errors.New("dispatch: prompt cannot be empty")
	}
	return prompt, nil
}

func renderDispatchResult(cmd *cobra.Command, result dispatchsvc.Result, jsonMode bool) error {
	if jsonMode {
		encoded, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
		return err
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Sprite: %s\n", result.Plan.Sprite); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Mode: %s\n", result.Plan.Mode); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Plan:"); err != nil {
		return err
	}
	for _, step := range result.Plan.Steps {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "- [%s] %s\n", step.Kind, step.Description); err != nil {
			return err
		}
	}

	if !result.Executed {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "Dry run only. Re-run with --execute to apply.")
		return err
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "State: %s\n", result.State); err != nil {
		return err
	}
	if result.AgentPID > 0 {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Agent PID: %d\n", result.AgentPID); err != nil {
			return err
		}
	}
	if strings.TrimSpace(result.CommandOutput) != "" {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Output:"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSpace(result.CommandOutput)); err != nil {
			return err
		}
	}
	return nil
}

func renderWaitResult(cmd *cobra.Command, dispatchResult dispatchsvc.Result, waitRes *waitResult, jsonMode bool) error {
	if jsonMode {
		combined := struct {
			Dispatch dispatchsvc.Result `json:"dispatch"`
			Wait     *waitResult        `json:"wait,omitempty"`
		}{
			Dispatch: dispatchResult,
			Wait:     waitRes,
		}
		encoded, err := json.MarshalIndent(combined, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
		return err
	}

	// Print dispatch result summary
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "\n=== Task Complete ===\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Sprite: %s\n", dispatchResult.Plan.Sprite); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "State: %s\n", waitRes.State); err != nil {
		return err
	}
	if waitRes.Task != "" && waitRes.Task != "-" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Task: %s\n", waitRes.Task); err != nil {
			return err
		}
	}
	if waitRes.Runtime != "" && waitRes.Runtime != "-" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Runtime: %s\n", waitRes.Runtime); err != nil {
			return err
		}
	}
	if waitRes.Blocked {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Status: BLOCKED\n"); err != nil {
			return err
		}
		if waitRes.BlockedReason != "" {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Reason: %s\n", waitRes.BlockedReason); err != nil {
				return err
			}
		}
	} else if waitRes.Complete {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Status: COMPLETE\n"); err != nil {
			return err
		}
	}
	if waitRes.PRURL != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "PR URL: %s\n", waitRes.PRURL); err != nil {
			return err
		}
	}
	if waitRes.Error != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Error: %s\n", waitRes.Error); err != nil {
			return err
		}
	}
	return nil
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// pollSpriteStatus polls a sprite for task completion.
func pollSpriteStatus(ctx context.Context, remote *spriteCLIRemote, sprite string, timeout time.Duration, progress func(string)) (*waitResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	workspace := "/home/sprite/workspace"
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Initial delay to let the task start
	progress(fmt.Sprintf("Waiting for %s to start...", sprite))
	select {
	case <-time.After(2 * time.Second):
		// Continue
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	for {
		select {
		case <-ctx.Done():
			return &waitResult{
				State: "timeout",
				Error: "polling timed out",
			}, nil
		case <-ticker.C:
			result, done, err := checkSpriteStatus(ctx, remote, sprite, workspace)
			if err != nil {
				// Log error but continue polling (graceful degradation)
				progress(fmt.Sprintf("Polling error (retrying): %v", err))
				continue
			}
			if result != nil {
				progress(fmt.Sprintf("Status: %s", result.State))
			}
			if done && result != nil {
				return result, nil
			}
		}
	}
}

// checkSpriteStatus checks the current status of a sprite task.
func checkSpriteStatus(ctx context.Context, remote *spriteCLIRemote, sprite, workspace string) (*waitResult, bool, error) {
	script := buildStatusCheckScript(workspace)
	output, err := remote.Exec(ctx, sprite, script, nil)
	if err != nil {
		return nil, false, err
	}

	return parseStatusCheckOutput(output, workspace)
}

// buildStatusCheckScript creates a script to check sprite status.
func buildStatusCheckScript(workspace string) string {
	return strings.Join([]string{
		"set -euo pipefail",
		"WORKSPACE=" + shellQuote(workspace),
		"",
		"# Check for status file",
		"STATUS_JSON='{}'",
		"if [ -f \"$WORKSPACE/STATUS.json\" ]; then",
		"  STATUS_JSON=\"$(tr -d '\\n' < \"$WORKSPACE/STATUS.json\")\"",
		"fi",
		"echo \"__STATUS_JSON__${STATUS_JSON}\"",
		"",
		"# Check agent state",
		"AGENT_STATE=dead",
		"PID_PATH=\"$WORKSPACE/agent.pid\"",
		"if [ -f \"$PID_PATH\" ]; then",
		"  PID=\"$(cat \"$PID_PATH\")\"",
		"  if kill -0 \"$PID\" 2>/dev/null; then",
		"    AGENT_STATE=alive",
		"  fi",
		"elif pgrep -f 'claude -p' >/dev/null 2>&1; then",
		"  AGENT_STATE=alive",
		"fi",
		"echo \"__AGENT_STATE__${AGENT_STATE}\"",
		"",
		"# Check for completion markers",
		"HAS_COMPLETE=no",
		"if [ -f \"$WORKSPACE/TASK_COMPLETE\" ]; then",
		"  HAS_COMPLETE=yes",
		"fi",
		"echo \"__HAS_COMPLETE__${HAS_COMPLETE}\"",
		"",
		"# Check for blocked marker",
		"HAS_BLOCKED=no",
		"BLOCKED_SUMMARY=\"\"",
		"if [ -f \"$WORKSPACE/BLOCKED.md\" ]; then",
		"  HAS_BLOCKED=yes",
		"  BLOCKED_SUMMARY=\"$(head -5 \"$WORKSPACE/BLOCKED.md\" 2>/dev/null | tr '\\n' ' ' | sed 's/[[:space:]]\\+/ /g')\"",
		"fi",
		"echo \"__HAS_BLOCKED__${HAS_BLOCKED}\"",
		"echo \"__BLOCKED_B64__$(printf '%s' \"$BLOCKED_SUMMARY\" | base64 | tr -d '\\n')\"",
		"",
		"# Check for PR URL",
		"PR_URL=\"\"",
		"if [ -f \"$WORKSPACE/PR_URL\" ]; then",
		"  PR_URL=\"$(cat \"$WORKSPACE/PR_URL\")\"",
		"elif [ -f \"$WORKSPACE/TASK_COMPLETE\" ]; then",
		"  # Try to extract PR URL from TASK_COMPLETE",
		"  PR_URL=\"$(grep -oE 'https://github.com/[^/]+/[^/]+/pull/[0-9]+' \"$WORKSPACE/TASK_COMPLETE\" 2>/dev/null || true)\"",
		"fi",
		"echo \"__PR_URL__${PR_URL}\"",
	}, "\n")
}

// parseStatusCheckOutput parses the output from the status check script.
func parseStatusCheckOutput(output, workspace string) (*waitResult, bool, error) {
	type statusFile struct {
		Repo    string `json:"repo"`
		Started string `json:"started"`
		Mode    string `json:"mode"`
		Task    string `json:"task"`
	}

	var (
		fileStatus    statusFile
		agentState    string
		hasComplete   bool
		hasBlocked    bool
		blockedB64    string
		prURL         string
	)

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "__STATUS_JSON__"):
			payload := strings.TrimPrefix(line, "__STATUS_JSON__")
			if payload == "" {
				payload = "{}"
			}
			_ = json.Unmarshal([]byte(payload), &fileStatus)
		case strings.HasPrefix(line, "__AGENT_STATE__"):
			agentState = strings.TrimPrefix(line, "__AGENT_STATE__")
		case strings.HasPrefix(line, "__HAS_COMPLETE__"):
			hasComplete = strings.TrimPrefix(line, "__HAS_COMPLETE__") == "yes"
		case strings.HasPrefix(line, "__HAS_BLOCKED__"):
			hasBlocked = strings.TrimPrefix(line, "__HAS_BLOCKED__") == "yes"
		case strings.HasPrefix(line, "__BLOCKED_B64__"):
			blockedB64 = strings.TrimPrefix(line, "__BLOCKED_B64__")
		case strings.HasPrefix(line, "__PR_URL__"):
			prURL = strings.TrimPrefix(line, "__PR_URL__")
		}
	}

	// Calculate runtime
	var runtime string
	if fileStatus.Started != "" {
		if parsed, err := time.Parse(time.RFC3339, fileStatus.Started); err == nil {
			delta := time.Since(parsed)
			if delta > 0 {
				runtime = delta.Round(time.Second).String()
			}
		}
	}

	// Determine state
	state := "idle"
	complete := false
	blocked := false

	switch {
	case hasBlocked:
		state = "blocked"
		blocked = true
		complete = true
	case hasComplete:
		state = "completed"
		complete = true
	case agentState == "alive":
		state = "running"
	}

	// Decode blocked reason
	var blockedReason string
	if blocked && blockedB64 != "" {
		if decoded, err := base64.StdEncoding.DecodeString(blockedB64); err == nil {
			blockedReason = strings.TrimSpace(string(decoded))
		}
	}

	result := &waitResult{
		State:         state,
		Task:          fileStatus.Task,
		Repo:          fileStatus.Repo,
		Started:       fileStatus.Started,
		Runtime:       runtime,
		PRURL:         prURL,
		Blocked:       blocked,
		BlockedReason: blockedReason,
		Complete:      complete,
	}

	return result, complete, nil
}
