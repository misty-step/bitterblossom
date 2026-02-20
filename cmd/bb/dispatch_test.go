package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func TestDispatchTextMessageHandlerWritesStructuredOutput(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	handler := newDispatchTextMessageHandler(&stdout, &stderr)

	handler([]byte(`{"type":"stdout","data":"hello\n"}`))
	handler([]byte(`{"type":"stderr","data":"warn\n"}`))
	handler([]byte(`{"type":"error","error":"boom\n"}`))
	handler([]byte(`{"type":"info","data":"note\n"}`))

	if got := stdout.String(); got != "hello\nnote\n" {
		t.Fatalf("stdout = %q, want %q", got, "hello\nnote\n")
	}
	if got := stderr.String(); got != "warn\nboom\n" {
		t.Fatalf("stderr = %q, want %q", got, "warn\nboom\n")
	}
}

func TestDispatchTextMessageHandlerIgnoresControlFrames(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	handler := newDispatchTextMessageHandler(&stdout, &stderr)

	handler([]byte(`control:{"type":"op.complete","id":"1"}`))
	handler([]byte(`{"type":"session_info","session_id":"abc"}`))
	handler([]byte(`{"type":"exit","exit_code":0}`))

	if stdout.Len() != 0 {
		t.Fatalf("stdout should be empty, got %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr should be empty, got %q", stderr.String())
	}
}

func TestDispatchTextMessageHandlerWritesPlainTextFrames(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	handler := newDispatchTextMessageHandler(&stdout, &stderr)

	handler([]byte("plain frame"))

	if got := stdout.String(); got != "plain frame\n" {
		t.Fatalf("stdout = %q, want %q", got, "plain frame\n")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr should be empty, got %q", stderr.String())
	}
}

func TestActivityWriterMarksWrites(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	marks := 0
	w := &activityWriter{
		out: &out,
		mark: func() {
			marks++
		},
	}

	n, err := w.Write([]byte("ok"))
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if n != 2 {
		t.Fatalf("write length = %d, want 2", n)
	}
	if marks != 1 {
		t.Fatalf("marks = %d, want 1", marks)
	}
	if !strings.Contains(out.String(), "ok") {
		t.Fatalf("output = %q, want to contain %q", out.String(), "ok")
	}
}

func TestOffRailsDetectorEmitsWarningOnSilence(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	var out lockedBuffer
	d := newOffRailsDetector(offRailsConfig{
		SilenceAbort:  time.Hour,
		SilenceWarn:   10 * time.Millisecond,
		CheckInterval: 10 * time.Millisecond,
		Cancel:        cancel,
		Alert:         &out,
	})
	d.start()
	defer d.stop()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), "[off-rails] no output for") {
			// Ensure dispatch context was NOT cancelled (warn, not abort)
			select {
			case <-ctx.Done():
				t.Fatal("context should not be cancelled for warning")
			default:
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("expected warning line, got %q", out.String())
}

type fakeSpriteScriptRunner struct {
	out         []byte
	exitCode    int
	err         error
	called      bool
	gotDeadline bool
	script      string
}

func (r *fakeSpriteScriptRunner) run(ctx context.Context, script string) ([]byte, int, error) {
	r.called = true
	_, r.gotDeadline = ctx.Deadline()
	r.script = script
	return r.out, r.exitCode, r.err
}

func TestEnsureNoActiveDispatchLoop_AllowsIdle(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{out: nil, exitCode: 0, err: nil}
	if err := ensureNoActiveDispatchLoopWithRunner(context.Background(), r.run); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !r.called {
		t.Fatal("runner should be called")
	}
	if !r.gotDeadline {
		t.Fatal("runner ctx should have a deadline (timeout)")
	}
	if !strings.Contains(r.script, "pgrep -af") {
		t.Fatalf("script = %q, want to contain %q", r.script, "pgrep -af")
	}
	if !strings.Contains(r.script, "[r]alph") {
		t.Fatalf("script = %q, want to contain %q", r.script, "[r]alph")
	}
	if strings.Contains(r.script, "claude") || strings.Contains(r.script, "opencode") {
		t.Fatalf("script = %q, want ralph-only busy check", r.script)
	}
}

func TestEnsureNoActiveDispatchLoop_BlocksWhenBusy(t *testing.T) {
	t.Parallel()

	const busy = "1234 bash /home/sprite/workspace/.ralph.sh\n"
	r := &fakeSpriteScriptRunner{out: []byte(busy), exitCode: 1, err: nil}
	err := ensureNoActiveDispatchLoopWithRunner(context.Background(), r.run)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "active dispatch loop detected:") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "active dispatch loop detected:")
	}
	if !strings.Contains(err.Error(), strings.TrimSpace(busy)) {
		t.Fatalf("err = %q, want to contain %q", err.Error(), strings.TrimSpace(busy))
	}
}

