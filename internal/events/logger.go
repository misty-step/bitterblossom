package events

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	pkgevents "github.com/misty-step/bitterblossom/pkg/events"
)

// LoggerConfig controls event logger construction.
type LoggerConfig struct {
	Dir string
}

// Logger appends validated events to daily JSONL files.
type Logger struct {
	mu  sync.Mutex
	dir string
}

// NewLogger creates a daily-rotating JSONL event logger.
func NewLogger(cfg LoggerConfig) (*Logger, error) {
	dir := strings.TrimSpace(cfg.Dir)
	if dir == "" {
		dir = DefaultDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("events: create dir %s: %w", dir, err)
	}
	return &Logger{dir: filepath.Clean(dir)}, nil
}

// Dir returns the root directory used for persisted daily event files.
func (l *Logger) Dir() string {
	if l == nil {
		return ""
	}
	return l.dir
}

// Log appends a validated event to a daily JSONL file.
func (l *Logger) Log(event Event) error {
	if l == nil {
		return fmt.Errorf("events: logger is nil")
	}
	payload, err := pkgevents.MarshalEvent(event)
	if err != nil {
		return err
	}

	day := event.Timestamp().UTC().Format(dailyLayout)
	path := filepath.Join(l.dir, day+".jsonl")

	l.mu.Lock()
	defer l.mu.Unlock()

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("events: open %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("events: lock %s: %w", path, err)
	}
	defer func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }()

	if _, err := file.Write(payload); err != nil {
		return fmt.Errorf("events: write %s: %w", path, err)
	}
	if _, err := file.Write([]byte{'\n'}); err != nil {
		return fmt.Errorf("events: write newline %s: %w", path, err)
	}
	return nil
}

