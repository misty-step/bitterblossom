package prs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/internal/clients"
)

// Config controls PR shepherd behavior.
type Config struct {
	Org      string
	Author   string
	PerPage  int
	DryRun   bool
	JSONOnly bool
}

// Report is one PR status/action row.
type Report struct {
	Repo       string `json:"repo"`
	PR         int    `json:"pr"`
	Title      string `json:"title"`
	CI         string `json:"ci"`
	Reviews    string `json:"reviews"`
	AgeHours   int    `json:"age_hours"`
	StaleHours int    `json:"stale_hours"`
	Action     string `json:"action"`
	URL        string `json:"url"`
	DryRun     bool   `json:"dry_run"`
}

// Shepherd evaluates open sprite-authored PRs.
type Shepherd struct {
	GH  clients.GitHubClient
	Out io.Writer
	Now func() time.Time
}

// Run collects PR status and required actions.
func (s *Shepherd) Run(ctx context.Context, cfg Config) ([]Report, error) {
	cfg = withDefaults(cfg)
	if s.GH == nil {
		return nil, fmt.Errorf("github client required")
	}
	if s.Out == nil {
		s.Out = os.Stdout
	}
	if s.Now == nil {
		s.Now = time.Now
	}

	prs, err := s.GH.SearchOpenPRs(ctx, cfg.Org, cfg.Author, cfg.PerPage)
	if err != nil {
		if cfg.JSONOnly {
			_, _ = fmt.Fprintln(s.Out, "[]")
		}
		return nil, err
	}

	reports := make([]Report, 0, len(prs))
	for _, pr := range prs {
		checks, _ := s.GH.PRChecks(ctx, cfg.Org, pr.Repo, pr.Number)
		review, _ := s.GH.LastReviewState(ctx, cfg.Org, pr.Repo, pr.Number)
		ci := classifyCI(checks)
		ageHours := hoursSince(pr.CreatedAt, s.Now())
		staleHours := hoursSince(pr.UpdatedAt, s.Now())
		action := chooseAction(ci, review, staleHours)
		reports = append(reports, Report{
			Repo:       pr.Repo,
			PR:         pr.Number,
			Title:      pr.Title,
			CI:         ci,
			Reviews:    review,
			AgeHours:   ageHours,
			StaleHours: staleHours,
			Action:     action,
			URL:        pr.HTMLURL,
			DryRun:     cfg.DryRun,
		})
	}

	enc := json.NewEncoder(s.Out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(reports); err != nil {
		return reports, err
	}

	return reports, nil
}

func withDefaults(cfg Config) Config {
	if cfg.Org == "" {
		cfg.Org = "misty-step"
	}
	if cfg.Author == "" {
		cfg.Author = "kaylee-mistystep"
	}
	if cfg.PerPage <= 0 {
		cfg.PerPage = 50
	}
	return cfg
}

func classifyCI(checks string) string {
	checks = strings.ToLower(checks)
	switch {
	case strings.Contains(checks, "fail"):
		return "failing"
	case strings.Contains(checks, "pass"):
		return "passing"
	case strings.Contains(checks, "pending"):
		return "pending"
	default:
		return "unknown"
	}
}

func chooseAction(ci, review string, staleHours int) string {
	switch {
	case ci == "failing":
		return "fix_ci"
	case review == "CHANGES_REQUESTED":
		return "address_reviews"
	case ci == "passing" && review != "CHANGES_REQUESTED":
		return "ready_for_final_review"
	case staleHours > 24:
		return "stale_investigate"
	default:
		return "none"
	}
}

func hoursSince(ts string, now time.Time) int {
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return 0
	}
	if parsed.After(now) {
		return 0
	}
	return int(now.Sub(parsed).Hours())
}