func TestEnsureNoActiveDispatchLoop_WrapsRunnerError(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{out: nil, exitCode: 0, err: errors.New("network")}
	err := ensureNoActiveDispatchLoopWithRunner(context.Background(), r.run)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "check dispatch loop") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "check dispatch loop")
	}
	if !strings.Contains(err.Error(), "network") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "network")
	}
}

func TestEnsureNoActiveDispatchLoop_ErrorsOnUnexpectedExitCode(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{out: []byte("syntax error"), exitCode: 2, err: nil}
	err := ensureNoActiveDispatchLoopWithRunner(context.Background(), r.run)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "exited 2") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "exited 2")
	}
	if !strings.Contains(err.Error(), "syntax error") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "syntax error")
	}
}

func TestHasTaskCompleteSignalReturnsTrue(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{exitCode: 0, out: nil, err: nil}
	completed, err := hasTaskCompleteSignalWithRunner(context.Background(), r.run, "/tmp/ws")
	if err != nil {
		t.Fatalf("hasTaskCompleteSignalWithRunner() error = %v", err)
	}
	if !completed {
		t.Fatal("expected completion signal to be present")
	}
}

func TestHasTaskCompleteSignalReturnsFalseWhenMissing(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{exitCode: 1, out: nil, err: nil}
	completed, err := hasTaskCompleteSignalWithRunner(context.Background(), r.run, "/tmp/ws")
	if err != nil {
		t.Fatalf("hasTaskCompleteSignalWithRunner() error = %v", err)
	}
	if completed {
		t.Fatal("expected completion signal to be missing")
	}
}

func TestHasTaskCompleteSignalReturnsErrorOnUnexpectedExitCode(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{exitCode: 2, out: []byte("failed"), err: nil}
	_, err := hasTaskCompleteSignalWithRunner(context.Background(), r.run, "/tmp/ws")
	if err == nil {
		t.Fatal("expected error for unexpected completion check exit code")
	}
	if !strings.Contains(err.Error(), "completion signal check exited 2") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "completion signal check exited 2")
	}
}

func TestHasTaskCompleteSignalReturnsErrorOnRunnerError(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{exitCode: 0, out: nil, err: errors.New("network")}
	_, err := hasTaskCompleteSignalWithRunner(context.Background(), r.run, "/tmp/ws")
	if err == nil {
		t.Fatal("expected error for runner failure")
	}
	if !strings.Contains(err.Error(), "check completion signal command failed") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "check completion signal command failed")
	}
	if !strings.Contains(err.Error(), "network") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "network")
	}
}

func TestDispatchGracePeriodBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		timeout time.Duration
		want    time.Duration
	}{
		{name: "min floor", timeout: 1 * time.Minute, want: 30 * time.Second},
		{name: "proportional", timeout: 20 * time.Minute, want: 5 * time.Minute},
		{name: "max cap", timeout: 2 * time.Hour, want: 5 * time.Minute},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := dispatchGracePeriod(tc.timeout); got != tc.want {
				t.Fatalf("dispatchGracePeriod(%s) = %s, want %s", tc.timeout, got, tc.want)
			}
		})
	}
}

func TestParseKVOutput(t *testing.T) {
	t.Parallel()

	raw := []byte("a=1\nb = two \ninvalid\n c=3 \n")
	got := parseKVOutput(raw)
	if got["a"] != "1" {
		t.Fatalf("a = %q, want %q", got["a"], "1")
	}
	if got["b"] != "two" {
		t.Fatalf("b = %q, want %q", got["b"], "two")
	}
	if got["c"] != "3" {
		t.Fatalf("c = %q, want %q", got["c"], "3")
	}
}

func TestParseKVInt(t *testing.T) {
	t.Parallel()

	n, err := parseKVInt("dirty_files", "12")
	if err != nil {
		t.Fatalf("parseKVInt returned error: %v", err)
	}
	if n != 12 {
		t.Fatalf("parseKVInt returned %d, want 12", n)
	}

	if _, err := parseKVInt("dirty_files", ""); err == nil {
		t.Fatal("expected error for missing int")
	}
	if _, err := parseKVInt("dirty_files", "x"); err == nil {
		t.Fatal("expected error for invalid int")
	}
	if _, err := parseKVInt("dirty_files", "-1"); err == nil {
		t.Fatal("expected error for negative int")
	}
}

