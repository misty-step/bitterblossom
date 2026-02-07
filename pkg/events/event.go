package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Kind identifies the event type in the JSONL protocol.
type Kind string

const (
	KindProvision Kind = "provision"
	KindDispatch  Kind = "dispatch"
	KindProgress  Kind = "progress"
	KindDone      Kind = "done"
	KindBlocked   Kind = "blocked"
	KindError     Kind = "error"
)

// Event is the common interface for all fleet events.
type Event interface {
	Timestamp() time.Time
	Sprite() string
	Kind() Kind
	GetTimestamp() time.Time
	GetSprite() string
	GetKind() Kind
}

// Meta carries shared event fields.
type Meta struct {
	TS         time.Time `json:"ts"`
	SpriteName string    `json:"sprite"`
	EventKind  Kind      `json:"event"`
}

// Timestamp returns the event timestamp.
func (m Meta) Timestamp() time.Time { return m.TS }

// Sprite returns the sprite name.
func (m Meta) Sprite() string { return m.SpriteName }

// Kind returns the event kind.
func (m Meta) Kind() Kind { return m.EventKind }

// GetTimestamp returns the event timestamp.
func (m Meta) GetTimestamp() time.Time { return m.Timestamp() }

// GetSprite returns the sprite name.
func (m Meta) GetSprite() string { return m.Sprite() }

// GetKind returns the event kind.
func (m Meta) GetKind() Kind { return m.Kind() }

// ProvisionEvent reports sprite provisioning.
type ProvisionEvent struct {
	Meta
	Persona string `json:"persona,omitempty"`
}

// DispatchEvent reports task assignment.
type DispatchEvent struct {
	Meta
	Task   string `json:"task"`
	Repo   string `json:"repo,omitempty"`
	Branch string `json:"branch,omitempty"`
}

// ProgressEvent reports in-flight task progress.
type ProgressEvent struct {
	Meta
	Commits      int `json:"commits"`
	FilesChanged int `json:"files_changed"`
}

// DoneEvent reports task completion.
type DoneEvent struct {
	Meta
	Branch string `json:"branch,omitempty"`
	PR     int    `json:"pr,omitempty"`
}

// BlockedEvent reports a blocked task with reason.
type BlockedEvent struct {
	Meta
	Reason string `json:"reason"`
}

// ErrorEvent reports runtime failures.
type ErrorEvent struct {
	Meta
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

var (
	// ErrUnknownKind indicates the event discriminator does not match known types.
	ErrUnknownKind = errors.New("events: unknown event kind")
	// ErrInvalidEvent indicates malformed event payload.
	ErrInvalidEvent = errors.New("events: invalid event")
)

// Valid reports whether kind is recognized by this package.
func (k Kind) Valid() bool {
	switch k {
	case KindProvision, KindDispatch, KindProgress, KindDone, KindBlocked, KindError:
		return true
	default:
		return false
	}
}

// ParseKind parses a kind name from user input.
func ParseKind(raw string) (Kind, error) {
	kind := Kind(strings.TrimSpace(strings.ToLower(raw)))
	if !kind.Valid() {
		return "", fmt.Errorf("%w: %q", ErrUnknownKind, raw)
	}
	return kind, nil
}

// MarshalEvent encodes an event as one JSON object (one JSONL line).
func MarshalEvent(event Event) ([]byte, error) {
	if event == nil {
		return nil, fmt.Errorf("%w: nil event", ErrInvalidEvent)
	}

	if err := validateEvent(event); err != nil {
		return nil, err
	}
	return json.Marshal(event)
}

// UnmarshalEvent decodes one JSONL object into a concrete event type.
func UnmarshalEvent(data []byte) (Event, error) {
	var envelope struct {
		Event Kind `json:"event"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}

	var event Event
	switch envelope.Event {
	case KindProvision:
		event = &ProvisionEvent{}
	case KindDispatch:
		event = &DispatchEvent{}
	case KindProgress:
		event = &ProgressEvent{}
	case KindDone:
		event = &DoneEvent{}
	case KindBlocked:
		event = &BlockedEvent{}
	case KindError:
		event = &ErrorEvent{}
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownKind, envelope.Event)
	}

	if err := json.Unmarshal(data, event); err != nil {
		return nil, err
	}
	if err := validateEvent(event); err != nil {
		return nil, err
	}
	return event, nil
}

func validateEvent(event Event) error {
	if event.Kind() == "" {
		return fmt.Errorf("%w: missing event kind", ErrInvalidEvent)
	}
	if !event.Kind().Valid() {
		return fmt.Errorf("%w: unknown event kind %q", ErrInvalidEvent, event.Kind())
	}
	if event.Sprite() == "" {
		return fmt.Errorf("%w: missing sprite name", ErrInvalidEvent)
	}

	switch typed := event.(type) {
	case *DispatchEvent:
		if typed.Task == "" {
			return fmt.Errorf("%w: dispatch task is required", ErrInvalidEvent)
		}
	case DispatchEvent:
		if typed.Task == "" {
			return fmt.Errorf("%w: dispatch task is required", ErrInvalidEvent)
		}
	case *BlockedEvent:
		if typed.Reason == "" {
			return fmt.Errorf("%w: blocked reason is required", ErrInvalidEvent)
		}
	case BlockedEvent:
		if typed.Reason == "" {
			return fmt.Errorf("%w: blocked reason is required", ErrInvalidEvent)
		}
	case *ErrorEvent:
		if typed.Message == "" {
			return fmt.Errorf("%w: error message is required", ErrInvalidEvent)
		}
	case ErrorEvent:
		if typed.Message == "" {
			return fmt.Errorf("%w: error message is required", ErrInvalidEvent)
		}
	}

	return nil
}
