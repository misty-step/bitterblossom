// Package contracts defines versioned JSON/NDJSON output schemas and
// machine error semantics for the bb CLI.
package contracts

import (
	"encoding/json"
	"io"
	"time"
)

// SchemaVersion is the current contract version.
const SchemaVersion = "v1"

// Error code constants classify failure categories.
const (
	ErrorCodeValidation  = "VALIDATION_ERROR"
	ErrorCodeAuth        = "AUTH_ERROR"
	ErrorCodeNetwork     = "NETWORK_ERROR"
	ErrorCodeRemoteState = "REMOTE_STATE_ERROR"
	ErrorCodeInternal    = "INTERNAL_ERROR"
)

// Exit codes by failure class.
const (
	ExitOK          = 0
	ExitInternal    = 1
	ExitValidation  = 2
	ExitAuth        = 3
	ExitNetwork     = 4
	ExitRemoteState = 5
	ExitInterrupted = 130
)

var exitCodeByErrorCode = map[string]int{
	ErrorCodeValidation:  ExitValidation,
	ErrorCodeAuth:        ExitAuth,
	ErrorCodeNetwork:     ExitNetwork,
	ErrorCodeRemoteState: ExitRemoteState,
	ErrorCodeInternal:    ExitInternal,
}

// Response wraps all JSON command output.
type Response struct {
	Version string `json:"version"`
	Command string `json:"command"`
	Data    any    `json:"data,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

// Error is the unified machine error object.
type Error struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Details     any    `json:"details,omitempty"`
	Remediation string `json:"remediation,omitempty"`
	TraceID     string `json:"trace_id,omitempty"`
}

// StatusFile represents the STATUS.json file written by the dispatch pipeline.
// This schema is used across multiple components (dispatch, watchdog, monitor, lifecycle).
type StatusFile struct {
	Repo    string `json:"repo,omitempty"`
	Started string `json:"started,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Task    string `json:"task,omitempty"`
}

// ExitCodeForError returns the exit code for an error code string.
func ExitCodeForError(code string) int {
	if c, ok := exitCodeByErrorCode[code]; ok {
		return c
	}
	return ExitInternal
}

// WriteJSON writes an indented JSON response envelope to w.
func WriteJSON(w io.Writer, command string, data any) error {
	return writeResponse(w, Response{Version: SchemaVersion, Command: command, Data: data})
}

// WriteJSONError writes an error response envelope to w.
func WriteJSONError(w io.Writer, command string, cerr *Error) error {
	return writeResponse(w, Response{Version: SchemaVersion, Command: command, Error: cerr})
}

// WriteJSONL writes a single JSONL line (no indent) to w.
func WriteJSONL(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(value)
}

func writeResponse(w io.Writer, resp Response) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(resp)
}

// TaskState represents the state of a task in the ledger.
type TaskState string

const (
	TaskStatePending    TaskState = "pending"
	TaskStateSettingUp  TaskState = "setting_up"
	TaskStateRunning    TaskState = "running"
	TaskStateBlocked    TaskState = "blocked"
	TaskStateCompleted  TaskState = "completed"
	TaskStateFailed     TaskState = "failed"
	TaskStateUnknown    TaskState = "unknown"
	TaskStateStale      TaskState = "stale"
)

// ProbeStatus represents the result of a remote probe.
type ProbeStatus string

const (
	ProbeStatusUnknown  ProbeStatus = "unknown"
	ProbeStatusSuccess  ProbeStatus = "success"
	ProbeStatusFailed   ProbeStatus = "failed"
	ProbeStatusDegraded ProbeStatus = "degraded"
)

// TaskSnapshot is the materialized latest-state snapshot for a sprite/task.
// This is returned by the ledger for non-blocking status queries.
type TaskSnapshot struct {
	Sprite        string      `json:"sprite"`
	TaskID        string      `json:"task_id"`
	Repo          string      `json:"repo,omitempty"`
	Branch        string      `json:"branch,omitempty"`
	Issue         int         `json:"issue,omitempty"`
	State         TaskState   `json:"state"`
	LastSeenAt    *time.Time  `json:"last_seen_at,omitempty"`
	FreshnessAge  time.Duration `json:"freshness_age_ns,omitempty"`
	ProbeStatus   ProbeStatus `json:"probe_status"`
	Error         string      `json:"error,omitempty"`
	BlockedReason string      `json:"blocked_reason,omitempty"`
	EventCount    int         `json:"event_count"`
	StartedAt     *time.Time  `json:"started_at,omitempty"`
	CompletedAt   *time.Time  `json:"completed_at,omitempty"`
}

// FleetLedgerStatus represents the fleet status derived from the ledger.
// This is used for non-blocking status queries.
type FleetLedgerStatus struct {
	Sprites      []TaskSnapshot `json:"sprites"`
	Total        int            `json:"total"`
	FromCache    bool           `json:"from_cache"`
	GeneratedAt  time.Time      `json:"generated_at"`
	StaleCount   int            `json:"stale_count"`
	UnknownCount int            `json:"unknown_count"`
}
