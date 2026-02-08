package events

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

const (
	// DefaultWatchPollInterval is used when no poll interval is provided.
	DefaultWatchPollInterval = 500 * time.Millisecond
	// DefaultSubscriberBuffer is the default channel buffer used by Subscribe.
	DefaultSubscriberBuffer = 64
)

// WatcherConfig controls a JSONL tail watcher.
type WatcherConfig struct {
	Paths []string

	PollInterval time.Duration
	Filter       Filter

	// StartAtEnd skips current file contents and only emits future appends.
	StartAtEnd bool

	// SubscriberBuffer is used when Subscribe is called with buffer <= 0.
	SubscriberBuffer int
}

type fileState struct {
	offset    int64
	remainder []byte
	ready     bool
}

// Watcher tails append-only JSONL event files and fans out decoded events.
type Watcher struct {
	paths        []string
	pollInterval time.Duration
	filter       Filter
	startAtEnd   bool
	buffer       int

	mu          sync.RWMutex
	subscribers map[int]chan Event
	nextID      int

	stateMu sync.Mutex
	files   map[string]*fileState

	closeOnce sync.Once
}

// NewWatcher constructs a watcher for one or more JSONL files.
func NewWatcher(cfg WatcherConfig) (*Watcher, error) {
	if len(cfg.Paths) == 0 {
		return nil, errors.New("events: watcher requires at least one path")
	}

	paths := make([]string, 0, len(cfg.Paths))
	seen := make(map[string]struct{}, len(cfg.Paths))
	for _, path := range cfg.Paths {
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return nil, errors.New("events: watcher requires non-empty paths")
	}

	if cfg.PollInterval <= 0 {
		cfg.PollInterval = DefaultWatchPollInterval
	}
	if cfg.SubscriberBuffer <= 0 {
		cfg.SubscriberBuffer = DefaultSubscriberBuffer
	}

	return &Watcher{
		paths:        paths,
		pollInterval: cfg.PollInterval,
		filter:       cfg.Filter,
		startAtEnd:   cfg.StartAtEnd,
		buffer:       cfg.SubscriberBuffer,
		subscribers:  make(map[int]chan Event),
		files:        make(map[string]*fileState, len(paths)),
	}, nil
}

// Subscribe registers a channel subscriber. The returned cancel function
// unregisters and closes the channel.
func (w *Watcher) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer <= 0 {
		buffer = w.buffer
	}
	ch := make(chan Event, buffer)

	w.mu.Lock()
	id := w.nextID
	w.nextID++
	w.subscribers[id] = ch
	w.mu.Unlock()

	cancel := func() {
		w.mu.Lock()
		existing, ok := w.subscribers[id]
		if ok {
			delete(w.subscribers, id)
			close(existing)
		}
		w.mu.Unlock()
	}
	return ch, cancel
}

// Run starts the tail loop and exits on context cancellation.
func (w *Watcher) Run(ctx context.Context) error {
	defer w.closeAll()

	if err := w.scanOnce(); err != nil {
		return err
	}

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil
			}
			return ctx.Err()
		case <-ticker.C:
			if err := w.scanOnce(); err != nil {
				return err
			}
		}
	}
}

// RunOnce scans configured files once and publishes any newly decoded events.
func (w *Watcher) RunOnce() error {
	return w.scanOnce()
}

func (w *Watcher) scanOnce() error {
	for _, path := range w.paths {
		if err := w.readPath(path); err != nil {
			return err
		}
	}
	return nil
}

func (w *Watcher) readPath(path string) (retErr error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("events: open %s: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("events: close %s: %w", path, closeErr))
		}
	}()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("events: stat %s: %w", path, err)
	}

	w.stateMu.Lock()
	state, ok := w.files[path]
	if !ok {
		state = &fileState{}
		w.files[path] = state
	}
	if !state.ready {
		if w.startAtEnd {
			state.offset = info.Size()
		}
		state.ready = true
	}
	if info.Size() < state.offset {
		// Truncate/rotate: restart from the beginning.
		state.offset = 0
		state.remainder = nil
	}
	start := state.offset
	w.stateMu.Unlock()

	if info.Size() == start {
		return nil
	}

	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return fmt.Errorf("events: seek %s: %w", path, err)
	}
	chunk, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("events: read %s: %w", path, err)
	}

	w.stateMu.Lock()
	state = w.files[path]
	payload := append(append([]byte(nil), state.remainder...), chunk...)
	state.offset = start + int64(len(chunk))
	w.stateMu.Unlock()

	lineStart := 0
	for {
		idx := bytes.IndexByte(payload[lineStart:], '\n')
		if idx < 0 {
			break
		}

		line := bytes.TrimSpace(payload[lineStart : lineStart+idx])
		if len(line) > 0 {
			event, err := UnmarshalEvent(line)
			if err != nil {
				return fmt.Errorf("events: decode %s: %w", path, err)
			}
			if w.filter == nil || w.filter(event) {
				w.publish(event)
			}
		}
		lineStart += idx + 1
	}

	w.stateMu.Lock()
	state = w.files[path]
	state.remainder = append(state.remainder[:0], payload[lineStart:]...)
	w.stateMu.Unlock()

	return nil
}

func (w *Watcher) publish(event Event) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	for _, ch := range w.subscribers {
		select {
		case ch <- event:
		default:
			// Avoid head-of-line blocking from slow subscribers: drop oldest item.
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- event:
			default:
			}
		}
	}
}

func (w *Watcher) closeAll() {
	w.closeOnce.Do(func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		for id, ch := range w.subscribers {
			delete(w.subscribers, id)
			close(ch)
		}
	})
}
