package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	sprites "github.com/superfly/sprites-go"

	"github.com/spf13/cobra"
)

func newDispatchCmd() *cobra.Command {
	var (
		repo            string
		timeout         time.Duration
		maxIterations   int
		noOutputTimeout time.Duration
		dryRun          bool
		prCheckTimeout  time.Duration
	)

	cmd := &cobra.Command{
		Use:   "dispatch <sprite> <prompt>",
		Short: "Dispatch a task to a sprite via the ralph loop",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			spriteName := args[0]
			prompt := args[1]

			return runDispatch(cmd.Context(), spriteName, prompt, repo, maxIterations, timeout, noOutputTimeout, dryRun, prCheckTimeout)
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repo (owner/repo)")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "Max wall-clock time for the ralph loop")
	cmd.Flags().IntVar(&maxIterations, "max-iterations", 50, "Max ralph loop iterations")
	cmd.Flags().DurationVar(&noOutputTimeout, "no-output-timeout", defaultSilenceAbortThreshold, "Abort if no output for this duration (0 to disable)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate credentials and sprite readiness without starting the agent")
	cmd.Flags().DurationVar(&prCheckTimeout, "pr-check-timeout", 0, "After task complete, wait up to this long for PR CI checks to pass (0 to skip)")
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}

func runDispatch(ctx context.Context, spriteName, prompt, repo string, maxIter int, timeout time.Duration, noOutputTimeout time.Duration, dryRun bool, prCheckTimeout time.Duration) error {
	// Validate credentials
	token, err := spriteToken()
	if err != nil {
		return err
	}
	ghToken, err := requireEnv("GITHUB_TOKEN")
	if err != nil {
		return err
	}

	// LLM auth is handled by settings.json on the sprite (baked in during setup).
	// Dispatch only validates that GITHUB_TOKEN is set for git operations.

	client := sprites.New(token)
	defer func() { _ = client.Close() }()
	s := client.Sprite(spriteName)

	// 1. Probe connectivity (15s)
	_, _ = fmt.Fprintf(os.Stderr, "probing %s...\n", spriteName)
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if _, err := s.CommandContext(probeCtx, "echo", "ok").Output(); err != nil {
		return fmt.Errorf("sprite %q unreachable: %w", spriteName, err)
	}

	// 2. Check that setup was run (ralph.sh must exist)
	ralphScript := "/home/sprite/workspace/.ralph.sh"
	checkCtx, checkCancel := context.WithTimeout(ctx, 10*time.Second)
	defer checkCancel()
	if _, err := s.CommandContext(checkCtx, "test", "-f", ralphScript).Output(); err != nil {
		return fmt.Errorf("sprite %q not configured — run: bb setup %s --repo %s", spriteName, spriteName, repo)
	}

	// 3. Refuse overlapping dispatches against an active ralph loop
	if err := ensureNoActiveDispatchLoop(ctx, s); err != nil {
		return fmt.Errorf("sprite %q is currently working: %w", spriteName, err)
	}

	// Dry-run: all pre-flight checks passed — do not start the agent.
	if dryRun {
		_, _ = fmt.Fprintf(os.Stderr, "dry-run: sprite %q is ready to dispatch\n", spriteName)
		return nil
	}

	// 4. Kill stale agent processes from prior dispatches
	// We intentionally only treat an active ralph loop as "busy" to allow self-healing
	// orphaned agent processes from prior dispatch attempts.
	// Without this, concurrent claude processes compete for resources and hang.
	killCtx, killCancel := context.WithTimeout(ctx, 10*time.Second)
	defer killCancel()
	_, _ = s.CommandContext(killCtx, "bash", "-c", "pkill -9 -f 'ralph\\.sh|claude' 2>/dev/null; sleep 1").Output()

	// 5. Repo sync (pull latest on default branch)
	repoName := path.Base(repo)
	workspace := "/home/sprite/workspace/" + repoName

	_, _ = fmt.Fprintf(os.Stderr, "syncing repo %s...\n", repo)
	syncScript := fmt.Sprintf(
		`git config --global --add safe.directory %s 2>/dev/null; export GH_TOKEN=%q && cd %s && git checkout master 2>/dev/null || git checkout main 2>/dev/null; git pull --ff-only 2>&1`,
		workspace, ghToken, workspace,
	)
	syncCmd := s.CommandContext(ctx, "bash", "-c", syncScript)
	if out, err := syncCmd.Output(); err != nil {
		return fmt.Errorf("repo sync failed: %w\n%s", err, out)
	}

	// 6. Clean stale signals
	cleanScript := fmt.Sprintf(
		"rm -f %s/TASK_COMPLETE %s/TASK_COMPLETE.md %s/BLOCKED.md",
		workspace, workspace, workspace,
	)
	_, _ = s.CommandContext(ctx, "bash", "-c", cleanScript).Output()

	// Record HEAD SHA before the ralph loop so the off-rails commit check is
	// scoped to work produced by this dispatch, not prior stale commits.
	preSHA, shaErr := captureHeadSHAWithRunner(ctx, spriteBashRunner(s), workspace)
	if shaErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: could not capture pre-dispatch HEAD SHA: %v\n", shaErr)
	}

	// 7. Render and upload prompt
	rendered, err := renderPrompt(prompt, repo, spriteName)
	if err != nil {
		return fmt.Errorf("render prompt: %w", err)
	}

	promptPath := workspace + "/.dispatch-prompt.md"
	if err := s.Filesystem().WriteFileContext(ctx, promptPath, []byte(rendered), 0644); err != nil {
		return fmt.Errorf("upload prompt: %w", err)
	}

	// 8. Run ralph loop — foreground, streaming
	_, _ = fmt.Fprintf(os.Stderr, "starting ralph loop (max %d iterations, %s timeout, harness=claude)...\n", maxIter, timeout)

	// Only pass operational env vars — LLM auth/model come from settings.json.
	totalSec := int(timeout.Seconds())
	iterSec := 900 // default per-iteration timeout
	if totalSec < iterSec {
		iterSec = totalSec // cap per-iteration at total timeout (#389)
	}
	// Heartbeat must fire well before the silence-abort threshold.
	heartbeatSec := int(noOutputTimeout.Seconds() / 3)
	if heartbeatSec < 30 {
		heartbeatSec = 30
	}
	ralphEnv := fmt.Sprintf(
		`export MAX_ITERATIONS=%d MAX_TIME_SEC=%d ITER_TIMEOUT_SEC=%d HEARTBEAT_INTERVAL_SEC=%d WORKSPACE=%q GH_TOKEN=%q LEFTHOOK=0 ANTHROPIC_MODEL=%q ANTHROPIC_DEFAULT_SONNET_MODEL=%q CLAUDE_CODE_SUBAGENT_MODEL=%q`,
		maxIter, totalSec, iterSec, heartbeatSec, workspace, ghToken,
		"anthropic/claude-sonnet-4-6",
		"anthropic/claude-sonnet-4-6",
		"anthropic/claude-sonnet-4-6",
	)

	ralphEnv += fmt.Sprintf(` && exec bash %s`, ralphScript)

	gracePeriod := graceFor(timeout)
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, timeout+gracePeriod)
	defer timeoutCancel()

	ralphCtx, ralphCancel := context.WithCancelCause(timeoutCtx)
	defer ralphCancel(nil)

	detector := newOffRailsDetector(offRailsConfig{
		SilenceAbort: noOutputTimeout,
		Cancel:       ralphCancel,
		Alert:        os.Stderr,
	})
	defer detector.stop()
	detector.start()

	ralphCmd := s.CommandContext(ralphCtx, "bash", "-c", ralphEnv)
	ralphCmd.Dir = workspace
	ralphCmd.SetTTY(true)

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
	ralphCmd.Stdout = stdout
	ralphCmd.Stderr = stderr
	ralphCmd.TextMessageHandler = newDispatchTextMessageHandler(stdout, stderr)

	ralphErr := ralphCmd.Run()

	// Stop off-rails immediately — the agent has finished. Any post-ralph work
	// (PR check polling, verify) is intentional and must not trigger the silence
	// detector.
	detector.stop()

	// 9. Verify work produced
	_, _ = fmt.Fprintf(os.Stderr, "\n=== work produced ===\n")
	verifyScript := fmt.Sprintf(
		`cd %s && echo "--- commits ---" && git log --oneline origin/master..HEAD 2>/dev/null || git log --oneline origin/main..HEAD 2>/dev/null; echo "--- PRs ---" && gh pr list --json url,title 2>/dev/null || echo "(gh not available)"`,
		workspace,
	)
	verifyScript = fmt.Sprintf(`export GH_TOKEN=%q && %s`, ghToken, verifyScript)
	verifyCmd := s.CommandContext(ctx, "bash", "-c", verifyScript)
	verifyCmd.Stdout = os.Stderr
	verifyCmd.Stderr = os.Stderr
	_ = verifyCmd.Run()

	// 10. Snapshot PR CI status (informational; gating is controlled by --pr-check-timeout).
	prs := snapshotPRChecksWithRunner(ctx, spriteBashRunner(s), workspace, ghToken)
	_, _ = fmt.Fprintf(os.Stderr, "dispatch pr-checks: status=%s checks_exit=%d\n", prs.status, prs.checksExit)

	// 11. Return appropriate exit code
	// Check if off-rails detector killed the dispatch
	if cause := context.Cause(ralphCtx); cause != nil && errors.Is(cause, errOffRails) {
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
			hasWork, checkErr = hasNewCommitsWithRunner(ctx, spriteBashRunner(s), workspace)
		}
		if checkErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "\n=== off-rails: new commits check failed: %v — treating as failure ===\n", checkErr)
		} else if hasWork {
			_, _ = fmt.Fprintf(os.Stderr, "\n=== off-rails fired mid-task: new commits found — treating as success ===\n")
			return nil
		}

		return &exitError{Code: 4, Err: cause}
	}

	if ralphErr != nil {
		if exitErr, ok := ralphErr.(*sprites.ExitError); ok {
			code := exitErr.ExitCode()
			switch code {
			case 0:
				// fall through to optional PR check polling
			case 2:
				return &exitError{Code: 2, Err: fmt.Errorf("agent blocked — check BLOCKED.md on sprite")}
			default:
				return &exitError{Code: code, Err: fmt.Errorf("ralph exited %d", code)}
			}
		} else {
			return fmt.Errorf("ralph failed: %w", ralphErr)
		}
	}

	// 12. Optionally wait for PR CI checks to pass.
	if prCheckTimeout > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "\n=== waiting for PR checks (timeout %s) ===\n", prCheckTimeout)
		pollInterval := 30 * time.Second
		if err := waitForPRChecksWithRunner(ctx, spriteBashRunner(s), workspace, ghToken, prCheckTimeout, pollInterval, os.Stderr); err != nil {
			return fmt.Errorf("PR checks: %w", err)
		}
		_, _ = fmt.Fprintf(os.Stderr, "=== PR checks passed ===\n")
	}

	return nil
}

