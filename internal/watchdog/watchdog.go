package watchdog

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/internal/clients"
)

var (
	// ErrNeedsAttention indicates unhealthy fleet state.
	ErrNeedsAttention = errors.New("fleet needs attention")
)

// Config controls watchdog behavior.
type Config struct {
	Org               string
	ActiveAgentsFile  string
	DryRun            bool
	EnableRedispatch  bool
	ConfirmRedispatch bool
	AutoPushOnDone    bool
	StaleMinutes      int
	MarkerFile        string
	JSONOutput        bool
}

// Alert reports actionable status from one sprite.
type Alert struct {
	Type    string `json:"type"`
	Sprite  string `json:"sprite"`
	Task    string `json:"task,omitempty"`
	Message string `json:"message"`
}

// Summary captures watchdog run output.
type Summary struct {
	Healthy bool    `json:"healthy"`
	Alerts  []Alert `json:"alerts"`
}

// Runner executes watchdog checks.
type Runner struct {
	Sprite clients.SpriteClient
	Log    *slog.Logger
	Out    io.Writer
}

// Run executes one watchdog pass.
func (r *Runner) Run(ctx context.Context, cfg Config) (Summary, error) {
	cfg = withDefaults(cfg)
	if r.Sprite == nil {
		return Summary{}, fmt.Errorf("sprite client required")
	}
	if r.Out == nil {
		r.Out = os.Stdout
	}
	if r.Log == nil {
		r.Log = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}

	assignments := readAssignments(cfg.ActiveAgentsFile)
	sprites, err := r.Sprite.List(ctx, cfg.Org)
	if err != nil {
		return Summary{}, fmt.Errorf("list sprites: %w", err)
	}
	if len(sprites) == 0 {
		return Summary{Healthy: true}, nil
	}

	var alerts []Alert
	for _, name := range sprites {
		state := r.inspectSprite(ctx, cfg, name)
		if state.Completed {
			task := assignments[name].Task
			if cfg.AutoPushOnDone {
				r.autopushRepos(ctx, cfg, name)
			}
			alerts = append(alerts, Alert{Type: "COMPLETE", Sprite: name, Task: task, Message: "task complete; needs reassignment"})
			continue
		}
		if state.Blocked {
			alerts = append(alerts, Alert{Type: "BLOCKED", Sprite: name, Task: assignments[name].Task, Message: state.BlockedReason})
			continue
		}
		if state.ClaudeCount == 0 {
			if assignments[name].Task == "" {
				continue
			}
			msg := "claude process not running"
			if cfg.EnableRedispatch {
				redispatchMsg := r.handleRedispatch(ctx, cfg, name)
				if redispatchMsg != "" {
					msg = msg + "; " + redispatchMsg
				}
			}
			alerts = append(alerts, Alert{Type: "DEAD", Sprite: name, Task: assignments[name].Task, Message: msg})
			continue
		}
		if !state.RecentChanges {
			alerts = append(alerts, Alert{Type: "STALE", Sprite: name, Task: assignments[name].Task, Message: "claude running but no recent file changes"})
		}
	}

	_ = touchMarker(cfg.MarkerFile)
	summary := Summary{Healthy: len(alerts) == 0, Alerts: alerts}
	if cfg.JSONOutput {
		enc := json.NewEncoder(r.Out)
		enc.SetIndent("", "  ")
		_ = enc.Encode(summary)
	} else {
		r.renderText(summary)
	}
	if !summary.Healthy {
		return summary, ErrNeedsAttention
	}
	return summary, nil
}

func (r *Runner) renderText(summary Summary) {
	if summary.Healthy {
		_, _ = fmt.Fprintln(r.Out, "All sprites healthy.")
		return
	}
	_, _ = fmt.Fprintln(r.Out, "=== ALERTS ===")
	for _, alert := range summary.Alerts {
		if alert.Task != "" {
			_, _ = fmt.Fprintf(r.Out, "%s: %s (%s) - %s\n", alert.Type, alert.Sprite, alert.Task, alert.Message)
			continue
		}
		_, _ = fmt.Fprintf(r.Out, "%s: %s - %s\n", alert.Type, alert.Sprite, alert.Message)
	}
}

type spriteState struct {
	ClaudeCount   int
	Completed     bool
	Blocked       bool
	BlockedReason string
	RecentChanges bool
}

