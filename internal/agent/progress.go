package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/misty-step/bitterblossom/pkg/events"
)

const (
	DefaultProgressInterval = 20 * time.Second
	DefaultStallTimeout     = 10 * time.Minute
)

// ProgressSignalType identifies terminal outcomes from the progress detector.
type ProgressSignalType string

const (
	ProgressSignalStalled ProgressSignalType = "stalled"
)

// ProgressSignal is emitted when the monitor detects a fleet-relevant condition.
type ProgressSignal struct {
	Type   ProgressSignalType
	Reason string
}

// EventEmitter is the subset of pkg/events emitter used by the supervisor package.
type EventEmitter interface {
	Emit(event events.Event) error
}

// ProgressConfig controls polling and stall-detection behavior.
type ProgressConfig struct {
	Sprite       string
	RepoDir      string
	PollInterval time.Duration
	StallTimeout time.Duration
	OnActivity   func(activity string, at time.Time, stalled bool)
}

// GitSnapshot captures repo activity used by progress + heartbeat.
type GitSnapshot struct {
	Branch       string
	Head         string
	HeadTime     time.Time
	ChangedFiles int
	Uncommitted  bool
	Branches     []string
	CommitCount  int
}

// GitClient reports structured git repository state.
type GitClient interface {
	Snapshot(ctx context.Context) (GitSnapshot, error)
}

// ProgressMonitor watches git and agent output for progress signals.
type ProgressMonitor struct {
	cfg     ProgressConfig
	emitter EventEmitter
	git     GitClient
	now     func() time.Time

	signals chan ProgressSignal
	once    sync.Once

	mu               sync.RWMutex
	snapshot         GitSnapshot
	hasSnapshot      bool
	lastGitActivity  time.Time
	lastOutActivity  time.Time
	stalled          bool
	lastActivityKind string
}

// NewProgressMonitor returns a progress detector with production defaults.
func NewProgressMonitor(cfg ProgressConfig, emitter EventEmitter) *ProgressMonitor {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = DefaultProgressInterval
	}
	if cfg.StallTimeout <= 0 {
		cfg.StallTimeout = DefaultStallTimeout
	}

	return &ProgressMonitor{
		cfg:     cfg,
		emitter: emitter,
		git:     newGitCLI(cfg.RepoDir),
		now:     time.Now,
		signals: make(chan ProgressSignal, 1),
	}
}

// Signals exposes terminal monitor outcomes.
func (m *ProgressMonitor) Signals() <-chan ProgressSignal {
	return m.signals
}

// Snapshot returns the latest observed git state.
func (m *ProgressMonitor) Snapshot() (GitSnapshot, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.hasSnapshot {
		return GitSnapshot{}, false
	}
	copySnapshot := m.snapshot
	copySnapshot.Branches = append([]string(nil), copySnapshot.Branches...)
	return copySnapshot, true
}

// LastActivityTime returns the latest meaningful git/output activity timestamp.
func (m *ProgressMonitor) LastActivityTime() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()

	latest := m.lastGitActivity
	if m.lastOutActivity.After(latest) {
		latest = m.lastOutActivity
	}
	return latest
}

// ObserveOutput classifies agent output and emits structured progress events.
func (m *ProgressMonitor) ObserveOutput(line string, stderr bool) {
	activity, detail, success, meaningful := classifyAgentOutput(line, stderr)
	if !meaningful {
		return
	}

	now := m.now().UTC()
	var branch string

	m.mu.Lock()
	if m.hasSnapshot {
		branch = m.snapshot.Branch
	}
	m.lastOutActivity = now
	m.lastActivityKind = activity
	if m.stalled {
		m.stalled = false
	}
	isStalled := m.stalled
	m.mu.Unlock()

	_ = m.emitter.Emit(&events.ProgressEvent{
		Meta:     events.Meta{TS: now, SpriteName: m.cfg.Sprite, EventKind: events.KindProgress},
		Branch:   branch,
		Activity: activity,
		Detail:   detail,
		Success:  success,
	})
	m.notifyActivity(activity, now, isStalled)
}

// Run starts periodic git polling until context cancellation.
func (m *ProgressMonitor) Run(ctx context.Context, wg *sync.WaitGroup) {
	if wg != nil {
		defer wg.Done()
	}

	m.poll(ctx)

	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.poll(ctx)
		}
	}
}