// activeRalphLoopCheckScript checks for an in-flight ralph loop process.
//
// Use the bracket trick to avoid self-matching under `pgrep -f` (pattern appears in argv).
const activeRalphLoopCheckScript = `if ! command -v pgrep >/dev/null 2>&1; then
  echo "pgrep missing" >&2
  exit 2
fi

busy="$(pgrep -af '/home/sprite/workspace/\.[r]alph\.sh' 2>&1)"
status=$?
if [ "$status" -eq 0 ]; then
  echo "$busy"
  exit 1
fi
if [ "$status" -eq 1 ]; then
  exit 0
fi
echo "$busy" >&2
exit "$status"`

const taskCompleteSignalCheckScript = `if [ -f "$WORKSPACE/TASK_COMPLETE" ] || [ -f "$WORKSPACE/TASK_COMPLETE.md" ]; then
  exit 0
fi
exit 1`

// newCommitsCheckScript checks if any commits on HEAD are not yet on origin/master or
// origin/main. Exits 0 with commit list on stdout when new commits exist, exits 1 when
// the branch is flush with upstream (no new work). Exits 2 when not a git repo or when
// neither origin/master nor origin/main exists (no valid upstream baseline).
const newCommitsCheckScript = `
cd "$WORKSPACE" 2>/dev/null || exit 2
commits="$(git log origin/master..HEAD --oneline 2>/dev/null || git log origin/main..HEAD --oneline 2>/dev/null)" || exit 2
if [ -n "$commits" ]; then
  printf '%s\n' "$commits"
  exit 0
fi
exit 1`

