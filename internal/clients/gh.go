package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

// PullRequest is the subset of fields used by pr-shepherd.
type PullRequest struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Repo      string `json:"repo"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	HTMLURL   string `json:"html_url"`
}

// GitHubClient wraps gh CLI operations.
type GitHubClient interface {
	SearchOpenPRs(ctx context.Context, org, author string, perPage int) ([]PullRequest, error)
	PRChecks(ctx context.Context, org, repo string, number int) (string, error)
	LastReviewState(ctx context.Context, org, repo string, number int) (string, error)
}

// GHCLI implements GitHubClient.
type GHCLI struct {
	Bin    string
	Runner Runner
}

// NewGHCLI builds a GHCLI.
func NewGHCLI(r Runner, binary string) *GHCLI {
	if binary == "" {
		binary = "gh"
	}
	return &GHCLI{Bin: binary, Runner: r}
}

func (g *GHCLI) run(ctx context.Context, args ...string) (string, error) {
	out, _, err := g.Runner.Run(ctx, g.Bin, args...)
	if err != nil {
		return out, err
	}
	return out, nil
}

// SearchOpenPRs returns open PRs for an org/author.
func (g *GHCLI) SearchOpenPRs(ctx context.Context, org, author string, perPage int) ([]PullRequest, error) {
	if perPage <= 0 {
		perPage = 50
	}
	query := fmt.Sprintf("org:%s is:pr is:open author:%s", org, author)
	out, err := g.run(ctx,
		"api", "search/issues",
		"--method", "GET",
		"-f", "q="+query,
		"-f", fmt.Sprintf("per_page=%d", perPage),
	)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Items []struct {
			Number        int    `json:"number"`
			Title         string `json:"title"`
			RepositoryURL string `json:"repository_url"`
			CreatedAt     string `json:"created_at"`
			UpdatedAt     string `json:"updated_at"`
			HTMLURL       string `json:"html_url"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	prs := make([]PullRequest, 0, len(raw.Items))
	for _, item := range raw.Items {
		repo := path.Base(item.RepositoryURL)
		prs = append(prs, PullRequest{
			Number:    item.Number,
			Title:     item.Title,
			Repo:      repo,
			CreatedAt: item.CreatedAt,
			UpdatedAt: item.UpdatedAt,
			HTMLURL:   item.HTMLURL,
		})
	}
	return prs, nil
}

// PRChecks returns raw checks output from gh pr checks.
func (g *GHCLI) PRChecks(ctx context.Context, org, repo string, number int) (string, error) {
	fullRepo := strings.Trim(org+"/"+repo, "/")
	return g.run(ctx, "pr", "checks", fmt.Sprintf("%d", number), "--repo", fullRepo)
}

// LastReviewState returns the latest non-commented review state.
func (g *GHCLI) LastReviewState(ctx context.Context, org, repo string, number int) (string, error) {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/reviews", org, repo, number)
	out, err := g.run(ctx, "api", endpoint)
	if err != nil {
		return "none", err
	}
	var reviews []struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal([]byte(out), &reviews); err != nil {
		return "none", fmt.Errorf("decode reviews: %w", err)
	}
	state := "none"
	for _, review := range reviews {
		if review.State == "" || review.State == "COMMENTED" {
			continue
		}
		state = review.State
	}
	return state, nil
}
