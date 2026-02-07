package events

import (
	"fmt"
	"strings"
	"time"
)

// Filter decides whether an event should be included.
type Filter func(Event) bool

// Chain combines multiple filters with logical AND.
func Chain(filters ...Filter) Filter {
	return func(event Event) bool {
		for _, filter := range filters {
			if filter == nil {
				continue
			}
			if !filter(event) {
				return false
			}
		}
		return true
	}
}

// Apply returns only events matching all provided filters.
func Apply(input []Event, filters ...Filter) []Event {
	matcher := Chain(filters...)
	out := make([]Event, 0, len(input))
	for _, event := range input {
		if matcher(event) {
			out = append(out, event)
		}
	}
	return out
}

// BySprite filters events by sprite name (case-insensitive).
func BySprite(names ...string) Filter {
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(strings.ToLower(name))
		if trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}

	if len(allowed) == 0 {
		return func(Event) bool { return true }
	}

	return func(event Event) bool {
		_, ok := allowed[strings.ToLower(event.Sprite())]
		return ok
	}
}

// ByKind filters events by kind.
func ByKind(kinds ...Kind) Filter {
	allowed := make(map[Kind]struct{}, len(kinds))
	for _, kind := range kinds {
		if kind != "" {
			allowed[kind] = struct{}{}
		}
	}

	if len(allowed) == 0 {
		return func(Event) bool { return true }
	}

	return func(event Event) bool {
		_, ok := allowed[event.Kind()]
		return ok
	}
}

// ByTimeRange filters events to a closed interval.
func ByTimeRange(start, end time.Time) Filter {
	return func(event Event) bool {
		ts := event.Timestamp()
		if !start.IsZero() && ts.Before(start) {
			return false
		}
		if !end.IsZero() && ts.After(end) {
			return false
		}
		return true
	}
}

// ParseKinds parses comma-separated event kind names.
func ParseKinds(raw string) ([]Kind, error) {
	parts := strings.Split(raw, ",")
	kinds := make([]Kind, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(strings.ToLower(part))
		if trimmed == "" {
			continue
		}
		kinds = append(kinds, Kind(trimmed))
	}
	if len(kinds) == 0 {
		return nil, fmt.Errorf("events: no kinds specified")
	}
	return kinds, nil
}