// captureHeadSHAScript records the current HEAD commit SHA.
// Exits 0 with SHA on stdout; exits non-zero when not in a git repo.
const captureHeadSHAScript = `
cd "$WORKSPACE" 2>/dev/null || exit 2
git rev-parse HEAD 2>/dev/null || exit 2`

// newCommitsSinceSHACheckScript checks whether new commits exist since BASE_SHA.
// Exits 0 with commit list when commits exist, exits 1 when HEAD == BASE_SHA (no new
// work), exits 2 when not a git repo or BASE_SHA is not a valid ref.
const newCommitsSinceSHACheckScript = `
cd "$WORKSPACE" 2>/dev/null || exit 2
git rev-parse --verify "$BASE_SHA" >/dev/null 2>&1 || exit 2
commits="$(git log "$BASE_SHA"..HEAD --oneline 2>/dev/null)" || exit 2
if [ -n "$commits" ]; then
  printf '%s\n' "$commits"
  exit 0
fi
exit 1`

// prChecksScript checks whether all PR CI checks for the current HEAD have passed.
// Exits 0 when all checks pass, 1 when checks are still pending, 2 on error (no PR, no git, etc.).
const prChecksScript = `
cd "$WORKSPACE" 2>/dev/null || exit 2
gh pr checks HEAD --exit-status 2>&1
exit_code=$?
if [ "$exit_code" -eq 0 ] || [ "$exit_code" -eq 1 ]; then
  exit $exit_code
fi
exit 2`

