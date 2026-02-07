package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// Event represents one structured daemon event.
type Event struct {
	Sprite    string         `json:"sprite"`
	Event     string         `json:"event"`
	Timestamp string         `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// EventSink receives event emissions.
type EventSink interface {
	Emit(eventType string, metadata map[string]any) error
}

// NDJSONSink writes one JSON object per line.
type NDJSONSink struct {
	mu     sync.Mutex
	w      io.Writer
	now    func() time.Time
	sprite string
}

// NewNDJSONSink builds an event sink.
func NewNDJSONSink(w io.Writer, sprite string) *NDJSONSink {
	return &NDJSONSink{
		w:      w,
		now:    time.Now,
		sprite: sprite,
	}
}

// Emit writes an event line.
func (s *NDJSONSink) Emit(eventType string, metadata map[string]any) error {
	if eventType == "" {
		return fmt.Errorf("event type required")
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	record := Event{
		Sprite:    s.sprite,
		Event:     eventType,
		Timestamp: s.now().UTC().Format(time.RFC3339),
		Metadata:  metadata,
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.w.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("write event: %w", err)
	}
	return nil
}
