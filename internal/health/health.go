package health

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/internal/clients"
)

// Status values returned by health checks.
const (
	StatusActive    = "active"
	StatusRunning   = "running"
	StatusStale     = "stale"
	StatusDead      = "dead"
	StatusCompleted = "completed"
	StatusBlocked   = "blocked"
	StatusIdle      = "idle"
)

// Config for health checks.
type Config struct {
	Org               string
	Sprite            string
	JSONOutput        bool
	StaleThresholdMin int
	ActiveAgentsFile  string
}

// Result for one sprite health check.
type Result struct {
	Name           string `json:"name"`
	Status         string `json:"status"`
	ClaudeRunning  bool   `json:"claude_running"`
	HasGitChanges  bool   `json:"has_git_changes"`
	CommitCount    int    `json:"commit_count"`
	Stale          bool   `json:"stale"`
	LastFileChange string `json:"last_file_change"`
	Signals        string `json:"signals"`
	CurrentTask    string `json:"current_task"`
	ProcCount      int    `json:"proc_count"`
}

// Checker runs deep health checks.
type Checker struct {
	Sprite clients.SpriteClient
	Out    io.Writer
}

// Run executes health checks for one or all sprites.
func (c *Checker) Run(ctx context.Context, cfg Config) ([]Result, error) {
	cfg = withDefaults(cfg)
	if c.Sprite == nil {
		return nil, fmt.Errorf("sprite client required")
	}
	if c.Out == nil {
		c.Out = os.Stdout
	}

	sprites, err := c.resolveSprites(ctx, cfg)
	if err != nil {
		return nil, err
	}
	assignments := readAssignments(cfg.ActiveAgentsFile)

	results := make([]Result, 0, len(sprites))
	for _, name := range sprites {
		res := c.checkOne(ctx, cfg, name, assignments[name].Task)
		results = append(results, res)
	}

	if cfg.JSONOutput {
		enc := json.NewEncoder(c.Out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			return results, err
		}
	} else {
		renderText(c.Out, results, cfg.StaleThresholdMin)
	}

	return results, nil
}

type assignment struct {
	Task string
}

func readAssignments(path string) map[string]assignment {
	result := map[string]assignment{}
	fh, err := os.Open(path)
	if err != nil {
		return result
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
		if name == "" {
			continue
		}
		result[name] = assignment{Task: task}
	}
	return result
}

func (c *Checker) resolveSprites(ctx context.Context, cfg Config) ([]string, error) {
	if cfg.Sprite != "" {
		return []string{cfg.Sprite}, nil
	}
	sprites, err := c.Sprite.List(ctx, cfg.Org)
	if err != nil {
		return nil, fmt.Errorf("list sprites: %w", err)
	}
	if len(sprites) == 0 {
		return nil, fmt.Errorf("no sprites found")
	}
	return sprites, nil
}

func (c *Checker) checkOne(ctx context.Context, cfg Config, name, task string) Result {
	procCount := parseInt(c.execDefault(ctx, cfg.Org, name, "pgrep -c claude 2>/dev/null || echo 0", "0"))
	claudeRunning := procCount > 0

	complete := isYes(c.execDefault(ctx, cfg.Org, name, "test -f /home/sprite/workspace/TASK_COMPLETE && echo yes || echo no", "no"))
	blocked := isYes(c.execDefault(ctx, cfg.Org, name, "test -f /home/sprite/workspace/BLOCKED.md && echo yes || echo no", "no"))

	signals := ""
	status := "unknown"
	if complete {
		signals = "TASK_COMPLETE"
		status = StatusCompleted
	} else if blocked {
		signals = "BLOCKED"
		status = StatusBlocked
	}

	gitCmd := `
for d in /home/sprite/workspace/*/; do
  [ -d "$d/.git" ] || continue
  cd "$d"
  BRANCH=$(git branch --show-current 2>/dev/null || echo unknown)
  UNCOMMITTED=$(git diff --stat HEAD 2>/dev/null | tail -1)
  COMMITS_AHEAD=$(git log --oneline origin/master..HEAD 2>/dev/null | wc -l || echo 0)
  echo "BRANCH:$BRANCH AHEAD:$COMMITS_AHEAD UNCOMMITTED:$UNCOMMITTED"
  break
done
`
	gitInfo := c.execDefault(ctx, cfg.Org, name, gitCmd, "")
	commitCount := extractInt(gitInfo, "AHEAD:")
	hasGitChanges := extractString(gitInfo, "UNCOMMITTED:") != ""

	recentCmd := `find /home/sprite/workspace -name '*.ts' -o -name '*.tsx' -o -name '*.js' -o -name '*.py' -o -name '*.sh' -o -name '*.md' | head -200 | xargs stat -c '%Y %n' 2>/dev/null | sort -rn | head -1`
	recent := c.execDefault(ctx, cfg.Org, name, recentCmd, "")
	ageMin := extractAgeMinutes(recent)
	lastFileChange := ""
	stale := false
	if ageMin >= 0 {
		lastFileChange = fmt.Sprintf("%dm ago", ageMin)
		if ageMin > cfg.StaleThresholdMin {
			stale = true
		}
	}

	if status == "unknown" {
		status = classify(task, claudeRunning, stale, hasGitChanges, commitCount)
	}

	return Result{
		Name:           name,
		Status:         status,
		ClaudeRunning:  claudeRunning,
		HasGitChanges:  hasGitChanges,
		CommitCount:    commitCount,
		Stale:          stale,
		LastFileChange: lastFileChange,
		Signals:        signals,
		CurrentTask:    task,
		ProcCount:      procCount,
	}
}

