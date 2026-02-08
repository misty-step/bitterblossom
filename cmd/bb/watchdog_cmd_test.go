package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	watchdogsvc "github.com/misty-step/bitterblossom/internal/watchdog"
)

type fakeWatchdogRunner struct {
	lastReq watchdogsvc.Request
	report  watchdogsvc.Report
	err     error
}

func (f *fakeWatchdogRunner) Check(_ context.Context, req watchdogsvc.Request) (watchdogsvc.Report, error) {
	f.lastReq = req
	return f.report, f.err
}

func TestWatchdogCommandReturnsExitErrorOnAttention(t *testing.T) {
	t.Parallel()

	runner := &fakeWatchdogRunner{
		report: watchdogsvc.Report{
			GeneratedAt: time.Date(2026, time.February, 8, 13, 0, 0, 0, time.UTC),
			Execute:     false,
			StaleAfter:  "2h0m0s",
			Summary: watchdogsvc.Summary{
				Total:          1,
				NeedsAttention: 1,
				Dead:           1,
			},
			Sprites: []watchdogsvc.SpriteReport{
				{
					Sprite: "bramble",
					State:  watchdogsvc.StateDead,
					Action: watchdogsvc.ActionResult{Type: watchdogsvc.ActionRedispatch},
				},
			},
		},
	}

	deps := watchdogDeps{
		newRemote: func(binary, org string) *spriteCLIRemote {
			return &spriteCLIRemote{}
		},
		newService: func(cfg watchdogsvc.Config) (watchdogRunner, error) {
			return runner, nil
		},
	}

	cmd := newWatchdogCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--sprite", "bramble"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected non-zero exit error")
	}
	var coded *exitError
	if !errors.As(err, &coded) {
		t.Fatalf("expected *exitError, got %T", err)
	}
	if coded.Code != 1 {
		t.Fatalf("exit code = %d, want 1", coded.Code)
	}
	if !strings.Contains(out.String(), "bramble") {
		t.Fatalf("output missing row data: %q", out.String())
	}
}

func TestWatchdogCommandJSON(t *testing.T) {
	t.Parallel()

	runner := &fakeWatchdogRunner{
		report: watchdogsvc.Report{
			GeneratedAt: time.Date(2026, time.February, 8, 13, 0, 0, 0, time.UTC),
			Execute:     true,
			StaleAfter:  "2h0m0s",
			Summary: watchdogsvc.Summary{
				Total:  1,
				Active: 1,
			},
			Sprites: []watchdogsvc.SpriteReport{
				{
					Sprite: "fern",
					State:  watchdogsvc.StateActive,
				},
			},
		},
	}

	deps := watchdogDeps{
		newRemote: func(binary, org string) *spriteCLIRemote {
			return &spriteCLIRemote{}
		},
		newService: func(cfg watchdogsvc.Config) (watchdogRunner, error) {
			return runner, nil
		},
	}

	cmd := newWatchdogCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--json", "--execute", "--sprite", "fern"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), `"sprite": "fern"`) {
		t.Fatalf("output = %q, expected json row", out.String())
	}
	if !runner.lastReq.Execute {
		t.Fatalf("runner.lastReq.Execute = false, want true")
	}
}
