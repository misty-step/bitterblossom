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
	"github.com/misty-step/bitterblossom/internal/fleet"
	"github.com/misty-step/bitterblossom/internal/registry"
	"github.com/misty-step/bitterblossom/internal/shellutil"
	"github.com/misty-step/bitterblossom/pkg/fly"
	"github.com/spf13/cobra"
)

type dispatchOptions struct {
	Repo                 string
	PromptFile           string
	Skills               []string
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
	Issue                int
	SkipValidation       bool
	Strict               bool
	RegistryPath         string
	RegistryRequired     bool
}

type dispatchDeps struct {
	readFile     func(path string) ([]byte, error)
	newFlyClient func(token, apiURL string) (fly.MachineClient, error)
	newRemote    func(binary, org string) *spriteCLIRemote
	newService   func(cfg dispatchsvc.Config) (dispatchRunner, error)
	selectSprite func(ctx context.Context, remote *spriteCLIRemote, opts dispatchOptions) (string, error)
	pollSprite   func(ctx context.Context, remote *spriteCLIRemote, sprite string, timeout time.Duration, progress func(string)) (*waitResult, error)
}

type dispatchRunner interface {
	Run(ctx context.Context, req dispatchsvc.Request) (dispatchsvc.Result, error)
}

// waitResult contains the final result from waiting for a sprite task.
type waitResult struct {
	State         string `json:"state"`
	Task          string `json:"task,omitempty"`
	Repo          string `json:"repo,omitempty"`
	Started       string `json:"started,omitempty"`
	Runtime       string `json:"runtime,omitempty"`
	PRURL         string `json:"pr_url,omitempty"`
	Blocked       bool   `json:"blocked,omitempty"`
	BlockedReason string `json:"blocked_reason,omitempty"`
	Complete      bool   `json:"complete"`
	Error         string `json:"error,omitempty"`
}

