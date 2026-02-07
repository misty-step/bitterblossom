package clients

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// RepoProgress summarizes git activity for one repo.
type RepoProgress struct {
	Path            string `json:"path"`
	Name            string `json:"name"`
	Branch          string `json:"branch"`
	Ahead           int    `json:"ahead"`
	HasUncommitted  bool   `json:"has_uncommitted"`
	LastCommitEpoch int64  `json:"last_commit_epoch"`
}

// GitClient wraps git operations.
type GitClient interface {
	ListRepos(ctx context.Context, workspace string) ([]string, error)
	CurrentBranch(ctx context.Context, repoPath string) (string, error)
	CommitsAhead(ctx context.Context, repoPath, upstream string) (int, error)
	HasUncommittedChanges(ctx context.Context, repoPath string) (bool, error)
	LastCommitEpoch(ctx context.Context, repoPath string) (int64, error)
	Push(ctx context.Context, repoPath, remote, branch string) error
	CollectProgress(ctx context.Context, workspace string) ([]RepoProgress, error)
}

// GitCLI implements GitClient.
type GitCLI struct {
	Bin    string
	Runner Runner
}

// NewGitCLI builds a GitCLI.
func NewGitCLI(r Runner, binary string) *GitCLI {
	if binary == "" {
		binary = "git"
	}
	return &GitCLI{Bin: binary, Runner: r}
}

func (g *GitCLI) runGit(ctx context.Context, args ...string) (string, error) {
	out, _, err := g.Runner.Run(ctx, g.Bin, args...)
	if err != nil {
		return out, err
	}
	return strings.TrimSpace(out), nil
}

// ListRepos finds direct child directories in workspace that have .git.
func (g *GitCLI) ListRepos(ctx context.Context, workspace string) ([]string, error) {
	_ = ctx
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return nil, fmt.Errorf("read workspace: %w", err)
	}
	var repos []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		repoPath := filepath.Join(workspace, entry.Name())
		if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
			repos = append(repos, repoPath)
		}
	}
	return repos, nil
}

// CurrentBranch returns the checked-out branch.
func (g *GitCLI) CurrentBranch(ctx context.Context, repoPath string) (string, error) {
	out, err := g.runGit(ctx, "-C", repoPath, "branch", "--show-current")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// CommitsAhead returns commit count for upstream..HEAD.
func (g *GitCLI) CommitsAhead(ctx context.Context, repoPath, upstream string) (int, error) {
	if upstream == "" {
		upstream = "origin/master"
	}
	out, err := g.runGit(ctx, "-C", repoPath, "rev-list", "--count", upstream+"..HEAD")
	if err != nil {
		return 0, err
	}
	count, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, fmt.Errorf("parse ahead count %q: %w", out, err)
	}
	return count, nil
}

// HasUncommittedChanges returns true when status is dirty.
func (g *GitCLI) HasUncommittedChanges(ctx context.Context, repoPath string) (bool, error) {
	out, err := g.runGit(ctx, "-C", repoPath, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// LastCommitEpoch returns the timestamp of latest commit in Unix seconds.
func (g *GitCLI) LastCommitEpoch(ctx context.Context, repoPath string) (int64, error) {
	out, err := g.runGit(ctx, "-C", repoPath, "log", "-1", "--format=%ct")
	if err != nil {
		return 0, err
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse commit epoch %q: %w", out, err)
	}
	return ts, nil
}

// Push pushes a branch to remote.
func (g *GitCLI) Push(ctx context.Context, repoPath, remote, branch string) error {
	if remote == "" {
		remote = "origin"
	}
	if branch == "" {
		return fmt.Errorf("branch required")
	}
	_, _, err := g.Runner.Run(ctx, g.Bin, "-C", repoPath, "push", remote, branch)
	return err
}

// CollectProgress aggregates progress snapshots for all repos.
func (g *GitCLI) CollectProgress(ctx context.Context, workspace string) ([]RepoProgress, error) {
	repos, err := g.ListRepos(ctx, workspace)
	if err != nil {
		return nil, err
	}
	progress := make([]RepoProgress, 0, len(repos))
	for _, repo := range repos {
		branch, berr := g.CurrentBranch(ctx, repo)
		if berr != nil || branch == "" {
			continue
		}
		ahead, _ := g.CommitsAhead(ctx, repo, "origin/"+branch)
		dirty, _ := g.HasUncommittedChanges(ctx, repo)
		last, _ := g.LastCommitEpoch(ctx, repo)
		progress = append(progress, RepoProgress{
			Path:            repo,
			Name:            filepath.Base(repo),
			Branch:          branch,
			Ahead:           ahead,
			HasUncommitted:  dirty,
			LastCommitEpoch: last,
		})
	}
	return progress, nil
}
