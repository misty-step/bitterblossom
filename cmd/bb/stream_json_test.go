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
	_, _ = w.Write([]byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hello"}]}}` + "\n"))
	w.Flush()

	if got := out.String(); got != "hello\n" {
		t.Fatalf("out = %q, want %q", got, "hello\n")
	}
}

func TestStreamJSONWriterPrettyFormatsToolUse(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	w := newStreamJSONWriter(&out, false)
	_, _ = w.Write([]byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{"command":"echo hi"}}]}}` + "\n"))
	w.Flush()

	if got := out.String(); got != "[tool Bash] echo hi\n" {
		t.Fatalf("out = %q, want %q", got, "[tool Bash] echo hi\n")
	}
}

func TestStreamJSONWriterPrettyFormatsToolResultStdout(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	w := newStreamJSONWriter(&out, false)
	_, _ = w.Write([]byte(`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"hi","tool_use_id":"toolu_123","is_error":false}]},"tool_use_result":{"stdout":"hi\n","stderr":""}}` + "\n"))
	w.Flush()

	if got := out.String(); got != "hi\n" {
		t.Fatalf("out = %q, want %q", got, "hi\n")
	}
}

func TestStreamJSONWriterPrettyIgnoresSystemEvents(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	w := newStreamJSONWriter(&out, false)
	_, _ = w.Write([]byte(`{"type":"system","subtype":"init","cwd":"/tmp"}` + "\n"))
	w.Flush()

	if out.Len() != 0 {
		t.Fatalf("out should be empty, got %q", out.String())
	}
}

func TestStreamJSONWriterJSONModeEmitsOnlyJSONObjects(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	w := newStreamJSONWriter(&out, true)
	_, _ = w.Write([]byte("[ralph] iteration 1\n"))
	_, _ = w.Write([]byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hello"}]}}` + "\n"))
	w.Flush()

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
	_, _ = w.Write([]byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hel`))
	_, _ = w.Write([]byte(`lo"}]}}` + "\n"))
	w.Flush()

	if got := out.String(); got != "hello\n" {
		t.Fatalf("out = %q, want %q", got, "hello\n")
	}
}