type spriteScriptRunner func(ctx context.Context, script string) ([]byte, int, error)

type prCheckSummary struct {
	status     string
	checksExit int
}

func ensureNoActiveDispatchLoop(ctx context.Context, s *sprites.Sprite) error {
	return ensureNoActiveDispatchLoopWithRunner(ctx, spriteBashRunner(s))
}

// isDispatchLoopActive returns true when a ralph loop is running on s.
// It uses the same pgrep check as ensureNoActiveDispatchLoop.
func isDispatchLoopActive(ctx context.Context, s *sprites.Sprite) (bool, error) {
	return isDispatchLoopActiveWithRunner(ctx, spriteBashRunner(s))
}

// runRalphLoopCheck executes the pgrep check and returns the raw result.
func runRalphLoopCheck(ctx context.Context, run spriteScriptRunner) (output string, exitCode int, err error) {
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, code, runErr := run(checkCtx, activeRalphLoopCheckScript)
	if runErr != nil {
		return "", 0, fmt.Errorf("check dispatch loop: %w", runErr)
	}
	return strings.TrimSpace(string(out)), code, nil
}

func isDispatchLoopActiveWithRunner(ctx context.Context, run spriteScriptRunner) (bool, error) {
	_, exitCode, err := runRalphLoopCheck(ctx, run)
	if err != nil {
		return false, err
	}

	switch exitCode {
	case 0:
		return false, nil // pgrep found no ralph loop
	case 1:
		return true, nil // active ralph loop detected
	default:
		return false, fmt.Errorf("check dispatch loop exited %d", exitCode)
	}
}

