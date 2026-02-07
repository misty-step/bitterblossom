package contracts

import (
	"encoding/json"
	"io"
)

// WriteJSON writes an indented JSON response to w.
func WriteJSON(w io.Writer, command string, data any) error {
	resp := Response{Version: SchemaVersion, Command: command, Data: data}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(resp)
}

// WriteJSONError writes an error response to w.
func WriteJSONError(w io.Writer, command string, cerr *Error) error {
	resp := Response{Version: SchemaVersion, Command: command, Error: cerr}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(resp)
}

// WriteJSONL writes a single JSONL line (no indent) to w.
func WriteJSONL(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(value)
}