func (m *ProgressMonitor) poll(ctx context.Context) {
	now := m.now().UTC()
	current, err := m.git.Snapshot(ctx)
	if err != nil {
		_ = m.emitter.Emit(&events.ErrorEvent{
			Meta:    events.Meta{TS: now, SpriteName: m.cfg.Sprite, EventKind: events.KindError},
			Code:    "git_snapshot",
			Message: err.Error(),
		})
		return
	}

	emitted := false
	stalled := false

	m.mu.Lock()
	previous := m.snapshot
	hadSnapshot := m.hasSnapshot
	if !hadSnapshot {
		m.hasSnapshot = true
		m.snapshot = current
		m.lastGitActivity = now
		m.mu.Unlock()
		m.notifyActivity("git_snapshot", now, false)
		return
	}

	if current.Head != previous.Head {
		emitted = true
		m.lastGitActivity = now
		m.lastActivityKind = "git_commit"
	}

	if current.ChangedFiles != previous.ChangedFiles || current.Uncommitted != previous.Uncommitted {
		emitted = true
		m.lastGitActivity = now
		if m.lastActivityKind == "" {
			m.lastActivityKind = "file_change"
		}
	}

	newBranches := branchesAdded(previous.Branches, current.Branches)
	if len(newBranches) > 0 {
		emitted = true
		m.lastGitActivity = now
		if m.lastActivityKind == "" {
			m.lastActivityKind = "branch_created"
		}
	}

	latest := m.lastGitActivity
	if m.lastOutActivity.After(latest) {
		latest = m.lastOutActivity
	}
	if latest.IsZero() {
		latest = now
	}
	if m.cfg.StallTimeout > 0 && now.Sub(latest) >= m.cfg.StallTimeout {
		if !m.stalled {
			m.stalled = true
			stalled = true
		}
	}

	m.snapshot = current
	isStalled := m.stalled
	m.mu.Unlock()

	if current.Head != previous.Head {
		_ = m.emitter.Emit(&events.ProgressEvent{
			Meta:         events.Meta{TS: now, SpriteName: m.cfg.Sprite, EventKind: events.KindProgress},
			Branch:       current.Branch,
			Commits:      current.CommitCount,
			FilesChanged: current.ChangedFiles,
			Activity:     "git_commit",
			Detail:       fmt.Sprintf("new commit %s", shortHash(current.Head)),
			LastCommit:   shortHash(current.Head),
		})
		m.notifyActivity("git_commit", now, isStalled)
	}

	for _, created := range branchesAdded(previous.Branches, current.Branches) {
		_ = m.emitter.Emit(&events.ProgressEvent{
			Meta:          events.Meta{TS: now, SpriteName: m.cfg.Sprite, EventKind: events.KindProgress},
			Branch:        current.Branch,
			Activity:      "branch_created",
			BranchCreated: created,
			Detail:        fmt.Sprintf("branch created: %s", created),
		})
		m.notifyActivity("branch_created", now, isStalled)
	}

	if current.ChangedFiles != previous.ChangedFiles || current.Uncommitted != previous.Uncommitted {
		_ = m.emitter.Emit(&events.ProgressEvent{
			Meta:         events.Meta{TS: now, SpriteName: m.cfg.Sprite, EventKind: events.KindProgress},
			Branch:       current.Branch,
			Commits:      current.CommitCount,
			FilesChanged: current.ChangedFiles,
			Activity:     "file_change",
			Detail:       fmt.Sprintf("uncommitted changes=%t files=%d", current.Uncommitted, current.ChangedFiles),
			LastCommit:   shortHash(current.Head),
		})
		m.notifyActivity("file_change", now, isStalled)
	}

	if stalled {
		blockedReason := fmt.Sprintf("stalled: no git or output activity for %s", m.cfg.StallTimeout)
		_ = m.emitter.Emit(&events.BlockedEvent{
			Meta:   events.Meta{TS: now, SpriteName: m.cfg.Sprite, EventKind: events.KindBlocked},
			Reason: blockedReason,
		})
		stalledValue := true
		_ = m.emitter.Emit(&events.ProgressEvent{
			Meta:     events.Meta{TS: now, SpriteName: m.cfg.Sprite, EventKind: events.KindProgress},
			Branch:   current.Branch,
			Activity: "stalled",
			Detail:   blockedReason,
			Stalled:  &stalledValue,
		})
		m.notifyActivity("stalled", now, true)
		m.signal(ProgressSignal{Type: ProgressSignalStalled, Reason: blockedReason})
		return
	}

	if emitted {
		return
	}
}

func (m *ProgressMonitor) notifyActivity(activity string, at time.Time, stalled bool) {
	if m.cfg.OnActivity != nil {
		m.cfg.OnActivity(activity, at, stalled)
	}
}

func (m *ProgressMonitor) signal(signal ProgressSignal) {
	m.once.Do(func() {
		m.signals <- signal
	})
}

var (
	testRunPattern = regexp.MustCompile(`(?i)\b(go test|pytest|npm test|pnpm test|yarn test|jest|cargo test)\b`)
	successPattern = regexp.MustCompile(`(?i)\b(build succeeded|build successful|compiled successfully|tests passed|all checks passed)\b`)
	failurePattern = regexp.MustCompile(`(?i)\b(build failed|tests failed|panic:|fatal:|exception|\berror\b|\bfail\b)\b`)

	toolCallPattern = regexp.MustCompile(`(?i)\b(tool( call)?|using tool|invoking tool|exec_command|apply_patch|mcp__|search_query|read_mcp_resource|write_stdin)\b`)
	fileEditPattern = regexp.MustCompile(`(?i)(\*\*\*\s+(add|update|delete|move)\s+file:|\b(edit|edited|update|updated|create|created|delete|deleted|rename|renamed)\b\s+\S+(\.\w+)?|\bapply_patch\b)`)
	commandPattern  = regexp.MustCompile(`(?i)(^[$#]\s+\S+|^\s*(bash|sh|zsh)\s+-lc\b|\b(running|executing)\s+command\b|\b(git|go|pnpm|npm|yarn|pytest|cargo|make|docker|kubectl|uv|bun)\s+\S+)`)
)