func TestHasSuccessfulDispatchArtifacts(t *testing.T) {
	t.Parallel()

	if !hasSuccessfulDispatchArtifacts(dispatchOutcome{OpenPRCount: 1, DirtyFiles: 0}) {
		t.Fatal("expected successful artifacts to be true with open PR and clean workspace")
	}
	if hasSuccessfulDispatchArtifacts(dispatchOutcome{OpenPRCount: 0, DirtyFiles: 0}) {
		t.Fatal("expected false when no open PR")
	}
	if hasSuccessfulDispatchArtifacts(dispatchOutcome{OpenPRCount: 1, DirtyFiles: 2}) {
		t.Fatal("expected false when workspace is dirty")
	}
}

func TestInspectDispatchOutcome(t *testing.T) {
	t.Parallel()

	out := strings.Join([]string{
		"task_complete=1",
		"blocked=0",
		"branch=feat/test",
		"dirty_files=0",
		"commits_ahead=2",
		"open_pr_count=1",
		"pr_number=417",
		"pr_query_state=ok",
		"",
	}, "\n")
	r := &fakeSpriteScriptRunner{out: []byte(out), exitCode: 0, err: nil}

	got, err := inspectDispatchOutcome(context.Background(), r.run, "/tmp/ws", "gh-token")
	if err != nil {
		t.Fatalf("inspectDispatchOutcome() error = %v", err)
	}
	if !r.called {
		t.Fatal("runner should be called")
	}
	if !r.gotDeadline {
		t.Fatal("runner ctx should have a deadline (timeout)")
	}
	if !strings.Contains(r.script, "open_pr_count") {
		t.Fatalf("script = %q, want to contain %q", r.script, "open_pr_count")
	}
	if !strings.Contains(r.script, "GH_TOKEN=") {
		t.Fatalf("script = %q, want to contain %q", r.script, "GH_TOKEN=")
	}

	if !got.TaskComplete {
		t.Fatal("TaskComplete should be true")
	}
	if got.Blocked {
		t.Fatal("Blocked should be false")
	}
	if got.Branch != "feat/test" {
		t.Fatalf("Branch = %q, want %q", got.Branch, "feat/test")
	}
	if got.DirtyFiles != 0 || got.CommitsAhead != 2 || got.OpenPRCount != 1 || got.PRNumber != 417 {
		t.Fatalf("unexpected parsed outcome: %+v", got)
	}
	if got.PRQueryState != prQueryStateOK {
		t.Fatalf("PRQueryState = %q, want %q", got.PRQueryState, prQueryStateOK)
	}
}

func TestInspectPRCheckOutcome(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{
		out:      []byte("status=pass\nchecks_exit=0\n"),
		exitCode: 0,
		err:      nil,
	}

	got, err := inspectPRCheckOutcome(context.Background(), r.run, 417, "gh-token")
	if err != nil {
		t.Fatalf("inspectPRCheckOutcome() error = %v", err)
	}
	if got.Status != prCheckStatusPass {
		t.Fatalf("Status = %q, want %q", got.Status, prCheckStatusPass)
	}
	if got.ChecksExit != 0 {
		t.Fatalf("ChecksExit = %d, want 0", got.ChecksExit)
	}
	if got.TimedOut {
		t.Fatal("TimedOut should be false")
	}
}

func TestWaitForPRChecks_StopsOnPassAfterPending(t *testing.T) {
	t.Parallel()

	call := 0
	run := func(ctx context.Context, script string) ([]byte, int, error) {
		call++
		if call == 1 {
			return []byte("status=pending\nchecks_exit=1\n"), 0, nil
		}
		return []byte("status=pass\nchecks_exit=0\n"), 0, nil
	}

	got, err := waitForPRChecks(context.Background(), run, 417, "gh-token", 100*time.Millisecond, 1*time.Millisecond, nil)
	if err != nil {
		t.Fatalf("waitForPRChecks() error = %v", err)
	}
	if got.Status != prCheckStatusPass {
		t.Fatalf("Status = %q, want %q", got.Status, prCheckStatusPass)
	}
	if got.TimedOut {
		t.Fatal("TimedOut should be false")
	}
	if call < 2 {
		t.Fatalf("runner calls = %d, want at least 2", call)
	}
}

