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
		Short: "Dispatch a task to a sprite agent",
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

func workspaceCheckoutCheckScriptFor(workspace string) string {
	return fmt.Sprintf(`git -C %q rev-parse --is-inside-work-tree 2>/dev/null`, workspace)
}

func ensureWorkspaceCheckoutReadyWithRunner(ctx context.Context, run spriteScriptRunner, workspace string) error {
	output, exitCode, err := runDispatchCheck(ctx, run, dispatchCheck{
		timeout: 10 * time.Second,
		script:  workspaceCheckoutCheckScriptFor(workspace),
	})
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("git rev-parse failed")
	}
	if output != "true" {
		return fmt.Errorf("git rev-parse returned %q", output)
	}
	return nil
}

func syncWorkspaceRepoWithRunner(ctx context.Context, run spriteScriptRunner, workspace, branch string) error {
	output, exitCode, err := runDispatchCheck(ctx, run, dispatchCheck{
		timeout: 45 * time.Second,
		script: fmt.Sprintf(
			`git config --global --get-all safe.directory 2>/dev/null | grep -qxF %q || git config --global --add safe.directory %q 2>/dev/null; cd %q && git checkout %q && git pull --rebase 2>&1`,
			workspace, workspace, workspace, branch,
		),
	})
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("%s", output)
	}
	return nil
}

func prepareDispatchWorkspaceWithRunner(ctx context.Context, run spriteScriptRunner, spriteName, repo, workspace, workspaceOverride, defaultBranch string, dryRun bool, progress io.Writer) error {
	if workspaceOverride != "" {
		if err := ensureWorkspaceCheckoutReadyWithRunner(ctx, run, workspace); err != nil {
			return fmt.Errorf("workspace override %q is not ready on sprite %q: %w", workspace, spriteName, err)
		}
		return nil
	}

	if dryRun {
		if err := ensureWorkspaceCheckoutReadyWithRunner(ctx, run, workspace); err != nil {
			return fmt.Errorf("repo workspace %q is not ready on sprite %q: %w", workspace, spriteName, err)
		}
		return nil
	}

	if progress != nil {
		_, _ = fmt.Fprintf(progress, "syncing repo %s (branch: %s)...\n", repo, defaultBranch)
	}
	if err := syncWorkspaceRepoWithRunner(ctx, run, workspace, defaultBranch); err != nil {
		return fmt.Errorf("repo sync failed: %w", err)
	}
	return nil
}

