package ledger

import (
	"testing"
	"time"
)

func TestMaterializerDispatchStarted(t *testing.T) {
	now := time.Now().UTC()
	event := TaskEvent{
		ID:        "test-1",
		Sprite:    "spr-001",
		TaskID:    "task-123",
		Kind:      EventDispatchStarted,
		Timestamp: now,
		Repo:      "misty-step/test",
		Branch:    "feature/test",
		Issue:     42,
	}

	snapshot := materializer(event, now, 0)

	if snapshot.State != TaskStatePending {
		t.Errorf("expected state pending, got %s", snapshot.State)
	}
	if snapshot.Sprite != "spr-001" {
		t.Errorf("expected sprite spr-001, got %s", snapshot.Sprite)
	}
	if snapshot.TaskID != "task-123" {
		t.Errorf("expected taskID task-123, got %s", snapshot.TaskID)
	}
	if snapshot.Repo != "misty-step/test" {
		t.Errorf("expected repo misty-step/test, got %s", snapshot.Repo)
	}
	if snapshot.Branch != "feature/test" {
		t.Errorf("expected branch feature/test, got %s", snapshot.Branch)
	}
	if snapshot.Issue != 42 {
		t.Errorf("expected issue 42, got %d", snapshot.Issue)
	}
	if snapshot.FreshnessAge != 0 {
		t.Errorf("expected freshness 0, got %s", snapshot.FreshnessAge)
	}
}

func TestMaterializerAgentStarted(t *testing.T) {
	now := time.Now().UTC()
	event := TaskEvent{
		ID:        "test-2",
		Sprite:    "spr-002",
		TaskID:    "task-456",
		Kind:      EventAgentStarted,
		Timestamp: now,
	}

	snapshot := materializer(event, now, 0)

	if snapshot.State != TaskStateRunning {
		t.Errorf("expected state running, got %s", snapshot.State)
	}
}

func TestMaterializerHeartbeat(t *testing.T) {
	now := time.Now().UTC()
	event := TaskEvent{
		ID:        "test-3",
		Sprite:    "spr-003",
		TaskID:    "task-789",
		Kind:      EventHeartbeat,
		Timestamp: now,
		Commits:   5,
	}

	snapshot := materializer(event, now, 0)

	if snapshot.State != TaskStateRunning {
		t.Errorf("expected state running, got %s", snapshot.State)
	}
}

func TestMaterializerBlocked(t *testing.T) {
	now := time.Now().UTC()
	event := TaskEvent{
		ID:        "test-4",
		Sprite:    "spr-004",
		TaskID:    "task-111",
		Kind:      EventBlocked,
		Timestamp: now,
		Reason:    "Waiting for code review",
	}

	snapshot := materializer(event, now, 0)

	if snapshot.State != TaskStateBlocked {
		t.Errorf("expected state blocked, got %s", snapshot.State)
	}
	if snapshot.BlockedReason != "Waiting for code review" {
		t.Errorf("expected blocked reason 'Waiting for code review', got %s", snapshot.BlockedReason)
	}
}

func TestMaterializerCompleted(t *testing.T) {
	now := time.Now().UTC()
	event := TaskEvent{
		ID:        "test-5",
		Sprite:    "spr-005",
		TaskID:    "task-222",
		Kind:      EventCompleted,
		Timestamp: now,
	}

	snapshot := materializer(event, now, 0)

	if snapshot.State != TaskStateCompleted {
		t.Errorf("expected state completed, got %s", snapshot.State)
	}
	if snapshot.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}
}

func TestMaterializerFailed(t *testing.T) {
	now := time.Now().UTC()
	event := TaskEvent{
		ID:        "test-6",
		Sprite:    "spr-006",
		TaskID:    "task-333",
		Kind:      EventFailed,
		Timestamp: now,
		Reason:    "Build failed",
	}

	snapshot := materializer(event, now, 0)

	if snapshot.State != TaskStateFailed {
		t.Errorf("expected state failed, got %s", snapshot.State)
	}
	if snapshot.Error != "Build failed" {
		t.Errorf("expected error 'Build failed', got %s", snapshot.Error)
	}
}

func TestMaterializerStaleThreshold(t *testing.T) {
	now := time.Now().UTC()
	oldTime := now.Add(-3 * time.Hour)
	event := TaskEvent{
		ID:        "test-7",
		Sprite:    "spr-007",
		TaskID:    "task-444",
		Kind:      EventAgentStarted,
		Timestamp: oldTime,
	}

	// With 2 hour threshold, this should be stale
	snapshot := materializer(event, now, 2*time.Hour)

	if snapshot.State != TaskStateStale {
		t.Errorf("expected state stale, got %s", snapshot.State)
	}
	if snapshot.FreshnessAge < 2*time.Hour {
		t.Errorf("expected freshness >= 2h, got %s", snapshot.FreshnessAge)
	}
}

