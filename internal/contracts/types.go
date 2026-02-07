package contracts

import "time"

// SpriteState represents the lifecycle state of a sprite.
type SpriteState string

const (
	SpriteStateRunning SpriteState = "running"
	SpriteStateIdle    SpriteState = "idle"
	SpriteStateDead    SpriteState = "dead"
	SpriteStateStuck   SpriteState = "stuck"
)

// SpriteStatus reports current operational status for one sprite.
type SpriteStatus struct {
	Name       string      `json:"name"`
	State      SpriteState `json:"state"`
	ClaudePID  int         `json:"claude_pid"`
	Uptime     string      `json:"uptime"`
	LastCommit string      `json:"last_commit"`
	DiskUsage  string      `json:"disk_usage"`
}

// DispatchResult captures metadata for a dispatched task.
type DispatchResult struct {
	Sprite    string    `json:"sprite"`
	Task      string    `json:"task"`
	StartedAt time.Time `json:"started_at"`
	PID       int       `json:"pid"`
	LogPath   string    `json:"log_path"`
}

// FleetSummary aggregates sprite state counts for quick status reporting.
type FleetSummary struct {
	Running int `json:"running"`
	Idle    int `json:"idle"`
	Dead    int `json:"dead"`
}

// FleetStatus captures full-fleet status and summary counts.
type FleetStatus struct {
	Sprites []SpriteStatus `json:"sprites"`
	Summary FleetSummary   `json:"summary"`
}

// TaskComplete captures task completion details from a sprite.
type TaskComplete struct {
	Sprite   string `json:"sprite"`
	Task     string `json:"task"`
	PRURL    string `json:"pr_url"`
	Duration string `json:"duration"`
	ExitCode int    `json:"exit_code"`
}

// Error represents a structured control plane error payload.
type Error struct {
	Code      string    `json:"code"`
	Message   string    `json:"message"`
	Details   any       `json:"details"`
	Timestamp time.Time `json:"timestamp"`
}
