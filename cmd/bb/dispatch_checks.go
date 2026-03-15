package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	sprites "github.com/superfly/sprites-go"
)

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

// newCommitsCheckScript checks if any commits on HEAD are not yet on the remote
// default branch ($BRANCH). Exits 0 with commit list on stdout when new commits
// exist, exits 1 when the branch is flush with upstream (no new work). Exits 2
// when not a git repo or origin/$BRANCH does not exist.
const newCommitsCheckScript = `
cd "$WORKSPACE" 2>/dev/null || exit 2
commits="$(git log "origin/$BRANCH"..HEAD --oneline 2>/dev/null)" || exit 2
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

// detectDefaultBranchScript resolves the remote default branch from origin/HEAD.
// Exits 0 with the branch name (e.g. "main", "master", "development") on stdout.
// Falls back to checking whether origin/master exists, then "main" as a last resort.
//
// Guard: when origin/HEAD is unset, git rev-parse may output "origin/HEAD" literally
// (stripped to "HEAD"). We reject that value and fall through to the master/main probe.
const detectDefaultBranchScript = `
cd "$WORKSPACE" 2>/dev/null || { echo "main"; exit 0; }
branch="$(git rev-parse --abbrev-ref origin/HEAD 2>/dev/null)"
branch="${branch#origin/}"
if [ -n "$branch" ] && [ "$branch" != "HEAD" ]; then
  echo "$branch"
  exit 0
fi
if git rev-parse --verify origin/master >/dev/null 2>&1; then
  echo "master"
  exit 0
fi
echo "main"`

// prChecksScript checks whether all PR CI checks for the current HEAD have passed.
// Exits 0 when all checks pass, 1 when checks are still pending, 2 on error (no PR, no git, etc.).
const prChecksScript = `
cd "$WORKSPACE" 2>/dev/null || exit 2
gh pr checks HEAD --exit-status 2>&1
exit_code=$?
if [ "$exit_code" -eq 0 ] || [ "$exit_code" -eq 1 ] || [ "$exit_code" -eq 8 ]; then
  exit $exit_code
