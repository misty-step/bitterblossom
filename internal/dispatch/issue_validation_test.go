package dispatch

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestIssueValidator_ValidateIssue_MissingRequiredLabel(t *testing.T) {
	t.Parallel()

	validator := &IssueValidator{
		RequiredLabels:            []string{"ralph-ready"},
		CheckAcceptanceCriteria:   true,
		CheckBlockingDependencies: true,
		MinDescriptionLength:      50,
		RunGH: func(ctx context.Context, args ...string) ([]byte, error) {
			json := `{
				"number": 123,
				"title": "Test Issue",
				"body": "Some description with acceptance criteria:\n- [ ] Task 1\n- [ ] Task 2",
				"state": "open",
				"labels": [{"name": "bug"}],
				"url": "https://github.com/misty-step/test/issues/123"
			}`
			return []byte(json), nil
		},
	}

	result, err := validator.ValidateIssue(context.Background(), 123, "misty-step/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Valid {
		t.Fatal("expected issue to be invalid due to missing label")
	}

	if len(result.Errors) == 0 {
		t.Fatal("expected errors for missing label")
	}

	hasMissingLabelError := false
	for _, e := range result.Errors {
		if strings.Contains(e, "ralph-ready") {
			hasMissingLabelError = true
			break
		}
	}
	if !hasMissingLabelError {
		t.Fatalf("expected missing label error, got: %v", result.Errors)
	}
}

func TestIssueValidator_ValidateIssue_HasRequiredLabel(t *testing.T) {
	t.Parallel()

	validator := &IssueValidator{
		RequiredLabels:            []string{"ralph-ready"},
		CheckAcceptanceCriteria:   true,
		CheckBlockingDependencies: true,
		MinDescriptionLength:      50,
		RunGH: func(ctx context.Context, args ...string) ([]byte, error) {
			json := `{
				"number": 124,
				"title": "Test Issue with good title",
				"body": "This issue has acceptance criteria:\n\n## Acceptance Criteria\n- [ ] Task 1\n- [ ] Task 2",
				"state": "open",
				"labels": [{"name": "bug"}, {"name": "ralph-ready"}],
				"url": "https://github.com/misty-step/test/issues/124"
			}`
			return []byte(json), nil
		},
	}

	result, err := validator.ValidateIssue(context.Background(), 124, "misty-step/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Valid {
		t.Fatalf("expected issue to be valid, got errors: %v, warnings: %v", result.Errors, result.Warnings)
	}
}

func TestIssueValidator_ValidateIssue_ClosedIssue(t *testing.T) {
	t.Parallel()

	validator := &IssueValidator{
		RequiredLabels:            []string{"ralph-ready"},
		CheckAcceptanceCriteria:   true,
		CheckBlockingDependencies: true,
		RunGH: func(ctx context.Context, args ...string) ([]byte, error) {
			json := `{
				"number": 125,
				"title": "Closed Issue",
				"body": "Description",
				"state": "closed",
				"labels": [{"name": "ralph-ready"}],
				"url": "https://github.com/misty-step/test/issues/125"
			}`
			return []byte(json), nil
		},
	}

	result, err := validator.ValidateIssue(context.Background(), 125, "misty-step/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Valid {
		t.Fatal("expected closed issue to be invalid")
	}

	hasClosedError := false
	for _, e := range result.Errors {
		if strings.Contains(e, "closed") {
			hasClosedError = true
			break
		}
	}
	if !hasClosedError {
		t.Fatalf("expected closed error, got: %v", result.Errors)
	}
}

func TestIssueValidator_ValidateIssue_BlockingLabels(t *testing.T) {
	t.Parallel()

	validator := &IssueValidator{
		RequiredLabels:            []string{"ralph-ready"},
		CheckAcceptanceCriteria:   true,
		CheckBlockingDependencies: true,
		MinDescriptionLength:      50,
		RunGH: func(ctx context.Context, args ...string) ([]byte, error) {
			json := `{
				"number": 126,
				"title": "Test Issue with good title",
				"body": "Description with acceptance criteria:\n\n## Tasks\n- [ ] Task 1",
				"state": "open",
				"labels": [
					{"name": "ralph-ready"},
					{"name": "blocked-by-other"}
				],
				"url": "https://github.com/misty-step/test/issues/126"
			}`
			return []byte(json), nil
		},
	}

	result, err := validator.ValidateIssue(context.Background(), 126, "misty-step/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Valid {
		t.Fatalf("expected issue to be valid (warnings only), got errors: %v", result.Errors)
	}

	if !result.HasBlockingLabel {
		t.Fatal("expected HasBlockingLabel to be true")
	}

	foundBlockingWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "blocked") {
			foundBlockingWarning = true
			break
		}
	}
	if !foundBlockingWarning {
		t.Fatalf("expected blocking warning, got: %v", result.Warnings)
	}
}

func TestIssueValidator_ValidateIssue_NoAcceptanceCriteria(t *testing.T) {
	t.Parallel()

	validator := &IssueValidator{
		RequiredLabels:            []string{"ralph-ready"},
		CheckAcceptanceCriteria:   true,
		CheckBlockingDependencies: true,
		MinDescriptionLength:      50,
		RunGH: func(ctx context.Context, args ...string) ([]byte, error) {
			json := `{
				"number": 127,
				"title": "Test Issue with good title",
				"body": "This is a very short description without clear acceptance criteria.",
				"state": "open",
				"labels": [{"name": "ralph-ready"}],
				"url": "https://github.com/misty-step/test/issues/127"
			}`
			return []byte(json), nil
		},
	}

	result, err := validator.ValidateIssue(context.Background(), 127, "misty-step/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Valid {
		t.Fatalf("expected issue to be valid, got errors: %v", result.Errors)
	}

	hasWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "short") || strings.Contains(w, "acceptance criteria") {
			hasWarning = true
			break
		}
	}
	if !hasWarning {
		t.Fatalf("expected short description or acceptance criteria warning, got: %v", result.Warnings)
	}
}

