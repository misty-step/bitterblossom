// Package ledger provides a durable task event store with materialized snapshots
// for non-blocking status and watchdog operations.
package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

// Event kinds for the task ledger.
type EventKind string

const (
	EventDispatchStarted  EventKind = "dispatch_started"
	EventRepoSetupStarted EventKind = "repo_setup_started"
	EventAgentStarted     EventKind = "agent_started"
	EventHeartbeat        EventKind = "heartbeat"
	EventBlocked          EventKind = "blocked"
	EventCompleted        EventKind = "completed"
	EventFailed           EventKind = "failed"
)

// TaskEvent is an append-only event in the task ledger.
type TaskEvent struct {
	// ID is a unique identifier for this event (UUID-like).
	ID string `json:"id"`
	// Sprite is the sprite name.
	Sprite string `json:"sprite"`
	// TaskID is the task identifier.
	TaskID string `json:"task_id"`
	// Kind is the event type.
	Kind EventKind `json:"kind"`
	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`
	// Repo is the repository (for dispatch_started, etc.).
	Repo string `json:"repo,omitempty"`
	// Branch is the branch name.
	Branch string `json:"branch,omitempty"`
	// Issue is the GitHub issue number.
	Issue int `json:"issue,omitempty"`
	// Reason is the reason (for blocked, failed).
	Reason string `json:"reason,omitempty"`
	// Commits is the commit count (for heartbeat).
	Commits int `json:"commits,omitempty"`
	// Details is additional event details.
	Details map[string]string `json:"details,omitempty"`
}

// TaskSnapshot is the materialized latest-state snapshot for a sprite/task.
type TaskSnapshot struct {
	// Sprite is the sprite name.
	Sprite string `json:"sprite"`
	// TaskID is the task identifier.
	TaskID string `json:"task_id"`
	// Repo is the repository.
	Repo string `json:"repo,omitempty"`
	// Branch is the branch name.
	Branch string `json:"branch,omitempty"`
	// Issue is the GitHub issue number.
	Issue int `json:"issue,omitempty"`
	// State is the current state (derived from latest event).
	State TaskState `json:"state"`
	// LastSeenAt is the timestamp of the most recent event.
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	// FreshnessAge is how long since last_seen_at.
	FreshnessAge time.Duration `json:"freshness_age_ns,omitempty"`
	// ProbeStatus is the probe result (success/failure/unknown).
	ProbeStatus ProbeStatus `json:"probe_status"`
	// Error is the error message if state is failed.
	Error string `json:"error,omitempty"`
	// BlockedReason is the reason if state is blocked.
	BlockedReason string `json:"blocked_reason,omitempty"`
	// EventCount is the total number of events for this task.
	EventCount int `json:"event_count"`
	// StartedAt is when the task was dispatched.
	StartedAt *time.Time `json:"started_at,omitempty"`
	// CompletedAt is when the task completed (if completed).
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// TaskState is the derived state for a task.
type TaskState string

const (
	TaskStatePending    TaskState = "pending"    // dispatch_started received
	TaskStateSettingUp  TaskState = "setting_up" // repo_setup_started
	TaskStateRunning    TaskState = "running"    // agent_started, heartbeat
	TaskStateBlocked    TaskState = "blocked"    // blocked event
	TaskStateCompleted  TaskState = "completed"  // completed event
	TaskStateFailed     TaskState = "failed"     // failed event
	TaskStateUnknown    TaskState = "unknown"    // no events or stale
	TaskStateStale      TaskState = "stale"      // no recent events
)

// ProbeStatus represents the result of a remote probe.
type ProbeStatus string

const (
	ProbeStatusUnknown  ProbeStatus = "unknown"
	ProbeStatusSuccess  ProbeStatus = "success"
	ProbeStatusFailed   ProbeStatus = "failed"
	ProbeStatusDegraded ProbeStatus = "degraded" // slow response
)

// Config controls the ledger service.
type Config struct {
	// Dir is the ledger storage directory.
	Dir string
	// Now is the time provider (defaults to time.Now).
	Now func() time.Time
}

// Store provides append-only event storage and snapshot materialization.
type Store struct {
	dir string
	now func() time.Time
}

// NewStore creates a new ledger store.
func NewStore(cfg Config) (*Store, error) {
	dir := strings.TrimSpace(cfg.Dir)
	if dir == "" {
		dir = DefaultDir()
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Store{dir: dir, now: now}, nil
}

// DefaultDir returns the default ledger storage directory.
func DefaultDir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil || home == "" {
			return filepath.Join(".config", "bb", "ledger")
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "bb", "ledger")
}

// Dir returns the store's directory.
func (s *Store) Dir() string {
	return s.dir
}

// Append adds a new event to the ledger.
func (s *Store) Append(event TaskEvent) error {
	if event.ID == "" {
		return errors.New("ledger: event ID is required")
	}
	if event.Sprite == "" {
		return errors.New("ledger: sprite name is required")
	}
	if event.TaskID == "" {
		return errors.New("ledger: task ID is required")
	}
	if event.Kind == "" {
		return errors.New("ledger: event kind is required")
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = s.now().UTC()
	}

	// Ensure directory exists
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return fmt.Errorf("ledger: mkdir: %w", err)
	}

	// Write to daily file
	filename := dailyFilename(event.Timestamp)
	path := filepath.Join(s.dir, filename)

	// Lock file for writing
	lockPath := path + ".lock"
	lock := flock.New(lockPath)
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("ledger: lock: %w", err)
	}
	defer func() {
		_ = lock.Unlock()
	}()

	// Open file for append
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("ledger: open: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	// Marshal and write event
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("ledger: marshal: %w", err)
	}
	data = append(data, '\n')
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("ledger: write: %w", err)
	}

	return nil
}

// QueryOptions filters event queries.
type QueryOptions struct {
	// Sprite filters by sprite name.
	Sprite string
	// TaskID filters by task ID.
	TaskID string
	// Kind filters by event kind.
	Kind EventKind
	// Since filters events after this time.
	Since time.Time
	// Until filters events before this time.
	Until time.Time
	// Limit caps the number of events returned.
	Limit int
}

// Query retrieves events matching the options.
func (s *Store) Query(opts QueryOptions) ([]TaskEvent, error) {
	if opts.Limit < 0 {
		return nil, errors.New("ledger: limit must be >= 0")
	}

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []TaskEvent{}, nil
		}
		return nil, fmt.Errorf("ledger: read dir: %w", err)
	}

	paths := listDailyPaths(entries, s.dir, opts.Since, opts.Until)
	out := make([]TaskEvent, 0, 32)

	readPath := func(path string) error {
		lockPath := path + ".lock"
		lock := flock.New(lockPath)
		if err := lock.RLock(); err != nil {
			return fmt.Errorf("ledger: lock: %w", err)
		}
		defer func() {
			_ = lock.Unlock()
		}()

		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil // File was deleted, skip
			}
			return fmt.Errorf("ledger: read: %w", err)
		}

		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var event TaskEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue // Skip malformed lines
			}
			if !opts.Since.IsZero() && event.Timestamp.Before(opts.Since) {
				continue
			}
			if !opts.Until.IsZero() && event.Timestamp.After(opts.Until) {
				continue
			}
			if opts.Sprite != "" && event.Sprite != opts.Sprite {
				continue
			}
			if opts.TaskID != "" && event.TaskID != opts.TaskID {
				continue
			}
			if opts.Kind != "" && event.Kind != opts.Kind {
				continue
			}
			out = append(out, event)
		}
		return nil
	}

	if opts.Limit > 0 {
		// Read newest first for limit
		for i := len(paths) - 1; i >= 0; i-- {
			if err := readPath(paths[i]); err != nil {
				return nil, err
			}
			if len(out) >= opts.Limit {
				break
			}
		}
	} else {
		for _, path := range paths {
			if err := readPath(path); err != nil {
				return nil, err
			}
		}
	}

	// Sort by timestamp ascending
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.Before(out[j].Timestamp)
	})

	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[len(out)-opts.Limit:]
	}

	return out, nil
}

// LatestEvents returns the most recent event per (sprite, taskID).
func (s *Store) LatestEvents() (map[string]TaskEvent, error) {
	events, err := s.Query(QueryOptions{Limit: 10000})
	if err != nil {
		return nil, err
	}

	latest := make(map[string]TaskEvent)
	for _, event := range events {
		key := event.Sprite + "/" + event.TaskID
		if existing, ok := latest[key]; !ok || event.Timestamp.After(existing.Timestamp) {
			latest[key] = event
		}
	}
	return latest, nil
}

// Snapshot returns materialized snapshots for all known tasks.
// The staleThreshold determines when a task is considered stale.
func (s *Store) Snapshot(staleThreshold time.Duration) (map[string]TaskSnapshot, error) {
	// Get all events (not just latest) to derive full snapshot state
	events, err := s.Query(QueryOptions{Limit: 10000})
	if err != nil {
		return nil, err
	}

	// Group events by (sprite, taskID)
	byKey := make(map[string][]TaskEvent)
	for _, event := range events {
		key := event.Sprite + "/" + event.TaskID
		byKey[key] = append(byKey[key], event)
	}

	now := s.now().UTC()
	snapshots := make(map[string]TaskSnapshot)

	for key, evts := range byKey {
		if len(evts) == 0 {
			continue
		}
		// Sort by timestamp
		sort.Slice(evts, func(i, j int) bool {
			return evts[i].Timestamp.Before(evts[j].Timestamp)
		})

		// Get the first event for repo/branch/issue
		first := evts[0]
		// Get the last event for state
		last := evts[len(evts)-1]

		snapshot := materializer(last, now, staleThreshold)
		// Carry forward from first event
		snapshot.Repo = first.Repo
		snapshot.Branch = first.Branch
		snapshot.Issue = first.Issue
		if first.Timestamp.After(time.Time{}) {
			snapshot.StartedAt = &first.Timestamp
		}
		snapshots[key] = snapshot
	}

	return snapshots, nil
}

// SnapshotForSprite returns snapshots for a specific sprite.
func (s *Store) SnapshotForSprite(sprite string, staleThreshold time.Duration) (map[string]TaskSnapshot, error) {
	events, err := s.Query(QueryOptions{
		Sprite: sprite,
		Limit:  1000,
	})
	if err != nil {
		return nil, err
	}

	// Get latest event per task
	latestByTask := make(map[string]TaskEvent)
	for _, event := range events {
		if existing, ok := latestByTask[event.TaskID]; !ok || event.Timestamp.After(existing.Timestamp) {
			latestByTask[event.TaskID] = event
		}
	}

	now := s.now().UTC()
	snapshots := make(map[string]TaskSnapshot)
	for taskID, event := range latestByTask {
		snapshot := materializer(event, now, staleThreshold)
		snapshots[taskID] = snapshot
	}

	return snapshots, nil
}

// UpdateProbeStatus updates the probe status for a snapshot.
func (s *Store) UpdateProbeStatus(sprite, taskID string, status ProbeStatus, staleThreshold time.Duration) error {
	events, err := s.Query(QueryOptions{
		Sprite: sprite,
		TaskID: taskID,
		Limit:  1,
	})
	if err != nil {
		return err
	}

	if len(events) == 0 {
		return errors.New("ledger: no events found for task")
	}

	latest := events[len(events)-1]
	now := s.now().UTC()
	snapshot := materializer(latest, now, staleThreshold)
	snapshot.ProbeStatus = status

	// Emit a synthetic heartbeat with the probe status
	event := TaskEvent{
		ID:        fmt.Sprintf("probe-%d", now.UnixNano()),
		Sprite:    sprite,
		TaskID:    taskID,
		Kind:      EventHeartbeat,
		Timestamp: now,
		Details: map[string]string{
			"probe_status": string(status),
		},
	}

	return s.Append(event)
}

// materializer converts an event to a snapshot.
func materializer(event TaskEvent, now time.Time, staleThreshold time.Duration) TaskSnapshot {
	snapshot := TaskSnapshot{
		Sprite:    event.Sprite,
		TaskID:    event.TaskID,
		State:     TaskStateUnknown,
		StartedAt: &event.Timestamp,
	}

	// Carry forward repo, branch, issue from dispatch_started or earlier events
	snapshot.Repo = event.Repo
	snapshot.Branch = event.Branch
	snapshot.Issue = event.Issue

	// Derive state from event kind
	switch event.Kind {
	case EventDispatchStarted:
		snapshot.State = TaskStatePending
	case EventRepoSetupStarted:
		snapshot.State = TaskStateSettingUp
	case EventAgentStarted:
		snapshot.State = TaskStateRunning
	case EventHeartbeat:
		snapshot.State = TaskStateRunning
		if event.Commits == 0 {
			// No commits might indicate stalled
			snapshot.State = TaskStateRunning
		}
	case EventBlocked:
		snapshot.State = TaskStateBlocked
		snapshot.BlockedReason = event.Reason
	case EventCompleted:
		snapshot.State = TaskStateCompleted
		snapshot.CompletedAt = &event.Timestamp
	case EventFailed:
		snapshot.State = TaskStateFailed
		snapshot.Error = event.Reason
	default:
		snapshot.State = TaskStateUnknown
	}

	snapshot.LastSeenAt = &event.Timestamp
	snapshot.FreshnessAge = now.Sub(event.Timestamp)

	// Check staleness
	if staleThreshold > 0 && snapshot.FreshnessAge >= staleThreshold {
		snapshot.State = TaskStateStale
	}

	snapshot.ProbeStatus = ProbeStatusUnknown

	return snapshot
}

const dailyLayout = "2006-01-02"

func dailyFilename(ts time.Time) string {
	return ts.UTC().Format(dailyLayout) + ".jsonl"
}

func listDailyPaths(entries []fs.DirEntry, dir string, since, until time.Time) []string {
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		dayRaw := strings.TrimSuffix(name, ".jsonl")
		day, err := time.Parse(dailyLayout, dayRaw)
		if err != nil {
			continue
		}
		day = day.UTC()
		if !since.IsZero() && day.Before(truncateUTCDate(since)) {
			continue
		}
		if !until.IsZero() && day.After(truncateUTCDate(until)) {
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
	}
	sort.Strings(paths)
	return paths
}

func truncateUTCDate(ts time.Time) time.Time {
	if ts.IsZero() {
		return time.Time{}
	}
	t := ts.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// InMemoryStore is a thread-safe in-memory store for testing.
type InMemoryStore struct {
	mu      sync.RWMutex
	events  []TaskEvent
	now     func() time.Time
	snapshots map[string]TaskSnapshot
}

// NewInMemoryStore creates an in-memory store for testing.
func NewInMemoryStore(now func() time.Time) *InMemoryStore {
	if now == nil {
		now = time.Now
	}
	return &InMemoryStore{
		events:   make([]TaskEvent, 0),
		now:      now,
		snapshots: make(map[string]TaskSnapshot),
	}
}

// Append adds an event to the store.
func (m *InMemoryStore) Append(event TaskEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if event.ID == "" {
		return errors.New("ledger: event ID is required")
	}
	if event.Sprite == "" {
		return errors.New("ledger: sprite name is required")
	}
	if event.TaskID == "" {
		return errors.New("ledger: task ID is required")
	}
	if event.Kind == "" {
		return errors.New("ledger: event kind is required")
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = m.now().UTC()
	}

	m.events = append(m.events, event)

	// Update snapshot
	key := event.Sprite + "/" + event.TaskID
	now := m.now().UTC()
	m.snapshots[key] = materializer(event, now, 0)

	return nil
}

// Query retrieves events matching the options.
func (m *InMemoryStore) Query(opts QueryOptions) ([]TaskEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]TaskEvent, 0)
	for _, event := range m.events {
		if !opts.Since.IsZero() && event.Timestamp.Before(opts.Since) {
			continue
		}
		if !opts.Until.IsZero() && event.Timestamp.After(opts.Until) {
			continue
		}
		if opts.Sprite != "" && event.Sprite != opts.Sprite {
			continue
		}
		if opts.TaskID != "" && event.TaskID != opts.TaskID {
			continue
		}
		if opts.Kind != "" && event.Kind != opts.Kind {
			continue
		}
		out = append(out, event)
	}

	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[len(out)-opts.Limit:]
	}

	return out, nil
}

// LatestEvents returns the most recent event per (sprite, taskID).
func (m *InMemoryStore) LatestEvents() (map[string]TaskEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	latest := make(map[string]TaskEvent)
	for _, event := range m.events {
		key := event.Sprite + "/" + event.TaskID
		if existing, ok := latest[key]; !ok || event.Timestamp.After(existing.Timestamp) {
			latest[key] = event
		}
	}
	return latest, nil
}

// Snapshot returns materialized snapshots for all known tasks.
func (m *InMemoryStore) Snapshot(staleThreshold time.Duration) (map[string]TaskSnapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Group events by (sprite, taskID)
	byKey := make(map[string][]TaskEvent)
	for _, event := range m.events {
		key := event.Sprite + "/" + event.TaskID
		byKey[key] = append(byKey[key], event)
	}

	now := m.now().UTC()
	snapshots := make(map[string]TaskSnapshot)

	for key, evts := range byKey {
		if len(evts) == 0 {
			continue
		}
		// Sort by timestamp
		sort.Slice(evts, func(i, j int) bool {
			return evts[i].Timestamp.Before(evts[j].Timestamp)
		})

		// Get the first event for repo/branch/issue
		first := evts[0]
		// Get the last event for state
		last := evts[len(evts)-1]

		snapshot := materializer(last, now, staleThreshold)
		// Carry forward from first event
		snapshot.Repo = first.Repo
		snapshot.Branch = first.Branch
		snapshot.Issue = first.Issue
		if first.Timestamp.After(time.Time{}) {
			snapshot.StartedAt = &first.Timestamp
		}
		snapshots[key] = snapshot
	}

	return snapshots, nil
}

// SnapshotForSprite returns snapshots for a specific sprite.
func (m *InMemoryStore) SnapshotForSprite(sprite string, staleThreshold time.Duration) (map[string]TaskSnapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	events, err := m.queryLocked(QueryOptions{Sprite: sprite, Limit: 1000})
	if err != nil {
		return nil, err
	}

	// Get latest event per task
	latestByTask := make(map[string]TaskEvent)
	for _, event := range events {
		if existing, ok := latestByTask[event.TaskID]; !ok || event.Timestamp.After(existing.Timestamp) {
			latestByTask[event.TaskID] = event
		}
	}

	now := m.now().UTC()
	snapshots := make(map[string]TaskSnapshot)
	for taskID, event := range latestByTask {
		snapshot := materializer(event, now, staleThreshold)
		snapshots[taskID] = snapshot
	}

	return snapshots, nil
}

func (m *InMemoryStore) queryLocked(opts QueryOptions) ([]TaskEvent, error) {
	out := make([]TaskEvent, 0)
	for _, event := range m.events {
		if !opts.Since.IsZero() && event.Timestamp.Before(opts.Since) {
			continue
		}
		if !opts.Until.IsZero() && event.Timestamp.After(opts.Until) {
			continue
		}
		if opts.Sprite != "" && event.Sprite != opts.Sprite {
			continue
		}
		if opts.TaskID != "" && event.TaskID != opts.TaskID {
			continue
		}
		if opts.Kind != "" && event.Kind != opts.Kind {
			continue
		}
		out = append(out, event)
	}

	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[len(out)-opts.Limit:]
	}

	return out, nil
}

// UpdateProbeStatus updates the probe status for a task.
func (m *InMemoryStore) UpdateProbeStatus(sprite, taskID string, status ProbeStatus, staleThreshold time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	events, err := m.queryLocked(QueryOptions{Sprite: sprite, TaskID: taskID, Limit: 1})
	if err != nil {
		return err
	}

	if len(events) == 0 {
		return errors.New("ledger: no events found for task")
	}

	latest := events[len(events)-1]
	now := m.now().UTC()
	snapshot := materializer(latest, now, staleThreshold)

	// Carry forward from first event
	allEvents, err := m.queryLocked(QueryOptions{Sprite: sprite, TaskID: taskID, Limit: 1000})
	if err == nil && len(allEvents) > 0 {
		first := allEvents[0]
		snapshot.Repo = first.Repo
		snapshot.Branch = first.Branch
		snapshot.Issue = first.Issue
		if first.Timestamp.After(time.Time{}) {
			snapshot.StartedAt = &first.Timestamp
		}
	}

	snapshot.ProbeStatus = status

	// Update snapshot in map
	key := sprite + "/" + taskID
	m.snapshots[key] = snapshot

	return nil
}

// Events returns all events (for testing).
func (m *InMemoryStore) Events() []TaskEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]TaskEvent, len(m.events))
	copy(out, m.events)
	return out
}
