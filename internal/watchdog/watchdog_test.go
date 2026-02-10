package watchdog

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"
)

type execRecord struct {
	sprite  string
	command string
}

type fakeRemote struct {
	listOutput []string
	listErr    error
	probes     map[string]string
	probeErr   map[string]error
	redispatch map[string]string
	redisErr   map[string]error
	execCalls  []execRecord
}

func (f *fakeRemote) List(context.Context) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]string, len(f.listOutput))
	copy(out, f.listOutput)
	return out, nil
}

func (f *fakeRemote) Exec(_ context.Context, sprite, command string, _ []byte) (string, error) {
	f.execCalls = append(f.execCalls, execRecord{sprite: sprite, command: command})
	if strings.Contains(command, "__CLAUDE_COUNT__") {
		if err, ok := f.probeErr[sprite]; ok {
			return "", err
		}
		return f.probes[sprite], nil
	}
	if err, ok := f.redisErr[sprite]; ok {
		return "", err
	}
	return f.redispatch[sprite], nil
}

func TestCheckFleetDryRun(t *testing.T) {
	now := time.Date(2026, time.February, 8, 13, 0, 0, 0, time.UTC)
	remote := &fakeRemote{
		listOutput: []string{"bramble", "fern", "thorn"},
		probes: map[string]string{
			"bramble": probeOutput(probeFixture{
				agentRunning:  true,
				commitsLast2h: 2,
				status:        `{"repo":"misty-step/heartbeat","issue":43,"started":"2026-02-08T12:00:00Z"}`,
			}),
			"fern": probeOutput(probeFixture{
				agentRunning: false,
				hasPrompt:    true,
				status:       `{"repo":"misty-step/api","started":"2026-02-08T10:00:00Z"}`,
			}),
			"thorn": probeOutput(probeFixture{
				agentRunning:  true,
				commitsLast2h: 0,
				status:        `{"repo":"misty-step/api","started":"2026-02-08T08:00:00Z"}`,
			}),
		},
	}

	service, err := NewService(Config{
		Remote:     remote,
		StaleAfter: 2 * time.Hour,
		Now: func() time.Time {
			return now
		},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	report, err := service.Check(context.Background(), Request{})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if report.Summary.Total != 3 {
		t.Fatalf("summary.total = %d, want 3", report.Summary.Total)
	}
	if report.Summary.Active != 1 {
		t.Fatalf("summary.active = %d, want 1", report.Summary.Active)
	}
	if report.Summary.Dead != 1 {
		t.Fatalf("summary.dead = %d, want 1", report.Summary.Dead)
	}
	if report.Summary.Stale != 1 {
		t.Fatalf("summary.stale = %d, want 1", report.Summary.Stale)
	}
	if report.Summary.Redispatched != 0 {
		t.Fatalf("summary.redispatched = %d, want 0 in dry-run", report.Summary.Redispatched)
	}

	fern := findRow(report.Sprites, "fern")
	if fern.State != StateDead {
		t.Fatalf("fern.state = %q, want dead", fern.State)
	}
	if fern.Action.Type != ActionRedispatch {
		t.Fatalf("fern.action.type = %q, want redispatch", fern.Action.Type)
	}
	if fern.Action.Executed {
		t.Fatalf("fern.action.executed = true, want false")
	}

	thorn := findRow(report.Sprites, "thorn")
	if thorn.State != StateStale {
		t.Fatalf("thorn.state = %q, want stale", thorn.State)
	}
	if thorn.Action.Type != ActionInvestigate {
		t.Fatalf("thorn.action.type = %q, want investigate", thorn.Action.Type)
	}
}

func TestCheckFleetExecuteRedispatch(t *testing.T) {
	now := time.Date(2026, time.February, 8, 13, 0, 0, 0, time.UTC)
	remote := &fakeRemote{
		probes: map[string]string{
			"bramble": probeOutput(probeFixture{
				agentRunning: false,
				hasPrompt:    true,
				status:       `{"repo":"misty-step/heartbeat","started":"2026-02-08T09:00:00Z"}`,
			}),
		},
		redispatch: map[string]string{
			"bramble": "redispatched pid=1234",
		},
	}

	service, err := NewService(Config{
		Remote:     remote,
		StaleAfter: 2 * time.Hour,
		Now: func() time.Time {
			return now
		},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	report, err := service.Check(context.Background(), Request{
		Sprites: []string{"bramble"},
		Execute: true,
	})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	row := findRow(report.Sprites, "bramble")
	if row.Action.Type != ActionRedispatch {
		t.Fatalf("action.type = %q, want redispatch", row.Action.Type)
	}
	if !row.Action.Executed {
		t.Fatalf("action.executed = false, want true")
	}
	if !row.Action.Success {
		t.Fatalf("action.success = false, want true")
	}
	if report.Summary.Redispatched != 1 {
		t.Fatalf("summary.redispatched = %d, want 1", report.Summary.Redispatched)
	}

	redispatchExecCount := 0
	for _, call := range remote.execCalls {
		if call.sprite == "bramble" && strings.Contains(call.command, "redispatched pid") {
			redispatchExecCount++
		}
	}
	if redispatchExecCount != 1 {
		t.Fatalf("redispatch exec count = %d, want 1", redispatchExecCount)
	}
}

func TestCheckFleetProbeError(t *testing.T) {
	remote := &fakeRemote{
		probes: map[string]string{"bramble": ""},
		probeErr: map[string]error{
			"bramble": fmt.Errorf("ssh timeout"),
		},
	}
	service, err := NewService(Config{Remote: remote})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	report, err := service.Check(context.Background(), Request{
		Sprites: []string{"bramble"},
	})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	row := findRow(report.Sprites, "bramble")
	if row.State != StateError {
		t.Fatalf("row.state = %q, want error", row.State)
	}
	if row.Error == "" {
		t.Fatal("expected error message")
	}
}

func findRow(rows []SpriteReport, sprite string) SpriteReport {
	for _, row := range rows {
		if row.Sprite == sprite {
			return row
		}
	}
	return SpriteReport{}
}

type probeFixture struct {
	claudeCount      int
	agentRunning     bool
	hasComplete      bool
	hasBlocked       bool
	hasPrompt        bool
	branch           string
	commitsLast2h    int
	dirtyRepos       int
	aheadCommits     int
	blockedSummary   string
	taskID           string
	status           string
	supervisorState  string
}

func probeOutput(f probeFixture) string {
	yesNo := func(value bool) string {
		if value {
			return "yes"
		}
		return "no"
	}
	return strings.Join([]string{
		"__CLAUDE_COUNT__" + strconvOrZero(f.claudeCount),
		"__AGENT_RUNNING__" + yesNo(f.agentRunning),
		"__HAS_COMPLETE__" + yesNo(f.hasComplete),
		"__HAS_BLOCKED__" + yesNo(f.hasBlocked),
		"__COMMITS_LAST_2H__" + strconvOrZero(f.commitsLast2h),
		"__DIRTY_REPOS__" + strconvOrZero(f.dirtyRepos),
		"__AHEAD_COMMITS__" + strconvOrZero(f.aheadCommits),
		"__HAS_PROMPT__" + yesNo(f.hasPrompt),
		"__BLOCKED_B64__" + b64(f.blockedSummary),
		"__BRANCH_B64__" + b64(f.branch),
		"__STATUS_B64__" + b64(f.status),
		"__TASK_ID_B64__" + b64(f.taskID),
		"__SUPERVISOR_STATE_B64__" + b64(f.supervisorState),
		"",
	}, "\n")
}

func b64(value string) string {
	return base64.StdEncoding.EncodeToString([]byte(value))
}

func strconvOrZero(value int) string {
	if value < 0 {
		return "0"
	}
	return fmt.Sprintf("%d", value)
}

func TestIdleDetection(t *testing.T) {
	now := time.Date(2026, time.February, 8, 14, 0, 0, 0, time.UTC)
	lastActivity := now.Add(-45 * time.Minute) // 45 minutes ago

	remote := &fakeRemote{
		probes: map[string]string{
			"idle-sprite": probeOutput(probeFixture{
				agentRunning:  true,
				commitsLast2h: 1,
				status:        `{"repo":"misty-step/test","started":"2026-02-08T12:00:00Z"}`,
				supervisorState: fmt.Sprintf(`{"last_progress_at":"%s","last_activity":"git_commit","stalled":false}`,
					lastActivity.Format(time.RFC3339)),
			}),
			"active-sprite": probeOutput(probeFixture{
				agentRunning:  true,
				commitsLast2h: 2,
				status:        `{"repo":"misty-step/test2","started":"2026-02-08T13:00:00Z"}`,
				supervisorState: fmt.Sprintf(`{"last_progress_at":"%s","last_activity":"file_change","stalled":false}`,
					now.Add(-5*time.Minute).Format(time.RFC3339)),
			}),
			"no-activity": probeOutput(probeFixture{
				agentRunning:  true,
				commitsLast2h: 0,
				status:        `{"repo":"misty-step/test3","started":"2026-02-08T12:00:00Z"}`,
				supervisorState: `{}`,
			}),
		},
	}

	service, err := NewService(Config{
		Remote:      remote,
		IdleTimeout: 30 * time.Minute,
		StaleAfter:  2 * time.Hour,
		Now: func() time.Time {
			return now
		},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	report, err := service.Check(context.Background(), Request{
		Sprites: []string{"idle-sprite", "active-sprite", "no-activity"},
	})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	// Check idle sprite detection
	idleSprite := findRow(report.Sprites, "idle-sprite")
	if idleSprite.IdleMinutes != 45 {
		t.Errorf("idle-sprite.idle_minutes = %d, want 45", idleSprite.IdleMinutes)
	}
	if idleSprite.IdleSince == "" {
		t.Errorf("idle-sprite.idle_since should not be empty")
	}
	if idleSprite.LastActivityAge != 45 {
		t.Errorf("idle-sprite.last_activity_age_minutes = %d, want 45", idleSprite.LastActivityAge)
	}

	// Check active sprite - should not be idle
	activeSprite := findRow(report.Sprites, "active-sprite")
	if activeSprite.IdleMinutes != 0 {
		t.Errorf("active-sprite.idle_minutes = %d, want 0", activeSprite.IdleMinutes)
	}
	if activeSprite.IdleSince != "" {
		t.Errorf("active-sprite.idle_since should be empty")
	}
	if activeSprite.LastActivityAge != 5 {
		t.Errorf("active-sprite.last_activity_age_minutes = %d, want 5", activeSprite.LastActivityAge)
	}

	// Check sprite with no supervisor state
	noActivity := findRow(report.Sprites, "no-activity")
	if noActivity.IdleMinutes != 0 {
		t.Errorf("no-activity.idle_minutes = %d, want 0", noActivity.IdleMinutes)
	}
	if noActivity.LastActivityAt != "" {
		t.Errorf("no-activity.last_activity_at should be empty")
	}

	// Check summary
	if report.Summary.IdleTimedOut != 1 {
		t.Errorf("summary.idle_timed_out = %d, want 1", report.Summary.IdleTimedOut)
	}
	if report.Summary.NeedsAttention < 1 {
		t.Errorf("summary.needs_attention = %d, should include idle timeout", report.Summary.NeedsAttention)
	}
}

func TestIdleTimeoutThreshold(t *testing.T) {
	now := time.Date(2026, time.February, 8, 14, 0, 0, 0, time.UTC)
	testCases := []struct {
		name              string
		idleTimeout       time.Duration
		minutesSinceActivity int
		expectIdle        bool
	}{
		{
			name:              "exactly at threshold",
			idleTimeout:       30 * time.Minute,
			minutesSinceActivity: 30,
			expectIdle:        true,
		},
		{
			name:              "just below threshold",
			idleTimeout:       30 * time.Minute,
			minutesSinceActivity: 29,
			expectIdle:        false,
		},
		{
			name:              "well above threshold",
			idleTimeout:       30 * time.Minute,
			minutesSinceActivity: 60,
			expectIdle:        true,
		},
		{
			name:              "zero timeout disables idle detection",
			idleTimeout:       0,
			minutesSinceActivity: 120,
			expectIdle:        false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			lastActivity := now.Add(-time.Duration(tc.minutesSinceActivity) * time.Minute)
			remote := &fakeRemote{
				probes: map[string]string{
					"test-sprite": probeOutput(probeFixture{
						agentRunning:  true,
						commitsLast2h: 1,
						status:        `{"repo":"test","started":"2026-02-08T12:00:00Z"}`,
						supervisorState: fmt.Sprintf(`{"last_progress_at":"%s","last_activity":"test","stalled":false}`,
							lastActivity.Format(time.RFC3339)),
					}),
				},
			}

			service, err := NewService(Config{
				Remote:      remote,
				IdleTimeout: tc.idleTimeout,
				StaleAfter:  2 * time.Hour,
				Now: func() time.Time {
					return now
				},
			})
			if err != nil {
				t.Fatalf("NewService() error = %v", err)
			}

			report, err := service.Check(context.Background(), Request{
				Sprites: []string{"test-sprite"},
			})
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}

			sprite := findRow(report.Sprites, "test-sprite")
			isIdle := sprite.IdleMinutes > 0

			if isIdle != tc.expectIdle {
				t.Errorf("sprite idle = %v, want %v (idle_minutes=%d)", isIdle, tc.expectIdle, sprite.IdleMinutes)
			}

			if tc.expectIdle {
				if sprite.IdleMinutes != tc.minutesSinceActivity {
					t.Errorf("idle_minutes = %d, want %d", sprite.IdleMinutes, tc.minutesSinceActivity)
				}
				if sprite.IdleSince == "" {
					t.Errorf("idle_since should not be empty when idle")
				}
			} else {
				if sprite.IdleMinutes != 0 {
					t.Errorf("idle_minutes = %d, want 0", sprite.IdleMinutes)
				}
				if sprite.IdleSince != "" {
					t.Errorf("idle_since should be empty when not idle")
				}
			}
		})
	}
}

func TestIdleDetectionWithMultipleStates(t *testing.T) {
	now := time.Date(2026, time.February, 8, 14, 0, 0, 0, time.UTC)
	remote := &fakeRemote{
		probes: map[string]string{
			"idle-active": probeOutput(probeFixture{
				agentRunning:  true,
				commitsLast2h: 2,
				status:        `{"repo":"test","started":"2026-02-08T12:00:00Z"}`,
				supervisorState: fmt.Sprintf(`{"last_progress_at":"%s","last_activity":"git_commit","stalled":false}`,
					now.Add(-40*time.Minute).Format(time.RFC3339)),
			}),
			"idle-dead": probeOutput(probeFixture{
				agentRunning: false,
				hasPrompt:    true,
				status:       `{"repo":"test","started":"2026-02-08T12:00:00Z"}`,
				supervisorState: fmt.Sprintf(`{"last_progress_at":"%s","last_activity":"agent_exited","stalled":false}`,
					now.Add(-50*time.Minute).Format(time.RFC3339)),
			}),
			"idle-complete": probeOutput(probeFixture{
				agentRunning:  false,
				hasComplete:   true,
				commitsLast2h: 3,
				status:        `{"repo":"test","started":"2026-02-08T12:00:00Z"}`,
				supervisorState: fmt.Sprintf(`{"last_progress_at":"%s","last_activity":"done","stalled":false}`,
					now.Add(-35*time.Minute).Format(time.RFC3339)),
			}),
		},
	}

	service, err := NewService(Config{
		Remote:      remote,
		IdleTimeout: 30 * time.Minute,
		StaleAfter:  2 * time.Hour,
		Now: func() time.Time {
			return now
		},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	report, err := service.Check(context.Background(), Request{
		Sprites: []string{"idle-active", "idle-dead", "idle-complete"},
	})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	// All should report idle time
	if report.Summary.IdleTimedOut != 3 {
		t.Errorf("summary.idle_timed_out = %d, want 3", report.Summary.IdleTimedOut)
	}

	// Check individual states are preserved
	idleActive := findRow(report.Sprites, "idle-active")
	if idleActive.State != StateActive {
		t.Errorf("idle-active.state = %q, want active", idleActive.State)
	}
	if idleActive.IdleMinutes != 40 {
		t.Errorf("idle-active.idle_minutes = %d, want 40", idleActive.IdleMinutes)
	}

	idleDead := findRow(report.Sprites, "idle-dead")
	if idleDead.State != StateDead {
		t.Errorf("idle-dead.state = %q, want dead", idleDead.State)
	}
	if idleDead.IdleMinutes != 50 {
		t.Errorf("idle-dead.idle_minutes = %d, want 50", idleDead.IdleMinutes)
	}

	idleComplete := findRow(report.Sprites, "idle-complete")
	if idleComplete.State != StateComplete {
		t.Errorf("idle-complete.state = %q, want complete", idleComplete.State)
	}
	if idleComplete.IdleMinutes != 35 {
		t.Errorf("idle-complete.idle_minutes = %d, want 35", idleComplete.IdleMinutes)
	}
}
