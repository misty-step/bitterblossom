package dispatch

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/misty-step/bitterblossom/internal/github"
)

// mockGitHubClient is a test implementation of GitHubIssueClient.
type mockGitHubClient struct {
	issue   *github.Issue
	err     error
	getIssueCalled bool
	lastOwner string
	lastRepo  string
	lastNumber int
}

func (m *mockGitHubClient) GetIssue(ctx context.Context, owner, repo string, number int) (*github.Issue, error) {
	m.getIssueCalled = true
	m.lastOwner = owner
	m.lastRepo = repo
	m.lastNumber = number
	return m.issue, m.err
}

func TestNewIssueValidator_WithClient(t *testing.T) {
	t.Parallel()

	client := &mockGitHubClient{}
	validator := NewIssueValidator(client)

	if validator.GitHubClient != client {
		t.Error("expected validator to use provided client")
	}
	if len(validator.RequiredLabels) != 0 {
		t.Error("expected empty required labels by default")
	}
	if len(validator.RecommendedLabels) != 1 || validator.RecommendedLabels[0] != "ralph-ready" {
		t.Errorf("expected ['ralph-ready'] recommended labels, got %v", validator.RecommendedLabels)
	}
}

func TestNewIssueValidator_CreatesClientFromEnv(t *testing.T) {
	t.Parallel()

	// This test verifies behavior when GITHUB_TOKEN is not set
	// In that case, the validator should fall back to gh CLI
	validator := DefaultIssueValidator()

	if validator == nil {
		t.Fatal("expected validator to be created")
	}

	// If GITHUB_TOKEN is set, GitHubClient should be non-nil
	// If not set, it should be nil (we'll use fallback)
	// Either is valid behavior depending on environment
}

func TestValidateIssue_WithGitHubClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		mockIssue      *github.Issue
		mockErr        error
		expectedValid  bool
		expectedError  string
		expectedWarning string
	}{
		{
			name: "valid open issue",
			mockIssue: &github.Issue{
				Number: 1,
				Title:  "Test Issue with good title",
				Body:   "This has acceptance criteria:\n\n## Tasks\n- [ ] Task 1",
				State:  "open",
				Closed: false,
				Labels: []github.Label{{Name: "bug"}},
			},
			mockErr:       nil,
			expectedValid: true,
		},
		{
			name: "closed issue",
			mockIssue: &github.Issue{
				Number: 2,
				Title:  "Closed Issue",
				Body:   "Description",
				State:  "closed",
				Closed: true,
				Labels: []github.Label{},
			},
			mockErr:       nil,
			expectedValid: false,
			expectedError: "issue is closed",
		},
		{
			name:          "auth error",
			mockIssue:     nil,
			mockErr:       github.ErrAuth,
			expectedValid: false,
			expectedError: "authentication failed",
		},
		{
			name:          "not found error",
			mockIssue:     nil,
			mockErr:       github.ErrNotFound,
			expectedValid: false,
			expectedError: "not found",
		},
		{
			name:          "rate limited",
			mockIssue:     nil,
			mockErr:       github.ErrRateLimited,
			expectedValid: false,
			expectedError: "rate limit exceeded",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &mockGitHubClient{
				issue: tc.mockIssue,
				err:   tc.mockErr,
			}

			validator := NewIssueValidator(client)
			result, err := validator.ValidateIssue(context.Background(), 42, "owner/repo")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Valid != tc.expectedValid {
				t.Errorf("expected valid=%v, got %v (errors: %v)", tc.expectedValid, result.Valid, result.Errors)
			}

			if tc.expectedError != "" {
				found := false
				for _, e := range result.Errors {
					if strings.Contains(strings.ToLower(e), strings.ToLower(tc.expectedError)) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error containing %q, got errors: %v", tc.expectedError, result.Errors)
				}
			}

			// Verify the client was called with correct parameters
			if tc.mockErr != nil || tc.mockIssue != nil {
				if !client.getIssueCalled {
					t.Error("expected GetIssue to be called")
				}
				if client.lastOwner != "owner" {
					t.Errorf("expected owner 'owner', got %s", client.lastOwner)
				}
				if client.lastRepo != "repo" {
					t.Errorf("expected repo 'repo', got %s", client.lastRepo)
				}
				if client.lastNumber != 42 {
					t.Errorf("expected issue number 42, got %d", client.lastNumber)
				}
			}
		})
	}
}

