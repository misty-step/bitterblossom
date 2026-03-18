package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	sprites "github.com/superfly/sprites-go"

	"github.com/spf13/cobra"
)

func newDispatchCmd() *cobra.Command {
	var (
		repo            string
		workspace       string
		promptTemplate  string
		timeout         time.Duration
		noOutputTimeout time.Duration
		dryRun          bool
		prCheckTimeout  time.Duration
		waitForComplete bool
	)

	cmd := &cobra.Command{
		Use:   "dispatch <sprite> <prompt>",
		Short: "Dispatch a task to a sprite via the configured agent harness",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			spriteName := args[0]
			prompt := args[1]

			return runDispatch(
				cmd.Context(),
				spriteName,
				prompt,
				repo,
				workspace,
				promptTemplate,
				timeout,
				noOutputTimeout,
				dryRun,
				prCheckTimeout,
				waitForComplete,
			)
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repo (owner/repo)")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Remote workspace path override (skip default repo sync)")
	cmd.Flags().StringVar(&promptTemplate, "prompt-template", "scripts/builder-prompt-template.md", "Local prompt template to render before upload")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "Max wall-clock time for the agent run")
	cmd.Flags().DurationVar(&noOutputTimeout, "no-output-timeout", defaultSilenceAbortThreshold, "Abort if no output for this duration (0 to disable)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate credentials and sprite readiness without starting the agent")
	cmd.Flags().DurationVar(&prCheckTimeout, "pr-check-timeout", 0, "After task complete, wait up to this long for PR CI checks to pass (0 to skip)")
	cmd.Flags().BoolVar(&waitForComplete, "wait", false, "Wait for task completion signal after dispatch")
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}

func dispatchWorkspace(repo, override string) string {
	if override != "" {
		return override
	}
	return spriteRepoWorkspace(repo)
}

func verifyWorkScriptFor(workspace, branch string) string {
	return fmt.Sprintf(
		`export WORKSPACE=%q BRANCH=%q; cd "$WORKSPACE" && echo "--- commits ---" && git log --oneline "origin/$BRANCH..HEAD" 2>/dev/null; echo "--- PRs ---" && gh pr list --json url,title 2>/dev/null || echo "(gh not available)"`,
		workspace, branch,
	)
}

