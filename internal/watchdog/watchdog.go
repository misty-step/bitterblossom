package watchdog

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/internal/claude"
	"github.com/misty-step/bitterblossom/internal/shellutil"
	"github.com/misty-step/bitterblossom/internal/signals"
)

const (
	// DefaultWorkspace is where on-sprite status files live.
	DefaultWorkspace = "/home/sprite/workspace"
	// DefaultStaleAfter marks a sprite as stale when no commits were made in this window.
	DefaultStaleAfter = 2 * time.Hour
	// DefaultMaxRalphIterations controls redispatched Ralph loops.
	DefaultMaxRalphIterations = 50
)

// RemoteClient executes commands on sprites and lists fleet members.
type RemoteClient interface {
	List(ctx context.Context) ([]string, error)
	Exec(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error)
}

// Config wires watchdog dependencies.
type Config struct {
	Remote             RemoteClient
	Workspace          string
	StaleAfter         time.Duration
	MaxRalphIterations int
	Logger             *slog.Logger
	Now                func() time.Time
}

// Request controls target selection and whether side effects are allowed.
type Request struct {
	Sprites []string
	Execute bool
}

// ActionResult records side effects taken by watchdog.
type ActionResult struct {
	Type     ActionType `json:"type,omitempty"`
	Executed bool       `json:"executed,omitempty"`
	Success  bool       `json:"success,omitempty"`
	Message  string     `json:"message,omitempty"`
}

// SpriteReport is one row in the watchdog report.
type SpriteReport struct {
	Sprite         string       `json:"sprite"`
	State          State        `json:"state"`
	Task           string       `json:"task,omitempty"`
	StartedAt      string       `json:"started_at,omitempty"`
	ElapsedMinutes int          `json:"elapsed_minutes,omitempty"`
	Branch         string       `json:"branch,omitempty"`
	CommitsLast2h  int          `json:"commits_last_2h"`
	DirtyRepos     int          `json:"dirty_repos"`
	AheadCommits   int          `json:"ahead_commits"`
	AgentRunning   bool         `json:"agent_running"`
	BlockedReason  string       `json:"blocked_reason,omitempty"`
	Action         ActionResult `json:"action,omitempty"`
	Error          string       `json:"error,omitempty"`
}

// Summary aggregates fleet-level counters.
type Summary struct {
	Total          int `json:"total"`
	Active         int `json:"active"`
	Idle           int `json:"idle"`
	Complete       int `json:"complete"`
	Blocked        int `json:"blocked"`
	Dead           int `json:"dead"`
	Stale          int `json:"stale"`
	Error          int `json:"error"`
	Redispatched   int `json:"redispatched"`
	NeedsAttention int `json:"needs_attention"`
}

// Report is returned from Check.
type Report struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Execute     bool           `json:"execute"`
	StaleAfter  string         `json:"stale_after"`
	Sprites     []SpriteReport `json:"sprites"`
	Summary     Summary        `json:"summary"`
}

// Service monitors the sprite fleet and optionally repairs dead workers.
type Service struct {
	remote             RemoteClient
	workspace          string
	staleAfter         time.Duration
	maxRalphIterations int
	logger             *slog.Logger
	now                func() time.Time
}