func TestMaterializerNotStale(t *testing.T) {
	now := time.Now().UTC()
	recentTime := now.Add(-30 * time.Minute)
	event := TaskEvent{
		ID:        "test-8",
		Sprite:    "spr-008",
		TaskID:    "task-555",
		Kind:      EventAgentStarted,
		Timestamp: recentTime,
	}

	// With 2 hour threshold, this should NOT be stale
	snapshot := materializer(event, now, 2*time.Hour)

	if snapshot.State != TaskStateRunning {
		t.Errorf("expected state running, got %s", snapshot.State)
	}
}

func TestInMemoryStoreAppend(t *testing.T) {
	store := NewInMemoryStore(nil)
	now := time.Now().UTC()

	event := TaskEvent{
		ID:        "test-1",
		Sprite:    "spr-001",
		TaskID:    "task-123",
		Kind:      EventDispatchStarted,
		Timestamp: now,
		Repo:      "test/repo",
	}

	if err := store.Append(event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	events, err := store.Query(QueryOptions{})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ID != "test-1" {
		t.Errorf("expected event ID test-1, got %s", events[0].ID)
	}
}

func TestInMemoryStoreAppendDuplicate(t *testing.T) {
	store := NewInMemoryStore(nil)
	now := time.Now().UTC()

	event := TaskEvent{
		ID:        "test-1",
		Sprite:    "spr-001",
		TaskID:    "task-123",
		Kind:      EventDispatchStarted,
		Timestamp: now,
	}

	if err := store.Append(event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	// Append duplicate (different ID) for same task
	event2 := TaskEvent{
		ID:        "test-2",
		Sprite:    "spr-001",
		TaskID:    "task-123",
		Kind:      EventAgentStarted,
		Timestamp: now.Add(time.Minute),
	}

	if err := store.Append(event2); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	events, err := store.Query(QueryOptions{})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}

func TestInMemoryStoreLatestEvents(t *testing.T) {
	store := NewInMemoryStore(nil)
	now := time.Now().UTC()

	// Add multiple events for different tasks
	events := []TaskEvent{
		{
			ID:        "1",
			Sprite:    "spr-001",
			TaskID:    "task-A",
			Kind:      EventDispatchStarted,
			Timestamp: now,
		},
		{
			ID:        "2",
			Sprite:    "spr-001",
			TaskID:    "task-B",
			Kind:      EventDispatchStarted,
			Timestamp: now.Add(time.Minute),
		},
		{
			ID:        "3",
			Sprite:    "spr-001",
			TaskID:    "task-A",
			Kind:      EventAgentStarted,
			Timestamp: now.Add(2 * time.Minute),
		},
	}

	for _, e := range events {
		if err := store.Append(e); err != nil {
			t.Fatalf("Append() error = %v", err)
		}
	}

	latest, err := store.LatestEvents()
	if err != nil {
		t.Fatalf("LatestEvents() error = %v", err)
	}

	if len(latest) != 2 {
		t.Fatalf("expected 2 latest events, got %d", len(latest))
	}

	// task-A should have agent_started (latest)
	if latest["spr-001/task-A"].Kind != EventAgentStarted {
		t.Errorf("expected task-A latest to be agent_started, got %s", latest["spr-001/task-A"].Kind)
	}

	// task-B should have dispatch_started
	if latest["spr-001/task-B"].Kind != EventDispatchStarted {
		t.Errorf("expected task-B latest to be dispatch_started, got %s", latest["spr-001/task-B"].Kind)
	}
}

func TestInMemoryStoreSnapshot(t *testing.T) {
	store := NewInMemoryStore(func() time.Time {
		return time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	})
	now := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	events := []TaskEvent{
		{
			ID:        "1",
			Sprite:    "spr-001",
			TaskID:    "task-123",
			Kind:      EventDispatchStarted,
			Timestamp: now.Add(-30 * time.Minute),
			Repo:      "test/repo",
			Branch:    "main",
			Issue:     42,
		},
		{
			ID:        "2",
			Sprite:    "spr-001",
			TaskID:    "task-123",
			Kind:      EventAgentStarted,
			Timestamp: now.Add(-20 * time.Minute),
		},
		{
			ID:        "3",
			Sprite:    "spr-001",
			TaskID:    "task-123",
			Kind:      EventHeartbeat,
			Timestamp: now.Add(-5 * time.Minute),
			Commits:   10,
		},
	}

	for _, e := range events {
		if err := store.Append(e); err != nil {
			t.Fatalf("Append() error = %v", err)
		}
	}

	snapshots, err := store.Snapshot(2 * time.Hour)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}

	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}

	snap := snapshots["spr-001/task-123"]
	if snap.State != TaskStateRunning {
		t.Errorf("expected state running, got %s", snap.State)
	}
	if snap.Repo != "test/repo" {
		t.Errorf("expected repo test/repo, got %s", snap.Repo)
	}
	if snap.Issue != 42 {
		t.Errorf("expected issue 42, got %d", snap.Issue)
	}
}

