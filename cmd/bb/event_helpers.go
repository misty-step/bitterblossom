package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/pkg/events"
)

type logsEnvelope struct {
	Type  string       `json:"type"`
	Event events.Event `json:"event"`
}

func writeLogEvent(stdout io.Writer, event events.Event, jsonMode bool) error {
	if jsonMode {
		return json.NewEncoder(stdout).Encode(logsEnvelope{Type: "event", Event: event})
	}

	line := fmt.Sprintf(
		"%s %-12s %-9s",
		event.Timestamp().UTC().Format(time.RFC3339),
		event.Sprite(),
		event.Kind(),
	)
	if issue := event.GetIssue(); issue > 0 {
		line += fmt.Sprintf(" issue=%d", issue)
	}

	switch typed := event.(type) {
	case events.DispatchEvent:
		line += " task=" + typed.Task
	case *events.DispatchEvent:
		line += " task=" + typed.Task
	case events.ProgressEvent:
		line = appendProgressSummary(line, typed)
	case *events.ProgressEvent:
		line = appendProgressSummary(line, *typed)
	case events.BlockedEvent:
		line += " reason=" + typed.Reason
	case *events.BlockedEvent:
		line += " reason=" + typed.Reason
	case events.ErrorEvent:
		line += " message=" + typed.Message
	case *events.ErrorEvent:
		line += " message=" + typed.Message
	}
	_, err := fmt.Fprintln(stdout, line)
	return err
}

func appendProgressSummary(line string, progress events.ProgressEvent) string {
	if progress.Activity != "" {
		line += " activity=" + progress.Activity
	}
	if progress.Branch != "" {
		line += " branch=" + progress.Branch
	}
	if progress.Detail != "" {
		line += " detail=" + progress.Detail
	}
	if progress.Success != nil {
		line += fmt.Sprintf(" success=%t", *progress.Success)
	}
	return line
}

func parseTimeRange(now time.Time, sinceRaw, untilRaw string) (time.Time, time.Time, error) {
	var start time.Time
	var end time.Time

	if strings.TrimSpace(sinceRaw) != "" {
		if duration, err := time.ParseDuration(strings.TrimSpace(sinceRaw)); err == nil {
			start = now.Add(-duration)
		} else {
			ts, err := time.Parse(time.RFC3339, strings.TrimSpace(sinceRaw))
			if err != nil {
				return time.Time{}, time.Time{}, fmt.Errorf("invalid --since value %q", sinceRaw)
			}
			start = ts.UTC()
		}
	}

	if strings.TrimSpace(untilRaw) != "" {
		ts, err := time.Parse(time.RFC3339, strings.TrimSpace(untilRaw))
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --until value %q", untilRaw)
		}
		end = ts.UTC()
	}

	if !start.IsZero() && !end.IsZero() && end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("--until must be after --since")
	}

	return start, end, nil
}

func buildEventFilter(sprites []string, kindsRaw []string, start, end time.Time) (events.Filter, error) {
	filters := make([]events.Filter, 0, 3)
	spriteNames := splitCSV(strings.Join(sprites, ","))
	if len(spriteNames) > 0 {
		filters = append(filters, events.BySprite(spriteNames...))
	}

	if joined := strings.Join(kindsRaw, ","); strings.TrimSpace(joined) != "" {
		parts := splitCSV(joined)
		kinds := make([]events.Kind, 0, len(parts))
		for _, part := range parts {
			kind, err := events.ParseKind(part)
			if err != nil {
				return nil, fmt.Errorf("invalid --type %q", part)
			}
			kinds = append(kinds, kind)
		}
		filters = append(filters, events.ByKind(kinds...))
	}

	if !start.IsZero() || !end.IsZero() {
		filters = append(filters, events.ByTimeRange(start, end))
	}

	if len(filters) == 0 {
		return nil, nil
	}
	return events.Chain(filters...), nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
