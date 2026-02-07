package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/pkg/events"
	"github.com/spf13/cobra"
)

const defaultWatchRefresh = time.Second

type watchEnvelope struct {
	Type     string           `json:"type"`
	Event    events.Event     `json:"event,omitempty"`
	Signal   *events.Signal   `json:"signal,omitempty"`
	Snapshot *events.Snapshot `json:"snapshot,omitempty"`
}

func newWatchCmd(stdout, _ io.Writer) *cobra.Command {
	var files []string
	var sprites []string
	var types []string
	var severities []string
	var sinceRaw string
	var untilRaw string
	var jsonMode bool
	var pollInterval time.Duration
	var refreshInterval time.Duration
	var startAtEnd bool
	var once bool

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Watch event streams in real time",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(files) == 0 {
				return fmt.Errorf("watch: at least one --file path is required")
			}

			now := time.Now().UTC()
			start, end, err := parseTimeRange(now, sinceRaw, untilRaw)
			if err != nil {
				return err
			}
			filter, err := buildEventFilter(sprites, types, start, end)
			if err != nil {
				return err
			}
			severityFilter, err := buildSeverityFilter(severities)
			if err != nil {
				return err
			}

			watcher, err := events.NewWatcher(events.WatcherConfig{
				Paths:        files,
				PollInterval: pollInterval,
				Filter:       filter,
				StartAtEnd:   startAtEnd,
			})
			if err != nil {
				return err
			}

			sub, cancelSub := watcher.Subscribe(512)
			defer cancelSub()

			aggregator := events.NewAggregator(events.AggregatorConfig{
				Window:       30 * time.Minute,
				GapThreshold: events.DefaultGapThreshold,
				Now: func() time.Time {
					return time.Now().UTC()
				},
			})
			signalEngine := events.NewConfiguredSignalEngine(events.DefaultSignalConfig())

			if once {
				if err := watcher.RunOnce(); err != nil {
					return err
				}
				return drainWatchBatch(stdout, sub, aggregator, signalEngine, jsonMode, severityFilter)
			}

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			errCh := make(chan error, 1)
			go func() {
				errCh <- watcher.Run(ctx)
			}()

			refreshTicker := time.NewTicker(refreshInterval)
			defer refreshTicker.Stop()
			recent := make([]string, 0, 32)

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
					aggregator.Add(event)

					if jsonMode {
						if err := writeJSONLine(stdout, watchEnvelope{Type: "event", Event: event}); err != nil {
							return err
						}
					} else {
						recent = appendRecent(recent, formatEvent(event), 30)
					}

					for _, signal := range signalEngine.Observe(event) {
						if !severityFilter.match(signal.Severity) {
							continue
						}
						if jsonMode {
							s := signal
							if err := writeJSONLine(stdout, watchEnvelope{Type: "signal", Signal: &s}); err != nil {
								return err
							}
						} else {
							recent = appendRecent(recent, formatSignal(signal), 30)
						}
					}
				case <-refreshTicker.C:
					for _, signal := range signalEngine.Tick() {
						if !severityFilter.match(signal.Severity) {
							continue
						}
						if jsonMode {
							s := signal
							if err := writeJSONLine(stdout, watchEnvelope{Type: "signal", Signal: &s}); err != nil {
								return err
							}
						} else {
							recent = appendRecent(recent, formatSignal(signal), 30)
						}
					}

					if jsonMode {
						continue
					}
					snapshot := aggregator.Snapshot()
					if err := renderWatch(stdout, snapshot, recent); err != nil {
						return err
					}
				}
			}
		},
	}

	cmd.Flags().StringSliceVar(&files, "file", nil, "JSONL event file(s) to watch")
	cmd.Flags().StringSliceVar(&sprites, "sprite", nil, "filter by sprite name")
	cmd.Flags().StringSliceVar(&types, "type", nil, "filter by event type")
	cmd.Flags().StringSliceVar(&severities, "severity", nil, "filter emitted signals by severity (info,warning,critical)")
	cmd.Flags().StringVar(&sinceRaw, "since", "", "include events since duration or RFC3339 timestamp")
	cmd.Flags().StringVar(&untilRaw, "until", "", "include events until RFC3339 timestamp")
	cmd.Flags().BoolVar(&jsonMode, "json", false, "emit JSONL output for machine consumption")
	cmd.Flags().DurationVar(&pollInterval, "poll-interval", events.DefaultWatchPollInterval, "file tail polling interval")
	cmd.Flags().DurationVar(&refreshInterval, "refresh", defaultWatchRefresh, "pretty output refresh interval")
	cmd.Flags().BoolVar(&startAtEnd, "start-at-end", true, "ignore existing lines and watch for new appends")
	cmd.Flags().BoolVar(&once, "once", false, "scan once and exit")

	return cmd
}

