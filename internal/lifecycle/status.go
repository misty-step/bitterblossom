package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/misty-step/bitterblossom/internal/fleet"
	"github.com/misty-step/bitterblossom/internal/shellutil"
	"github.com/misty-step/bitterblossom/internal/sprite"
)

// SpriteState represents the operational state of a sprite for display purposes.
type SpriteState string

const (
	StateOperational SpriteState = "operational" // Sprite is running and responsive
	StateIdle        SpriteState = "idle"        // Sprite is running but idle
	StateBusy        SpriteState = "busy"        // Sprite is running and working
	StateOffline     SpriteState = "offline"     // Sprite is not running or unreachable
	StateUnknown     SpriteState = "unknown"     // Sprite state is unknown
)

// TaskInfo represents information about a task assigned to a sprite.
type TaskInfo struct {
	ID          string            `json:"id,omitempty"`
	Description string            `json:"description"`
	Repo        string            `json:"repo,omitempty"`
	Branch      string            `json:"branch,omitempty"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// SpriteStatus describes one live sprite from the Sprite API with enhanced information.
type SpriteStatus struct {
	Name         string            `json:"name"`
	Status       string            `json:"status"` // Raw status from API (running, stopped, etc.)
	State        SpriteState       `json:"state"`  // Derived state (idle, busy, offline)
	Stale        bool              `json:"stale,omitempty"`
	Probed       bool              `json:"probed,omitempty"`    // Whether connectivity was probed
	Reachable    bool              `json:"reachable,omitempty"` // Verified via exec probe
	URL          string            `json:"url,omitempty"`
	Persona      string            `json:"persona,omitempty"`
	CurrentTask  *TaskInfo         `json:"current_task,omitempty"`
	QueueDepth   int               `json:"queue_depth"`
	Provisioned  bool              `json:"provisioned"`
	Uptime       string            `json:"uptime,omitempty"`
	LastActivity *time.Time        `json:"last_activity,omitempty"`
	Version      string            `json:"version,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// CompositionEntry maps composition membership to provisioning state.
type CompositionEntry struct {
	Name        string `json:"name"`
	Provisioned bool   `json:"provisioned"`
}

// FleetSummary provides aggregated statistics about the fleet.
type FleetSummary struct {
	Total      int `json:"total"`
	Idle       int `json:"idle"`
	Busy       int `json:"busy"`
	Offline    int `json:"offline"`
	Unknown    int `json:"unknown"`
	Orphaned   int `json:"orphaned"`
	Stale      int `json:"stale"`
	WithTasks  int `json:"with_tasks"`
}

// FleetStatus contains fleet and composition state with enhanced visibility.
type FleetStatus struct {
	Sprites             []SpriteStatus     `json:"sprites"`
	Composition         []CompositionEntry `json:"composition"`
	Orphans             []SpriteStatus     `json:"orphans"`
	Checkpoints         map[string]string  `json:"checkpoints"`
	CheckpointsIncluded bool               `json:"checkpoints_included"`
	Summary             FleetSummary       `json:"summary"`
}

// SpriteDetailResult captures detailed status for one sprite.
type SpriteDetailResult struct {
	Name        string            `json:"name"`
	API         map[string]any    `json:"api,omitempty"`
	Workspace   string            `json:"workspace"`
	Memory      string            `json:"memory"`
	Checkpoints string            `json:"checkpoints"`
	State       SpriteState       `json:"state"`
	CurrentTask *TaskInfo         `json:"current_task,omitempty"`
	QueueDepth  int               `json:"queue_depth"`
	Uptime      string            `json:"uptime,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type spriteAPIListResponse struct {
	Sprites []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		URL    string `json:"url"`
	} `json:"sprites"`
}

type spriteAPIDetailResponse struct {
	Name        string            `json:"name"`
	Status      string            `json:"status"`
	State       string            `json:"state"`
	Persona     map[string]string `json:"persona,omitempty"`
	CurrentTask *struct {
		ID          string            `json:"id"`
		Description string            `json:"description"`
		Repo        string            `json:"repo"`
		Branch      string            `json:"branch"`
		StartedAt   *time.Time        `json:"started_at"`
		Metadata    map[string]string `json:"metadata"`
	} `json:"current_task,omitempty"`
	QueueDepth   int               `json:"queue_depth"`
	Uptime       string            `json:"uptime"`
	LastActivity *time.Time        `json:"last_activity"`
	Version      string            `json:"version"`
	Metadata     map[string]string `json:"metadata"`
}

// DefaultStaleThreshold is the default duration after which a sprite with no
// recent activity is flagged as stale.
const DefaultStaleThreshold = 2 * time.Hour

// DefaultProbeTimeout is the default timeout for connectivity probes.
const DefaultProbeTimeout = 5 * time.Second

// FleetOverviewOpts configures expensive fleet overview features.
type FleetOverviewOpts struct {
	IncludeCheckpoints bool
	IncludeTasks       bool
	IncludeProbe       bool          // Probe connectivity via exec
	ProbeTimeout       time.Duration // Timeout for each probe exec
	StaleThreshold     time.Duration
}

// FleetOverview returns live fleet status + composition coverage + checkpoint summaries.
func FleetOverview(ctx context.Context, cli sprite.SpriteCLI, cfg Config, compositionPath string, opts FleetOverviewOpts) (FleetStatus, error) {
	if err := requireConfig(cfg); err != nil {
		return FleetStatus{}, err
	}
	composition, err := fleet.ParseComposition(compositionPath)
	if err != nil {
		return FleetStatus{}, err
	}

	live, err := fetchLiveSprites(ctx, cli, cfg, opts)
	if err != nil {
		return FleetStatus{}, err
	}

	provisioned := make(map[string]struct{}, len(live))
	for _, item := range live {
		provisioned[item.Name] = struct{}{}
	}

	entries := make([]CompositionEntry, 0, len(composition.Sprites))
	compositionNames := make([]string, 0, len(composition.Sprites))
	for _, spec := range composition.Sprites {
		compositionNames = append(compositionNames, spec.Name)
		_, ok := provisioned[spec.Name]
		entries = append(entries, CompositionEntry{Name: spec.Name, Provisioned: ok})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	sort.Strings(compositionNames)

	orphans := make([]SpriteStatus, 0, len(live))
	for _, item := range live {
		if !slices.Contains(compositionNames, item.Name) {
			orphans = append(orphans, item)
		}
	}
	sort.Slice(orphans, func(i, j int) bool {
		return orphans[i].Name < orphans[j].Name
	})

	checkpoints := make(map[string]string)
	if opts.IncludeCheckpoints {
		checkpoints = make(map[string]string, len(live))
		for _, item := range live {
			value, err := cli.CheckpointList(ctx, item.Name, cfg.Org)
			if err != nil {
				checkpoints[item.Name] = "(none)"
				continue
			}
			value = strings.TrimSpace(value)
			if value == "" {
				value = "(none)"
			}
			checkpoints[item.Name] = value
		}
	}

	summary := calculateFleetSummary(live, orphans)

	return FleetStatus{
		Sprites:             live,
		Composition:         entries,
		Orphans:             orphans,
		Checkpoints:         checkpoints,
		CheckpointsIncluded: opts.IncludeCheckpoints,
		Summary:             summary,
	}, nil
}

// SpriteDetail returns API + workspace + memory + checkpoint status for one sprite.
func SpriteDetail(ctx context.Context, cli sprite.SpriteCLI, cfg Config, name string) (SpriteDetailResult, error) {
	if err := requireConfig(cfg); err != nil {
		return SpriteDetailResult{}, err
	}
	if err := ValidateSpriteName(name); err != nil {
		return SpriteDetailResult{}, err
	}

	result := SpriteDetailResult{Name: name}

	apiRaw, err := cli.APISprite(ctx, cfg.Org, name, "/")
	if err == nil {
		var payload map[string]any
		if decodeErr := json.Unmarshal([]byte(apiRaw), &payload); decodeErr == nil {
			result.API = payload
		}

		// Parse detailed sprite info if available
		var detail spriteAPIDetailResponse
		if decodeErr := json.Unmarshal([]byte(apiRaw), &detail); decodeErr == nil {
			result.State = deriveSpriteState(detail.State, detail.Status)
			result.QueueDepth = detail.QueueDepth
			result.Uptime = detail.Uptime
			if detail.CurrentTask != nil {
				result.CurrentTask = &TaskInfo{
					ID:          detail.CurrentTask.ID,
					Description: detail.CurrentTask.Description,
					Repo:        detail.CurrentTask.Repo,
					Branch:      detail.CurrentTask.Branch,
					StartedAt:   detail.CurrentTask.StartedAt,
					Metadata:    detail.CurrentTask.Metadata,
				}
			}
			result.Metadata = detail.Metadata
		}
	}

	workspaceCommand := "ls -la " + shellutil.Quote(path.Join(cfg.Workspace, "/"))
	workspaceOutput, workspaceErr := cli.Exec(ctx, name, workspaceCommand, nil)
	if workspaceErr != nil {
		result.Workspace = "(no workspace)"
	} else {
		result.Workspace = strings.TrimSpace(workspaceOutput)
	}

	memoryCommand := "head -20 " + shellutil.Quote(path.Join(cfg.Workspace, "MEMORY.md"))
	memoryOutput, memoryErr := cli.Exec(ctx, name, memoryCommand, nil)
	if memoryErr != nil {
		result.Memory = "(no MEMORY.md)"
	} else {
		result.Memory = strings.TrimSpace(memoryOutput)
	}

	checkpoints, err := cli.CheckpointList(ctx, name, cfg.Org)
	if err != nil {
		result.Checkpoints = "(none)"
	} else {
		result.Checkpoints = strings.TrimSpace(checkpoints)
		if result.Checkpoints == "" {
			result.Checkpoints = "(none)"
		}
	}

	return result, nil
}

func fetchLiveSprites(ctx context.Context, cli sprite.SpriteCLI, cfg Config, opts FleetOverviewOpts) ([]SpriteStatus, error) {
	raw, err := cli.API(ctx, cfg.Org, "/sprites")
	if err != nil {
		return nil, err
	}

	var decoded spriteAPIListResponse
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, fmt.Errorf("decode sprite api response: %w", err)
	}

	result := make([]SpriteStatus, 0, len(decoded.Sprites))
	for _, item := range decoded.Sprites {
		if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Status) == "" {
			continue
		}

		status := SpriteStatus{
			Name:        item.Name,
			Status:      item.Status,
			URL:         item.URL,
			Provisioned: true,
		}

		// Derive display state from raw status
		status.State = deriveSpriteState("", item.Status)

		// Compute effective stale threshold once (zero/negative defaults to DefaultStaleThreshold).
		threshold := opts.StaleThreshold
		if threshold <= 0 {
			threshold = DefaultStaleThreshold
		}

		// Fetch detailed info when sprite is running and we need tasks or stale detection.
		// LastActivity (required for stale detection) is only available from the detail endpoint.
		needsDetail := opts.IncludeTasks || threshold > 0
		if needsDetail && isRunningStatus(item.Status) {
			detail, err := fetchSpriteDetail(ctx, cli, cfg.Org, item.Name)
			if err == nil {
				status.State = detail.State
				status.Persona = detail.Persona
				status.QueueDepth = detail.QueueDepth
				status.Uptime = detail.Uptime
				status.LastActivity = detail.LastActivity
				status.Version = detail.Version
				status.Metadata = detail.Metadata
				if opts.IncludeTasks {
					status.CurrentTask = detail.CurrentTask
				}
			}
		}

		// Flag stale sprites: running but no recent activity.
		if status.LastActivity != nil && isRunningStatus(item.Status) {
			if time.Since(*status.LastActivity) >= threshold {
				status.Stale = true
			}
		}

		result = append(result, status)
	}

	// Probe connectivity in parallel to avoid O(N) sequential latency.
	if opts.IncludeProbe {
		var wg sync.WaitGroup
		for i := range result {
			if !isProbeableStatus(result[i].Status) {
				continue
			}
			result[i].Probed = true
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				result[idx].Reachable = probeSpriteConnectivity(ctx, cli, result[idx].Name, opts.ProbeTimeout)
			}(i)
		}
		wg.Wait()
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func fetchSpriteDetail(ctx context.Context, cli sprite.SpriteCLI, org, name string) (SpriteStatus, error) {
	raw, err := cli.APISprite(ctx, org, name, "/")
	if err != nil {
		return SpriteStatus{Name: name}, err
	}

	var detail spriteAPIDetailResponse
	if err := json.Unmarshal([]byte(raw), &detail); err != nil {
		return SpriteStatus{Name: name}, err
	}

	status := SpriteStatus{
		Name:        detail.Name,
		Status:      detail.Status,
		State:       deriveSpriteState(detail.State, detail.Status),
		QueueDepth:  detail.QueueDepth,
		Uptime:      detail.Uptime,
		LastActivity: detail.LastActivity,
		Version:     detail.Version,
		Metadata:    detail.Metadata,
		Provisioned: true,
	}

	if detail.Persona != nil {
		status.Persona = detail.Persona["name"]
	}

	if detail.CurrentTask != nil {
		status.CurrentTask = &TaskInfo{
			ID:          detail.CurrentTask.ID,
			Description: detail.CurrentTask.Description,
			Repo:        detail.CurrentTask.Repo,
			Branch:      detail.CurrentTask.Branch,
			StartedAt:   detail.CurrentTask.StartedAt,
			Metadata:    detail.CurrentTask.Metadata,
		}
	}

	return status, nil
}

func deriveSpriteState(state, status string) SpriteState {
	status = strings.ToLower(status)
	state = strings.ToLower(state)

	// If status indicates not running, it's offline
	if status == "stopped" || status == "error" || status == "dead" {
		return StateOffline
	}

	// Check explicit state from API
	switch state {
	case "idle":
		return StateIdle
	case "working":
		return StateBusy
	case "dead":
		return StateOffline
	}

	// Derive from status
	switch status {
	case "running", "warm":
		// "warm" = API status for running+idle; "running" = generic running
		return StateIdle
	case "starting", "provisioning":
		return StateOperational
	}

	return StateUnknown
}

func isRunningStatus(status string) bool {
	s := strings.ToLower(status)
	return s == "running" || s == "warm" || s == "starting" || s == "provisioning"
}

// isProbeableStatus returns true for sprites whose transport layer is ready.
// Excludes transitional states (starting, provisioning) where exec probes
// would always timeout.
func isProbeableStatus(status string) bool {
	s := strings.ToLower(status)
	return s == "running" || s == "warm"
}

// probeSpriteConnectivity verifies a sprite is reachable via exec transport.
// Returns true if the sprite responds to a lightweight echo command.
func probeSpriteConnectivity(ctx context.Context, cli sprite.SpriteCLI, name string, timeout time.Duration) bool {
	if timeout <= 0 {
		timeout = DefaultProbeTimeout
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Use a lightweight echo command to verify transport connectivity
	_, err := cli.Exec(probeCtx, name, "echo ok", nil)
	return err == nil
}

func calculateFleetSummary(sprites []SpriteStatus, orphans []SpriteStatus) FleetSummary {
	summary := FleetSummary{
		Total:    len(sprites),
		Orphaned: len(orphans),
	}

	for _, s := range sprites {
		switch s.State {
		case StateIdle:
			summary.Idle++
		case StateBusy:
			summary.Busy++
		case StateOffline:
			summary.Offline++
		default:
			summary.Unknown++
		}

		if s.Stale {
			summary.Stale++
		}
		if s.CurrentTask != nil {
			summary.WithTasks++
		}
	}

	return summary
}