func runDispatch(ctx context.Context, spriteName, prompt, repo, workspaceOverride, promptTemplate string, timeout time.Duration, noOutputTimeout time.Duration, dryRun bool, prCheckTimeout time.Duration, waitForComplete bool) error {
	// 1. Probe sprite and open the session.
	// LLM auth is handled by settings.json on the sprite and GitHub auth is
	// persisted on the sprite during setup.
	_, _ = fmt.Fprintf(os.Stderr, "probing %s...\n", spriteName)
	session, err := newSpriteSession(ctx, spriteName, spriteSessionOptions{probeTimeout: 15 * time.Second})
	if err != nil {
		return err
	}
	defer func() { _ = session.close() }()
	s := session.sprite

	// 2. Refuse overlapping dispatches against an active agent process.
	if err := ensureNoActiveDispatchLoop(ctx, s); err != nil {
		return fmt.Errorf("sprite %q is currently working: %w", spriteName, err)
	}

	// Dry-run: all pre-flight checks passed — do not start the agent.
	if dryRun {
		_, _ = fmt.Fprintf(os.Stderr, "dry-run: sprite %q is ready to dispatch\n", spriteName)
		return nil
	}

	// 3. Kill stale agent processes from prior dispatches.
	killCtx, killCancel := context.WithTimeout(ctx, 10*time.Second)
	defer killCancel()
	_, _ = s.CommandContext(killCtx, "bash", "-c", killAgentProcessesScript).Output()

	// 4. Resolve workspace. Default dispatch syncs the shared repo checkout.
	// Conductor-owned worktrees pass --workspace and handle preparation separately.
	workspace := dispatchWorkspace(repo, workspaceOverride)

	// Detect the remote default branch once; used in both sync and verification.
	defaultBranch, branchErr := detectDefaultBranchWithRunner(ctx, spriteBashRunner(s), workspace)
	if branchErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: could not detect default branch: %v — falling back to main\n", branchErr)
		defaultBranch = "main"
	}

	if workspaceOverride == "" {
		_, _ = fmt.Fprintf(os.Stderr, "syncing repo %s (branch: %s)...\n", repo, defaultBranch)
		syncScript := fmt.Sprintf(
			`git config --global --add safe.directory %q 2>/dev/null; cd %q && git checkout %q && git pull --ff-only 2>&1`,
			workspace, workspace, defaultBranch,
		)
		syncCmd := s.CommandContext(ctx, "bash", "-c", syncScript)
		if out, err := syncCmd.Output(); err != nil {
			return fmt.Errorf("repo sync failed: %w\n%s", err, out)
		}
	} else {
		checkWorkspaceCmd := s.CommandContext(ctx, "git", "-C", workspace, "rev-parse", "--is-inside-work-tree")
		out, err := checkWorkspaceCmd.Output()
		if err != nil {
			return fmt.Errorf("workspace override %q is not ready on sprite %q: %w", workspace, spriteName, err)
		}
		if strings.TrimSpace(string(out)) != "true" {
			return fmt.Errorf("workspace override %q is not ready on sprite %q: git rev-parse returned %q", workspace, spriteName, strings.TrimSpace(string(out)))
		}
	}

	// 5. Clean stale signals and capture pre-dispatch state.
	cleanScript := cleanSignalsScriptFor(workspace)
	_, _ = s.CommandContext(ctx, "bash", "-c", cleanScript).Output()

	// Record HEAD SHA before the agent run so the off-rails commit check is
	// scoped to work produced by this dispatch, not prior stale commits.
	preSHA, shaErr := captureHeadSHAWithRunner(ctx, spriteBashRunner(s), workspace)
	if shaErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: could not capture pre-dispatch HEAD SHA: %v\n", shaErr)
	}

	// 6. Render and upload prompt.
	rendered, err := renderPrompt(promptTemplate, prompt, repo, spriteName)
	if err != nil {
		return fmt.Errorf("render prompt: %w", err)
	}

	promptPath := workspaceDispatchPromptPath(workspace)
	if err := s.Filesystem().WriteFileContext(ctx, promptPath, []byte(rendered), 0644); err != nil {
		return fmt.Errorf("upload prompt: %w", err)
	}

	// 7. Run the agent directly in the workspace.
	logPath := workspaceDispatchLogPath(workspace)
	_, _ = fmt.Fprintf(os.Stderr, "starting agent dispatch (%s timeout, harness=claude)...\n", timeout)
	agentCommand := dispatchAgentCommand(workspace, promptPath, logPath)

	gracePeriod := graceFor(timeout)
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, timeout+gracePeriod)
	defer timeoutCancel()

	agentCtx, agentCancel := context.WithCancelCause(timeoutCtx)
	defer agentCancel(nil)

	detector := newOffRailsDetector(offRailsConfig{
		SilenceAbort: noOutputTimeout,
		Cancel:       agentCancel,
		Alert:        os.Stderr,
	})
	defer detector.stop()
	detector.start()

	agentCmd := s.CommandContext(agentCtx, "bash", "-lc", agentCommand)
	agentCmd.Dir = workspace
	agentCmd.SetTTY(true)

	prettyStdout := newStreamJSONWriter(os.Stdout, false)
	prettyStdout.onToolError = detector.recordError
	prettyStderr := newStreamJSONWriter(os.Stderr, false)
	prettyStderr.onToolError = detector.recordError
	defer func() {
		if err := prettyStdout.Flush(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "dispatch: flush stdout: %v\n", err)
		}
	}()
	defer func() {
		if err := prettyStderr.Flush(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "dispatch: flush stderr: %v\n", err)
		}
	}()

	stdout := detector.wrap(prettyStdout)
	stderr := detector.wrap(prettyStderr)
	agentCmd.Stdout = stdout
	agentCmd.Stderr = stderr
	agentCmd.TextMessageHandler = newDispatchTextMessageHandler(stdout, stderr)

	agentErr := agentCmd.Run()

	// Stop off-rails immediately — the agent has finished. Any post-dispatch work
	// (PR check polling, verify) is intentional and must not trigger the silence
	// detector.
	detector.stop()

	// 8. Verify work produced.
	_, _ = fmt.Fprintf(os.Stderr, "\n=== work produced ===\n")
	verifyScript := verifyWorkScriptFor(workspace, defaultBranch)
	verifyCmd := s.CommandContext(ctx, "bash", "-c", verifyScript)
	verifyCmd.Stdout = os.Stderr
	verifyCmd.Stderr = os.Stderr
	_ = verifyCmd.Run()

	// 9. Snapshot PR CI status (informational; gating is controlled by --pr-check-timeout).
	prs := snapshotPRChecksWithRunner(ctx, spriteBashRunner(s), workspace)
	_, _ = fmt.Fprintf(os.Stderr, "dispatch pr-checks: status=%s checks_exit=%d\n", prs.status, prs.checksExit)

	// 10. Return appropriate exit code.
	// Check if off-rails detector killed the dispatch.
	if cause := context.Cause(agentCtx); cause != nil && errors.Is(cause, errOffRails) {
		_, _ = fmt.Fprintf(os.Stderr, "\n=== off-rails detected: %v ===\n", cause)

		completed, completeErr := hasTaskCompleteSignalWithRunner(ctx, spriteBashRunner(s), workspace)
		if completeErr != nil {
			return &exitError{Code: 4, Err: fmt.Errorf("off-rails completion check failed: %w", completeErr)}
		}
		if completed {
			_, _ = fmt.Fprintf(os.Stderr, "\n=== task completed: TASK_COMPLETE signal found ===\n")
			return nil
		}

		// Secondary check: if new commits exist the agent was mid-task (e.g. waiting for CI).
		// Treat as success with a warning — the work landed, the loop just couldn't signal cleanly.
		// Use pre-dispatch HEAD SHA when available to scope the check to this dispatch only,
		// preventing stale commits from a prior run from triggering a false success.
		var hasWork bool
		var checkErr error
		if preSHA != "" {
			hasWork, checkErr = hasNewCommitsSinceSHAWithRunner(ctx, spriteBashRunner(s), workspace, preSHA)
		} else {
			_, _ = fmt.Fprintf(os.Stderr, "\n=== off-rails: no pre-dispatch SHA — falling back to origin baseline check ===\n")
			hasWork, checkErr = hasNewCommitsWithRunner(ctx, spriteBashRunner(s), workspace, defaultBranch)
		}
		if checkErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "\n=== off-rails: new commits check failed: %v — treating as failure ===\n", checkErr)
		} else if hasWork {
			_, _ = fmt.Fprintf(os.Stderr, "\n=== off-rails fired mid-task: new commits found — treating as success ===\n")
			return nil
		}

		return &exitError{Code: 4, Err: cause}
	}

	blocked, blockedErr := hasBlockedSignalWithRunner(ctx, spriteBashRunner(s), workspace)
	if blockedErr != nil {
		return fmt.Errorf("blocked signal check failed: %w", blockedErr)
	}
	if blocked {
		return &exitError{Code: 2, Err: fmt.Errorf("agent blocked — check BLOCKED.md on sprite")}
	}

	if agentErr != nil {
		completed, completeErr := hasTaskCompleteSignalWithRunner(ctx, spriteBashRunner(s), workspace)
		if completeErr != nil {
			return fmt.Errorf("completion signal check failed: %w", completeErr)
		}
		if completed {
			_, _ = fmt.Fprintf(os.Stderr, "\n=== task completed: TASK_COMPLETE signal found ===\n")
		} else if exitErr, ok := agentErr.(*sprites.ExitError); ok {
			code := exitErr.ExitCode()
			return &exitError{Code: code, Err: fmt.Errorf("agent exited %d", code)}
		} else {
			return fmt.Errorf("agent failed: %w", agentErr)
		}
	}

	// 11. Optionally wait for PR CI checks to pass.
	if prCheckTimeout > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "\n=== waiting for PR checks (timeout %s) ===\n", prCheckTimeout)
		pollInterval := 30 * time.Second
		if err := waitForPRChecksWithRunner(ctx, spriteBashRunner(s), workspace, prCheckTimeout, pollInterval, os.Stderr); err != nil {
			return fmt.Errorf("PR checks: %w", err)
		}
		_, _ = fmt.Fprintf(os.Stderr, "=== PR checks passed ===\n")
	}

	// 12. Optionally wait for the task-complete signal.
	if waitForComplete {
		_, _ = fmt.Fprintf(os.Stderr, "\n=== waiting for task complete (timeout %s) ===\n", timeout)
		pollInterval := 30 * time.Second
		if err := waitForTaskCompleteWithRunner(ctx, spriteBashRunner(s), workspace, timeout, pollInterval, os.Stderr); err != nil {
			return fmt.Errorf("wait for task complete: %w", err)
		}
		_, _ = fmt.Fprintf(os.Stderr, "=== task completed ===\n")
	}

	return nil
}

