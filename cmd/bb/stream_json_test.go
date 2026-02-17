package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestStreamJSONWriterPrettyFormatsAssistantText(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	w := newStreamJSONWriter(&out, false)
	_, err := w.Write([]byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hello"}]}}` + "\n"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if got := out.String(); got != "hello\n" {
		t.Fatalf("out = %q, want %q", got, "hello\n")
	}
}

func TestStreamJSONWriterPrettyFormatsToolUse(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	w := newStreamJSONWriter(&out, false)
	_, err := w.Write([]byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{"command":"echo hi"}}]}}` + "\n"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if got := out.String(); got != "[tool Bash] echo hi\n" {
		t.Fatalf("out = %q, want %q", got, "[tool Bash] echo hi\n")
	}
}

func TestStreamJSONWriterPrettyFormatsToolResultStdout(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	w := newStreamJSONWriter(&out, false)
	_, err := w.Write([]byte(`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"hi","tool_use_id":"toolu_123","is_error":false}]},"tool_use_result":{"stdout":"hi\n","stderr":""}}` + "\n"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if got := out.String(); got != "hi\n" {
		t.Fatalf("out = %q, want %q", got, "hi\n")
	}
}

func TestStreamJSONWriterPrettyIgnoresSystemEvents(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	w := newStreamJSONWriter(&out, false)
	_, err := w.Write([]byte(`{"type":"system","subtype":"init","cwd":"/tmp"}` + "\n"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if out.Len() != 0 {
		t.Fatalf("out should be empty, got %q", out.String())
	}
}

func TestStreamJSONWriterPrettyFormatsSystemEventWithContent(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	w := newStreamJSONWriter(&out, false)
	_, err := w.Write([]byte(`{"type":"system","message":{"role":"assistant","content":[{"type":"text","text":"hello"}]}}` + "\n"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if got := out.String(); got != "hello\n" {
		t.Fatalf("out = %q, want %q", got, "hello\n")
	}
}

func TestStreamJSONWriterJSONModeEmitsOnlyJSONObjects(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	w := newStreamJSONWriter(&out, true)
	_, err := w.Write([]byte("[ralph] iteration 1\n"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err = w.Write([]byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hello"}]}}` + "\n"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	got := out.String()
	if strings.Contains(got, "iteration") {
		t.Fatalf("out should not contain plain text, got %q", got)
	}
	if want := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hello"}]}}` + "\n"; got != want {
		t.Fatalf("out = %q, want %q", got, want)
	}
}

func TestStreamJSONWriterHandlesSplitWrites(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	w := newStreamJSONWriter(&out, false)
	_, err := w.Write([]byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hel`))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err = w.Write([]byte(`lo"}]}}` + "\n"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if got := out.String(); got != "hello\n" {
		t.Fatalf("out = %q, want %q", got, "hello\n")
	}
}

func TestStreamJSONWriterCallsOnToolErrorForErrorResults(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var errors []string
	w := newStreamJSONWriter(&out, false)
	w.onToolError = func(s string) { errors = append(errors, s) }

	// Tool result with is_error=true
	_, err := w.Write([]byte(`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"Error: command not found: foobar","tool_use_id":"toolu_1","is_error":true}]}}` + "\n"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	// Tool result with is_error=false (should NOT trigger)
	_, err = w.Write([]byte(`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"success","tool_use_id":"toolu_2","is_error":false}]}}` + "\n"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	if len(errors) != 1 {
		t.Fatalf("expected 1 error callback, got %d: %v", len(errors), errors)
	}
	if errors[0] != "Error: command not found: foobar" {
		t.Fatalf("error = %q, want %q", errors[0], "Error: command not found: foobar")
	}
}

func TestStreamJSONWriterPrettyFallsBackToRawForUnknownJSON(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	w := newStreamJSONWriter(&out, false)
	_, err := w.Write([]byte(`{"foo":"bar"}` + "\n"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if got := out.String(); got != "{\"foo\":\"bar\"}\n" {
		t.Fatalf("out = %q, want %q", got, "{\"foo\":\"bar\"}\n")
	}
}
