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
	"strconv"
	"strings"
	"time"

	sprites "github.com/superfly/sprites-go"

	"github.com/spf13/cobra"
)

func newDispatchCmd() *cobra.Command {
	var (
		repo            string
		timeout         time.Duration
		noOutputTimeout time.Duration
		requireGreenPR  bool
		prCheckTimeout  time.Duration
	)

	cmd := &cobra.Command{
		Use:   "dispatch <sprite> <prompt>",
		Short: "Dispatch a task to a sprite via Claude Code Ralph plugin flow",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			spriteName := args[0]
			prompt := args[1]

			return runDispatch(cmd.Context(), spriteName, prompt, repo, timeout, noOutputTimeout, requireGreenPR, prCheckTimeout)
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repo (owner/repo)")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "Max wall-clock time for the dispatch run")
	cmd.Flags().DurationVar(&noOutputTimeout, "no-output-timeout", defaultSilenceAbortThreshold, "Abort if no output for this duration (0 to disable)")
	cmd.Flags().BoolVar(&requireGreenPR, "require-green-pr", true, "Require open PR checks to be green before returning success")
	cmd.Flags().DurationVar(&prCheckTimeout, "pr-check-timeout", 4*time.Minute, "Max time to wait for open PR checks when --require-green-pr is true (0 = snapshot only)")
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}

func runDispatch(
	ctx context.Context,
	spriteName, prompt, repo string,
	timeout time.Duration,
	noOutputTimeout time.Duration,
	requireGreenPR bool,
	prCheckTimeout time.Duration,
) error {
	if timeout <= 0 {
		return fmt.Errorf("--timeout must be > 0")
	}
	if prCheckTimeout < 0 {
		return fmt.Errorf("--pr-check-timeout must be >= 0")
	}

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
	syncCtx, syncCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer syncCancel()
	syncCmd := s.CommandContext(syncCtx, "bash", "-c", syncScript)
	if out, err := syncCmd.Output(); err != nil {
		if errors.Is(syncCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("repo sync timed out after %s", 2*time.Minute)
		}
		return fmt.Errorf("repo sync failed: %w\n%s", err, out)
	}

	// 6. Clean stale signals
	cleanScript := fmt.Sprintf(
		"rm -f %s/TASK_COMPLETE %s/TASK_COMPLETE.md %s/BLOCKED.md",
		workspace, workspace, workspace,
	)
	_, _ = s.CommandContext(ctx, "bash", "-c", cleanScript).Output()

	// 7. Render and upload prompt
	rendered, err := renderPrompt(prompt, repo, spriteName)
	if err != nil {
		return fmt.Errorf("render prompt: %w", err)
	}

	promptPath := workspace + "/.dispatch-prompt.md"
	if err := s.Filesystem().WriteFileContext(ctx, promptPath, []byte(rendered), 0644); err != nil {
		return fmt.Errorf("upload prompt: %w", err)
	}

	// 8. Run the official Claude Ralph plugin flow via a thin wrapper script.
	_, _ = fmt.Fprintf(os.Stderr, "starting claude run (timeout %s)...\n", timeout)

	timeoutSec := int(timeout.Seconds())
	ralphEnv := fmt.Sprintf(
		`export WORKSPACE=%q GH_TOKEN=%q LEFTHOOK=0 BB_TIMEOUT_SEC=%d ANTHROPIC_MODEL=%q ANTHROPIC_DEFAULT_SONNET_MODEL=%q CLAUDE_CODE_SUBAGENT_MODEL=%q`,
		workspace, ghToken, timeoutSec,
		"anthropic/claude-sonnet-4-6",
		"anthropic/claude-sonnet-4-6",
		"anthropic/claude-sonnet-4-6",
	)

	ralphEnv += fmt.Sprintf(` && exec bash %s`, ralphScript)

	grace := dispatchGracePeriod(timeout)
	effectiveTimeout := timeout + grace
	_, _ = fmt.Fprintf(os.Stderr, "dispatch timeout window: requested=%s grace=%s effective=%s\n", timeout, grace, effectiveTimeout)
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, effectiveTimeout)
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
	// Off-rails detection only applies while the agent run is active.
	// Stop it before post-run verification / PR-check waiting.
	detector.stop()

	// 9. Verify work produced
	_, _ = fmt.Fprintf(os.Stderr, "\n=== work produced ===\n")
	verifyScript := fmt.Sprintf(
		`cd %s && echo "--- commits ---" && git log --oneline origin/master..HEAD 2>/dev/null || git log --oneline origin/main..HEAD 2>/dev/null; echo "--- PRs ---" && gh pr list --json url,title 2>/dev/null || echo "(gh not available)"`,
		workspace,
	)
	verifyScript = fmt.Sprintf(`export GH_TOKEN=%q && %s`, ghToken, verifyScript)
	verifyCtx, verifyCancel := context.WithTimeout(ctx, 30*time.Second)
	defer verifyCancel()
	verifyCmd := s.CommandContext(verifyCtx, "bash", "-c", verifyScript)
	verifyCmd.Stdout = os.Stderr
	verifyCmd.Stderr = os.Stderr
	_ = verifyCmd.Run()

	// 10. Return appropriate exit code
	runner := spriteBashRunner(s)
	outcome, outcomeErr := inspectDispatchOutcome(ctx, runner, workspace, ghToken)
	if outcomeErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "dispatch outcome check failed: %v\n", outcomeErr)
	} else {
		_, _ = fmt.Fprintf(os.Stderr,
			"dispatch outcome: task_complete=%t blocked=%t branch=%q dirty_files=%d commits_ahead=%d open_prs=%d pr_number=%d pr_query=%s\n",
			outcome.TaskComplete, outcome.Blocked, outcome.Branch, outcome.DirtyFiles, outcome.CommitsAhead, outcome.OpenPRCount, outcome.PRNumber, outcome.PRQueryState)
	}

	cause := context.Cause(ralphCtx)
	offRailsTriggered := cause != nil && errors.Is(cause, errOffRails)
	if offRailsTriggered {
		_, _ = fmt.Fprintf(os.Stderr, "\n=== off-rails detected: %v ===\n", cause)
	}

	var prChecks prCheckOutcome
	var prChecksErr error
	if outcomeErr == nil && outcome.OpenPRCount > 0 && outcome.PRQueryState == prQueryStateOK {
		_, _ = fmt.Fprintf(os.Stderr, "dispatch pr-checks: waiting pr_number=%d timeout=%s\n", outcome.PRNumber, prCheckTimeout)
		var lastProgress time.Duration
		prChecks, prChecksErr = waitForPRChecks(ctx, runner, outcome.PRNumber, ghToken, prCheckTimeout, 10*time.Second, func(elapsed time.Duration) {
			if elapsed-lastProgress < 30*time.Second {
				return
			}
			lastProgress = elapsed
			_, _ = fmt.Fprintf(os.Stderr, "dispatch pr-checks: still pending elapsed=%s timeout=%s\n", elapsed.Round(time.Second), prCheckTimeout)
		})
		if prChecksErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "dispatch pr-checks failed: %v\n", prChecksErr)
		} else {
			timeoutNote := ""
			if prChecks.TimedOut {
				timeoutNote = fmt.Sprintf(" timed_out_after=%s", prCheckTimeout)
			}
			_, _ = fmt.Fprintf(os.Stderr,
				"dispatch pr-checks: require_green=%t pr_number=%d status=%s checks_exit=%d%s\n",
				requireGreenPR, outcome.PRNumber, prChecks.Status, prChecks.ChecksExit, timeoutNote)
		}
	}

	if outcomeErr == nil {
		if err := enforceDispatchPRReadiness(outcome, requireGreenPR, prChecks, prChecksErr, prCheckTimeout); err != nil {
			return err
		}
	}

	if outcomeErr == nil {
		if outcome.Blocked {
			return &exitError{Code: 2, Err: fmt.Errorf("agent blocked — check BLOCKED.md on sprite")}
		}
		if outcome.TaskComplete {
			_, _ = fmt.Fprintf(os.Stderr, "\n=== task completed: TASK_COMPLETE signal found ===\n")
			return nil
		}
		if hasSuccessfulDispatchArtifacts(outcome) {
			_, _ = fmt.Fprintf(os.Stderr, "\n=== task completed: open PR + clean workspace fallback ===\n")
			return nil
		}
	}

	if offRailsTriggered {
		return &exitError{Code: 1, Err: fmt.Errorf("dispatch stopped by off-rails detector: %w", cause)}
	}

	if ralphErr != nil {
		if exitErr, ok := ralphErr.(*sprites.ExitError); ok {
			code := exitErr.ExitCode()
			switch code {
			case 0:
				// Continue to final outcome classification below.
			case 2:
				return &exitError{Code: 2, Err: fmt.Errorf("agent blocked — check BLOCKED.md on sprite")}
			default:
				return &exitError{Code: 1, Err: fmt.Errorf("ralph exited %d", code)}
			}
		} else {
			return fmt.Errorf("ralph failed: %w", ralphErr)
		}
	}

	if outcomeErr != nil {
		return &exitError{Code: 1, Err: fmt.Errorf("dispatch outcome unknown: %w", outcomeErr)}
	}

	if outcome.CommitsAhead > 0 {
		return &exitError{Code: 1, Err: fmt.Errorf("dispatch produced commits but no open PR and no TASK_COMPLETE signal")}
	}

	return &exitError{Code: 1, Err: fmt.Errorf("dispatch finished without completion signal or work artifacts")}
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

