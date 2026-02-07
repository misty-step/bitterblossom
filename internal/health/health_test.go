package health

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/internal/clients"
)

type fakeSprite struct {
	list      []string
	responses map[string]string
}

func (f *fakeSprite) List(context.Context, string) ([]string, error) { return f.list, nil }
func (f *fakeSprite) Exec(_ context.Context, _ string, sprite, cmd string) (string, error) {
	norm := strings.Join(strings.Fields(cmd), " ")
	for pattern, out := range f.responses {
		if strings.HasPrefix(pattern, sprite+"::") {
			pattern = strings.TrimPrefix(pattern, sprite+"::")
		}
		if strings.Contains(norm, pattern) {
			return out, nil
		}
	}
	return "", nil
}
func (f *fakeSprite) API(context.Context, string, string) ([]byte, error) { return nil, nil }
func (f *fakeSprite) SpriteAPI(context.Context, string, string, string) ([]byte, error) {
	return nil, nil
}
func (f *fakeSprite) ListCheckpoints(context.Context, string, string) (string, error) { return "", nil }
func (f *fakeSprite) ListSprites(context.Context, string) ([]clients.SpriteInfo, error) {
	return nil, nil
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name        string
		task        string
		claude      bool
		stale       bool
		gitChanges  bool
		commitCount int
		wantStatus  string
	}{
		{"active with changes", "task", true, false, true, 0, StatusActive},
		{"running no changes", "task", true, false, false, 0, StatusRunning},
		{"stale", "task", true, true, false, 0, StatusStale},
		{"dead with task", "task", false, false, false, 0, StatusDead},
		{"idle", "", false, false, false, 0, StatusIdle},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classify(tc.task, tc.claude, tc.stale, tc.gitChanges, tc.commitCount)
			if got != tc.wantStatus {
				t.Fatalf("classify got %s want %s", got, tc.wantStatus)
			}
		})
	}
}

func TestCheckerRunJSON(t *testing.T) {
	oldNow := timeNow
	timeNow = func() time.Time { return time.Unix(2000, 0) }
	defer func() { timeNow = oldNow }()

	recentEpoch := time.Unix(1900, 0).Unix()
	responses := map[string]string{
		"thorn::pgrep -c claude":        "1",
		"thorn::TASK_COMPLETE":          "no",
		"thorn::BLOCKED.md && echo yes": "no",
		"thorn::AHEAD:$COMMITS_AHEAD":   "BRANCH:main AHEAD:2 UNCOMMITTED: 1 file changed",
		"thorn::xargs stat -c '%Y %n'":  strconv.FormatInt(recentEpoch, 10) + " /home/sprite/workspace/main.go",
	}
	checker := Checker{Sprite: &fakeSprite{list: []string{"thorn"}, responses: responses}, Out: &bytes.Buffer{}}
	buf := &bytes.Buffer{}
	checker.Out = buf

	results, err := checker.Run(context.Background(), Config{JSONOutput: true, StaleThresholdMin: 30})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != StatusActive {
		t.Fatalf("status mismatch: %s", results[0].Status)
	}
	if !strings.Contains(buf.String(), "\"status\": \"active\"") {
		t.Fatalf("unexpected JSON output: %s", buf.String())
	}
}

func TestExtractAgeMinutes(t *testing.T) {
	oldNow := timeNow
	timeNow = func() time.Time { return time.Unix(1000, 0) }
	defer func() { timeNow = oldNow }()

	if got := extractAgeMinutes("900 /tmp/file"); got != 1 {
		t.Fatalf("expected 1 got %d", got)
	}
	if got := extractAgeMinutes("garbage"); got != -1 {
		t.Fatalf("expected -1 got %d", got)
	}
}

func TestRenderTextAndIcons(t *testing.T) {
	buf := &bytes.Buffer{}
	results := []Result{{
		Name:           "thorn",
		Status:         StatusStale,
		ClaudeRunning:  true,
		HasGitChanges:  false,
		CommitCount:    0,
		Stale:          true,
		LastFileChange: "45m ago",
		Signals:        "",
		CurrentTask:    "task",
		ProcCount:      1,
	}}
	renderText(buf, results, 30)
	out := buf.String()
	if !strings.Contains(out, "thorn") || !strings.Contains(out, "STALE") {
		t.Fatalf("unexpected render output: %s", out)
	}
	if iconFor(StatusBlocked) == "" {
		t.Fatal("expected icon for blocked")
	}
	if yesNo(true) != "yes" || yesNo(false) != "no" {
		t.Fatal("yesNo mismatch")
	}
}

func TestResolveSpritesError(t *testing.T) {
	checker := Checker{Sprite: &fakeSprite{list: nil}}
	_, err := checker.resolveSprites(context.Background(), Config{})
	if err == nil {
		t.Fatal("expected error for empty sprite list")
	}
}

var _ clients.SpriteClient = (*fakeSprite)(nil)
