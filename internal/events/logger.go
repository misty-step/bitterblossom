package events

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gofrs/flock"
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
	lockPath := path + ".lock"

	l.mu.Lock()
	defer l.mu.Unlock()

	lock := flock.New(lockPath)
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("events: lock %s: %w", lockPath, err)
	}
	defer func() { _ = lock.Unlock() }()

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("events: open %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	line := append(payload, '\n')
	if _, err := file.Write(line); err != nil {
		return fmt.Errorf("events: write %s: %w", path, err)
	}
	return nil
}