func spriteBashRunner(s *sprites.Sprite) spriteScriptRunner {
	return func(ctx context.Context, script string) ([]byte, int, error) {
		out, err := s.CommandContext(ctx, "bash", "-c", script).CombinedOutput()
		if err == nil {
			return out, 0, nil
		}

		var exitErr *sprites.ExitError
		if errors.As(err, &exitErr) {
			return out, exitErr.ExitCode(), nil
		}

		return out, 0, err
	}
}

// graceFor returns a proportional grace period: at least 30s, otherwise 25%
// of the dispatch timeout, capped at 5 minutes. This gives the ralph loop
// time to write TASK_COMPLETE/BLOCKED signals after its own timeout fires.
func graceFor(timeout time.Duration) time.Duration {
	grace := max(30*time.Second, timeout/4)
	if grace > 5*time.Minute {
		grace = 5 * time.Minute
	}
	return grace
}

// hasNewCommitsWithRunner returns true when commits exist on HEAD that are not
// present on origin/master or origin/main. This is used as a secondary off-rails
// backstop: if the detector fired while the agent was mid-task (e.g. waiting for CI),
// and work exists on the branch, dispatch succeeds with a warning rather than failing.
func hasNewCommitsWithRunner(ctx context.Context, run spriteScriptRunner, workspace string) (bool, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	checkScript := fmt.Sprintf("export WORKSPACE=%q\n%s", workspace, newCommitsCheckScript)
	out, exitCode, err := run(checkCtx, checkScript)
	if err != nil {
		return false, fmt.Errorf("check new commits: %w", err)
	}

	switch exitCode {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, fmt.Errorf("new commits check exited %d: %s", exitCode, strings.TrimSpace(string(out)))
	}
}

