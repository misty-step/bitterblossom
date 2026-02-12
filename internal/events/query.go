package events

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	pkgevents "github.com/misty-step/bitterblossom/pkg/events"
)

// QueryConfig controls event query construction.
type QueryConfig struct {
	Dir string
}

// Query supports filtered reads over daily JSONL event files.
type Query struct {
	dir string
}

// NewQuery creates an event query service.
func NewQuery(cfg QueryConfig) (*Query, error) {
	dir := strings.TrimSpace(cfg.Dir)
	if dir == "" {
		dir = DefaultDir()
	}
	return &Query{dir: filepath.Clean(dir)}, nil
}

// Dir returns the configured event directory.
func (q *Query) Dir() string {
	if q == nil {
		return ""
	}
	return q.dir
}

// Read returns events sorted by timestamp ascending.
func (q *Query) Read(opts QueryOptions) ([]Event, error) {
	if q == nil {
		return nil, fmt.Errorf("events: query is nil")
	}
	if !opts.Since.IsZero() && !opts.Until.IsZero() && opts.Until.Before(opts.Since) {
		return nil, fmt.Errorf("events: until before since")
	}

	entries, err := os.ReadDir(q.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Event{}, nil
		}
		return nil, fmt.Errorf("events: list %s: %w", q.dir, err)
	}

	paths := listDailyEventPaths(entries, q.dir, opts.Since, opts.Until)
	out := make([]Event, 0, 128)
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("events: open %s: %w", path, err)
		}
		items, readErr := pkgevents.ReadAll(file)
		closeErr := file.Close()
		if readErr != nil {
			return nil, fmt.Errorf("events: read %s: %w", path, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("events: close %s: %w", path, closeErr)
		}

		for _, event := range items {
			if opts.Issue > 0 && issueFor(event) != opts.Issue {
				continue
			}
			if opts.Filter != nil && !opts.Filter(event) {
				continue
			}
			out = append(out, event)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp().Before(out[j].Timestamp())
	})
	return out, nil
}

func listDailyEventPaths(entries []fs.DirEntry, dir string, since, until time.Time) []string {
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		dayRaw := strings.TrimSuffix(name, ".jsonl")
		day, err := time.Parse(dailyLayout, dayRaw)
		if err != nil {
			continue
		}
		day = day.UTC()
		if !since.IsZero() && day.Before(truncateUTCDate(since)) {
			continue
		}
		if !until.IsZero() && day.After(truncateUTCDate(until)) {
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
	}
	sort.Strings(paths)
	return paths
}

func truncateUTCDate(ts time.Time) time.Time {
	if ts.IsZero() {
		return time.Time{}
	}
	t := ts.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func issueFor(event Event) int {
	switch typed := event.(type) {
	case pkgevents.DispatchEvent:
		return typed.Issue
	case *pkgevents.DispatchEvent:
		return typed.Issue
	case pkgevents.DoneEvent:
		return typed.Issue
	case *pkgevents.DoneEvent:
		return typed.Issue
	case pkgevents.ProgressEvent:
		return typed.Issue
	case *pkgevents.ProgressEvent:
		return typed.Issue
	case pkgevents.HeartbeatEvent:
		return typed.Issue
	case *pkgevents.HeartbeatEvent:
		return typed.Issue
	case pkgevents.BlockedEvent:
		return typed.Issue
	case *pkgevents.BlockedEvent:
		return typed.Issue
	case pkgevents.ErrorEvent:
		return typed.Issue
	case *pkgevents.ErrorEvent:
		return typed.Issue
	case pkgevents.ProvisionEvent:
		return typed.Issue
	case *pkgevents.ProvisionEvent:
		return typed.Issue
	default:
		return 0
	}
}