const dispatchOutcomeCheckScript = `task_complete=0
blocked=0
branch=
dirty_files=0
commits_ahead=0
open_pr_count=0
pr_number=0
pr_query_state=unknown

if [ -f "$WORKSPACE/TASK_COMPLETE" ] || [ -f "$WORKSPACE/TASK_COMPLETE.md" ]; then
  task_complete=1
fi
if [ -f "$WORKSPACE/BLOCKED.md" ]; then
  blocked=1
fi

if [ -d "$WORKSPACE/.git" ]; then
  cd "$WORKSPACE" || true
  branch="$(git branch --show-current 2>/dev/null || echo '')"
  dirty_files="$(git status --porcelain 2>/dev/null | wc -l | tr -d ' ' || echo 0)"
  commits_ahead="$(git rev-list --count origin/master..HEAD 2>/dev/null || git rev-list --count origin/main..HEAD 2>/dev/null || echo 0)"

  if [ -z "$branch" ]; then
    pr_query_state=no_branch
  elif ! command -v gh >/dev/null 2>&1; then
    pr_query_state=gh_missing
  else
    # Retry briefly after ralph exits: PR creation visibility can lag.
    for attempt in 1 2 3; do
      prs="$(gh pr list --state open --json number,headRefName --jq '.[] | [.number, .headRefName] | @tsv' 2>/dev/null)"
      prs_exit=$?
      if [ "$prs_exit" -eq 0 ]; then
        pr_query_state=ok
        if [ -n "$prs" ]; then
          open_pr_count="$(printf '%s\n' "$prs" | awk -F '\t' -v b="$branch" '$2==b {c++} END {print c+0}')"
          pr_number="$(printf '%s\n' "$prs" | awk -F '\t' -v b="$branch" '$2==b {print $1; exit}')"
        else
          open_pr_count=0
          pr_number=0
        fi
        [ -z "$pr_number" ] && pr_number=0
        if [ "${open_pr_count:-0}" -gt 0 ] 2>/dev/null; then
          break
        fi
      else
        pr_query_state=query_error
        open_pr_count=0
        pr_number=0
      fi
      [ "$attempt" -lt 3 ] && sleep 2
    done
  fi
fi

printf 'task_complete=%s\n' "$task_complete"
printf 'blocked=%s\n' "$blocked"
printf 'branch=%s\n' "$branch"
printf 'dirty_files=%s\n' "$dirty_files"
printf 'commits_ahead=%s\n' "$commits_ahead"
printf 'open_pr_count=%s\n' "$open_pr_count"
printf 'pr_number=%s\n' "$pr_number"
printf 'pr_query_state=%s\n' "$pr_query_state"`

