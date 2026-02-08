// Package contracts defines versioned JSON/NDJSON output schemas and
// machine error semantics for the bb CLI.
package contracts

import (
	"encoding/json"
	"io"
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