func runDispatch(ctx context.Context, spriteName, prompt, repo, workspaceOverride, promptTemplate string, timeout time.Duration, noOutputTimeout time.Duration, dryRun bool, prCheckTimeout time.Duration, waitForComplete bool) error {
	// LLM auth is handled by settings.json on the sprite and GitHub auth is
	// persisted on the sprite during setup.
	_, _ = fmt.Fprintf(os.Stderr, "probing %s...\n", spriteName)
	session, err := newSpriteSession(ctx, spriteName, spriteSessionOptions{probeTimeout: 15 * time.Second})
	if err != nil {
		return err
	}
	defer func() { _ = session.close() }()
	s := session.sprite

	// 2. Refuse overlapping dispatches against an active agent run.
	// 3. Resolve workspace. Default dispatch syncs the shared repo checkout.
	// Conductor-owned worktrees pass --workspace and handle preparation separately.
	workspace := dispatchWorkspace(repo, workspaceOverride)

	if err := ensureNoActiveDispatchLoop(ctx, s, workspace); err != nil {
		return fmt.Errorf("sprite %q is currently working: %w", spriteName, err)
	}

	defaultBranch := "main"
	if !dryRun {
		// Detect the remote default branch once; used in both sync and verification.
		detectedBranch, branchErr := detectDefaultBranchWithRunner(ctx, spriteBashRunner(s), workspace)
		if branchErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warning: could not detect default branch: %v — falling back to main\n", branchErr)
		} else {
			defaultBranch = detectedBranch
		}
	}

	if err := prepareDispatchWorkspaceWithRunner(ctx, spriteBashRunner(s), spriteName, repo, workspace, workspaceOverride, defaultBranch, dryRun, os.Stderr); err != nil {
		return err
	}

	// Dry-run: readiness checks passed — do not start the agent.
	if dryRun {
		_, _ = fmt.Fprintf(os.Stderr, "dry-run: sprite %q is ready to dispatch\n", spriteName)
		return nil
	}

	// 4. Clean stale signals
	cleanScript := cleanSignalsScriptFor(workspace)
	_, _ = s.CommandContext(ctx, "bash", "-c", cleanScript).Output()

	// Record HEAD SHA before dispatch so the off-rails commit check is
	// scoped to work produced by this dispatch, not prior stale commits.
	preSHA, shaErr := captureHeadSHAWithRunner(ctx, spriteBashRunner(s), workspace)
	if shaErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: could not capture pre-dispatch HEAD SHA: %v\n", shaErr)
	}

	// 5. Render and upload prompt
	rendered, err := renderPrompt(promptTemplate, prompt, repo, spriteName)
	if err != nil {
		return fmt.Errorf("render prompt: %w", err)
	}

	promptPath := workspaceDispatchPromptPath(workspace)
	if err := s.Filesystem().WriteFileContext(ctx, promptPath, []byte(rendered), 0644); err != nil {
		return fmt.Errorf("upload prompt: %w", err)
	}

	// 6. Run the agent directly — foreground, streaming.
	_, _ = fmt.Fprintf(os.Stderr, "starting agent run (timeout %s, harness=claude)...\n", timeout)

	agentCmd := agentDispatchCommand(workspace, promptPath, noOutputTimeout)

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

	agentRun := s.CommandContext(agentCtx, "bash", "-c", agentCmd)
	agentRun.Dir = workspace
	agentRun.SetTTY(true)

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
	agentRun.Stdout = stdout
	agentRun.Stderr = stderr
	agentRun.TextMessageHandler = newDispatchTextMessageHandler(stdout, stderr)

	agentErr := agentRun.Run()

	// Stop off-rails immediately — the agent has finished. Any post-dispatch work
	// (PR check polling, verify) is intentional and must not trigger the silence
	// detector.
	detector.stop()

	// 9. Verify work produced
	_, _ = fmt.Fprintf(os.Stderr, "\n=== work produced ===\n")
	verifyScript := verifyWorkScriptFor(workspace, defaultBranch)
	verifyCmd := s.CommandContext(ctx, "bash", "-c", verifyScript)
	verifyCmd.Stdout = os.Stderr
	verifyCmd.Stderr = os.Stderr
	_ = verifyCmd.Run()

	// 10. Snapshot PR CI status (informational; gating is controlled by --pr-check-timeout).
	prs := snapshotPRChecksWithRunner(ctx, spriteBashRunner(s), workspace)
	_, _ = fmt.Fprintf(os.Stderr, "dispatch pr-checks: status=%s checks_exit=%d\n", prs.status, prs.checksExit)

	// 11. Return appropriate exit code
	// Check if off-rails detector killed the dispatch
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

	if agentErr != nil {
		if exitErr, ok := agentErr.(*sprites.ExitError); ok {
			code := exitErr.ExitCode()
			switch code {
			case 0:
				// fall through to optional PR check polling
			case 2:
				return &exitError{Code: 2, Err: fmt.Errorf("agent blocked — check BLOCKED.md on sprite")}
			default:
				return &exitError{Code: code, Err: fmt.Errorf("agent exited %d", code)}
			}
		} else {
			return fmt.Errorf("agent failed: %w", agentErr)
		}
	}

	// 12. Optionally wait for PR CI checks to pass.
	if prCheckTimeout > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "\n=== waiting for PR checks (timeout %s) ===\n", prCheckTimeout)
		pollInterval := 30 * time.Second
		if err := waitForPRChecksWithRunner(ctx, spriteBashRunner(s), workspace, prCheckTimeout, pollInterval, os.Stderr); err != nil {
			return fmt.Errorf("PR checks: %w", err)
		}
		_, _ = fmt.Fprintf(os.Stderr, "=== PR checks passed ===\n")
	}

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
// of the dispatch timeout, capped at 5 minutes. This gives the agent run
// time to write TASK_COMPLETE/BLOCKED signals after its own timeout fires.
func graceFor(timeout time.Duration) time.Duration {
	grace := max(30*time.Second, timeout/4)
	if grace > 5*time.Minute {
		grace = 5 * time.Minute
	}
	return grace
}

func heartbeatIntervalFor(noOutputTimeout time.Duration) time.Duration {
	if noOutputTimeout <= 0 {
		return 30 * time.Second
	}

	heartbeat := noOutputTimeout / 3
	if heartbeat < 30*time.Second {
		return 30 * time.Second
	}
	return heartbeat
}

func agentDispatchCommand(workspace, promptPath string, noOutputTimeout time.Duration) string {
	logPath := workspaceRalphLogPath(workspace)
	pidPath := workspaceDispatchPIDPath(workspace)
	heartbeatSec := int(heartbeatIntervalFor(noOutputTimeout).Seconds())

	return fmt.Sprintf(`
set -euo pipefail
WORKSPACE=%q
PROMPT=%q
LOG=%q
PID_FILE=%q
HEARTBEAT_SEC=%d

mkdir -p "$WORKSPACE"
touch "$LOG"
[ -f "$PROMPT" ] || { echo "[dispatch] no prompt file at $PROMPT" | tee -a "$LOG" >&2; exit 1; }
printf '%%s\n' "$$" > "$PID_FILE"

heartbeat() {
  while sleep "$HEARTBEAT_SEC"; do
    printf '[dispatch] heartbeat: %%s\n' "$(date -Iseconds)" | tee -a "$LOG"
  done
}

heartbeat &
HB_PID=$!
cleanup() {
  kill "$HB_PID" 2>/dev/null || true
  wait "$HB_PID" 2>/dev/null || true
  rm -f "$PID_FILE"
}
trap cleanup EXIT

cd "$WORKSPACE"
%s
export LEFTHOOK=0 ANTHROPIC_MODEL=%q ANTHROPIC_DEFAULT_SONNET_MODEL=%q CLAUDE_CODE_SUBAGENT_MODEL=%q

claude -p --dangerously-skip-permissions --permission-mode bypassPermissions --verbose --output-format stream-json < "$PROMPT" 2>&1 | tee -a "$LOG"
status=${PIPESTATUS[0]}
exit "$status"
`, workspace, promptPath, logPath, pidPath, heartbeatSec, runtimeEnvSourceCommand(spriteRuntimeEnvPath), spriteModel, spriteModel, spriteModel)
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
