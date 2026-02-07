package clients

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type stubRunner struct {
	responses map[string]string
}

func (s stubRunner) Run(_ context.Context, name string, args ...string) (string, int, error) {
	key := name
	for _, arg := range args {
		key += " " + arg
	}
	if out, ok := s.responses[key]; ok {
		return out, 0, nil
	}
	return "", 1, errors.New("missing")
}

func TestListRepos(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo1")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "notgit"), 0o755); err != nil {
		t.Fatal(err)
	}
	git := NewGitCLI(stubRunner{}, "git")
	repos, err := git.ListRepos(context.Background(), root)
	if err != nil {
		t.Fatalf("ListRepos error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo got %d", len(repos))
	}
}

func TestCollectProgress(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo1")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	runner := stubRunner{responses: map[string]string{
		"git -C " + repo + " branch --show-current":              "main",
		"git -C " + repo + " rev-list --count origin/main..HEAD": "2",
		"git -C " + repo + " status --porcelain":                 "M main.go",
		"git -C " + repo + " log -1 --format=%ct":                "1700000000",
	}}
	git := NewGitCLI(runner, "git")
	progress, err := git.CollectProgress(context.Background(), root)
	if err != nil {
		t.Fatalf("CollectProgress error: %v", err)
	}
	if len(progress) != 1 {
		t.Fatalf("expected 1 progress row, got %d", len(progress))
	}
	if progress[0].Ahead != 2 {
		t.Fatalf("ahead mismatch: %d", progress[0].Ahead)
	}
	if !progress[0].HasUncommitted {
		t.Fatal("expected dirty repo")
	}
}

func TestGitPush(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo1")
	_ = os.MkdirAll(filepath.Join(repo, ".git"), 0o755)
	runner := stubRunner{responses: map[string]string{
		"git -C " + repo + " push origin main": "",
	}}
	git := NewGitCLI(runner, "git")
	if err := git.Push(context.Background(), repo, "origin", "main"); err != nil {
		t.Fatalf("Push returned error: %v", err)
	}
}
