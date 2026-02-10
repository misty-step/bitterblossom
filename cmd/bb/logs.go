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
	var org string
	var lines int

	cmd := &cobra.Command{
		Use:   "logs [sprite-name]",
		Short: "Query historical JSONL event logs or stream sprite agent logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Mode 1: Remote sprite agent logs
			if len(args) == 1 {
				return runRemoteSpriteLog(cmd.Context(), args[0], org, lines, follow, stdout)
			}
			if len(args) > 1 {
				return fmt.Errorf("logs: only one sprite name can be provided")
			}

			// Mode 2: Local JSONL event logs (original behavior)
			if len(files) == 0 {
				return fmt.Errorf("logs: at least one --file path is required or provide a sprite name")
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

	cmd.Flags().StringSliceVar(&files, "file", nil, "JSONL event file(s) to read (for local mode)")
	cmd.Flags().StringSliceVar(&sprites, "sprite", nil, "filter by sprite name (for local mode)")
	cmd.Flags().StringSliceVar(&kindsRaw, "type", nil, "filter by event type (for local mode)")
	cmd.Flags().StringVar(&sinceRaw, "since", "", "include events since duration or RFC3339 timestamp (for local mode)")
	cmd.Flags().StringVar(&untilRaw, "until", "", "include events until RFC3339 timestamp (for local mode)")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output in real-time")
	cmd.Flags().BoolVar(&jsonMode, "json", false, "emit JSONL output (for local mode)")
	cmd.Flags().DurationVar(&pollInterval, "poll-interval", events.DefaultWatchPollInterval, "file tail polling interval (for local mode)")
	cmd.Flags().StringVar(&org, "org", envOrDefault("FLY_ORG", ""), "Fly.io organization (for remote mode)")
	cmd.Flags().IntVarP(&lines, "lines", "n", 100, "number of tail lines to show (for remote mode)")
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

func runRemoteSpriteLog(ctx context.Context, spriteName, org string, lines int, follow bool, out io.Writer) error {
	spriteName = strings.TrimSpace(spriteName)
	if spriteName == "" {
		return fmt.Errorf("sprite name is required")
	}

	remote := newSpriteCLIRemote("sprite", org)

	// Default agent log path on sprites
	logPath := ".bb-agent/agent.log"

	// Read tail lines first
	if lines > 0 {
		tailCmd := fmt.Sprintf("tail -n %d %s 2>/dev/null || echo '# no logs yet'", lines, shellQuote(logPath))
		output, err := remote.Exec(ctx, spriteName, tailCmd, nil)
		if err != nil {
			return fmt.Errorf("failed to read logs from sprite %q: %w", spriteName, err)
		}
		_, _ = fmt.Fprint(out, output)
	}

	if !follow {
		return nil
	}

	// Follow mode: poll for new log lines
	return followRemoteLog(ctx, remote, spriteName, logPath, out)
}

func followRemoteLog(ctx context.Context, remote *spriteCLIRemote, spriteName, logPath string, out io.Writer) error {
	// Track the last line we've seen to avoid duplicates
	var lastLines []string
	pollInterval := 500 * time.Millisecond
	tailCount := 10 // Read last 10 lines each poll

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		// Read recent lines
		tailCmd := fmt.Sprintf("tail -n %d %s 2>/dev/null || true", tailCount, shellQuote(logPath))
		output, err := remote.Exec(ctx, spriteName, tailCmd, nil)
		if err != nil {
			// Don't fail on transient errors during polling
			time.Sleep(pollInterval)
			continue
		}

		if strings.TrimSpace(output) == "" {
			time.Sleep(pollInterval)
			continue
		}

		currentLines := strings.Split(strings.TrimSpace(output), "\n")

		// Find new lines by comparing with last poll
		if len(lastLines) > 0 {
			// Find where the overlap ends
			newStartIdx := 0
			for i := 0; i < len(currentLines) && i < len(lastLines); i++ {
				if currentLines[i] == lastLines[len(lastLines)-len(currentLines)+i] {
					newStartIdx = i + 1
				} else {
					break
				}
			}

			// Print only new lines
			for i := newStartIdx; i < len(currentLines); i++ {
				_, _ = fmt.Fprintln(out, currentLines[i])
			}
		} else {
			// First poll - don't print anything (already shown by tail command above)
		}

		lastLines = currentLines
		time.Sleep(pollInterval)
	}
}