// NewService constructs a watchdog service.
func NewService(cfg Config) (*Service, error) {
	if cfg.Remote == nil {
		return nil, fmt.Errorf("watchdog: remote client is required")
	}
	workspace := strings.TrimSpace(cfg.Workspace)
	if workspace == "" {
		workspace = DefaultWorkspace
	}
	staleAfter := cfg.StaleAfter
	if staleAfter <= 0 {
		staleAfter = DefaultStaleAfter
	}
	maxIterations := cfg.MaxRalphIterations
	if maxIterations <= 0 {
		maxIterations = DefaultMaxRalphIterations
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	return &Service{
		remote:             cfg.Remote,
		workspace:          workspace,
		staleAfter:         staleAfter,
		maxRalphIterations: maxIterations,
		logger:             logger,
		now:                now,
	}, nil
}

// Check gathers health for all target sprites.
func (s *Service) Check(ctx context.Context, req Request) (Report, error) {
	targets, err := s.resolveTargets(ctx, req.Sprites)
	if err != nil {
		return Report{}, err
	}

	report := Report{
		GeneratedAt: s.now().UTC(),
		Execute:     req.Execute,
		StaleAfter:  s.staleAfter.String(),
		Sprites:     make([]SpriteReport, 0, len(targets)),
	}

	for _, sprite := range targets {
		row := s.inspectSprite(ctx, sprite, req.Execute)
		report.Sprites = append(report.Sprites, row)
	}

	sort.Slice(report.Sprites, func(i, j int) bool {
		return report.Sprites[i].Sprite < report.Sprites[j].Sprite
	})
	report.Summary = summarize(report.Sprites)
	return report, nil
}

func (s *Service) resolveTargets(ctx context.Context, explicit []string) ([]string, error) {
	if len(explicit) > 0 {
		return uniqueSorted(explicit), nil
	}
	listed, err := s.remote.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("watchdog: list sprites: %w", err)
	}
	return uniqueSorted(listed), nil
}

func (s *Service) inspectSprite(ctx context.Context, sprite string, execute bool) SpriteReport {
	out, err := s.remote.Exec(ctx, sprite, buildProbeScript(s.workspace), nil)
	if err != nil {
		return SpriteReport{
			Sprite: sprite,
			State:  StateError,
			Error:  err.Error(),
		}
	}

	probe, err := parseProbeOutput(out)
	if err != nil {
		return SpriteReport{
			Sprite: sprite,
			State:  StateError,
			Error:  err.Error(),
		}
	}

	startedAt, elapsedMinutes := s.elapsed(probe.Status.Started)
	task := composeTaskLabel(probe)
	hasTask := task != "" || strings.TrimSpace(probe.CurrentTaskID) != ""
	input := stateInput{
		AgentRunning:   probe.AgentRunning || probe.ClaudeCount > 0,
		HasComplete:    probe.HasComplete,
		HasBlocked:     probe.HasBlocked,
		HasTask:        hasTask || probe.HasPrompt,
		Elapsed:        time.Duration(elapsedMinutes) * time.Minute,
		CommitsLast2h:  probe.CommitsLast2h,
		StatusComplete: strings.TrimSpace(probe.Status.Completed) != "", // dispatch finished (#367)
	}
	state := evaluateState(input, s.staleAfter)
	actionType := decideAction(state, probe.HasPrompt)
	action := ActionResult{Type: actionType}

	if actionType == ActionInvestigate {
		action.Message = "running without commits beyond stale threshold"
	}
	if actionType == ActionManualAction {
		action.Message = "dead sprite has no PROMPT.md to restart from"
	}
	if actionType == ActionRedispatch && execute {
		action.Executed = true
		redispatchOut, redispatchErr := s.remote.Exec(ctx, sprite, buildRedispatchScript(s.workspace, sprite, s.maxRalphIterations), nil)
		action.Success = redispatchErr == nil
		if redispatchErr != nil {
			action.Message = redispatchErr.Error()
		} else {
			action.Message = strings.TrimSpace(redispatchOut)
		}
	}

	row := SpriteReport{
		Sprite:         sprite,
		State:          state,
		Task:           task,
		StartedAt:      startedAt,
		ElapsedMinutes: elapsedMinutes,
		Branch:         probe.Branch,
		CommitsLast2h:  probe.CommitsLast2h,
		DirtyRepos:     probe.DirtyRepos,
		AheadCommits:   probe.AheadCommits,
		AgentRunning:   input.AgentRunning,
		BlockedReason:  probe.BlockedSummary,
		Action:         action,
	}
	s.logger.Info("watchdog sprite", "sprite", sprite, "state", row.State, "task", row.Task, "action", row.Action.Type)
	return row
}

func (s *Service) elapsed(started string) (string, int) {
	started = strings.TrimSpace(started)
	if started == "" {
		return "", 0
	}
	parsed, err := time.Parse(time.RFC3339, started)
	if err != nil {
		return started, 0
	}
	delta := s.now().UTC().Sub(parsed)
	if delta < 0 {
		return started, 0
	}
	return parsed.UTC().Format(time.RFC3339), int(delta / time.Minute)
}

