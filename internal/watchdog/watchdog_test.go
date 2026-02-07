package watchdog

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/misty-step/bitterblossom/internal/clients"
)

type fakeSprite struct {
	list      []string
	responses map[string]string
	errors    map[string]error
	calls     []string
}

func (f *fakeSprite) List(context.Context, string) ([]string, error) { return f.list, nil }
func (f *fakeSprite) Exec(_ context.Context, _ string, sprite, command string) (string, error) {
	key := sprite + "::" + normalize(command)
	f.calls = append(f.calls, key)
	if err, ok := f.errors[key]; ok {
		return "", err
	}
	if out, ok := f.responses[key]; ok {
		return out, nil
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

func normalize(cmd string) string {
	return strings.Join(strings.Fields(cmd), " ")
}

func TestReadAssignments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "active.txt")
	content := "thorn|fix auth|x|misty-step/heartbeat\nfern|ops work||misty-step/bitterblossom\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := readAssignments(path)
	if got["thorn"].Task != "fix auth" {
		t.Fatalf("task mismatch: %+v", got["thorn"])
	}
	if got["fern"].Repo != "misty-step/bitterblossom" {
		t.Fatalf("repo mismatch: %+v", got["fern"])
	}
}

func TestRunnerRun_DeadAndBlockedAlerts(t *testing.T) {
	responses := map[string]string{
		"thorn::ps aux | grep 'claude -p' | grep -v grep | wc -l":                                                                      "0",
		"thorn::test -f /home/sprite/workspace/TASK_COMPLETE && echo yes || echo no":                                                   "no",
		"thorn::test -f /home/sprite/workspace/BLOCKED.md && echo yes || echo no":                                                      "no",
		"thorn::find /home/sprite/workspace -maxdepth 3 -type f -mmin -30 2>/dev/null | grep -v node_modules | grep -v .git | head -1": "",
		"fern::ps aux | grep 'claude -p' | grep -v grep | wc -l":                                                                       "1",
		"fern::test -f /home/sprite/workspace/TASK_COMPLETE && echo yes || echo no":                                                    "no",
		"fern::test -f /home/sprite/workspace/BLOCKED.md && echo yes || echo no":                                                       "yes",
		"fern::head -5 /home/sprite/workspace/BLOCKED.md":                                                                              "need API token",
		"fern::find /home/sprite/workspace -maxdepth 3 -type f -mmin -30 2>/dev/null | grep -v node_modules | grep -v .git | head -1":  "/home/sprite/workspace/a.go",
	}
	sprite := &fakeSprite{list: []string{"thorn", "fern"}, responses: responses, errors: map[string]error{}}
	out := &bytes.Buffer{}
	r := Runner{Sprite: sprite, Out: out}

	active := filepath.Join(t.TempDir(), "active-agents.txt")
	_ = os.WriteFile(active, []byte("thorn|task one|x|misty-step/heartbeat\nfern|task two|x|misty-step/heartbeat\n"), 0o644)
	cfg := Config{
		Org:               "misty-step",
		ActiveAgentsFile:  active,
		DryRun:            true,
		EnableRedispatch:  true,
		ConfirmRedispatch: false,
		AutoPushOnDone:    true,
		StaleMinutes:      30,
		MarkerFile:        filepath.Join(t.TempDir(), "marker"),
	}

	summary, err := r.Run(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected ErrNeedsAttention")
	}
	if summary.Healthy {
		t.Fatal("expected unhealthy summary")
	}
	if len(summary.Alerts) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(summary.Alerts))
	}
	if summary.Alerts[0].Type != "DEAD" {
		t.Fatalf("expected DEAD alert, got %s", summary.Alerts[0].Type)
	}
	if !strings.Contains(summary.Alerts[0].Message, "dry-run") {
		t.Fatalf("expected dry-run note in dead message: %s", summary.Alerts[0].Message)
	}
	if summary.Alerts[1].Type != "BLOCKED" {
		t.Fatalf("expected BLOCKED alert, got %s", summary.Alerts[1].Type)
	}
}