func classifyAgentOutput(line string, stderr bool) (activity string, detail string, success *bool, meaningful bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", "", nil, false
	}

	detail = normalizeDetail(trimmed)

	if testRunPattern.MatchString(trimmed) {
		return "test_run", detail, nil, true
	}

	if successPattern.MatchString(trimmed) {
		ok := true
		return "build_result", detail, &ok, true
	}

	if failurePattern.MatchString(trimmed) || stderr {
		ok := false
		return "error", detail, &ok, true
	}

	if toolCallPattern.MatchString(trimmed) {
		return "tool_call", detail, nil, true
	}

	if fileEditPattern.MatchString(trimmed) {
		return "file_edit", detail, nil, true
	}

	if commandPattern.MatchString(trimmed) {
		return "command_run", detail, nil, true
	}

	return "", "", nil, false
}

func normalizeDetail(detail string) string {
	if len(detail) > 240 {
		return detail[:240]
	}
	return detail
}

func shortHash(hash string) string {
	hash = strings.TrimSpace(hash)
	if len(hash) <= 12 {
		return hash
	}
	return hash[:12]
}

func branchesAdded(previous, current []string) []string {
	prevSet := make(map[string]struct{}, len(previous))
	for _, value := range previous {
		prevSet[value] = struct{}{}
	}
	result := make([]string, 0)
	for _, value := range current {
		if _, ok := prevSet[value]; ok {
			continue
		}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

// commandRunner executes subprocesses used by git and telemetry helpers.
type commandRunner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	if err := cmd.Run(); err != nil {
		output := strings.TrimSpace(combined.String())
		if output == "" {
			return "", err
		}
		return "", fmt.Errorf("%s: %w", output, err)
	}
	return strings.TrimSpace(combined.String()), nil
}

type gitCLI struct {
	repoDir string
	runner  commandRunner
}

func newGitCLI(repoDir string) *gitCLI {
	return &gitCLI{repoDir: repoDir, runner: execRunner{}}
}

func (g *gitCLI) Snapshot(ctx context.Context) (GitSnapshot, error) {
	branch, err := g.git(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return GitSnapshot{}, fmt.Errorf("read branch: %w", err)
	}

	headInfo, err := g.git(ctx, "show", "-s", "--format=%H %ct", "HEAD")
	if err != nil {
		return GitSnapshot{}, fmt.Errorf("read head commit: %w", err)
	}
	parts := strings.Fields(headInfo)
	if len(parts) != 2 {
		return GitSnapshot{}, fmt.Errorf("unexpected head format %q", headInfo)
	}
	epoch, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return GitSnapshot{}, fmt.Errorf("parse head timestamp: %w", err)
	}

	status, err := g.git(ctx, "status", "--porcelain")
	if err != nil {
		return GitSnapshot{}, fmt.Errorf("read git status: %w", err)
	}
	changedFiles := 0
	if strings.TrimSpace(status) != "" {
		changedFiles = len(strings.Split(strings.TrimSpace(status), "\n"))
	}

	branchesRaw, err := g.git(ctx, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return GitSnapshot{}, fmt.Errorf("read branches: %w", err)
	}
	branches := make([]string, 0)
	for _, line := range strings.Split(branchesRaw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		branches = append(branches, line)
	}
	sort.Strings(branches)

	commitCountRaw, err := g.git(ctx, "rev-list", "--count", "HEAD")
	if err != nil {
		return GitSnapshot{}, fmt.Errorf("read commit count: %w", err)
	}
	commitCount, err := strconv.Atoi(strings.TrimSpace(commitCountRaw))
	if err != nil {
		return GitSnapshot{}, fmt.Errorf("parse commit count: %w", err)
	}

	return GitSnapshot{
		Branch:       branch,
		Head:         parts[0],
		HeadTime:     time.Unix(epoch, 0).UTC(),
		ChangedFiles: changedFiles,
		Uncommitted:  changedFiles > 0,
		Branches:     branches,
		CommitCount:  commitCount,
	}, nil
}

func (g *gitCLI) git(ctx context.Context, args ...string) (string, error) {
	if strings.TrimSpace(g.repoDir) == "" {
		return "", errors.New("repo directory is required")
	}
	gitArgs := make([]string, 0, len(args)+2)
	gitArgs = append(gitArgs, "-C", g.repoDir)
	gitArgs = append(gitArgs, args...)
	return g.runner.Run(ctx, "git", gitArgs...)
}
