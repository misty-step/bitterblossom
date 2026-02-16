package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

// streamJSONWriter renders Claude Code --output-format stream-json output.
// In pretty mode it prints assistant text/tool activity; in json mode it emits raw JSONL objects only.
type streamJSONWriter struct {
	out      io.Writer
	jsonMode bool

	mu  sync.Mutex
	buf []byte
}

func newStreamJSONWriter(out io.Writer, jsonMode bool) *streamJSONWriter {
	return &streamJSONWriter{out: out, jsonMode: jsonMode}
}

func (w *streamJSONWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}

		line := bytes.TrimRight(w.buf[:i], "\r")
		w.buf = w.buf[i+1:]
		w.writeLine(line)
	}

	return len(p), nil
}

func (w *streamJSONWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.buf) == 0 {
		return
	}
	line := bytes.TrimRight(w.buf, "\r\n")
	w.buf = nil
	w.writeLine(line)
}

func (w *streamJSONWriter) writeLine(line []byte) {
	if len(line) == 0 {
		return
	}

	trim := bytes.TrimSpace(line)
	if len(trim) == 0 {
		return
	}

	if w.jsonMode {
		if isJSONObject(trim) {
			_, _ = w.out.Write(trim)
			_, _ = w.out.Write([]byte{'\n'})
		}
		return
	}

	if trim[0] != '{' {
		_, _ = w.out.Write(line)
		_, _ = w.out.Write([]byte{'\n'})
		return
	}

	var ev claudeStreamEvent
	if err := json.Unmarshal(trim, &ev); err != nil {
		_, _ = w.out.Write(line)
		_, _ = w.out.Write([]byte{'\n'})
		return
	}

	for _, s := range formatClaudeStreamEvent(ev) {
		if s == "" {
			continue
		}
		_, _ = io.WriteString(w.out, s)
		if !strings.HasSuffix(s, "\n") {
			_, _ = io.WriteString(w.out, "\n")
		}
	}
}

func isJSONObject(line []byte) bool {
	if len(line) == 0 || line[0] != '{' {
		return false
	}
	var v any
	return json.Unmarshal(line, &v) == nil
}

type claudeStreamEvent struct {
	Type          string               `json:"type"`
	Subtype       string               `json:"subtype,omitempty"`
	Message       *claudeStreamMessage `json:"message,omitempty"`
	ToolUseResult *claudeToolUseResult `json:"tool_use_result,omitempty"`
}

type claudeStreamMessage struct {
	Role    string                `json:"role,omitempty"`
	Content []claudeStreamContent `json:"content,omitempty"`
}

type claudeStreamContent struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	Content   string         `json:"content,omitempty"`
	IsError   bool           `json:"is_error,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
}

type claudeToolUseResult struct {
	Stdout string `json:"stdout,omitempty"`
	Stderr string `json:"stderr,omitempty"`
}

func formatClaudeStreamEvent(ev claudeStreamEvent) []string {
	switch ev.Type {
	case "assistant":
		return formatAssistantEvent(ev)
	case "user":
		return formatUserEvent(ev)
	case "system", "result":
		return nil
	default:
		return nil
	}
}

func formatAssistantEvent(ev claudeStreamEvent) []string {
	if ev.Message == nil {
		return nil
	}

	var out []string
	for _, block := range ev.Message.Content {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) == "" {
				continue
			}
			out = append(out, block.Text)
		case "tool_use":
			out = append(out, formatToolUseBlock(block))
		}
	}
	return out
}

func formatToolUseBlock(block claudeStreamContent) string {
	name := strings.TrimSpace(block.Name)
	if name == "" {
		name = "tool"
	}

	if cmd, ok := block.Input["command"].(string); ok && strings.TrimSpace(cmd) != "" {
		return fmt.Sprintf("[tool %s] %s", name, cmd)
	}
	if desc, ok := block.Input["description"].(string); ok && strings.TrimSpace(desc) != "" {
		return fmt.Sprintf("[tool %s] %s", name, desc)
	}
	return fmt.Sprintf("[tool %s]", name)
}

func formatUserEvent(ev claudeStreamEvent) []string {
	if ev.ToolUseResult != nil {
		var out []string
		if strings.TrimSpace(ev.ToolUseResult.Stdout) != "" {
			out = append(out, ev.ToolUseResult.Stdout)
		}
		if strings.TrimSpace(ev.ToolUseResult.Stderr) != "" {
			out = append(out, ev.ToolUseResult.Stderr)
		}
		if len(out) > 0 {
			return out
		}
	}

	if ev.Message == nil {
		return nil
	}

	var out []string
	for _, block := range ev.Message.Content {
		if block.Type != "tool_result" {
			continue
		}
		if strings.TrimSpace(block.Content) == "" {
			continue
		}
		out = append(out, block.Content)
	}
	return out
}