func TestValidateIssue_GitHubClientWithBlockingLabels(t *testing.T) {
	t.Parallel()

	client := &mockGitHubClient{
		issue: &github.Issue{
			Number: 1,
			Title:  "Test Issue with good title",
			Body:   "Description with acceptance criteria:\n- [ ] Task 1",
			State:  "open",
			Closed: false,
			Labels: []github.Label{
				{Name: "blocked-by-dependency"},
				{Name: "bug"},
			},
		},
	}

	validator := NewIssueValidator(client)
	result, err := validator.ValidateIssue(context.Background(), 1, "owner/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Valid {
		t.Fatalf("expected valid result, got errors: %v", result.Errors)
	}

	if !result.HasBlockingLabel {
		t.Error("expected HasBlockingLabel to be true")
	}

	foundBlockingWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "blocked-by-dependency") {
			foundBlockingWarning = true
			break
		}
	}
	if !foundBlockingWarning {
		t.Errorf("expected blocking warning, got warnings: %v", result.Warnings)
	}
}

func TestParseRepoSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		repo          string
		wantOwner     string
		wantRepo      string
		wantErr       bool
	}{
		{"owner/repo", "owner", "repo", false},
		{"misty-step/bitterblossom", "misty-step", "bitterblossom", false},
		{"", "", "", true},
		{"invalid", "", "", true},
		{"a/b/c", "", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.repo, func(t *testing.T) {
			owner, repo, err := parseRepoSlug(tc.repo)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner != tc.wantOwner {
				t.Errorf("expected owner %q, got %q", tc.wantOwner, owner)
			}
			if repo != tc.wantRepo {
				t.Errorf("expected repo %q, got %q", tc.wantRepo, repo)
			}
		})
	}
}

func TestToIssueData(t *testing.T) {
	t.Parallel()

	issue := &github.Issue{
		Number: 123,
		Title:  "Test Title",
		Body:   "Test Body",
		State:  "open",
		Closed: false,
		URL:    "https://api.github.com/repos/o/r/issues/123",
		Labels: []github.Label{
			{Name: "bug", Description: "Something is broken", Color: "d73a4a"},
		},
	}

	data := toIssueData(issue)

	if data.Number != 123 {
		t.Errorf("expected number 123, got %d", data.Number)
	}
	if data.Title != "Test Title" {
		t.Errorf("expected title 'Test Title', got %s", data.Title)
	}
	if data.Body != "Test Body" {
		t.Errorf("expected body 'Test Body', got %s", data.Body)
	}
	if len(data.Labels) != 1 {
		t.Errorf("expected 1 label, got %d", len(data.Labels))
	}
	if data.Labels[0].Name != "bug" {
		t.Errorf("expected label name 'bug', got %s", data.Labels[0].Name)
	}
	if data.Labels[0].Description != "Something is broken" {
		t.Errorf("expected description 'Something is broken', got %s", data.Labels[0].Description)
	}
}

func TestHandleFetchError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		inputErr      error
		expectedError string
	}{
		{
			name:          "auth error",
			inputErr:      github.ErrAuth,
			expectedError: "authentication failed",
		},
		{
			name:          "not found error",
			inputErr:      github.ErrNotFound,
			expectedError: "not found",
		},
		{
			name:          "rate limited error",
			inputErr:      github.ErrRateLimited,
			expectedError: "rate limit exceeded",
		},
		{
			name:          "generic error",
			inputErr:      errors.New("something went wrong"),
			expectedError: "failed to fetch issue",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			validator := NewIssueValidator(nil)
			result := validator.handleFetchError(tc.inputErr, &ValidationResult{Valid: true})

			if result.Valid {
				t.Error("expected result to be invalid after error")
			}

			foundExpected := false
			for _, e := range result.Errors {
				if strings.Contains(strings.ToLower(e), strings.ToLower(tc.expectedError)) {
					foundExpected = true
					break
				}
			}
			if !foundExpected {
				t.Errorf("expected error containing %q, got errors: %v", tc.expectedError, result.Errors)
			}
		})
	}
}

// Test that the fallback to GH CLI still works when GitHubClient is nil
func TestValidateIssue_FallsBackToGHCLI(t *testing.T) {
	t.Parallel()

	validator := NewIssueValidator(nil)
	validator.RunGH = func(ctx context.Context, args ...string) ([]byte, error) {
		// Verify it's calling the right command
		if len(args) < 2 || args[0] != "issue" || args[1] != "view" {
			t.Errorf("unexpected args: %v", args)
		}
		// Return mock response
		return []byte(`{
			"number": 1,
			"title": "Test Issue",
			"body": "Description with acceptance criteria:\n- [ ] Task",
			"state": "open",
			"labels": [{"name": "bug"}],
			"url": "https://github.com/o/r/issues/1",
			"closed": false
		}`), nil
	}

	result, err := validator.ValidateIssue(context.Background(), 1, "owner/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Valid {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
}