func composeTaskLabel(probe probe) string {
	if strings.TrimSpace(probe.Status.Repo) != "" {
		if probe.Status.Issue > 0 {
			return fmt.Sprintf("%s#%d", probe.Status.Repo, probe.Status.Issue)
		}
		return probe.Status.Repo
	}
	if strings.TrimSpace(probe.Status.Task) != "" {
		return strings.TrimSpace(probe.Status.Task)
	}
	return strings.TrimSpace(probe.CurrentTaskID)
}

func summarize(rows []SpriteReport) Summary {
	summary := Summary{Total: len(rows)}
	for _, row := range rows {
		switch row.State {
		case StateActive:
			summary.Active++
		case StateIdle:
			summary.Idle++
		case StateComplete:
			summary.Complete++
			summary.NeedsAttention++
		case StateBlocked:
			summary.Blocked++
			summary.NeedsAttention++
		case StateDead:
			summary.Dead++
			summary.NeedsAttention++
		case StateStale:
			summary.Stale++
			summary.NeedsAttention++
		case StateError:
			summary.Error++
			summary.NeedsAttention++
		}
		if row.Action.Type == ActionRedispatch && row.Action.Executed && row.Action.Success {
			summary.Redispatched++
		}
	}
	return summary
}

func buildProbeScript(workspace string) string {
	return strings.Join([]string{
		"set -euo pipefail",
		"WORKSPACE=" + shellutil.Quote(workspace),
		"claude_count=\"$(pgrep -fc 'claude -p' 2>/dev/null || echo 0)\"",
		"claude_count=\"$(echo \"$claude_count\" | tr -d '[:space:]')\"",
		"agent_running=no",
		"if [ -f \"$WORKSPACE/agent.pid\" ] && kill -0 \"$(cat \"$WORKSPACE/agent.pid\")\" 2>/dev/null; then agent_running=yes; fi",
		"if [ \"$agent_running\" = no ] && [ \"${claude_count:-0}\" -gt 0 ] 2>/dev/null; then agent_running=yes; fi",
		signals.DetectCompleteScript(workspace),
		"has_complete=$HAS_COMPLETE",
		signals.DetectBlockedScript(workspace),
		"has_blocked=$HAS_BLOCKED",
		"blocked_summary=\"$BLOCKED_SUMMARY\"",
		"has_prompt=no; [ -f \"$WORKSPACE/PROMPT.md\" ] && has_prompt=yes",
		"branch=\"\"; commits=0; dirty_repos=0; ahead_commits=0",
		"for dir in \"$WORKSPACE\"/*/; do",
		"  [ -d \"$dir/.git\" ] || continue",
		"  b=\"$(git -C \"$dir\" branch --show-current 2>/dev/null || true)\"",
		"  if [ -z \"$branch\" ] && [ -n \"$b\" ] && [ \"$b\" != \"main\" ] && [ \"$b\" != \"master\" ]; then branch=\"$b\"; fi",
		"  c=\"$(git -C \"$dir\" log --oneline --since='2 hours ago' 2>/dev/null | wc -l || echo 0)\"",
		"  c=\"$(echo \"$c\" | tr -d '[:space:]')\"",
		"  case \"$c\" in ''|*[!0-9]*) c=0 ;; esac",
		"  commits=$((commits + c))",
		"  if ! git -C \"$dir\" diff --quiet --ignore-submodules -- 2>/dev/null; then dirty_repos=$((dirty_repos + 1)); fi",
		"  upstream=\"$(git -C \"$dir\" rev-parse --abbrev-ref --symbolic-full-name '@{u}' 2>/dev/null || true)\"",
		"  if [ -z \"$upstream\" ]; then",
		"    branch_name=\"$(git -C \"$dir\" rev-parse --abbrev-ref HEAD 2>/dev/null || true)\"",
		"    if [ -n \"$branch_name\" ] && git -C \"$dir\" show-ref --verify --quiet \"refs/remotes/origin/$branch_name\"; then upstream=\"origin/$branch_name\"; fi",
		"  fi",
		"  if [ -n \"$upstream\" ]; then",
		"    ahead=\"$(git -C \"$dir\" rev-list --count \"$upstream..HEAD\" 2>/dev/null || echo 0)\"",
		"    ahead=\"$(echo \"$ahead\" | tr -d '[:space:]')\"",
		"    case \"$ahead\" in ''|*[!0-9]*) ahead=0 ;; esac",
		"    ahead_commits=$((ahead_commits + ahead))",
		"  fi",
		"done",
		"status_json=\"\"; [ -f \"$WORKSPACE/STATUS.json\" ] && status_json=\"$(tr -d '\\n' < \"$WORKSPACE/STATUS.json\")\"",
		"task_id=\"$(cat \"$WORKSPACE/.current-task-id\" 2>/dev/null || true)\"",
		"echo \"__CLAUDE_COUNT__${claude_count:-0}\"",
		"echo \"__AGENT_RUNNING__${agent_running}\"",
		"echo \"__HAS_COMPLETE__${has_complete}\"",
		"echo \"__HAS_BLOCKED__${has_blocked}\"",
		"echo \"__COMMITS_LAST_2H__${commits}\"",
		"echo \"__DIRTY_REPOS__${dirty_repos}\"",
		"echo \"__AHEAD_COMMITS__${ahead_commits}\"",
		"echo \"__HAS_PROMPT__${has_prompt}\"",
		"echo \"__BLOCKED_B64__$(printf '%s' \"$blocked_summary\" | base64 | tr -d '\\n')\"",
		"echo \"__BRANCH_B64__$(printf '%s' \"$branch\" | base64 | tr -d '\\n')\"",
		"echo \"__STATUS_B64__$(printf '%s' \"$status_json\" | base64 | tr -d '\\n')\"",
		"echo \"__TASK_ID_B64__$(printf '%s' \"$task_id\" | base64 | tr -d '\\n')\"",
	}, "\n")
}