func defaultDispatchDeps() dispatchDeps {
	return dispatchDeps{
		readFile: os.ReadFile,
		newFlyClient: func(token, apiURL string) (fly.MachineClient, error) {
			return fly.NewClient(token, fly.WithBaseURL(apiURL))
		},
		newRemote:  newSpriteCLIRemote,
		newService: newDispatchService,
		selectSprite: func(ctx context.Context, remote *spriteCLIRemote, opts dispatchOptions) (string, error) {
			return selectSpriteFromRegistry(ctx, remote, opts)
		},
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
		Token:           "", // Token is resolved at runtime from env vars to avoid exposing in help
		APIURL:          fly.DefaultBaseURL,
		Org:             strings.TrimSpace(os.Getenv("FLY_ORG")),
		SpriteCLI:       strings.TrimSpace(os.Getenv("SPRITE_CLI")),
		CompositionPath: "compositions/v1.yaml",
		MaxIterations:   dispatchsvc.DefaultMaxRalphIterations,
		MaxTokens:       dispatchsvc.DefaultMaxTokens,
		MaxTime:         dispatchsvc.DefaultMaxTime,
		WebhookURL:      strings.TrimSpace(os.Getenv("SPRITE_WEBHOOK_URL")),
		RegistryPath:    registry.DefaultPath(),
	}

	command := &cobra.Command{
		Use:   "dispatch [sprite] [prompt]",
		Short: "Dispatch a task prompt to a sprite (dry-run by default)",
		Args:  cobra.ArbitraryArgs,
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

			spriteArg := ""
			if len(args) > 0 {
				spriteArg = args[0]
			}

			prompt, err := resolveDispatchPrompt(args, opts, deps)
			if err != nil {
				return err
			}

			// Resolve token from flag or environment
			opts.Token = resolveFlyToken(opts.Token)

			if strings.TrimSpace(opts.App) == "" {
				return errors.New("Error: FLY_APP environment variable is required. Set it to your Fly.io app name (e.g., export FLY_APP=sprites-main)")
			}
			if strings.TrimSpace(opts.Token) == "" {
				return errors.New("Error: FLY_API_TOKEN environment variable is required. Get one from https://fly.io/user/personal_access_tokens")
			}

			// Pre-dispatch issue validation
			// Skip validation for dry-run (Execute=false) to keep planning fast and offline.
			if opts.Execute && !opts.SkipValidation && opts.Issue > 0 {
				validationResult, err := dispatchsvc.ValidateIssueFromRequest(cmd.Context(), dispatchsvc.Request{
					Sprite:  spriteArg,
					Prompt:  prompt,
					Repo:    opts.Repo,
					Issue:   opts.Issue,
					Execute: opts.Execute,
				}, opts.Strict)
				if err != nil {
					return fmt.Errorf("dispatch: issue validation failed: %w", err)
				}

				// Output validation results if not in JSON mode
				if !opts.JSON && !validationResult.Valid {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Issue validation failed:")
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), validationResult.FormatValidationOutput())
				}

				if !validationResult.Valid {
					if validationErr := validationResult.ToError(); validationErr != nil {
						return validationErr
					}
				}

				// In non-strict mode with warnings, still proceed but log them
				if len(validationResult.Warnings) > 0 && !opts.JSON {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Issue validation warnings:")
					for _, w := range validationResult.Warnings {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  ⚠ %s\n", w)
					}
				}
			}

			flyClient, err := deps.newFlyClient(opts.Token, opts.APIURL)
			if err != nil {
				return err
			}
			remote := deps.newRemote(opts.SpriteCLI, opts.Org)

			if strings.TrimSpace(spriteArg) == "" {
				if deps.selectSprite == nil {
					return errors.New("dispatch: no sprite provided and auto-assign is not available")
				}
				selected, err := deps.selectSprite(cmd.Context(), remote, opts)
				if err != nil {
					return err
				}
				spriteArg = selected
			}

			// Collect auth-related environment variables to pass to sprites
			envVars := make(map[string]string)
			for _, key := range []string{
				"OPENROUTER_API_KEY",
				"ANTHROPIC_AUTH_TOKEN",
				"ANTHROPIC_API_KEY",
				"MOONSHOT_AI_API_KEY",
				"XAI_API_KEY",
				"GEMINI_API_KEY",
				"OPENAI_API_KEY",
				"GH_TOKEN",
				"GITHUB_TOKEN",
			} {
				if value := os.Getenv(key); value != "" {
					envVars[key] = value
				}
			}

			service, err := deps.newService(dispatchsvc.Config{
				Remote:             remote,
				Fly:                flyClient,
				App:                opts.App,
				Workspace:          dispatchsvc.DefaultWorkspace,
				CompositionPath:    opts.CompositionPath,
				RalphTemplatePath:  "scripts/ralph-prompt-template.md",
				MaxRalphIterations: opts.MaxIterations,
				EnvVars:            envVars,
				RegistryPath:       opts.RegistryPath,
				RegistryRequired:   opts.RegistryRequired,
				// Persist dispatch lifecycle events locally for operator visibility.
				// Best-effort; dispatch continues if the logger cannot be created.
				EventLogger: nil,
			})
			if err != nil {
				return err
			}

			result, err := service.Run(contextOrBackground(cmd.Context()), dispatchsvc.Request{
				Sprite:               spriteArg,
				Prompt:               prompt,
				Repo:                 opts.Repo,
				Skills:               opts.Skills,
				Issue:                opts.Issue,
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
				pollTarget := spriteArg
				if cmd.Flags().Changed("registry") || opts.RegistryRequired {
					if resolved, err := dispatchsvc.ResolveSprite(spriteArg, opts.RegistryPath); err == nil && strings.TrimSpace(resolved) != "" {
						pollTarget = resolved
					}
				}
				waitRes, waitErr := deps.pollSprite(cmd.Context(), remote, pollTarget, opts.Timeout, func(msg string) {
					// Intentionally ignoring write errors for progress output
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), msg)
				})
				if waitErr != nil {
					// Graceful degradation: return dispatch result with warning
					// Intentionally ignoring write errors for warning message
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
	command.Flags().StringArrayVar(&opts.Skills, "skill", nil, "Path to skill directory or SKILL.md to mount in sprite workspace (repeatable)")
	command.Flags().BoolVar(&opts.Ralph, "ralph", false, "Start persistent Ralph loop instead of one-shot")
	command.Flags().BoolVar(&opts.Execute, "execute", false, "Execute dispatch actions (default is dry-run)")
	command.Flags().BoolVar(&opts.DryRun, "dry-run", true, "Preview dispatch plan without side effects")
	command.Flags().BoolVar(&opts.JSON, "json", false, "Emit JSON output")
	command.Flags().BoolVar(&opts.Wait, "wait", false, "Wait for task completion and stream progress")
	command.Flags().DurationVar(&opts.Timeout, "timeout", opts.Timeout, "Timeout for --wait (default: 30m)")
	command.Flags().StringVar(&opts.App, "app", opts.App, "Sprites app name")
	command.Flags().StringVar(&opts.Token, "token", opts.Token, "API token (or set FLY_API_TOKEN/FLY_TOKEN env var)")
	command.Flags().StringVar(&opts.APIURL, "api-url", opts.APIURL, "Sprites API base URL")
	command.Flags().StringVar(&opts.Org, "org", opts.Org, "Sprite org passed to sprite CLI")
	command.Flags().StringVar(&opts.SpriteCLI, "sprite-cli", opts.SpriteCLI, "Sprite CLI binary path")
	command.Flags().StringVar(&opts.CompositionPath, "composition", opts.CompositionPath, "Composition YAML used for provisioning metadata")
	command.Flags().IntVar(&opts.MaxIterations, "max-iterations", opts.MaxIterations, "Ralph loop iteration safety cap")
	command.Flags().IntVar(&opts.MaxTokens, "max-tokens", opts.MaxTokens, "Ralph stuck-loop token safety cap (only with --ralph)")
	command.Flags().DurationVar(&opts.MaxTime, "max-time", opts.MaxTime, "Ralph stuck-loop runtime safety cap (only with --ralph)")
	command.Flags().StringVar(&opts.WebhookURL, "webhook-url", opts.WebhookURL, "Optional sprite-agent webhook URL")
	command.Flags().BoolVar(&opts.AllowAnthropicDirect, "allow-anthropic-direct", false, "Allow dispatch even if sprite has a real ANTHROPIC_API_KEY")
	command.Flags().IntVar(&opts.Issue, "issue", 0, "GitHub issue number to validate before dispatch")
	command.Flags().BoolVar(&opts.SkipValidation, "skip-validation", false, "Skip pre-dispatch issue validation (emergency bypass)")
	command.Flags().BoolVar(&opts.Strict, "strict", false, "Fail on any validation warning (strict mode)")
	command.Flags().StringVar(&opts.RegistryPath, "registry", opts.RegistryPath, "Path to sprite registry file")
	command.Flags().BoolVar(&opts.RegistryRequired, "registry-required", false, "Require sprites to exist in registry (fail if not found)")

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
		if opts.Issue > 0 {
			// Allow empty prompt: internal/dispatch will synthesize a default IssuePrompt.
			return "", nil
		}
		return "", errors.New("dispatch: prompt is required when --file is not set (or use --issue)")
	}
	prompt := strings.TrimSpace(strings.Join(args[1:], " "))
	if prompt == "" {
		return "", errors.New("dispatch: prompt cannot be empty")
	}
	return prompt, nil
}

