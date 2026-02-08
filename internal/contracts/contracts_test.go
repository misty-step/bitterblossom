package contracts

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

func TestResponseGolden(t *testing.T) {
	cases := []struct {
		name     string
		response Response
		golden   string
	}{
		{
			name: "success",
			response: Response{
				Version: SchemaVersion,
				Command: "compose.status",
				Data: struct {
					Composition string `json:"composition"`
					Desired     int    `json:"desired"`
					Actual      int    `json:"actual"`
				}{
					Composition: "v1",
					Desired:     2,
					Actual:      1,
				},
			},
			golden: "response_success.golden.json",
		},
		{
			name: "error",
			response: Response{
				Version: SchemaVersion,
				Command: "agent.status",
				Error: &Error{
					Code:    ErrorCodeValidation,
					Message: "--state-file is required",
					Details: struct {
						Flag   string `json:"flag"`
						Reason string `json:"reason"`
					}{
						Flag:   "state-file",
						Reason: "missing",
					},
					Remediation: "Pass --state-file with a readable path.",
					TraceID:     "trace-123",
				},
			},
			golden: "response_error.golden.json",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.MarshalIndent(tc.response, "", "  ")
			if err != nil {
				t.Fatalf("marshal response: %v", err)
			}

			goldenPath := filepath.Join("testdata", tc.golden)
			if *update {
				if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
					t.Fatalf("write golden file: %v", err)
				}
				return
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden file: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("golden mismatch for %s\nwant:\n%s\n\ngot:\n%s", tc.name, want, got)
			}
		})
	}
}

func TestExitCodeForError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		code string
		want int
	}{
		{name: "validation", code: ErrorCodeValidation, want: ExitValidation},
		{name: "auth", code: ErrorCodeAuth, want: ExitAuth},
		{name: "network", code: ErrorCodeNetwork, want: ExitNetwork},
		{name: "remote-state", code: ErrorCodeRemoteState, want: ExitRemoteState},
		{name: "internal", code: ErrorCodeInternal, want: ExitInternal},
		{name: "unknown", code: "UNKNOWN_ERROR", want: ExitInternal},
		{name: "empty", code: "", want: ExitInternal},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ExitCodeForError(tc.code); got != tc.want {
				t.Fatalf("ExitCodeForError(%q) = %d, want %d", tc.code, got, tc.want)
			}
		})
	}
}

func TestAllErrorCodesHaveExitCodes(t *testing.T) {
	t.Parallel()

	errorCodes := []string{
		ErrorCodeValidation,
		ErrorCodeAuth,
		ErrorCodeNetwork,
		ErrorCodeRemoteState,
		ErrorCodeInternal,
	}
	for _, code := range errorCodes {
		if _, ok := exitCodeByErrorCode[code]; !ok {
			t.Fatalf("missing exit-code mapping for %q", code)
		}
	}
	if len(exitCodeByErrorCode) != len(errorCodes) {
		t.Fatalf("exit-code mapping size = %d, want %d", len(exitCodeByErrorCode), len(errorCodes))
	}
}
