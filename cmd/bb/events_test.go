package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	storeevents "github.com/misty-step/bitterblossom/internal/events"
	pkgevents "github.com/misty-step/bitterblossom/pkg/events"
)

func TestEventsCommand(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 2, 12, 16, 0, 0, 0, time.UTC)

	cases := []struct {
		name            string
		events          []pkgevents.Event
		args            []string
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "json output includes envelope and issue",
			events: []pkgevents.Event{
				pkgevents.DispatchEvent{
					Meta: pkgevents.Meta{
						TS:         base,
						SpriteName: "bramble",
						EventKind:  pkgevents.KindDispatch,
						Issue:      13,
					},
					Task: "ship issue 13",
				},
			},
			args: []string{
				"--issue", "13",
				"--json",
			},
			wantContains: []string{
				`"type":"event"`,
				`"issue":13`,
			},
		},
		{
			name: "filters by type and sprite",
			events: []pkgevents.Event{
				pkgevents.ProgressEvent{
					Meta: pkgevents.Meta{
						TS:         base,
						SpriteName: "bramble",
						EventKind:  pkgevents.KindProgress,
						Issue:      13,
					},
					Activity: "tool_call",
					Detail:   "apply_patch",
				},
				pkgevents.ErrorEvent{
					Meta: pkgevents.Meta{
						TS:         base.Add(time.Minute),
						SpriteName: "thorn",
						EventKind:  pkgevents.KindError,
						Issue:      13,
					},
					Message: "failed",
				},
			},
			args: []string{
				"--sprite", "bramble",
				"--type", "progress",
			},
			wantContains: []string{
				"bramble",
				"progress",
			},
			wantNotContains: []string{
				"thorn",
				"error",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			logger, err := storeevents.NewLogger(storeevents.LoggerConfig{Dir: dir})
			if err != nil {
				t.Fatalf("NewLogger() error = %v", err)
			}

			for _, event := range tc.events {
				if err := logger.Log(event); err != nil {
					t.Fatalf("Log() error = %v", err)
				}
			}

			var out bytes.Buffer
			args := append([]string{"events", "--dir", dir}, tc.args...)
			if err := run(context.Background(), args, &out, &bytes.Buffer{}); err != nil {
				t.Fatalf("run(events) error = %v", err)
			}

			got := out.String()
			for _, needle := range tc.wantContains {
				if !strings.Contains(got, needle) {
					t.Fatalf("output missing %q: %q", needle, got)
				}
			}
			for _, needle := range tc.wantNotContains {
				if strings.Contains(got, needle) {
					t.Fatalf("output unexpectedly contains %q: %q", needle, got)
				}
			}
		})
	}
}