fi
exit 2`

type spriteScriptRunner func(ctx context.Context, script string) ([]byte, int, error)

type prCheckSummary struct {
	status     string
	checksExit int
}

type dispatchCheck struct {
	timeout time.Duration
	script  string
}

type dispatchPollConfig struct {
	check             dispatchCheck
	waitTimeout       time.Duration
	pollInterval      time.Duration
	runnerErrorPrefix string
	timeoutMessage    string
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
	output, exitCode, err = runDispatchCheck(ctx, run, dispatchCheck{
		timeout: 10 * time.Second,
		script:  activeRalphLoopCheckScript,
	})
	if err != nil {
		return "", 0, fmt.Errorf("check dispatch loop: %w", err)
	}
	return output, exitCode, nil
}

func isDispatchLoopActiveWithRunner(ctx context.Context, run spriteScriptRunner) (bool, error) {
	_, exitCode, err := runRalphLoopCheck(ctx, run)
	if err != nil {
		return false, err
	}

	switch exitCode {
	case 0:
		return false, nil
	case 1:
		return true, nil
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

// runDispatchCheck executes one dispatch-side remote check with its own bounded timeout.
func runDispatchCheck(ctx context.Context, run spriteScriptRunner, check dispatchCheck) (output string, exitCode int, err error) {
	checkCtx, cancel := context.WithTimeout(ctx, check.timeout)
	defer cancel()

	out, exitCode, err := run(checkCtx, check.script)
	if err != nil {
		return "", 0, err
	}
	return strings.TrimSpace(string(out)), exitCode, nil
}

func pollDispatchCheck(ctx context.Context, run spriteScriptRunner, cfg dispatchPollConfig, progress io.Writer, step func(exitCode int, output string) (done bool, progressLine string, err error)) error {
	if cfg.waitTimeout == 0 {
		return nil
	}
	if cfg.pollInterval <= 0 {
		return fmt.Errorf("invalid poll interval %s", cfg.pollInterval)
	}

	deadline := time.Now().Add(cfg.waitTimeout)
	pollCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	ticker := time.NewTicker(cfg.pollInterval)
	defer ticker.Stop()

	for {
		output, exitCode, err := runDispatchCheck(pollCtx, run, cfg.check)
		if err != nil {
			if progress != nil {
				if _, writeErr := fmt.Fprintf(progress, "[dispatch] %s: %v\n", cfg.runnerErrorPrefix, err); writeErr != nil {
					return fmt.Errorf("write dispatch progress: %w", writeErr)
				}
			}
		} else {
			done, progressLine, stepErr := step(exitCode, output)
			if stepErr != nil {
				return stepErr
			}
			if done {
				return nil
			}
			if progress != nil && progressLine != "" {
				if _, writeErr := fmt.Fprintf(progress, "[dispatch] %s\n", progressLine); writeErr != nil {
					return fmt.Errorf("write dispatch progress: %w", writeErr)
				}
			}
		}

		select {
		case <-pollCtx.Done():
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.New(cfg.timeoutMessage)
		case <-ticker.C:
		}
	}
}

// detectDefaultBranchWithRunner resolves the remote default branch from origin/HEAD.
// Returns "main" when origin/HEAD is not configured and origin/master is absent.
// Returns an error when the detected name contains characters unsafe for shell use.
func detectDefaultBranchWithRunner(ctx context.Context, run spriteScriptRunner, workspace string) (string, error) {
	output, exitCode, err := runDispatchCheck(ctx, run, dispatchCheck{
		timeout: 10 * time.Second,
		script:  fmt.Sprintf("export WORKSPACE=%q\n%s", workspace, detectDefaultBranchScript),
	})
	if err != nil {
		return "", fmt.Errorf("detect default branch: %w", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("detect default branch: exited %d: %s", exitCode, output)
	}
	if output == "" {
		return "main", nil
	}
	if !isValidBranchName(output) {
		return "", fmt.Errorf("detect default branch: invalid branch name %q", output)
	}
	return output, nil
}

// isValidBranchName reports whether name is safe to use in shell commands.
// Accepts only characters that valid git branch names use in practice; rejects
// shell metacharacters and the literal "HEAD" (produced when origin/HEAD is unset).
func isValidBranchName(name string) bool {
	if name == "" || name == "HEAD" {
		return false
	}
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '-' || c == '_' || c == '.' || c == '/':
		default:
			return false
		}
	}
	return true
}

// hasNewCommitsWithRunner returns true when commits exist on HEAD that are not
// present on origin/<defaultBranch>. This is used as a secondary off-rails
// backstop: if the detector fired while the agent was mid-task (e.g. waiting for CI),
// and work exists on the branch, dispatch succeeds with a warning rather than failing.
func hasNewCommitsWithRunner(ctx context.Context, run spriteScriptRunner, workspace, defaultBranch string) (bool, error) {
	output, exitCode, err := runDispatchCheck(ctx, run, dispatchCheck{
		timeout: 15 * time.Second,
		script:  fmt.Sprintf("export WORKSPACE=%q BRANCH=%q\n%s", workspace, defaultBranch, newCommitsCheckScript),
	})
	if err != nil {
		return false, fmt.Errorf("check new commits: %w", err)
	}

	switch exitCode {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, fmt.Errorf("new commits check exited %d: %s", exitCode, output)
	}
}

// captureHeadSHAWithRunner records the current HEAD SHA in workspace before the ralph
// loop starts. The returned SHA is used by hasNewCommitsSinceSHAWithRunner to scope
// the off-rails secondary commit check to the current dispatch.
func captureHeadSHAWithRunner(ctx context.Context, run spriteScriptRunner, workspace string) (string, error) {
	output, exitCode, err := runDispatchCheck(ctx, run, dispatchCheck{
		timeout: 10 * time.Second,
		script:  fmt.Sprintf("export WORKSPACE=%q\n%s", workspace, captureHeadSHAScript),
	})
	if err != nil {
		return "", fmt.Errorf("capture HEAD SHA: %w", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("capture HEAD SHA: exited %d: %s", exitCode, output)
	}
	if output == "" {
		return "", fmt.Errorf("capture HEAD SHA: git returned empty output")
	}
	return output, nil
}

// hasNewCommitsSinceSHAWithRunner returns true when commits exist on HEAD that were
// not present at baseSHA. This scopes the off-rails secondary commit check to work
// produced during the current dispatch, ignoring stale commits from prior runs.
func hasNewCommitsSinceSHAWithRunner(ctx context.Context, run spriteScriptRunner, workspace, baseSHA string) (bool, error) {
	output, exitCode, err := runDispatchCheck(ctx, run, dispatchCheck{
		timeout: 15 * time.Second,
		script:  fmt.Sprintf("export WORKSPACE=%q BASE_SHA=%q\n%s", workspace, baseSHA, newCommitsSinceSHACheckScript),
	})
	if err != nil {
		return false, fmt.Errorf("check new commits since SHA: %w", err)
	}

	switch exitCode {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, fmt.Errorf("new commits since SHA check exited %d: %s", exitCode, output)
	}
}

func hasTaskCompleteSignalWithRunner(ctx context.Context, run spriteScriptRunner, workspace string) (bool, error) {
	output, exitCode, err := runDispatchCheck(ctx, run, dispatchCheck{
		timeout: 10 * time.Second,
		script:  taskCompleteSignalCheckScriptFor(workspace),
	})
	if err != nil {
		return false, fmt.Errorf("check completion signal command failed: %w", err)
	}

	switch exitCode {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, fmt.Errorf("completion signal check exited %d: %s", exitCode, output)
	}
}

// prChecksScriptFor builds the shell snippet that runs prChecksScript with
// the given workspace. Shared by snapshot and wait-for-checks.
func prChecksScriptFor(workspace string) string {
	return fmt.Sprintf("export WORKSPACE=%q\n%s", workspace, prChecksScript)
}

func snapshotPRChecksWithRunner(ctx context.Context, run spriteScriptRunner, workspace string) prCheckSummary {
	_, exitCode, err := runDispatchCheck(ctx, run, dispatchCheck{
		timeout: 30 * time.Second,
		script:  prChecksScriptFor(workspace),
	})
	if err != nil {
		return prCheckSummary{status: "error", checksExit: -1}
	}

	switch exitCode {
	case 0:
		return prCheckSummary{status: "pass", checksExit: exitCode}
	case 1, 8:
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
func waitForPRChecksWithRunner(ctx context.Context, run spriteScriptRunner, workspace string, prCheckTimeout, pollInterval time.Duration, progress io.Writer) error {
	return pollDispatchCheck(ctx, run, dispatchPollConfig{
		check: dispatchCheck{
			timeout: 30 * time.Second,
			script:  prChecksScriptFor(workspace),
		},
		waitTimeout:       prCheckTimeout,
		pollInterval:      pollInterval,
		runnerErrorPrefix: "PR checks: runner error",
		timeoutMessage:    fmt.Sprintf("pr-check-timeout (%s) elapsed: CI checks did not pass in time", prCheckTimeout),
	}, progress, func(exitCode int, _ string) (bool, string, error) {
		switch exitCode {
		case 0:
			return true, "", nil
		case 1, 8:
			return false, fmt.Sprintf("PR checks: pending — next poll in %s", pollInterval), nil
		case 2:
			return false, "", fmt.Errorf("pr checks error (gh unavailable, no PR for HEAD, or auth failure)")
		default:
			return false, fmt.Sprintf("PR checks: unexpected exit %d — retrying", exitCode), nil
		}
	})
}

// waitForTaskCompleteWithRunner polls the sprite for the TASK_COMPLETE signal
// status until found, timeout elapses, or ctx is cancelled. It emits a progress
// line to progress on every poll interval so operators can see activity.
// If the signal is found immediately, returns nil without polling.
func waitForTaskCompleteWithRunner(ctx context.Context, run spriteScriptRunner, workspace string, waitTimeout, pollInterval time.Duration, progress io.Writer) error {
	return pollDispatchCheck(ctx, run, dispatchPollConfig{
		check: dispatchCheck{
			timeout: 30 * time.Second,
			script:  taskCompleteSignalCheckScriptFor(workspace),
		},
		waitTimeout:       waitTimeout,
		pollInterval:      pollInterval,
		runnerErrorPrefix: "task complete check: runner error",
		timeoutMessage:    fmt.Sprintf("wait-timeout (%s) elapsed: task did not complete in time", waitTimeout),
	}, progress, func(exitCode int, _ string) (bool, string, error) {
		switch exitCode {
		case 0:
			return true, "", nil
		case 1:
			return false, fmt.Sprintf("task complete: not yet complete — next poll in %s", pollInterval), nil
		default:
			return false, "", fmt.Errorf("task complete check error (unexpected exit %d)", exitCode)
		}
	})
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
