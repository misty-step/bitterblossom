package lifecycle

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/internal/sprite"
)

func TestFleetOverviewCompositionAndOrphans(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, "bramble")
	compositionPath := filepath.Join(fx.rootDir, "compositions", "v1.yaml")
	writeFixtureFile(t, compositionPath, `version: 1
name: "test"
sprites:
  bramble:
    definition: sprites/bramble.md
`)

	cli := &sprite.MockSpriteCLI{
		APIFn: func(context.Context, string, string) (string, error) {
			return `{"sprites":[{"name":"bramble","status":"running","url":"https://bramble"},{"name":"thorn","status":"stopped","url":"https://thorn"}]}`, nil
		},
		CheckpointListFn: func(_ context.Context, name, _ string) (string, error) {
			if name == "bramble" {
				return "checkpoint-a", nil
			}
			return "", nil
		},
	}

	status, err := FleetOverview(context.Background(), cli, fx.cfg, compositionPath, FleetOverviewOpts{
		IncludeCheckpoints: true,
		IncludeTasks:       false,
	})
	if err != nil {
		t.Fatalf("FleetOverview() error = %v", err)
	}
	if len(status.Sprites) != 2 {
		t.Fatalf("len(Sprites) = %d, want 2", len(status.Sprites))
	}
	if len(status.Composition) != 1 || !status.Composition[0].Provisioned {
		t.Fatalf("unexpected composition entries: %#v", status.Composition)
	}
	if len(status.Orphans) != 1 || status.Orphans[0].Name != "thorn" {
		t.Fatalf("unexpected orphans: %#v", status.Orphans)
	}
	if status.Checkpoints["bramble"] != "checkpoint-a" {
		t.Fatalf("checkpoint for bramble = %q", status.Checkpoints["bramble"])
	}
	if status.Checkpoints["thorn"] != "(none)" {
		t.Fatalf("checkpoint for thorn = %q", status.Checkpoints["thorn"])
	}
	if !status.CheckpointsIncluded {
		t.Fatalf("CheckpointsIncluded = false, want true")
	}

	// Check summary
	if status.Summary.Total != 2 {
		t.Fatalf("summary.total = %d, want 2", status.Summary.Total)
	}
	if status.Summary.Orphaned != 1 {
		t.Fatalf("summary.orphaned = %d, want 1", status.Summary.Orphaned)
	}
}

func TestFleetOverviewSkipsCheckpointsByDefault(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, "bramble")
	compositionPath := filepath.Join(fx.rootDir, "compositions", "v1.yaml")
	writeFixtureFile(t, compositionPath, `version: 1
name: "test"
sprites:
  bramble:
    definition: sprites/bramble.md
`)

	var checkpointCalls int
	cli := &sprite.MockSpriteCLI{
		APIFn: func(context.Context, string, string) (string, error) {
			return `{"sprites":[{"name":"bramble","status":"running","url":"https://bramble"}]}`, nil
		},
		CheckpointListFn: func(_ context.Context, _ string, _ string) (string, error) {
			checkpointCalls++
			return "checkpoint-a", nil
		},
	}

	status, err := FleetOverview(context.Background(), cli, fx.cfg, compositionPath, FleetOverviewOpts{})
	if err != nil {
		t.Fatalf("FleetOverview() error = %v", err)
	}
	if checkpointCalls != 0 {
		t.Fatalf("checkpoint calls = %d, want 0", checkpointCalls)
	}
	if status.CheckpointsIncluded {
		t.Fatalf("CheckpointsIncluded = true, want false")
	}
	if len(status.Checkpoints) != 0 {
		t.Fatalf("len(Checkpoints) = %d, want 0", len(status.Checkpoints))
	}
}

