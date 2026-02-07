package contracts

// SchemaVersion is the current contract version.
const SchemaVersion = "v1"

const (
	// ErrorCodeValidation indicates invalid input or missing required flags.
	ErrorCodeValidation = "VALIDATION_ERROR"
	// ErrorCodeAuth indicates missing or invalid credentials.
	ErrorCodeAuth = "AUTH_ERROR"
	// ErrorCodeNetwork indicates network or connectivity failures.
	ErrorCodeNetwork = "NETWORK_ERROR"
	// ErrorCodeRemoteState indicates remote resource state issues.
	ErrorCodeRemoteState = "REMOTE_STATE_ERROR"
	// ErrorCodeInternal indicates unexpected internal failures.
	ErrorCodeInternal = "INTERNAL_ERROR"
)

// Response wraps all JSON command output.
type Response struct {
	Version string `json:"version"`         // always SchemaVersion
	Command string `json:"command"`         // e.g. "compose.status"
	Data    any    `json:"data,omitempty"`  // success payload
	Error   *Error `json:"error,omitempty"` // present on failure
}

// Error is the unified machine error object.
type Error struct {
	Code        string `json:"code"`                  // e.g. "VALIDATION_ERROR"
	Message     string `json:"message"`               // human-readable
	Details     any    `json:"details,omitempty"`     // structured context
	Remediation string `json:"remediation,omitempty"` // suggested fix
	TraceID     string `json:"trace_id,omitempty"`    // optional correlation
}
