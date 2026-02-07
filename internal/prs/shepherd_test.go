package prs

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/internal/clients"
)

type fakeGH struct {
	prs    []clients.PullRequest
	checks map[int]string
	review map[int]string
}

func (f *fakeGH) SearchOpenPRs(context.Context, string, string, int) ([]clients.PullRequest, error) {
	return f.prs, nil
}
func (f *fakeGH) PRChecks(_ context.Context, _ string, _ string, number int) (string, error) {
	return f.checks[number], nil
}
func (f *fakeGH) LastReviewState(_ context.Context, _ string, _ string, number int) (string, error) {
	return f.review[number], nil
}

func TestClassifyCI(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"all pass", "passing"},
		{"pending job", "pending"},
		{"FAIL fast", "failing"},
		{"", "unknown"},
	}
	for _, tc := range cases {
		if got := classifyCI(tc.in); got != tc.want {
			t.Fatalf("classifyCI(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestChooseAction(t *testing.T) {
	cases := []struct {
		ci    string
		rev   string
		stale int
		want  string
	}{
		{"failing", "none", 1, "fix_ci"},
		{"passing", "CHANGES_REQUESTED", 1, "address_reviews"},
		{"passing", "APPROVED", 1, "ready_for_final_review"},
		{"pending", "none", 25, "stale_investigate"},
		{"pending", "none", 1, "none"},
	}
	for _, tc := range cases {
		if got := chooseAction(tc.ci, tc.rev, tc.stale); got != tc.want {
			t.Fatalf("chooseAction got %q want %q", got, tc.want)
		}
	}
}

func TestShepherdRun(t *testing.T) {
	now := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	gh := &fakeGH{
		prs: []clients.PullRequest{{
			Number:    12,
			Title:     "Fix test",
			Repo:      "heartbeat",
			CreatedAt: "2026-01-02T08:00:00Z",
			UpdatedAt: "2026-01-02T09:00:00Z",
			HTMLURL:   "https://example/pr/12",
		}},
		checks: map[int]string{12: "pass"},
		review: map[int]string{12: "APPROVED"},
	}
	buf := &bytes.Buffer{}
	s := Shepherd{GH: gh, Out: buf, Now: func() time.Time { return now }}
	reports, err := s.Run(context.Background(), Config{Org: "misty-step", Author: "kaylee-mistystep", DryRun: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].Action != "ready_for_final_review" {
		t.Fatalf("action mismatch: %s", reports[0].Action)
	}
	if !strings.Contains(buf.String(), "ready_for_final_review") {
		t.Fatalf("expected output JSON to include action: %s", buf.String())
	}
}

var _ clients.GitHubClient = (*fakeGH)(nil)