func buildRedispatchScript(workspace, sprite string, maxIterations int) string {
	lines := []string{
		"set -euo pipefail",
		"WORKSPACE=" + shellutil.Quote(workspace),
		"if [ ! -f \"$WORKSPACE/PROMPT.md\" ]; then echo \"missing PROMPT.md\"; exit 0; fi",
		"if [ -f \"$WORKSPACE/agent.pid\" ] && kill -0 \"$(cat \"$WORKSPACE/agent.pid\")\" 2>/dev/null; then kill \"$(cat \"$WORKSPACE/agent.pid\")\" 2>/dev/null || true; fi",
		"if [ -f \"$WORKSPACE/ralph.pid\" ] && kill -0 \"$(cat \"$WORKSPACE/ralph.pid\")\" 2>/dev/null; then kill \"$(cat \"$WORKSPACE/ralph.pid\")\" 2>/dev/null || true; fi",
		"AGENT_BIN=\"$HOME/.local/bin/sprite-agent\"",
		"if [ ! -x \"$AGENT_BIN\" ]; then AGENT_BIN=\"$WORKSPACE/.sprite-agent.sh\"; fi",
		"cd \"$WORKSPACE\"",
	}

	if maxIterations <= 0 {
		maxIterations = DefaultMaxRalphIterations
	}

	lines = append(lines,
		"if [ -x \"$AGENT_BIN\" ]; then",
		"  nohup env SPRITE_NAME="+shellutil.Quote(sprite)+" MAX_ITERATIONS="+strconv.Itoa(maxIterations)+" \"$AGENT_BIN\" >/dev/null 2>&1 &",
		"else",
		"  nohup bash -lc 'cat \"$WORKSPACE/PROMPT.md\" | claude "+claude.FlagSetWithPrefix()+"' > \"$WORKSPACE/watchdog-recovery-$(date +%s).log\" 2>&1 &",
		"fi",
		"PID=\"$!\"",
		"echo \"$PID\" > \"$WORKSPACE/agent.pid\"",
		"echo \"$PID\" > \"$WORKSPACE/ralph.pid\"",
		"echo \"redispatched pid=$PID\"",
	)
	return strings.Join(lines, "\n")
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	sort.Strings(result)
	return result
}

