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
	claudeCount    int
	agentRunning   bool
	hasComplete    bool
	hasBlocked     bool
	hasPrompt      bool
	branch         string
	commitsLast2h  int
	dirtyRepos     int
	aheadCommits   int
	blockedSummary string
	taskID         string
	status         string
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