func TestInMemoryStoreSnapshotStale(t *testing.T) {
	store := NewInMemoryStore(func() time.Time {
		return time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	})
	now := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	event := TaskEvent{
		ID:        "1",
		Sprite:    "spr-001",
		TaskID:    "task-123",
		Kind:      EventAgentStarted,
		Timestamp: now.Add(-3 * time.Hour), // 3 hours ago
	}

	if err := store.Append(event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	snapshots, err := store.Snapshot(2 * time.Hour) // 2 hour threshold
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}

	snap := snapshots["spr-001/task-123"]
	if snap.State != TaskStateStale {
		t.Errorf("expected state stale, got %s", snap.State)
	}
}

func TestInMemoryStoreSnapshotForSprite(t *testing.T) {
	store := NewInMemoryStore(nil)
	now := time.Now().UTC()

	events := []TaskEvent{
		{
			ID:        "1",
			Sprite:    "spr-001",
			TaskID:    "task-A",
			Kind:      EventDispatchStarted,
			Timestamp: now,
		},
		{
			ID:        "2",
			Sprite:    "spr-002",
			TaskID:    "task-B",
			Kind:      EventDispatchStarted,
			Timestamp: now,
		},
	}

	for _, e := range events {
		if err := store.Append(e); err != nil {
			t.Fatalf("Append() error = %v", err)
		}
	}

	snapshots, err := store.SnapshotForSprite("spr-001", 0)
	if err != nil {
		t.Fatalf("SnapshotForSprite() error = %v", err)
	}

	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}

	if _, ok := snapshots["task-A"]; !ok {
		t.Error("expected task-A snapshot")
	}
}

func TestInMemoryStoreQueryFilter(t *testing.T) {
	store := NewInMemoryStore(nil)
	now := time.Now().UTC()

	events := []TaskEvent{
		{
			ID:        "1",
			Sprite:    "spr-001",
			TaskID:    "task-A",
			Kind:      EventDispatchStarted,
			Timestamp: now,
		},
		{
			ID:        "2",
			Sprite:    "spr-001",
			TaskID:    "task-A",
			Kind:      EventAgentStarted,
			Timestamp: now,
		},
		{
			ID:        "3",
			Sprite:    "spr-002",
			TaskID:    "task-B",
			Kind:      EventDispatchStarted,
			Timestamp: now,
		},
	}

	for _, e := range events {
		if err := store.Append(e); err != nil {
			t.Fatalf("Append() error = %v", err)
		}
	}

	// Filter by sprite
	result, err := store.Query(QueryOptions{Sprite: "spr-001"})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 events for spr-001, got %d", len(result))
	}

	// Filter by task
	result, err = store.Query(QueryOptions{TaskID: "task-A"})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 events for task-A, got %d", len(result))
	}

	// Filter by kind
	result, err = store.Query(QueryOptions{Kind: EventAgentStarted})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 event for agent_started, got %d", len(result))
	}
}

func TestInMemoryStoreUpdateProbeStatus(t *testing.T) {
	store := NewInMemoryStore(nil)
	now := time.Now().UTC()

	event := TaskEvent{
		ID:        "1",
		Sprite:    "spr-001",
		TaskID:    "task-123",
		Kind:      EventAgentStarted,
		Timestamp: now,
	}

	if err := store.Append(event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	// Update probe status
	if err := store.UpdateProbeStatus("spr-001", "task-123", ProbeStatusSuccess, 0); err != nil {
		t.Fatalf("UpdateProbeStatus() error = %v", err)
	}

	// Verify probe status is stored in the internal snapshots map
	store.mu.RLock()
	snap, ok := store.snapshots["spr-001/task-123"]
	store.mu.RUnlock()

	if !ok {
		t.Fatal("expected snapshot to exist")
	}
	if snap.ProbeStatus != ProbeStatusSuccess {
		t.Errorf("expected probe_status success, got %s", snap.ProbeStatus)
	}
}

func TestInMemoryStoreAppendInvalid(t *testing.T) {
	store := NewInMemoryStore(nil)

	tests := []struct {
		name  string
		event TaskEvent
	}{
		{
			name:  "missing ID",
			event: TaskEvent{Sprite: "spr-001", TaskID: "task-1", Kind: EventDispatchStarted},
		},
		{
			name:  "missing Sprite",
			event: TaskEvent{ID: "1", TaskID: "task-1", Kind: EventDispatchStarted},
		},
		{
			name:  "missing TaskID",
			event: TaskEvent{ID: "1", Sprite: "spr-001", Kind: EventDispatchStarted},
		},
		{
			name:  "missing Kind",
			event: TaskEvent{ID: "1", Sprite: "spr-001", TaskID: "task-1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := store.Append(tc.event)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}