func selectSpriteFromRegistry(ctx context.Context, remote *spriteCLIRemote, opts dispatchOptions) (string, error) {
	if opts.Issue <= 0 && strings.TrimSpace(opts.PromptFile) == "" {
		return "", errors.New("dispatch: auto-assign requires --issue (or --file)")
	}
	if remote == nil {
		return "", errors.New("dispatch: remote is required for auto-assign")
	}

	regPath := strings.TrimSpace(opts.RegistryPath)
	if regPath == "" {
		regPath = registry.DefaultPath()
	}
	if _, err := os.Stat(regPath); os.IsNotExist(err) {
		exampleArgs := "--file <path>"
		if opts.Issue > 0 {
			exampleArgs = fmt.Sprintf("--issue %d", opts.Issue)
		}
		return "", fmt.Errorf("dispatch: registry not found at %s\n\n  Run 'bb init' to create it, or specify --registry <path>.\n  Without a registry, provide a sprite name explicitly:\n    bb dispatch <sprite> %s", regPath, exampleArgs)
	}

	checker := remoteStatusChecker{
		remote:    remote,
		workspace: "/home/sprite/workspace",
	}
	f, err := fleet.NewDispatchFleet(fleet.DispatchConfig{
		RegistryPath:     opts.RegistryPath,
		RegistryRequired: true,
		Status:           checker,
	})
	if err != nil {
		return "", err
	}
	req := fleet.DispatchRequest{
		Issue: opts.Issue,
		Repo:  opts.Repo,
	}
	var assignment *fleet.Assignment
	if opts.Execute {
		assignment, err = f.Dispatch(ctx, req)
	} else {
		assignment, err = f.PlanDispatch(ctx, req)
	}
	if err != nil {
		return "", err
	}
	return assignment.Sprite, nil
}

type remoteStatusChecker struct {
	remote    *spriteCLIRemote
	workspace string
}

func (c remoteStatusChecker) Check(ctx context.Context, machineID string) (fleet.LiveStatus, error) {
	res, _, err := checkSpriteStatus(ctx, c.remote, machineID, c.workspace)
	if res == nil {
		return fleet.LiveStatus{State: "unknown"}, err
	}
	return fleet.LiveStatus{
		State:         res.State,
		Task:          res.Task,
		Repo:          res.Repo,
		Runtime:       res.Runtime,
		BlockedReason: res.BlockedReason,
	}, err
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
		"WORKSPACE=" + shellutil.Quote(workspace),
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
		fileStatus  statusFile
		agentState  string
		hasComplete bool
		hasBlocked  bool
		blockedB64  string
		prURL       string
	)

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "__STATUS_JSON__"):
			payload := strings.TrimPrefix(line, "__STATUS_JSON__")
			if payload == "" {
				payload = "{}"
			}
			// Parse errors are intentionally ignored; malformed JSON results in zero values
			// which is acceptable since these fields are for informational display only
			if err := json.Unmarshal([]byte(payload), &fileStatus); err != nil {
				// Reset to empty struct on parse failure to ensure clean state
				fileStatus = statusFile{}
			}
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