const prCheckStatusScript = `status=unknown
checks_exit=0

if [ "${PR_NUMBER:-0}" -le 0 ] 2>/dev/null; then
  status=no_pr
else
  if ! command -v gh >/dev/null 2>&1; then
    status=gh_missing
	else
		checks="$(gh pr checks "$PR_NUMBER" 2>&1)"
		checks_exit=$?
		if [ "$checks_exit" -eq 0 ]; then
			status=pass
		elif printf '%s\n' "$checks" | grep -Eiq '(^|[[:space:]])(fail|failing|cancel|cancelled|timed[- ]?out|action_required)([[:space:]]|$)'; then
			status=fail
		else
			# gh pr checks exits non-zero for pending/no-check-yet states and other transient conditions.
			# Treat unknown non-zero states as pending; timeout handling in Go converts prolonged pending to failure.
			status=pending
		fi
	fi
fi

printf 'status=%s\n' "$status"
printf 'checks_exit=%s\n' "$checks_exit"`

type spriteScriptRunner func(ctx context.Context, script string) ([]byte, int, error)

type dispatchOutcome struct {
	TaskComplete bool
	Blocked      bool
	Branch       string
	DirtyFiles   int
	CommitsAhead int
	OpenPRCount  int
	PRNumber     int
	PRQueryState prQueryState
}

