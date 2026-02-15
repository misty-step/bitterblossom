package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/internal/github"
)

// IssueValidator validates GitHub issues before dispatch.
type IssueValidator struct {
	// RequiredLabels are labels that must be present on the issue (hard error).
	RequiredLabels []string
	// RecommendedLabels are labels that should be present (warning if missing).
	RecommendedLabels []string
	// CheckAcceptanceCriteria verifies the issue has a clear description/acceptance criteria.
	CheckAcceptanceCriteria bool
	// CheckBlockingDependencies checks for "blocked-by" or similar labels/dependencies.
	CheckBlockingDependencies bool
	// MinDescriptionLength is the minimum character count for a valid description.
	MinDescriptionLength int
	// RunGH is the function to execute gh CLI commands (for testing) - DEPRECATED: Use GitHubClient instead.
	RunGH func(ctx context.Context, args ...string) ([]byte, error)
	// GitHubClient is the typed GitHub API client. If nil, falls back to RunGH/gh CLI.
	GitHubClient GitHubIssueClient
}

// GitHubIssueClient defines the interface needed for issue validation.
// This interface is satisfied by *github.Client.
type GitHubIssueClient interface {
	GetIssue(ctx context.Context, owner, repo string, number int) (*github.Issue, error)
}

// ValidationResult contains the outcome of issue validation.
type ValidationResult struct {
	Valid            bool     `json:"valid"`
	Warnings         []string `json:"warnings,omitempty"`
	Errors           []string `json:"errors,omitempty"`
	IssueNumber      int      `json:"issue_number,omitempty"`
	Repo             string   `json:"repo,omitempty"`
	Labels           []string `json:"labels,omitempty"`
	HasDescription   bool     `json:"has_description"`
	HasBlockingLabel bool     `json:"has_blocking_label"`
}

// IssueData represents a GitHub issue fetched via gh CLI.
// DEPRECATED: Use github.Issue instead.
type IssueData struct {
	Number       int      `json:"number"`
	Title        string   `json:"title"`
	Body         string   `json:"body"`
	State        string   `json:"state"`
	Labels       []Label  `json:"labels"`
	URL          string   `json:"url"`
	Closed       bool     `json:"closed"`
	Dependencies []string `json:"-"`
}

// Label represents a GitHub issue label.
// DEPRECATED: Use github.Label instead.
type Label struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
}

// NewIssueValidator creates a validator with the specified GitHub client.
// If client is nil, falls back to gh CLI via GITHUB_TOKEN environment variable.
func NewIssueValidator(client GitHubIssueClient) *IssueValidator {
	return &IssueValidator{
		RequiredLabels:            []string{},
		RecommendedLabels:         []string{"ralph-ready"},
		CheckAcceptanceCriteria:   true,
		CheckBlockingDependencies: true,
		MinDescriptionLength:      50,
		RunGH:                     defaultRunGH,
		GitHubClient:              client,
	}
}

// DefaultIssueValidator returns a validator with sensible defaults.
// Uses typed GitHub client if GITHUB_TOKEN is available, otherwise falls back to gh CLI.
func DefaultIssueValidator() *IssueValidator {
	// Try to create a GitHub client from environment token first
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		client := github.NewClientFromToken(token)
		return NewIssueValidator(client)
	}
	return NewIssueValidator(nil)
}

// IssueValidatorForRalphMode returns a validator configured for the specified mode.
// When ralphMode is true, it includes ralph-ready in recommended labels.
// When ralphMode is false, ralph-ready is not recommended (only relevant for Ralph dispatches).
func IssueValidatorForRalphMode(ralphMode bool) *IssueValidator {
	v := DefaultIssueValidator()
	if !ralphMode {
		v.RecommendedLabels = nil
	}
	return v
}

// defaultRunGH executes the gh CLI command.
func defaultRunGH(ctx context.Context, args ...string) ([]byte, error) {
	// Add a safety timeout if the caller didn't set one.
	// gh calls can hang on network issues; dispatch should have a bounded critical path.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	return cmd.Output()
}

// ErrIssueNotReady indicates the issue is not ready for dispatch.
type ErrIssueNotReady struct {
	Issue  int
	Repo   string
	Reason string
}

func (e *ErrIssueNotReady) Error() string {
	return fmt.Sprintf("issue %s#%d is not ready for dispatch: %s", e.Repo, e.Issue, e.Reason)
}