// captureHeadSHAWithRunner records the current HEAD SHA in workspace before the ralph
// loop starts. The returned SHA is used by hasNewCommitsSinceSHAWithRunner to scope
// the off-rails secondary commit check to the current dispatch.
func captureHeadSHAWithRunner(ctx context.Context, run spriteScriptRunner, workspace string) (string, error) {
	captureCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	script := fmt.Sprintf("export WORKSPACE=%q\n%s", workspace, captureHeadSHAScript)
	out, exitCode, err := run(captureCtx, script)
	if err != nil {
		return "", fmt.Errorf("capture HEAD SHA: %w", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("capture HEAD SHA: exited %d: %s", exitCode, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// hasNewCommitsSinceSHAWithRunner returns true when commits exist on HEAD that were
// not present at baseSHA. This scopes the off-rails secondary commit check to work
// produced during the current dispatch, ignoring stale commits from prior runs.
func hasNewCommitsSinceSHAWithRunner(ctx context.Context, run spriteScriptRunner, workspace, baseSHA string) (bool, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	script := fmt.Sprintf("export WORKSPACE=%q BASE_SHA=%q\n%s", workspace, baseSHA, newCommitsSinceSHACheckScript)
	out, exitCode, err := run(checkCtx, script)
	if err != nil {
		return false, fmt.Errorf("check new commits since SHA: %w", err)
	}

	switch exitCode {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, fmt.Errorf("new commits since SHA check exited %d: %s", exitCode, strings.TrimSpace(string(out)))
	}
}

func hasTaskCompleteSignalWithRunner(ctx context.Context, run spriteScriptRunner, workspace string) (bool, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	checkScript := fmt.Sprintf("export WORKSPACE=%q\n%s", workspace, taskCompleteSignalCheckScript)
	out, exitCode, err := run(checkCtx, checkScript)
	if err != nil {
		return false, fmt.Errorf("check completion signal command failed: %w", err)
	}

	switch exitCode {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, fmt.Errorf("completion signal check exited %d: %s", exitCode, strings.TrimSpace(string(out)))
	}
}

// prChecksScriptFor builds the shell snippet that runs prChecksScript with
// the given workspace and GitHub token. Shared by snapshot and wait-for-checks.
func prChecksScriptFor(workspace, ghToken string) string {
	return fmt.Sprintf("export WORKSPACE=%q GH_TOKEN=%q\n%s", workspace, ghToken, prChecksScript)
}

func snapshotPRChecksWithRunner(ctx context.Context, run spriteScriptRunner, workspace, ghToken string) prCheckSummary {
	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, exitCode, err := run(checkCtx, prChecksScriptFor(workspace, ghToken))
	if err != nil {
		return prCheckSummary{status: "error", checksExit: -1}
	}

	switch exitCode {
	case 0:
		return prCheckSummary{status: "pass", checksExit: exitCode}
	case 1:
		return prCheckSummary{status: "pending", checksExit: exitCode}
	default:
		return prCheckSummary{status: "error", checksExit: exitCode}
	}
}

// waitForPRChecksWithRunner polls the sprite for PR CI check status until all
// checks pass, prCheckTimeout elapses, or ctx is cancelled. It emits a progress
// line to progress on every poll interval so operators can see activity.
//
// prCheckTimeout == 0 is a no-op (returns nil immediately without calling run).
func waitForPRChecksWithRunner(ctx context.Context, run spriteScriptRunner, workspace, ghToken string, prCheckTimeout, pollInterval time.Duration, progress io.Writer) error {
	if prCheckTimeout == 0 {
		return nil
	}

	deadline := time.Now().Add(prCheckTimeout)
	pollCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	script := prChecksScriptFor(workspace, ghToken)

	for {
		checkCtx, checkCancel := context.WithTimeout(pollCtx, 30*time.Second)
		_, exitCode, err := run(checkCtx, script)
		checkCancel()

		if err != nil {
			_, _ = fmt.Fprintf(progress, "[dispatch] PR checks: runner error: %v\n", err)
		} else {
			switch exitCode {
			case 0:
				return nil // all checks passed
			case 1:
				_, _ = fmt.Fprintf(progress, "[dispatch] PR checks: pending — next poll in %s\n", pollInterval)
			case 2:
				return fmt.Errorf("pr checks error (gh unavailable, no PR for HEAD, or auth failure)")
			default:
				_, _ = fmt.Fprintf(progress, "[dispatch] PR checks: unexpected exit %d — retrying\n", exitCode)
			}
		}

		select {
		case <-pollCtx.Done():
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("pr-check-timeout (%s) elapsed: CI checks did not pass in time", prCheckTimeout)
		case <-ticker.C:
		}
	}
}

func ensureNoActiveDispatchLoopWithRunner(ctx context.Context, run spriteScriptRunner) error {
	output, exitCode, err := runRalphLoopCheck(ctx, run)
	if err != nil {
		return err
	}

	switch exitCode {
	case 0:
		if output != "" {
			return fmt.Errorf("unexpected output from idle check:\n%s", output)
		}
		return nil
	case 1:
		if output == "" {
			output = "(process list empty)"
		}
		return fmt.Errorf("active dispatch loop detected:\n%s", output)
	default:
		if output == "" {
			return fmt.Errorf("check dispatch loop exited %d", exitCode)
		}
		return fmt.Errorf("check dispatch loop exited %d:\n%s", exitCode, output)
	}
}

// renderPrompt reads the local ralph-prompt-template.md and substitutes placeholders.
func renderPrompt(taskDescription, repo, spriteName string) (string, error) {
	tmpl, err := os.ReadFile("scripts/ralph-prompt-template.md")
	if err != nil {
		return "", fmt.Errorf("read template: %w (are you running from the repo root?)", err)
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