func (c *Checker) execDefault(ctx context.Context, org, sprite, cmd, fallback string) string {
	out, err := c.Sprite.Exec(ctx, org, sprite, cmd)
	if err != nil {
		return fallback
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return fallback
	}
	return out
}

func renderText(w io.Writer, results []Result, staleThreshold int) {
	for _, res := range results {
		icon := iconFor(res.Status)
		_, _ = fmt.Fprintf(w, "%s %s: %s\n", icon, res.Name, res.Status)
		if res.CurrentTask != "" {
			_, _ = fmt.Fprintf(w, "   Task: %s\n", res.CurrentTask)
		}
		if res.ClaudeRunning {
			_, _ = fmt.Fprintf(w, "   Claude: running (%d)\n", res.ProcCount)
		} else {
			_, _ = fmt.Fprintln(w, "   Claude: not running")
		}
		_, _ = fmt.Fprintf(w, "   Git: %d commits ahead, changes=%s\n", res.CommitCount, yesNo(res.HasGitChanges))
		_, _ = fmt.Fprintf(w, "   Files: last change %s\n", res.LastFileChange)
		if res.Signals != "" {
			_, _ = fmt.Fprintf(w, "   Signal: %s\n", res.Signals)
		}
		if res.Stale {
			_, _ = fmt.Fprintf(w, "   STALE: No file changes in >%dmin\n", staleThreshold)
		}
		_, _ = fmt.Fprintln(w)
	}
}

func classify(task string, claudeRunning, stale, hasGitChanges bool, commitCount int) string {
	if claudeRunning {
		if stale {
			return StatusStale
		}
		if hasGitChanges || commitCount > 0 {
			return StatusActive
		}
		return StatusRunning
	}
	if task != "" {
		return StatusDead
	}
	return StatusIdle
}

func iconFor(status string) string {
	switch status {
	case StatusActive:
		return "🟢"
	case StatusRunning:
		return "🔵"
	case StatusStale:
		return "🟡"
	case StatusDead:
		return "🔴"
	case StatusCompleted:
		return "✅"
	case StatusBlocked:
		return "🚫"
	case StatusIdle:
		return "⚪"
	default:
		return "❓"
	}
}

func withDefaults(cfg Config) Config {
	if cfg.StaleThresholdMin <= 0 {
		cfg.StaleThresholdMin = 30
	}
	if cfg.ActiveAgentsFile == "" {
		cfg.ActiveAgentsFile = "/tmp/active-agents.txt"
	}
	return cfg
}

func parseInt(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0
	}
	n, err := strconv.Atoi(f[0])
	if err != nil {
		return 0
	}
	return n
}

func isYes(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.Contains(s, "yes")
}

func extractInt(s, key string) int {
	idx := strings.Index(s, key)
	if idx < 0 {
		return 0
	}
	rest := strings.TrimSpace(s[idx+len(key):])
	parts := strings.Fields(rest)
	if len(parts) == 0 {
		return 0
	}
	n, err := strconv.Atoi(strings.Trim(parts[0], "\n\r"))
	if err != nil {
		return 0
	}
	return n
}

func extractString(s, key string) string {
	idx := strings.Index(s, key)
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(s[idx+len(key):])
}

func extractAgeMinutes(recent string) int {
	fields := strings.Fields(recent)
	if len(fields) == 0 {
		return -1
	}
	epoch, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return -1
	}
	now := timeNow().Unix()
	if epoch > now {
		return 0
	}
	return int((now - epoch) / 60)
}

var timeNow = func() time.Time {
	return time.Now()
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