// graceFor returns a proportional grace period: at least 30s, otherwise 25%
// of the dispatch timeout, capped at 5 minutes. This gives the agent process
// time to write TASK_COMPLETE/BLOCKED signals after its own timeout fires.
func graceFor(timeout time.Duration) time.Duration {
	grace := max(30*time.Second, timeout/4)
	if grace > 5*time.Minute {
		grace = 5 * time.Minute
	}
	return grace
}

func dispatchAgentCommand(workspace, promptPath, logPath string) string {
	return fmt.Sprintf(`
set -euo pipefail
export WORKSPACE=%q
export PROMPT_PATH=%q
export LOG_PATH=%q
export RUNTIME_ENV=%q
cd "$WORKSPACE"
if [ -f "$RUNTIME_ENV" ]; then
  set -a
  . "$RUNTIME_ENV"
  set +a
fi
export LEFTHOOK=0
export ANTHROPIC_MODEL=%q
export ANTHROPIC_DEFAULT_SONNET_MODEL=%q
export CLAUDE_CODE_SUBAGENT_MODEL=%q
: > "$LOG_PATH"
claude -p --dangerously-skip-permissions --output-format stream-json --model %q --verbose < "$PROMPT_PATH" > >(tee -a "$LOG_PATH") 2> >(tee -a "$LOG_PATH" >&2)
`,
		workspace,
		promptPath,
		logPath,
		spriteRuntimeEnvPath,
		spriteModel,
		spriteModel,
		spriteModel,
		spriteModel,
	)
}