func TestIssueValidator_ValidateIssue_GHNotAvailable(t *testing.T) {
	t.Parallel()

	validator := &IssueValidator{
		RequiredLabels:            []string{"ralph-ready"},
		CheckAcceptanceCriteria:   true,
		CheckBlockingDependencies: true,
		RunGH: func(ctx context.Context, args ...string) ([]byte, error) {
			return nil, errors.New("gh not found")
		},
	}

	result, err := validator.ValidateIssue(context.Background(), 128, "misty-step/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Valid {
		t.Fatal("expected issue to be invalid when gh fails")
	}

	hasFetchError := false
	for _, e := range result.Errors {
		if strings.Contains(e, "failed to fetch") {
			hasFetchError = true
			break
		}
	}
	if !hasFetchError {
		t.Fatalf("expected fetch error, got: %v", result.Errors)
	}
}

func TestIssueValidator_ValidateIssue_NoRepo(t *testing.T) {
	t.Parallel()

	validator := DefaultIssueValidator()

	result, err := validator.ValidateIssue(context.Background(), 129, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Valid {
		t.Fatal("expected issue to be invalid without repo")
	}

	hasRepoError := false
	for _, e := range result.Errors {
		if strings.Contains(e, "repository is required") {
			hasRepoError = true
			break
		}
	}
	if !hasRepoError {
		t.Fatalf("expected repo error, got: %v", result.Errors)
	}
}

func TestValidateIssueFromRequest_NoIssue(t *testing.T) {
	t.Parallel()

	req := Request{
		Sprite:  "test-sprite",
		Prompt:  "Do something",
		Issue:   0,
		Execute: true,
	}

	result, err := ValidateIssueFromRequest(context.Background(), req, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Valid {
		t.Fatal("expected validation to pass when no issue specified")
	}
}

func TestValidateIssueFromRequest_StrictMode(t *testing.T) {
	t.Parallel()

	validator := &IssueValidator{
		RequiredLabels:            []string{"ralph-ready"},
		CheckAcceptanceCriteria:   true,
		CheckBlockingDependencies: true,
		MinDescriptionLength:      50,
		RunGH: func(ctx context.Context, args ...string) ([]byte, error) {
			json := `{
				"number": 130,
				"title": "Test Issue with good title",
				"body": "Description with acceptance criteria:\n\n## Tasks\n- [ ] Task 1",
				"state": "open",
				"labels": [
					{"name": "ralph-ready"},
					{"name": "blocked-by-other"}
				],
				"url": "https://github.com/misty-step/test/issues/130"
			}`
			return []byte(json), nil
		},
	}

	req := Request{
		Sprite:  "test-sprite",
		Prompt:  "Implement issue #130",
		Issue:   130,
		Repo:    "misty-step/test",
		Execute: true,
	}

	result, err := validator.ValidateIssue(context.Background(), req.Issue, req.Repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Valid {
		t.Fatalf("expected valid in non-strict mode, got errors: %v", result.Errors)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected warnings in non-strict mode")
	}

	result2, err := ValidateIssueFromRequest(context.Background(), req, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result2.Valid {
		t.Fatal("expected invalid in strict mode due to warnings becoming errors")
	}

	if len(result2.Warnings) != 0 {
		t.Fatalf("expected no warnings in strict mode (converted to errors), got: %v", result2.Warnings)
	}

	if len(result2.Errors) == 0 {
		t.Fatal("expected errors in strict mode")
	}
}

func TestValidationResult_ToError(t *testing.T) {
	t.Parallel()

	validResult := &ValidationResult{Valid: true}
	if err := validResult.ToError(); err != nil {
		t.Fatalf("expected nil error for valid result, got: %v", err)
	}

	invalidResult := &ValidationResult{
		Valid:       false,
		IssueNumber: 131,
		Repo:        "misty-step/test",
		Errors:      []string{"missing required labels: ralph-ready"},
		Warnings:    []string{"issue description is short"},
	}
	err := invalidResult.ToError()
	if err == nil {
		t.Fatal("expected error for invalid result")
	}

	issueErr, ok := err.(*ErrIssueNotReady)
	if !ok {
		t.Fatalf("expected ErrIssueNotReady, got %T", err)
	}

	if issueErr.Issue != 131 {
		t.Fatalf("expected issue 131, got %d", issueErr.Issue)
	}

	if issueErr.Repo != "misty-step/test" {
		t.Fatalf("expected repo misty-step/test, got %s", issueErr.Repo)
	}
}

func TestErrIssueNotReady_Error(t *testing.T) {
	t.Parallel()

	err := &ErrIssueNotReady{
		Issue:  157,
		Repo:   "misty-step/bitterblossom",
		Reason: "missing required labels",
	}

	expected := "issue misty-step/bitterblossom#157 is not ready for dispatch: missing required labels"
	if err.Error() != expected {
		t.Fatalf("expected %q, got %q", expected, err.Error())
	}
}

func TestNormalizeRepoSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"misty-step/bitterblossom", "misty-step/bitterblossom"},
		{"https://github.com/misty-step/bitterblossom", "misty-step/bitterblossom"},
		{"https://github.com/misty-step/bitterblossom.git", "misty-step/bitterblossom"},
		{"", ""},
		{"invalid", ""},
		{"owner/repo/extra", ""},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := normalizeRepoSlug(tc.input)
			if result != tc.expected {
				t.Errorf("normalizeRepoSlug(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}