func ensureNoActiveDispatchLoop(ctx context.Context, s *sprites.Sprite) error {
	return ensureNoActiveDispatchLoopWithRunner(ctx, spriteBashRunner(s))
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

func inspectDispatchOutcome(ctx context.Context, run spriteScriptRunner, workspace, ghToken string) (dispatchOutcome, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	checkScript := fmt.Sprintf("export WORKSPACE=%q GH_TOKEN=%q\n%s", workspace, ghToken, dispatchOutcomeCheckScript)
	out, exitCode, err := run(checkCtx, checkScript)
	if err != nil {
		return dispatchOutcome{}, fmt.Errorf("inspect dispatch outcome command failed: %w", err)
	}
	if exitCode != 0 {
		return dispatchOutcome{}, fmt.Errorf("inspect dispatch outcome exited %d: %s", exitCode, strings.TrimSpace(string(out)))
	}

	kv := parseKVOutput(out)
	outcome := dispatchOutcome{
		TaskComplete: parseKVBool(kv["task_complete"]),
		Blocked:      parseKVBool(kv["blocked"]),
		Branch:       kv["branch"],
	}
	queryState := prQueryState(strings.TrimSpace(kv["pr_query_state"]))
	if queryState == "" {
		queryState = prQueryStateUnknown
	}
	outcome.PRQueryState = queryState

	dirty, err := parseKVInt("dirty_files", kv["dirty_files"])
	if err != nil {
		return dispatchOutcome{}, err
	}
	outcome.DirtyFiles = dirty

	commitsAhead, err := parseKVInt("commits_ahead", kv["commits_ahead"])
	if err != nil {
		return dispatchOutcome{}, err
	}
	outcome.CommitsAhead = commitsAhead

	openPRCount, err := parseKVInt("open_pr_count", kv["open_pr_count"])
	if err != nil {
		return dispatchOutcome{}, err
	}
	outcome.OpenPRCount = openPRCount

	prNumber, err := parseKVInt("pr_number", kv["pr_number"])
	if err != nil {
		return dispatchOutcome{}, err
	}
	outcome.PRNumber = prNumber

	return outcome, nil
}

type prCheckStatus string

const (
	prCheckStatusPass      prCheckStatus = "pass"
	prCheckStatusFail      prCheckStatus = "fail"
	prCheckStatusPending   prCheckStatus = "pending"
	prCheckStatusNoPR      prCheckStatus = "no_pr"
	prCheckStatusGHMissing prCheckStatus = "gh_missing"
	prCheckStatusUnknown   prCheckStatus = "unknown"
)

type prCheckOutcome struct {
	Status     prCheckStatus
	ChecksExit int
	TimedOut   bool
}

func inspectPRCheckOutcome(ctx context.Context, run spriteScriptRunner, prNumber int, ghToken string) (prCheckOutcome, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	checkScript := fmt.Sprintf("export PR_NUMBER=%d GH_TOKEN=%q\n%s", prNumber, ghToken, prCheckStatusScript)
	out, exitCode, err := run(checkCtx, checkScript)
	if err != nil {
		return prCheckOutcome{}, fmt.Errorf("inspect pr checks command failed: %w", err)
	}
	if exitCode != 0 {
		return prCheckOutcome{}, fmt.Errorf("inspect pr checks exited %d: %s", exitCode, strings.TrimSpace(string(out)))
	}

	kv := parseKVOutput(out)
	status := prCheckStatus(strings.TrimSpace(kv["status"]))
	if status == "" {
		status = prCheckStatusUnknown
	}
	checksExit, err := parseKVInt("checks_exit", kv["checks_exit"])
	if err != nil {
		return prCheckOutcome{}, err
	}

	return prCheckOutcome{
		Status:     status,
		ChecksExit: checksExit,
	}, nil
}

func waitForPRChecks(
	ctx context.Context,
	run spriteScriptRunner,
	prNumber int,
	ghToken string,
	waitTimeout time.Duration,
	pollInterval time.Duration,
	onPending func(elapsed time.Duration),
) (prCheckOutcome, error) {
	if waitTimeout < 0 {
		return prCheckOutcome{}, fmt.Errorf("pr check timeout must be >= 0")
	}
	if pollInterval <= 0 {
		pollInterval = 10 * time.Second
	}

	outcome, err := inspectPRCheckOutcome(ctx, run, prNumber, ghToken)
	if err != nil {
		return prCheckOutcome{}, err
	}
	if outcome.Status != prCheckStatusPending || waitTimeout == 0 {
		return outcome, nil
	}

	started := time.Now()
	deadline := time.Now().Add(waitTimeout)
	for outcome.Status == prCheckStatusPending {
		if onPending != nil {
			onPending(time.Since(started))
		}
		if time.Now().After(deadline) {
			outcome.TimedOut = true
			return outcome, nil
		}

		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return prCheckOutcome{}, ctx.Err()
		case <-timer.C:
		}

		outcome, err = inspectPRCheckOutcome(ctx, run, prNumber, ghToken)
		if err != nil {
			return prCheckOutcome{}, err
		}
	}

	return outcome, nil
}

