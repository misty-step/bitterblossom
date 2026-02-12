package events

import (
	"sync"
	"testing"
	"time"

	pkgevents "github.com/misty-step/bitterblossom/pkg/events"
)

func TestQueryScenarios(t *testing.T) {
	t.Parallel()

	firstDay := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)
	secondDay := firstDay.Add(24 * time.Hour)
	filterBase := time.Date(2026, 2, 11, 9, 0, 0, 0, time.UTC)
	filter := pkgevents.Chain(
		pkgevents.BySprite("bramble"),
		pkgevents.ByKind(pkgevents.KindProgress),
	)

	logEvent := func(t *testing.T, logger *Logger, event Event) {
		t.Helper()
		if err := logger.Log(event); err != nil {
			t.Fatalf("Log() error = %v", err)
		}
	}

	cases := []struct {
		name      string
		setup     func(t *testing.T, logger *Logger)
		opts      QueryOptions
		wantCount int
		check     func(t *testing.T, events []Event)
	}{
		{
			name: "daily rotation returns both days",
			setup: func(t *testing.T, logger *Logger) {
				logEvent(t, logger, pkgevents.DispatchEvent{
					Meta: pkgevents.Meta{
						TS:         firstDay,
						SpriteName: "bramble",
						EventKind:  pkgevents.KindDispatch,
						Issue:      13,
					},
					Task: "issue-13",
					Repo: "misty-step/bitterblossom",
				})
				logEvent(t, logger, pkgevents.ErrorEvent{
					Meta: pkgevents.Meta{
						TS:         secondDay,
						SpriteName: "bramble",
						EventKind:  pkgevents.KindError,
						Issue:      13,
					},
					Message: "boom",
				})
			},
			opts:      QueryOptions{},
			wantCount: 2,
		},
		{
			name: "filters by sprite kind issue and range",
			setup: func(t *testing.T, logger *Logger) {
				logEvent(t, logger, pkgevents.DispatchEvent{
					Meta: pkgevents.Meta{
						TS:         filterBase,
						SpriteName: "bramble",
						EventKind:  pkgevents.KindDispatch,
						Issue:      13,
					},
					Task: "issue-13",
				})
				logEvent(t, logger, pkgevents.ProgressEvent{
					Meta: pkgevents.Meta{
						TS:         filterBase.Add(1 * time.Minute),
						SpriteName: "bramble",
						EventKind:  pkgevents.KindProgress,
						Issue:      13,
					},
					Activity: "tool_call",
					Detail:   "apply_patch",
				})
				logEvent(t, logger, pkgevents.ProgressEvent{
					Meta: pkgevents.Meta{
						TS:         filterBase.Add(2 * time.Minute),
						SpriteName: "thorn",
						EventKind:  pkgevents.KindProgress,
						Issue:      98,
					},
					Activity: "tool_call",
					Detail:   "exec",
				})
			},
			opts: QueryOptions{
				Filter: filter,
				Since:  filterBase.Add(30 * time.Second),
				Until:  filterBase.Add(90 * time.Second),
				Issue:  13,
			},
			wantCount: 1,
			check: func(t *testing.T, events []Event) {
				t.Helper()
				if events[0].Kind() != pkgevents.KindProgress {
					t.Fatalf("event kind = %q, want %q", events[0].Kind(), pkgevents.KindProgress)
				}
			},
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			logger, err := NewLogger(LoggerConfig{Dir: dir})
			if err != nil {
				t.Fatalf("NewLogger() error = %v", err)
			}
			tt.setup(t, logger)

			query, err := NewQuery(QueryConfig{Dir: dir})
			if err != nil {
				t.Fatalf("NewQuery() error = %v", err)
			}

			events, err := query.Read(tt.opts)
			if err != nil {
				t.Fatalf("Read() error = %v", err)
			}
			if len(events) != tt.wantCount {
				t.Fatalf("Read() count = %d, want %d", len(events), tt.wantCount)
			}
			if tt.check != nil {
				tt.check(t, events)
			}
		})
	}
}

func TestLoggerConcurrentWrites(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logger, err := NewLogger(LoggerConfig{Dir: dir})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	const writers = 16
	const eventsPerWriter = 25
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for n := 0; n < eventsPerWriter; n++ {
				ts := time.Date(2026, 2, 12, 10, idx, n, 0, time.UTC)
				err := logger.Log(pkgevents.ProgressEvent{
					Meta: pkgevents.Meta{
						TS:         ts,
						SpriteName: "bramble",
						EventKind:  pkgevents.KindProgress,
						Issue:      13,
					},
					Activity: "tool_call",
					Detail:   "write_file",
				})
				if err != nil {
					t.Errorf("Log() error = %v", err)
					return
				}
			}
		}(i)
	}
	wg.Wait()

	query, err := NewQuery(QueryConfig{Dir: dir})
	if err != nil {
		t.Fatalf("NewQuery() error = %v", err)
	}
	got, err := query.Read(QueryOptions{})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	want := writers * eventsPerWriter
	if len(got) != want {
		t.Fatalf("Read() count = %d, want %d", len(got), want)
	}
}