func TestRunnerRun_Healthy(t *testing.T) {
	responses := map[string]string{
		"thorn::ps aux | grep 'claude -p' | grep -v grep | wc -l":                                                                      "1",
		"thorn::test -f /home/sprite/workspace/TASK_COMPLETE && echo yes || echo no":                                                   "no",
		"thorn::test -f /home/sprite/workspace/BLOCKED.md && echo yes || echo no":                                                      "no",
		"thorn::find /home/sprite/workspace -maxdepth 3 -type f -mmin -30 2>/dev/null | grep -v node_modules | grep -v .git | head -1": "/home/sprite/workspace/main.go",
	}
	sprite := &fakeSprite{list: []string{"thorn"}, responses: responses, errors: map[string]error{}}
	out := &bytes.Buffer{}
	r := Runner{Sprite: sprite, Out: out}

	summary, err := r.Run(context.Background(), Config{
		Org:              "misty-step",
		EnableRedispatch: true,
		AutoPushOnDone:   true,
		StaleMinutes:     30,
		MarkerFile:       filepath.Join(t.TempDir(), "m"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !summary.Healthy {
		t.Fatal("expected healthy")
	}
	if !strings.Contains(out.String(), "All sprites healthy") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestAutopushAndRedispatchExecuted(t *testing.T) {
	responses := map[string]string{
		"thorn::ps aux | grep 'claude -p' | grep -v grep | wc -l":                                                                      "0",
		"thorn::test -f /home/sprite/workspace/TASK_COMPLETE && echo yes || echo no":                                                   "yes",
		"thorn::test -f /home/sprite/workspace/BLOCKED.md && echo yes || echo no":                                                      "no",
		"thorn::find /home/sprite/workspace -maxdepth 3 -type f -mmin -30 2>/dev/null | grep -v node_modules | grep -v .git | head -1": "",
	}
	sprite := &fakeSprite{list: []string{"thorn"}, responses: responses, errors: map[string]error{}}
	r := Runner{Sprite: sprite, Out: &bytes.Buffer{}}
	active := filepath.Join(t.TempDir(), "active-agents.txt")
	_ = os.WriteFile(active, []byte("thorn|task|x|repo\n"), 0o644)
	_, _ = r.Run(context.Background(), Config{
		ActiveAgentsFile:  active,
		DryRun:            false,
		EnableRedispatch:  true,
		ConfirmRedispatch: true,
		AutoPushOnDone:    true,
		StaleMinutes:      30,
		MarkerFile:        filepath.Join(t.TempDir(), "marker"),
	})

	joined := strings.Join(sprite.calls, "\n")
	if !strings.Contains(joined, "git push origin") {
		t.Fatalf("expected autopush command in calls: %s", joined)
	}
}

func TestHandleRedispatchRequiresPrompt(t *testing.T) {
	sprite := &fakeSprite{
		list: []string{"thorn"},
		responses: map[string]string{
			"thorn::test -f /home/sprite/workspace/PROMPT.md && echo yes || echo no": "no",
		},
		errors: map[string]error{},
	}
	r := Runner{Sprite: sprite, Out: &bytes.Buffer{}}
	msg := r.handleRedispatch(context.Background(), Config{DryRun: false, ConfirmRedispatch: true}, "thorn")
	if !strings.Contains(msg, "no PROMPT.md") {
		t.Fatalf("unexpected message: %s", msg)
	}
}

func TestParseInt(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"5", 5},
		{" 8\n", 8},
		{"not-number", 0},
		{"", 0},
	}
	for _, tc := range cases {
		if got := parseInt(tc.in); got != tc.want {
			t.Fatalf("parseInt(%q)=%d want %d", tc.in, got, tc.want)
		}
	}
}

var _ clients.SpriteClient = (*fakeSprite)(nil)