// ValidateIssue validates a GitHub issue is ready for Ralph dispatch.
func (v *IssueValidator) ValidateIssue(ctx context.Context, issue int, repo string) (*ValidationResult, error) {
	result := &ValidationResult{
		IssueNumber: issue,
		Repo:        repo,
		Valid:       true,
	}

	// Normalize repo format
	repoSlug := normalizeRepoSlug(repo)
	if repoSlug == "" {
		result.Errors = append(result.Errors, "repository is required for issue validation")
		result.Valid = false
		return result, nil
	}

	// Fetch issue data from GitHub using typed client first
	issueData, err := v.fetchIssue(ctx, issue, repoSlug)
	if err != nil {
		result = v.handleFetchError(err, result)
		return result, nil
	}

	// Check if issue is closed
	if issueData.State == "closed" {
		result.Errors = append(result.Errors, "issue is closed")
		result.Valid = false
	}

	// Extract label names
	labelNames := make([]string, len(issueData.Labels))
	for i, l := range issueData.Labels {
		labelNames[i] = l.Name
	}
	result.Labels = labelNames

	// Check for required labels
	missingLabels := v.checkRequiredLabels(labelNames)
	if len(missingLabels) > 0 {
		result.Errors = append(result.Errors, fmt.Sprintf("missing required labels: %s", strings.Join(missingLabels, ", ")))
		result.Valid = false
	}

	// Check for recommended labels
	missingRecommended := v.checkRecommendedLabels(labelNames)
	if len(missingRecommended) > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("missing recommended labels: %s (add for best results)", strings.Join(missingRecommended, ", ")))
	}

	// Check for blocking labels
	if v.CheckBlockingDependencies {
		blockingLabels := v.findBlockingLabels(labelNames)
		if len(blockingLabels) > 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("blocking labels found: %s", strings.Join(blockingLabels, ", ")))
			result.HasBlockingLabel = true
		}
	}

	// Check acceptance criteria / description
	if v.CheckAcceptanceCriteria {
		hasDesc := hasAcceptanceCriteria(issueData.Body)
		result.HasDescription = hasDesc
		if !hasDesc {
			result.Warnings = append(result.Warnings, "issue may lack clear acceptance criteria")
		}
		if len(issueData.Body) < v.MinDescriptionLength {
			result.Warnings = append(result.Warnings, fmt.Sprintf("issue description is short (%d chars, min %d)", len(issueData.Body), v.MinDescriptionLength))
		}
	}

	// Check title quality
	if len(issueData.Title) < 10 {
		result.Warnings = append(result.Warnings, "issue title is very short - may not be descriptive enough")
	}

	return result, nil
}

// handleFetchError converts fetch errors into user-friendly validation results.
func (v *IssueValidator) handleFetchError(err error, result *ValidationResult) *ValidationResult {
	var msg string
	switch {
	case errors.Is(err, github.ErrAuth):
		msg = "GitHub authentication failed - check GITHUB_TOKEN is valid and has required scopes"
	case errors.Is(err, github.ErrNotFound):
		msg = "issue not found - check the issue number and repository"
	case errors.Is(err, github.ErrRateLimited):
		msg = "GitHub rate limit exceeded - wait before retrying"
	case errors.Is(err, exec.ErrNotFound):
		msg = "gh CLI not found - install GitHub CLI to use issue validation"
	default:
		msg = fmt.Sprintf("failed to fetch issue: %v", err)
	}
	result.Errors = append(result.Errors, msg)
	result.Valid = false
	return result
}

// toIssueData converts github.Issue to legacy IssueData for internal use.
func toIssueData(issue *github.Issue) *IssueData {
	labels := make([]Label, len(issue.Labels))
	for i, l := range issue.Labels {
		labels[i] = Label{
			Name:        l.Name,
			Description: l.Description,
			Color:       l.Color,
		}
	}
	return &IssueData{
		Number: issue.Number,
		Title:  issue.Title,
		Body:   issue.Body,
		State:  issue.State,
		Labels: labels,
		URL:    issue.HTMLURL,
		Closed: issue.Closed(),
	}
}

// fetchIssue retrieves issue data from GitHub.
// Uses typed GitHubClient if available, falls back to gh CLI.
func (v *IssueValidator) fetchIssue(ctx context.Context, issueNum int, repoSlug string) (*IssueData, error) {
	// First try the typed GitHub client
	if v.GitHubClient != nil {
		owner, repo, err := parseRepoSlug(repoSlug)
		if err != nil {
			return nil, err
		}
		issue, err := v.GitHubClient.GetIssue(ctx, owner, repo, issueNum)
		if err != nil {
			return nil, err
		}
		return toIssueData(issue), nil
	}

	// Fall back to gh CLI
	return v.fetchIssueViaGH(ctx, issueNum, repoSlug)
}