func TestFleetOverviewWithTasks(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, "bramble")
	compositionPath := filepath.Join(fx.rootDir, "compositions", "v1.yaml")
	writeFixtureFile(t, compositionPath, `version: 1
name: "test"
sprites:
  bramble:
    definition: sprites/bramble.md
`)
	cli := &sprite.MockSpriteCLI{
		APIFn: func(ctx context.Context, org, endpoint string) (string, error) {
			if endpoint == "/sprites" {
				return `{"sprites":[{"name":"bramble","status":"running","url":"https://bramble"}]}`, nil
			}
			return "", nil
		},
		APISpriteFn: func(ctx context.Context, org, spriteName, endpoint string) (string, error) {
			return `{"name":"bramble","status":"running","state":"working","uptime":"2h30m","queue_depth":0,"current_task":{"id":"task-123","description":"Implement feature X","repo":"misty-step/bitterblossom","branch":"main","started_at":"2026-02-10T14:00:00Z"},"persona":{"name":"bramble"}}`, nil
		},
	}

	status, err := FleetOverview(context.Background(), cli, fx.cfg, compositionPath, FleetOverviewOpts{
		IncludeTasks: true,
	})
	if err != nil {
		t.Fatalf("FleetOverview() error = %v", err)
	}

	if len(status.Sprites) != 1 {
		t.Fatalf("len(Sprites) = %d, want 1", len(status.Sprites))
	}

	sprite := status.Sprites[0]
	if sprite.State != StateBusy {
		t.Fatalf("sprite state = %q, want busy", sprite.State)
	}
	if sprite.CurrentTask == nil {
		t.Fatalf("expected current task, got nil")
	}
	if sprite.CurrentTask.ID != "task-123" {
		t.Fatalf("task ID = %q, want task-123", sprite.CurrentTask.ID)
	}
	if sprite.CurrentTask.Description != "Implement feature X" {
		t.Fatalf("task description = %q", sprite.CurrentTask.Description)
	}
	if sprite.Uptime != "2h30m" {
		t.Fatalf("uptime = %q, want 2h30m", sprite.Uptime)
	}

	// Check summary reflects busy state
	if status.Summary.Busy != 1 {
		t.Fatalf("summary.busy = %d, want 1", status.Summary.Busy)
	}
	if status.Summary.WithTasks != 1 {
		t.Fatalf("summary.with_tasks = %d, want 1", status.Summary.WithTasks)
	}
}

func TestSpriteDetail(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, "bramble")
	startedAt := time.Date(2026, 2, 10, 14, 0, 0, 0, time.UTC)
	cli := &sprite.MockSpriteCLI{
		APISpriteFn: func(context.Context, string, string, string) (string, error) {
			return `{"name":"bramble","status":"running","state":"working","uptime":"3h45m","queue_depth":1,"current_task":{"id":"task-456","description":"Code review","started_at":"2026-02-10T14:00:00Z"}}`, nil
		},
		ExecFn: func(_ context.Context, _ string, command string, _ []byte) (string, error) {
			if strings.Contains(command, "ls -la") && strings.Contains(command, "workspace") {
				return "workspace listing", nil
			}
			if strings.Contains(command, "head -20") && strings.Contains(command, "MEMORY.md") {
				return "memory lines", nil
			}
			return "", nil
		},
		CheckpointListFn: func(context.Context, string, string) (string, error) {
			return "checkpoint-1", nil
		},
	}

	result, err := SpriteDetail(context.Background(), cli, fx.cfg, "bramble")
	if err != nil {
		t.Fatalf("SpriteDetail() error = %v", err)
	}
	if result.Name != "bramble" {
		t.Fatalf("name = %q, want bramble", result.Name)
	}
	if result.Workspace != "workspace listing" {
		t.Fatalf("workspace = %q", result.Workspace)
	}
	if result.Memory != "memory lines" {
		t.Fatalf("memory = %q", result.Memory)
	}
	if result.Checkpoints != "checkpoint-1" {
		t.Fatalf("checkpoints = %q", result.Checkpoints)
	}
	if result.State != StateBusy {
		t.Fatalf("state = %q, want busy", result.State)
	}
	if result.QueueDepth != 1 {
		t.Fatalf("queue_depth = %d, want 1", result.QueueDepth)
	}
	if result.Uptime != "3h45m" {
		t.Fatalf("uptime = %q", result.Uptime)
	}
	if result.CurrentTask == nil {
		t.Fatalf("expected current task")
	}
	if result.CurrentTask.ID != "task-456" {
		t.Fatalf("task ID = %q", result.CurrentTask.ID)
	}
	if !result.CurrentTask.StartedAt.Equal(startedAt) {
		t.Fatalf("task started_at = %v", result.CurrentTask.StartedAt)
	}
}