func TestWaitForPRChecks_ReportsPendingProgress(t *testing.T) {
	t.Parallel()

	call := 0
	run := func(ctx context.Context, script string) ([]byte, int, error) {
		call++
		if call == 1 {
			return []byte("status=pending\nchecks_exit=1\n"), 0, nil
		}
		return []byte("status=pass\nchecks_exit=0\n"), 0, nil
	}

	progressCalls := 0
	_, err := waitForPRChecks(context.Background(), run, 417, "gh-token", 100*time.Millisecond, 1*time.Millisecond, func(elapsed time.Duration) {
		progressCalls++
	})
	if err != nil {
		t.Fatalf("waitForPRChecks() error = %v", err)
	}
	if progressCalls == 0 {
		t.Fatal("expected pending progress callback to be called")
	}
}

func TestWaitForPRChecks_TimesOutWhenStillPending(t *testing.T) {
	t.Parallel()

	run := func(ctx context.Context, script string) ([]byte, int, error) {
		return []byte("status=pending\nchecks_exit=1\n"), 0, nil
	}

	got, err := waitForPRChecks(context.Background(), run, 417, "gh-token", 5*time.Millisecond, 1*time.Millisecond, nil)
	if err != nil {
		t.Fatalf("waitForPRChecks() error = %v", err)
	}
	if got.Status != prCheckStatusPending {
		t.Fatalf("Status = %q, want %q", got.Status, prCheckStatusPending)
	}
	if !got.TimedOut {
		t.Fatal("TimedOut should be true")
	}
}

func TestPRCheckStatusScriptDefaultsUnknownNonZeroToPending(t *testing.T) {
	t.Parallel()

	if !strings.Contains(prCheckStatusScript, "status=pending") {
		t.Fatalf("prCheckStatusScript = %q, want to contain %q", prCheckStatusScript, "status=pending")
	}
	if !strings.Contains(prCheckStatusScript, "timed[- ]?out") {
		t.Fatalf("prCheckStatusScript = %q, want to contain %q", prCheckStatusScript, "timed[- ]?out")
	}
}

func TestEnforceDispatchPRReadiness_AllowsPassingChecks(t *testing.T) {
	t.Parallel()

	outcome := dispatchOutcome{
		CommitsAhead: 2,
		OpenPRCount:  1,
		PRNumber:     417,
		PRQueryState: prQueryStateOK,
	}
	prChecks := prCheckOutcome{Status: prCheckStatusPass}
	if err := enforceDispatchPRReadiness(outcome, true, prChecks, nil, 4*time.Minute); err != nil {
		t.Fatalf("enforceDispatchPRReadiness() error = %v, want nil", err)
	}
}

func TestEnforceDispatchPRReadiness_BlocksUnknownPRStateWithCommits(t *testing.T) {
	t.Parallel()

	outcome := dispatchOutcome{
		CommitsAhead: 2,
		PRQueryState: prQueryStateQueryErr,
	}
	err := enforceDispatchPRReadiness(outcome, true, prCheckOutcome{}, nil, 4*time.Minute)
	if err == nil {
		t.Fatal("expected error for query failure with commits ahead")
	}
	var coded *exitError
	if !errors.As(err, &coded) {
		t.Fatalf("expected exitError, got %T", err)
	}
	if coded.Code != 1 {
		t.Fatalf("exit code = %d, want 1", coded.Code)
	}
}

func TestEnforceDispatchPRReadiness_BlocksPendingChecks(t *testing.T) {
	t.Parallel()

	outcome := dispatchOutcome{
		OpenPRCount:  1,
		PRNumber:     417,
		PRQueryState: prQueryStateOK,
	}
	prChecks := prCheckOutcome{Status: prCheckStatusPending}
	err := enforceDispatchPRReadiness(outcome, true, prChecks, nil, 4*time.Minute)
	if err == nil {
		t.Fatal("expected error for pending checks")
	}
	if !strings.Contains(err.Error(), "pending") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "pending")
	}
}

func TestEnforceDispatchPRReadiness_SkipsWhenRequireGreenDisabled(t *testing.T) {
	t.Parallel()

	outcome := dispatchOutcome{
		CommitsAhead: 5,
		PRQueryState: prQueryStateQueryErr,
	}
	if err := enforceDispatchPRReadiness(outcome, false, prCheckOutcome{}, errors.New("boom"), 4*time.Minute); err != nil {
		t.Fatalf("enforceDispatchPRReadiness() error = %v, want nil", err)
	}
}