// parseRepoSlug splits "owner/repo" into owner and repo.
func parseRepoSlug(repo string) (owner, name string, err error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repo format: %s (expected owner/repo)", repo)
	}
	return parts[0], parts[1], nil
}

// fetchIssueViaGH retrieves issue data via gh CLI (fallback).
func (v *IssueValidator) fetchIssueViaGH(ctx context.Context, issue int, repo string) (*IssueData, error) {
	args := []string{
		"issue", "view", strconv.Itoa(issue),
		"--repo", repo,
		"--json", "number,title,body,state,labels,url,closed",
	}

	output, err := v.RunGH(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("gh issue view failed: %w", err)
	}

	var issueData IssueData
	if err := json.Unmarshal(output, &issueData); err != nil {
		return nil, fmt.Errorf("parse issue JSON: %w", err)
	}

	return &issueData, nil
}

// findMissingLabels returns which of want are absent from have (case-insensitive).
func findMissingLabels(have, want []string) []string {
	set := make(map[string]bool, len(have))
	for _, l := range have {
		set[strings.ToLower(l)] = true
	}

	var missing []string
	for _, w := range want {
		if !set[strings.ToLower(w)] {
			missing = append(missing, w)
		}
	}
	return missing
}

// checkRequiredLabels returns required labels that are missing.
func (v *IssueValidator) checkRequiredLabels(labels []string) []string {
	return findMissingLabels(labels, v.RequiredLabels)
}

// checkRecommendedLabels returns recommended labels that are missing.
func (v *IssueValidator) checkRecommendedLabels(labels []string) []string {
	return findMissingLabels(labels, v.RecommendedLabels)
}

// blockingLabelPatterns matches labels that indicate the issue is blocked.
var blockingLabelPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^blocked`),
	regexp.MustCompile(`(?i)^blocked-by`),
	regexp.MustCompile(`(?i)^blocker`),
	regexp.MustCompile(`(?i)^needs-`),
	regexp.MustCompile(`(?i)^waiting`),
	regexp.MustCompile(`(?i)^on-hold`),
	regexp.MustCompile(`(?i)^paused`),
	regexp.MustCompile(`(?i)^dependency`),
}

// findBlockingLabels returns any labels that indicate the issue is blocked.
func (v *IssueValidator) findBlockingLabels(labels []string) []string {
	var blocking []string
	for _, label := range labels {
		for _, pattern := range blockingLabelPatterns {
			if pattern.MatchString(label) {
				blocking = append(blocking, label)
				break
			}
		}
	}
	return blocking
}

// acceptanceCriteriaPatterns match sections that indicate acceptance criteria.
var acceptanceCriteriaPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)#+\s*acceptance criteria`),
	regexp.MustCompile(`(?i)#+\s*requirements?`),
	regexp.MustCompile(`(?i)#+\s*definition of done`),
	regexp.MustCompile(`(?i)#+\s*tasks?`),
	regexp.MustCompile(`(?i)#+\s*todo`),
	regexp.MustCompile(`(?i)#+\s*checklist`),
	regexp.MustCompile(`(?i)-\s*\[.\]`), // Checkboxes
}

// hasAcceptanceCriteria checks if the body contains acceptance criteria patterns.
func hasAcceptanceCriteria(body string) bool {
	if strings.TrimSpace(body) == "" {
		return false
	}

	for _, pattern := range acceptanceCriteriaPatterns {
		if pattern.MatchString(body) {
			return true
		}
	}

	// Check for task list checkboxes
	if strings.Contains(body, "- [ ]") || strings.Contains(body, "- [x]") {
		return true
	}

	return false
}

// normalizeRepoSlug converts various repo formats to owner/repo.
func normalizeRepoSlug(repo string) string {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return ""
	}

	// Remove https://github.com/ prefix if present
	repo = strings.TrimPrefix(repo, "https://github.com/")
	repo = strings.TrimPrefix(repo, "http://github.com/")
	repo = strings.TrimSuffix(repo, ".git")

	// Validate format
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return ""
	}

	return repo
}