func TestFleetOverviewStaleDetectionWithoutTasks(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, "bramble")
	compositionPath := filepath.Join(fx.rootDir, "compositions", "v1.yaml")
	writeFixtureFile(t, compositionPath, `version: 1
name: "test"
sprites:
  bramble:
    definition: sprites/bramble.md
`)

	// last_activity is 3 hours ago — beyond the 1h threshold
	staleTime := time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC3339)
	var detailCalls int
	cli := &sprite.MockSpriteCLI{
		APIFn: func(ctx context.Context, org, endpoint string) (string, error) {
			return `{"sprites":[{"name":"bramble","status":"running","url":"https://bramble"}]}`, nil
		},
		APISpriteFn: func(ctx context.Context, org, spriteName, endpoint string) (string, error) {
			detailCalls++
			return `{"name":"bramble","status":"running","state":"idle","uptime":"5h","queue_depth":0,"last_activity":"` + staleTime + `","current_task":{"id":"task-99","description":"old task"}}`, nil
		},
	}

	status, err := FleetOverview(context.Background(), cli, fx.cfg, compositionPath, FleetOverviewOpts{
		IncludeTasks:   false,
		StaleThreshold: 1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("FleetOverview() error = %v", err)
	}

	if detailCalls == 0 {
		t.Fatal("expected detail fetch for stale detection, got 0 calls")
	}

	s := status.Sprites[0]
	if !s.Stale {
		t.Fatal("sprite should be flagged stale")
	}
	if s.CurrentTask != nil {
		t.Fatal("CurrentTask should be nil when IncludeTasks is false")
	}
	if s.LastActivity == nil {
		t.Fatal("LastActivity should be populated for stale detection")
	}
	if status.Summary.Stale != 1 {
		t.Fatalf("summary.stale = %d, want 1", status.Summary.Stale)
	}
}

func TestDeriveSpriteState(t *testing.T) {
	tests := []struct {
		state    string
		status   string
		expected SpriteState
	}{
		// Status-based derivation
		{"", "stopped", StateOffline},
		{"", "error", StateOffline},
		{"", "dead", StateOffline},
		{"", "running", StateIdle},
		{"", "starting", StateOperational},
		{"", "provisioning", StateOperational},
		{"", "unknown", StateUnknown},

		// State-based derivation (takes precedence)
		{"idle", "running", StateIdle},
		{"working", "running", StateBusy},
		{"dead", "running", StateOffline},

		// Case insensitivity
		{"IDLE", "RUNNING", StateIdle},
		{"Working", "Running", StateBusy},
	}

	for _, tt := range tests {
		t.Run(tt.status+"_"+tt.state, func(t *testing.T) {
			result := deriveSpriteState(tt.state, tt.status)
			if result != tt.expected {
				t.Errorf("deriveSpriteState(%q, %q) = %q, want %q", tt.state, tt.status, result, tt.expected)
			}
		})
	}
}

func TestCalculateFleetSummary(t *testing.T) {
	tests := []struct {
		name     string
		sprites  []SpriteStatus
		orphans  []SpriteStatus
		expected FleetSummary
	}{
		{
			name: "mixed fleet",
			sprites: []SpriteStatus{
				{Name: "s1", State: StateIdle},
				{Name: "s2", State: StateIdle},
				{Name: "s3", State: StateBusy, CurrentTask: &TaskInfo{ID: "t1"}},
				{Name: "s4", State: StateOffline},
				{Name: "s5", State: StateUnknown},
			},
			orphans: []SpriteStatus{{Name: "orphan", State: StateIdle}},
			expected: FleetSummary{
				Total:     5,
				Idle:      2,
				Busy:      1,
				Offline:   1,
				Unknown:   1,
				Orphaned:  1,
				WithTasks: 1,
			},
		},
		{
			name: "all busy with tasks",
			sprites: []SpriteStatus{
				{Name: "s1", State: StateBusy, CurrentTask: &TaskInfo{ID: "t1"}},
				{Name: "s2", State: StateBusy, CurrentTask: &TaskInfo{ID: "t2"}},
			},
			orphans: nil,
			expected: FleetSummary{
				Total:     2,
				Busy:      2,
				WithTasks: 2,
			},
		},
		{
			name: "stale sprites counted",
			sprites: []SpriteStatus{
				{Name: "s1", State: StateIdle, Stale: true},
				{Name: "s2", State: StateBusy, Stale: true},
				{Name: "s3", State: StateBusy},
			},
			orphans: nil,
			expected: FleetSummary{
				Total: 3,
				Idle:  1,
				Busy:  2,
				Stale: 2,
			},
		},
		{
			name:     "empty fleet",
			sprites:  []SpriteStatus{},
			orphans:  nil,
			expected: FleetSummary{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateFleetSummary(tt.sprites, tt.orphans)
			if result != tt.expected {
				t.Errorf("calculateFleetSummary() = %+v, want %+v", result, tt.expected)
			}
		})
	}
}

func TestIsRunningStatus(t *testing.T) {
	tests := []struct {
		status   string
		expected bool
	}{
		{"running", true},
		{"starting", true},
		{"provisioning", true},
		{"RUNNING", true},
		{"stopped", false},
		{"error", false},
		{"dead", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := isRunningStatus(tt.status)
			if result != tt.expected {
				t.Errorf("isRunningStatus(%q) = %v, want %v", tt.status, result, tt.expected)
			}
		})
	}
}