func (r *Runner) inspectSprite(ctx context.Context, cfg Config, name string) spriteState {
	state := spriteState{}
	state.ClaudeCount = parseInt(strings.TrimSpace(r.execDefault(ctx, cfg.Org, name, "ps aux | grep 'claude -p' | grep -v grep | wc -l", "0")))
	state.Completed = isYes(r.execDefault(ctx, cfg.Org, name, "test -f /home/sprite/workspace/TASK_COMPLETE && echo yes || echo no", "no"))
	state.Blocked = isYes(r.execDefault(ctx, cfg.Org, name, "test -f /home/sprite/workspace/BLOCKED.md && echo yes || echo no", "no"))
	if state.Blocked {
		state.BlockedReason = strings.TrimSpace(r.execDefault(ctx, cfg.Org, name, "head -5 /home/sprite/workspace/BLOCKED.md", "blocked"))
	}
	staleCmd := fmt.Sprintf("find /home/sprite/workspace -maxdepth 3 -type f -mmin -%d 2>/dev/null | grep -v node_modules | grep -v .git | head -1", cfg.StaleMinutes)
	state.RecentChanges = strings.TrimSpace(r.execDefault(ctx, cfg.Org, name, staleCmd, "")) != ""
	return state
}

func (r *Runner) autopushRepos(ctx context.Context, cfg Config, name string) {
	pushScript := `
for d in /home/sprite/workspace/*/; do
  [ -d "$d/.git" ] || continue
  cd "$d"
  BRANCH=$(git branch --show-current 2>/dev/null)
  [ -z "$BRANCH" ] && continue
  UNPUSHED=$(git log "origin/$BRANCH..HEAD" --oneline 2>/dev/null | wc -l)
  if [ "$UNPUSHED" -gt 0 ]; then
    git push origin "$BRANCH" 2>&1 || true
  fi
done
`
	if cfg.DryRun {
		r.Log.Info("dry-run autopush skipped", "sprite", name)
		return
	}
	_, _ = r.Sprite.Exec(ctx, cfg.Org, name, pushScript)
}

func (r *Runner) handleRedispatch(ctx context.Context, cfg Config, name string) string {
	if cfg.DryRun {
		return "dry-run: would redispatch"
	}
	if !cfg.ConfirmRedispatch {
		return "redispatch requires --confirm-redispatch"
	}
	hasPrompt := isYes(r.execDefault(ctx, cfg.Org, name, "test -f /home/sprite/workspace/PROMPT.md && echo yes || echo no", "no"))
	if !hasPrompt {
		return "no PROMPT.md for redispatch"
	}
	cmd := `
REPO_DIR=$(basename $(ls -d /home/sprite/workspace/*/ 2>/dev/null | head -1) 2>/dev/null || echo workspace)
cd /home/sprite/workspace/${REPO_DIR} 2>/dev/null || cd /home/sprite/workspace
nohup bash -c 'cat /home/sprite/workspace/PROMPT.md | claude -p --permission-mode bypassPermissions' > /home/sprite/workspace/watchdog-recovery-$(date +%s).log 2>&1 &
`
	if _, err := r.Sprite.Exec(ctx, cfg.Org, name, cmd); err != nil {
		return "redispatch failed: " + err.Error()
	}
	return "redispatched"
}

func (r *Runner) execDefault(ctx context.Context, org, sprite, command, fallback string) string {
	out, err := r.Sprite.Exec(ctx, org, sprite, command)
	if err != nil {
		return fallback
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return fallback
	}
	return out
}

// Assignment links sprite->task metadata.
type Assignment struct {
	Task string
	Repo string
}

func readAssignments(path string) map[string]Assignment {
	m := map[string]Assignment{}
	if path == "" {
		return m
	}
	fh, err := os.Open(path)
	if err != nil {
		return m
	}
	defer func() { _ = fh.Close() }()
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		task := strings.TrimSpace(parts[1])
		repo := ""
		if len(parts) > 3 {
			repo = strings.TrimSpace(parts[3])
		}
		if name == "" {
			continue
		}
		m[name] = Assignment{Task: task, Repo: repo}
	}
	return m
}

func parseInt(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0
	}
	n, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0
	}
	return n
}

func isYes(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.Contains(s, "yes")
}

func touchMarker(path string) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return os.WriteFile(path, []byte(now+"\n"), 0o644)
}

func withDefaults(cfg Config) Config {
	if cfg.ActiveAgentsFile == "" {
		cfg.ActiveAgentsFile = "/tmp/active-agents.txt"
	}
	if cfg.StaleMinutes <= 0 {
		cfg.StaleMinutes = 30
	}
	if cfg.MarkerFile == "" {
		cfg.MarkerFile = "/tmp/watchdog-marker"
	}
	return cfg
}