func parseKVOutput(raw []byte) map[string]string {
	out := map[string]string{}
	lines := strings.Split(string(raw), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		out[key] = val
	}
	return out
}

func parseKVBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func parseKVInt(field, v string) (int, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, fmt.Errorf("dispatch outcome missing %s", field)
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("dispatch outcome invalid %s=%q: %w", field, v, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("dispatch outcome invalid %s=%d: must be >= 0", field, n)
	}
	return n, nil
}

func hasSuccessfulDispatchArtifacts(outcome dispatchOutcome) bool {
	return outcome.OpenPRCount > 0 && outcome.DirtyFiles == 0
}

type prQueryState string

const (
	prQueryStateOK        prQueryState = "ok"
	prQueryStateNoBranch  prQueryState = "no_branch"
	prQueryStateGHMissing prQueryState = "gh_missing"
	prQueryStateQueryErr  prQueryState = "query_error"
	prQueryStateUnknown   prQueryState = "unknown"
)

func enforceDispatchPRReadiness(
	outcome dispatchOutcome,
	requireGreenPR bool,
	prChecks prCheckOutcome,
	prChecksErr error,
	prCheckTimeout time.Duration,
) error {
	if !requireGreenPR {
		return nil
	}

	if outcome.PRQueryState != prQueryStateOK {
		if outcome.CommitsAhead > 0 {
			return &exitError{Code: 1, Err: fmt.Errorf("cannot verify open PR state (state=%s) with commits_ahead=%d", outcome.PRQueryState, outcome.CommitsAhead)}
		}
		return nil
	}

	if outcome.OpenPRCount <= 0 {
		return nil
	}
	if outcome.PRNumber <= 0 {
		return &exitError{Code: 1, Err: fmt.Errorf("open PR detected but pr_number is missing")}
	}
	if prChecksErr != nil {
		return &exitError{Code: 1, Err: fmt.Errorf("cannot verify PR #%d checks: %w", outcome.PRNumber, prChecksErr)}
	}

	switch prChecks.Status {
	case prCheckStatusPass:
		return nil
	case prCheckStatusPending:
		if prChecks.TimedOut {
			return &exitError{Code: 1, Err: fmt.Errorf("PR #%d checks still pending after %s", outcome.PRNumber, prCheckTimeout)}
		}
		return &exitError{Code: 1, Err: fmt.Errorf("PR #%d checks are pending", outcome.PRNumber)}
	case prCheckStatusFail:
		return &exitError{Code: 1, Err: fmt.Errorf("PR #%d checks are failing", outcome.PRNumber)}
	case prCheckStatusGHMissing:
		return &exitError{Code: 1, Err: fmt.Errorf("cannot verify PR #%d checks: gh missing on sprite", outcome.PRNumber)}
	case prCheckStatusNoPR:
		return &exitError{Code: 1, Err: fmt.Errorf("cannot verify PR checks: missing PR number")}
	default:
		return &exitError{Code: 1, Err: fmt.Errorf("cannot verify PR #%d checks: status=%s checks_exit=%d", outcome.PRNumber, prChecks.Status, prChecks.ChecksExit)}
	}
}

func dispatchGracePeriod(timeout time.Duration) time.Duration {
	grace := timeout / 4
	if grace < 30*time.Second {
		grace = 30 * time.Second
	}
	if grace > 5*time.Minute {
		grace = 5 * time.Minute
	}
	return grace
}

func ensureNoActiveDispatchLoopWithRunner(ctx context.Context, run spriteScriptRunner) error {
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	out, exitCode, err := run(checkCtx, activeRalphLoopCheckScript)
	if err != nil {
		return fmt.Errorf("check dispatch loop: %w", err)
	}

	trim := strings.TrimSpace(string(out))
	switch exitCode {
	case 0:
		if trim != "" {
			return fmt.Errorf("active dispatch loop detected:\n%s", trim)
		}
		return nil
	case 1:
		if trim == "" {
			trim = "(process list empty)"
		}
		return fmt.Errorf("active dispatch loop detected:\n%s", trim)
	default:
		if trim == "" {
			return fmt.Errorf("check dispatch loop exited %d", exitCode)
		}
		return fmt.Errorf("check dispatch loop exited %d:\n%s", exitCode, trim)
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