// renderPrompt reads a local prompt template and substitutes placeholders.
func renderPrompt(templatePath, taskDescription, repo, spriteName string) (string, error) {
	tmpl, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("read template %q: %w (are you running from the repo root?)", templatePath, err)
	}

	rendered := string(tmpl)
	rendered = strings.ReplaceAll(rendered, "{{TASK_DESCRIPTION}}", taskDescription)
	rendered = strings.ReplaceAll(rendered, "{{REPO}}", repo)
	rendered = strings.ReplaceAll(rendered, "{{SPRITE_NAME}}", spriteName)

	return rendered, nil
}

func newDispatchTextMessageHandler(stdout, stderr io.Writer) func([]byte) {
	return func(data []byte) {
		if len(data) == 0 || bytes.HasPrefix(data, []byte("control:")) {
			return
		}

		trim := bytes.TrimSpace(data)
		if len(trim) == 0 {
			return
		}

		if trim[0] != '{' {
			_, _ = stdout.Write(data)
			if data[len(data)-1] != '\n' {
				_, _ = stdout.Write([]byte{'\n'})
			}
			return
		}

		var msg struct {
			Type  string `json:"type"`
			Data  string `json:"data,omitempty"`
			Error string `json:"error,omitempty"`
		}

		if err := json.Unmarshal(data, &msg); err != nil {
			_, _ = stdout.Write(data)
			if data[len(data)-1] != '\n' {
				_, _ = stdout.Write([]byte{'\n'})
			}
			return
		}

		switch msg.Type {
		case "stdout", "info":
			if msg.Data != "" {
				_, _ = io.WriteString(stdout, msg.Data)
			}
		case "stderr":
			if msg.Data != "" {
				_, _ = io.WriteString(stderr, msg.Data)
			}
		case "error":
			payload := msg.Error
			if payload == "" {
				payload = msg.Data
			}
			if payload != "" {
				_, _ = io.WriteString(stderr, payload)
			}
		}
	}
}