// ValidateIssueFromRequest validates an issue based on a dispatch request.
// Deprecated: Use ValidateIssueWithProfile instead for profile-based validation.
func ValidateIssueFromRequest(ctx context.Context, req Request, strict bool) (*ValidationResult, error) {
	// Only validate if issue number is provided
	if req.Issue <= 0 {
		return &ValidationResult{Valid: true}, nil
	}

	validator := IssueValidatorForRalphMode(req.Ralph)
	result, err := validator.ValidateIssue(ctx, req.Issue, req.Repo)
	if err != nil {
		return nil, err
	}

	// In strict mode, warnings become errors
	if strict && len(result.Warnings) > 0 {
		result.Errors = append(result.Errors, result.Warnings...)
		result.Warnings = nil
		result.Valid = false
	}

	return result, nil
}

// ValidateIssueWithProfile performs full validation (safety + policy) with profile support.
// Safety checks are always enforced. Policy checks are controlled by the profile.
func ValidateIssueWithProfile(ctx context.Context, req Request, profile ValidationProfile, env map[string]string, allowDirect bool) (*CombinedValidationResult, error) {
	// Build combined result
	result := &CombinedValidationResult{
		IssueNumber: req.Issue,
		Repo:        req.Repo,
	}

	// Always run safety checks (cannot be bypassed)
	safetyValidator := DefaultSafetyValidator()
	result.Safety = *safetyValidator.ValidateSafetyWithEnv(ctx, req, env, allowDirect)

	// Run policy checks only if profile is not "off"
	if profile != ValidationProfileOff && req.Issue > 0 {
		policyValidator := IssueValidatorForRalphMode(req.Ralph)
		policyResult, err := policyValidator.ValidateIssue(ctx, req.Issue, req.Repo)
		if err != nil {
			return nil, err
		}

		result.Policy.Warnings = policyResult.Warnings
		result.Policy.Errors = policyResult.Errors
		result.Policy.HasBlockingLabel = policyResult.HasBlockingLabel
		result.Policy.HasDescription = policyResult.HasDescription
		result.Policy.Labels = policyResult.Labels
		result.Labels = policyResult.Labels
	}

	// Compute legacy Valid field based on profile
	result.Valid = result.IsSafe() && result.IsPolicyCompliant(profile)

	// Populate legacy warnings/errors for backward compatibility
	if profile == ValidationProfileStrict {
		// In strict mode, policy warnings become errors
		result.Errors = append(result.Safety.Errors, result.Policy.Errors...)
		result.Errors = append(result.Errors, result.Policy.Warnings...)
	} else {
		result.Errors = append(result.Safety.Errors, result.Policy.Errors...)
		result.Warnings = result.Policy.Warnings
	}

	return result, nil
}

// ToError converts validation result to an error if invalid.
func (r *ValidationResult) ToError() error {
	if r.Valid && len(r.Errors) == 0 {
		return nil
	}

	var parts []string
	if len(r.Errors) > 0 {
		parts = append(parts, "Errors:")
		for _, e := range r.Errors {
			parts = append(parts, "  - "+e)
		}
	}
	if len(r.Warnings) > 0 {
		parts = append(parts, "Warnings:")
		for _, w := range r.Warnings {
			parts = append(parts, "  - "+w)
		}
	}

	return &ErrIssueNotReady{
		Issue:  r.IssueNumber,
		Repo:   r.Repo,
		Reason: strings.Join(parts, "\n"),
	}
}

// FormatValidationOutput returns a human-readable validation report.
func (r *ValidationResult) FormatValidationOutput() string {
	var lines []string

	if r.IssueNumber > 0 {
		lines = append(lines, fmt.Sprintf("Issue: %s#%d", r.Repo, r.IssueNumber))
	}

	if len(r.Labels) > 0 {
		lines = append(lines, fmt.Sprintf("Labels: %s", strings.Join(r.Labels, ", ")))
	}

	if r.HasDescription {
		lines = append(lines, "✓ Has acceptance criteria")
	} else {
		lines = append(lines, "⚠ May lack clear acceptance criteria")
	}

	if len(r.Errors) > 0 {
		lines = append(lines, "")
		lines = append(lines, "Errors:")
		for _, e := range r.Errors {
			lines = append(lines, "  ✗ "+e)
		}
	}

	if len(r.Warnings) > 0 {
		lines = append(lines, "")
		lines = append(lines, "Warnings:")
		for _, w := range r.Warnings {
			lines = append(lines, "  ⚠ "+w)
		}
	}

	if r.Valid && len(r.Errors) == 0 {
		lines = append(lines, "")
		lines = append(lines, "✓ Issue is ready for dispatch")
	} else {
		lines = append(lines, "")
		lines = append(lines, "✗ Issue is NOT ready for dispatch")
	}

	return strings.Join(lines, "\n")
}
