package events

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"
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
	if opts.Limit < 0 {
		return nil, fmt.Errorf("events: limit must be >= 0")
	}

	entries, err := os.ReadDir(q.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Event{}, nil
		}
		return nil, fmt.Errorf("events: list %s: %w", q.dir, err)
	}

	paths := listDailyEventPaths(entries, q.dir, opts.Since, opts.Until)
	out := make([]Event, 0, 32)

	readPath := func(path string) (retErr error) {
		lockPath := path + ".lock"
		lock := flock.New(lockPath)
		if err := lock.RLock(); err != nil {
			return fmt.Errorf("events: lock %s: %w", lockPath, err)
		}
		defer func() {
			if unlockErr := lock.Unlock(); unlockErr != nil {
				retErr = errors.Join(retErr, fmt.Errorf("events: unlock %s: %w", lockPath, unlockErr))
			}
		}()

		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("events: open %s: %w", path, err)
		}
		defer func() {
			if closeErr := file.Close(); closeErr != nil {
				retErr = errors.Join(retErr, fmt.Errorf("events: close %s: %w", path, closeErr))
			}
		}()

		items, readErr := pkgevents.ReadAll(file)
		if readErr != nil {
			return fmt.Errorf("events: read %s: %w", path, readErr)
		}

		for _, event := range items {
			if !opts.Since.IsZero() && event.Timestamp().Before(opts.Since) {
				continue
			}
			if !opts.Until.IsZero() && event.Timestamp().After(opts.Until) {
				continue
			}
			if opts.Issue > 0 && event.GetIssue() != opts.Issue {
				continue
			}
			if opts.Filter != nil && !opts.Filter(event) {
				continue
			}
			out = append(out, event)
		}
		return nil
	}

	if opts.Limit > 0 {
		// Read newest daily files first so limit can stop scanning older history.
		for i := len(paths) - 1; i >= 0; i-- {
			if err := readPath(paths[i]); err != nil {
				return nil, err
			}
			if len(out) >= opts.Limit {
				break
			}
		}
	} else {
		for _, path := range paths {
			if err := readPath(path); err != nil {
				return nil, err
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp().Before(out[j].Timestamp())
	})

	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[len(out)-opts.Limit:]
	}
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
