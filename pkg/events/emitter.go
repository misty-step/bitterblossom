package events

import (
	"errors"
	"io"
	"sync"
)

// Emitter writes newline-delimited JSON events to an output stream.
type Emitter struct {
	mu sync.Mutex
	w  io.Writer
}

// NewEmitter builds an event emitter.
func NewEmitter(w io.Writer) (*Emitter, error) {
	if w == nil {
		return nil, errors.New("events: writer cannot be nil")
	}
	return &Emitter{w: w}, nil
}

// Emit writes one event as a single JSONL line.
func (e *Emitter) Emit(event Event) error {
	payload, err := MarshalEvent(event)
	if err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if _, err := e.w.Write(payload); err != nil {
		return err
	}
	_, err = e.w.Write([]byte{'\n'})
	return err
}
