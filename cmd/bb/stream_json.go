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
	err error
}

func newStreamJSONWriter(out io.Writer, jsonMode bool) *streamJSONWriter {
	return &streamJSONWriter{out: out, jsonMode: jsonMode}
}

func (w *streamJSONWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.err != nil {
		return 0, w.err
	}

	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}

		line := bytes.TrimRight(w.buf[:i], "\r")
		w.buf = w.buf[i+1:]
		if err := w.writeLine(line); err != nil {
			w.err = err
			return len(p), err
		}
	}

	return len(p), nil
}

func (w *streamJSONWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.err != nil {
		return w.err
	}
	if len(w.buf) == 0 {
		return nil
	}
	line := bytes.TrimRight(w.buf, "\r\n")
	w.buf = nil
	if err := w.writeLine(line); err != nil {
		w.err = err
		return err
	}
	return nil
}

func (w *streamJSONWriter) writeLine(line []byte) error {
	if len(line) == 0 {
		return nil
	}

	trim := bytes.TrimSpace(line)
	if len(trim) == 0 {
		return nil
	}

	if w.jsonMode {
		if isJSONObject(trim) {
			if _, err := w.out.Write(trim); err != nil {
				return err
			}
			if _, err := w.out.Write([]byte{'\n'}); err != nil {
				return err
			}
		}
		return nil
	}

	if trim[0] != '{' {
		if _, err := w.out.Write(line); err != nil {
			return err
		}
		if _, err := w.out.Write([]byte{'\n'}); err != nil {
			return err
		}
		return nil
	}

	var ev claudeStreamEvent
	if err := json.Unmarshal(trim, &ev); err != nil {
		if _, err := w.out.Write(line); err != nil {
			return err
		}
		if _, err := w.out.Write([]byte{'\n'}); err != nil {
			return err
		}
		return nil
	}

	formatted := formatClaudeStreamEvent(ev)
	if len(formatted) == 0 {
		switch ev.Type {
		case "system", "result":
			return nil
		default:
			if _, err := w.out.Write(line); err != nil {
				return err
			}
			if _, err := w.out.Write([]byte{'\n'}); err != nil {
				return err
			}
			return nil
		}
	}

	for _, s := range formatted {
		if s == "" {
			continue
		}
		if _, err := io.WriteString(w.out, s); err != nil {
			return err
		}
		if !strings.HasSuffix(s, "\n") {
			if _, err := io.WriteString(w.out, "\n"); err != nil {
				return err
			}
		}
	}
	return nil
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
