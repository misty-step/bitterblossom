package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// IssueValidator validates GitHub issues before dispatch.
type IssueValidator struct {
	// RequiredLabels are labels that must be present on the issue.
	// Default: ["ralph-ready"]
	RequiredLabels []string
	// CheckAcceptanceCriteria verifies the issue has a clear description/acceptance criteria.
	CheckAcceptanceCriteria bool
	// CheckBlockingDependencies checks for "blocked-by" or similar labels/dependencies.
	CheckBlockingDependencies bool
	// MinDescriptionLength is the minimum character count for a valid description.
	MinDescriptionLength int
	// RunGH is the function to execute gh CLI commands (for testing).
	RunGH func(ctx context.Context, args ...string) ([]byte, error)
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
type IssueData struct {
	Number      int      `json:"number"`
	Title       string   `json:"title"`
	Body        string   `json:"body"`
	State       string   `json:"state"`
	Labels      []Label  `json:"labels"`
	URL         string   `json:"url"`
	Closed      bool     `json:"closed"`
	// These fields are populated from label descriptions or specific labels
	Dependencies []string `json:"-"`
}

// Label represents a GitHub issue label.
type Label struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
}

// DefaultIssueValidator returns a validator with sensible defaults.
func DefaultIssueValidator() *IssueValidator {
	return &IssueValidator{
		RequiredLabels:            []string{"ralph-ready"},
		CheckAcceptanceCriteria:   true,
		CheckBlockingDependencies: true,
		MinDescriptionLength:      50,
		RunGH:                     defaultRunGH,
	}
}

// defaultRunGH executes the gh CLI command.
func defaultRunGH(ctx context.Context, args ...string) ([]byte, error) {
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

	// Fetch issue data from GitHub
	issueData, err := v.fetchIssue(ctx, issue, repoSlug)
	if err != nil {
		// If gh is not available or issue doesn't exist, return error
		if errors.Is(err, exec.ErrNotFound) {
			result.Errors = append(result.Errors, "gh CLI not found - install GitHub CLI to use issue validation")
			result.Valid = false
			return result, nil
		}
		// Issue might be private or not accessible
		result.Errors = append(result.Errors, fmt.Sprintf("failed to fetch issue: %v", err))
		result.Valid = false
		return result, nil
	}

	// Check if issue is closed
	if issueData.Closed || issueData.State == "closed" {
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

// fetchIssue retrieves issue data from GitHub via gh CLI.
func (v *IssueValidator) fetchIssue(ctx context.Context, issue int, repo string) (*IssueData, error) {
	// Use gh CLI to fetch issue data as JSON
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

// checkRequiredLabels returns the labels that are missing.
func (v *IssueValidator) checkRequiredLabels(labels []string) []string {
	labelSet := make(map[string]bool)
	for _, l := range labels {
		labelSet[strings.ToLower(l)] = true
	}

	var missing []string
	for _, required := range v.RequiredLabels {
		if !labelSet[strings.ToLower(required)] {
			missing = append(missing, required)
		}
	}
	return missing
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
func ValidateIssueFromRequest(ctx context.Context, req Request, strict bool) (*ValidationResult, error) {
	// Only validate if issue number is provided
	if req.Issue <= 0 {
		return &ValidationResult{Valid: true}, nil
	}

	validator := DefaultIssueValidator()
	result, err := validator.ValidateIssue(ctx, req.Issue, req.Repo)
	if err != nil {
		return nil, err
	}

	// In strict mode, warnings become errors
	if strict && len(result.Warnings) > 0 {
		for _, w := range result.Warnings {
			result.Errors = append(result.Errors, w)
		}
		result.Warnings = nil
		result.Valid = false
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