func drainWatchBatch(
	stdout io.Writer,
	sub <-chan events.Event,
	aggregator *events.Aggregator,
	engine *events.SignalEngine,
	jsonMode bool,
	severity severityMatcher,
) error {
	recent := make([]string, 0, 32)
	for {
		select {
		case event := <-sub:
			if event == nil {
				if jsonMode {
					snapshot := aggregator.Snapshot()
					return writeJSONLine(stdout, watchEnvelope{Type: "snapshot", Snapshot: &snapshot})
				}
				return renderWatch(stdout, aggregator.Snapshot(), recent)
			}
			aggregator.Add(event)
			if jsonMode {
				if err := writeJSONLine(stdout, watchEnvelope{Type: "event", Event: event}); err != nil {
					return err
				}
			} else {
				recent = appendRecent(recent, formatEvent(event), 30)
			}

			for _, signal := range engine.Observe(event) {
				if !severity.match(signal.Severity) {
					continue
				}
				if jsonMode {
					s := signal
					if err := writeJSONLine(stdout, watchEnvelope{Type: "signal", Signal: &s}); err != nil {
						return err
					}
				} else {
					recent = appendRecent(recent, formatSignal(signal), 30)
				}
			}
		default:
			for _, signal := range engine.Tick() {
				if !severity.match(signal.Severity) {
					continue
				}
				if jsonMode {
					s := signal
					if err := writeJSONLine(stdout, watchEnvelope{Type: "signal", Signal: &s}); err != nil {
						return err
					}
				} else {
					recent = appendRecent(recent, formatSignal(signal), 30)
				}
			}
			if jsonMode {
				snapshot := aggregator.Snapshot()
				return writeJSONLine(stdout, watchEnvelope{Type: "snapshot", Snapshot: &snapshot})
			}
			return renderWatch(stdout, aggregator.Snapshot(), recent)
		}
	}
}

func renderWatch(stdout io.Writer, snapshot events.Snapshot, recent []string) error {
	lines := make([]string, 0, 64)
	lines = append(lines, "\033[H\033[2J")
	lines = append(lines, fmt.Sprintf("bb watch  %s", time.Now().UTC().Format(time.RFC3339)))
	lines = append(lines, fmt.Sprintf("window: %s -> %s", snapshot.Start.Format(time.RFC3339), snapshot.End.Format(time.RFC3339)))
	lines = append(lines, fmt.Sprintf(
		"events=%d sprites=%d events/min=%.2f error_rate=%.2f%% uptime=%.2f%%",
		snapshot.TotalEvents,
		snapshot.UniqueSprites,
		snapshot.EventsPerMin,
		snapshot.ErrorRate*100,
		snapshot.Uptime*100,
	))

	if len(snapshot.BySprite) > 0 {
		lines = append(lines, "")
		lines = append(lines, "sprites:")
		names := make([]string, 0, len(snapshot.BySprite))
		for name := range snapshot.BySprite {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			stats := snapshot.BySprite[name]
			lines = append(lines, fmt.Sprintf(
				"  %-16s events=%-4d error_rate=%5.1f%% uptime=%5.1f%% last=%s",
				name,
				stats.TotalEvents,
				stats.ErrorRate*100,
				stats.Uptime*100,
				stats.LastEventAt.Format("15:04:05"),
			))
		}
	}

	lines = append(lines, "")
	lines = append(lines, "recent:")
	if len(recent) == 0 {
		lines = append(lines, "  (no events yet)")
	} else {
		for _, item := range recent {
			lines = append(lines, "  "+item)
		}
	}

	_, err := io.WriteString(stdout, strings.Join(lines, "\n")+"\n")
	return err
}

func formatEvent(event events.Event) string {
	return fmt.Sprintf("%s  %-12s  %-9s", event.Timestamp().UTC().Format("15:04:05"), event.Sprite(), event.Kind())
}

func formatSignal(signal events.Signal) string {
	return fmt.Sprintf(
		"%s  %-12s  %-8s  %s",
		signal.At.UTC().Format("15:04:05"),
		signal.Source,
		string(signal.Severity),
		signal.Description,
	)
}

func appendRecent(items []string, line string, max int) []string {
	if max <= 0 {
		max = 1
	}
	items = append(items, line)
	if len(items) <= max {
		return items
	}
	return append([]string(nil), items[len(items)-max:]...)
}

type severityMatcher map[events.Severity]struct{}

func (m severityMatcher) match(severity events.Severity) bool {
	if len(m) == 0 {
		return true
	}
	_, ok := m[severity]
	return ok
}

func buildSeverityFilter(raw []string) (severityMatcher, error) {
	out := make(severityMatcher)
	for _, item := range raw {
		for _, part := range splitCSV(item) {
			switch strings.ToLower(strings.TrimSpace(part)) {
			case "info":
				out[events.SeverityInfo] = struct{}{}
			case "warning", "warn":
				out[events.SeverityWarning] = struct{}{}
			case "critical":
				out[events.SeverityCritical] = struct{}{}
			case "":
				continue
			default:
				return nil, fmt.Errorf("watch: invalid severity %q", part)
			}
		}
	}
	return out, nil
}

func writeJSONLine(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
