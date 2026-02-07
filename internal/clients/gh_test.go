package clients

import (
	"context"
	"errors"
	"testing"
)

type ghRunner struct {
	out string
	err error
}

func (g ghRunner) Run(context.Context, string, ...string) (string, int, error) {
	if g.err != nil {
		return g.out, 1, g.err
	}
	return g.out, 0, nil
}

func TestSearchOpenPRs(t *testing.T) {
	payload := `{"items":[{"number":1,"title":"T","repository_url":"https://api.github.com/repos/misty-step/heartbeat","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T01:00:00Z","html_url":"https://github.com/misty-step/heartbeat/pull/1"}]}`
	gh := NewGHCLI(ghRunner{out: payload}, "gh")
	prs, err := gh.SearchOpenPRs(context.Background(), "misty-step", "bot", 50)
	if err != nil {
		t.Fatalf("SearchOpenPRs error: %v", err)
	}
	if len(prs) != 1 || prs[0].Repo != "heartbeat" {
		t.Fatalf("unexpected prs: %+v", prs)
	}
}

func TestLastReviewState(t *testing.T) {
	payload := `[{"state":"COMMENTED"},{"state":"APPROVED"}]`
	gh := NewGHCLI(ghRunner{out: payload}, "gh")
	state, err := gh.LastReviewState(context.Background(), "misty-step", "heartbeat", 1)
	if err != nil {
		t.Fatalf("LastReviewState error: %v", err)
	}
	if state != "APPROVED" {
		t.Fatalf("state mismatch: %s", state)
	}
}

func TestPRChecks(t *testing.T) {
	gh := NewGHCLI(ghRunner{out: "pass"}, "gh")
	out, err := gh.PRChecks(context.Background(), "misty-step", "heartbeat", 10)
	if err != nil {
		t.Fatalf("PRChecks error: %v", err)
	}
	if out != "pass" {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestSearchOpenPRsError(t *testing.T) {
	gh := NewGHCLI(ghRunner{err: errors.New("boom")}, "gh")
	if _, err := gh.SearchOpenPRs(context.Background(), "misty-step", "bot", 50); err == nil {
		t.Fatal("expected error")
	}
}
