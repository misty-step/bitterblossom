package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/pkg/events"
	"github.com/spf13/cobra"
)

type logsEnvelope struct {
	Type  string       `json:"type"`
	Event events.Event `json:"event"`
}

func newLogsCmd(stdout, _ io.Writer) *cobra.Command {
	var files []string
	var sprites []string
	var kindsRaw []string
	var sinceRaw string
	var untilRaw string
	var follow bool
	var jsonMode bool
	var pollInterval time.Duration

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Query historical JSONL event logs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(files) == 0 {
				return fmt.Errorf("logs: at least one --file path is required")
			}

			now := time.Now().UTC()
			start, end, err := parseTimeRange(now, sinceRaw, untilRaw)
			if err != nil {
				return err
			}
			filter, err := buildEventFilter(sprites, kindsRaw, start, end)
			if err != nil {
				return err
			}

			historical, err := readHistoricalEvents(files, filter)
			if err != nil {
				return err
			}
			for _, event := range historical {
				if err := writeLogEvent(stdout, event, jsonMode); err != nil {
					return err
				}
			}
			if !follow {
				return nil
			}

			watcher, err := events.NewWatcher(events.WatcherConfig{
				Paths:        files,
				PollInterval: pollInterval,
				Filter:       filter,
				StartAtEnd:   true,
			})
			if err != nil {
				return err
			}

			sub, cancelSub := watcher.Subscribe(256)
			defer cancelSub()

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			errCh := make(chan error, 1)
			go func() {
				errCh <- watcher.Run(ctx)
			}()

			for {
				select {
				case <-ctx.Done():
					return nil
				case err := <-errCh:
					return err
				case event, ok := <-sub:
					if !ok {
						return nil
					}
					if err := writeLogEvent(stdout, event, jsonMode); err != nil {
						return err
					}
				}
			}
		},
	}

	cmd.Flags().StringSliceVar(&files, "file", nil, "JSONL event file(s) to read")
	cmd.Flags().StringSliceVar(&sprites, "sprite", nil, "filter by sprite name")
	cmd.Flags().StringSliceVar(&kindsRaw, "type", nil, "filter by event type")
	cmd.Flags().StringVar(&sinceRaw, "since", "", "include events since duration or RFC3339 timestamp")
	cmd.Flags().StringVar(&untilRaw, "until", "", "include events until RFC3339 timestamp")
	cmd.Flags().BoolVar(&follow, "follow", false, "follow events as files are appended")
	cmd.Flags().BoolVar(&jsonMode, "json", false, "emit JSONL output")
	cmd.Flags().DurationVar(&pollInterval, "poll-interval", events.DefaultWatchPollInterval, "file tail polling interval")
	return cmd
}

func readHistoricalEvents(paths []string, filter events.Filter) ([]events.Event, error) {
	all := make([]events.Event, 0, 256)
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("logs: open %s: %w", path, err)
		}

		items, err := events.ReadAll(file)
		_ = file.Close()
		if err != nil {
			return nil, fmt.Errorf("logs: read %s: %w", path, err)
		}
		for _, event := range items {
			if filter != nil && !filter(event) {
				continue
			}
			all = append(all, event)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Timestamp().Before(all[j].Timestamp())
	})
	return all, nil
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
