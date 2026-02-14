package dispatch

import (
	"context"
	"errors"
	"fmt"
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
			// Return issue without ralph-ready label
			json := `{
				"number": 123,
				"title": "Test Issue",
				"body": "Some description with acceptance criteria:\n- [ ] Task 1\n- [ ] Task 2",
				"state": "open",
				"labels": [{"name": "bug"}],
				"url": "https://github.com/misty-step/test/issues/123",
				"closed": false
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
				"url": "https://github.com/misty-step/test/issues/124",
				"closed": false
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
				"url": "https://github.com/misty-step/test/issues/125",
				"closed": true
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
				"url": "https://github.com/misty-step/test/issues/126",
				"closed": false
			}`
			return []byte(json), nil
		},
	}

	result, err := validator.ValidateIssue(context.Background(), 126, "misty-step/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Issue is valid but has warnings
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
				"url": "https://github.com/misty-step/test/issues/127",
				"closed": false
			}`
			return []byte(json), nil
		},
	}

	result, err := validator.ValidateIssue(context.Background(), 127, "misty-step/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Issue is valid but has warnings about short description
	if !result.Valid {
		t.Fatalf("expected issue to be valid, got errors: %v", result.Errors)
	}

	// Check for warning about acceptance criteria or short description
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
				"url": "https://github.com/misty-step/test/issues/130",
				"closed": false
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

	// Non-strict mode - warnings don't fail validation
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

	// Strict mode - warnings become errors
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

	// Valid result returns nil error
	validResult := &ValidationResult{Valid: true}
	if err := validResult.ToError(); err != nil {
		t.Fatalf("expected nil error for valid result, got: %v", err)
	}

	// Invalid result returns error
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

func TestHasAcceptanceCriteria(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		expected bool
	}{
		{
			name:     "empty body",
			body:     "",
			expected: false,
		},
		{
			name:     "simple description",
			body:     "This is just a simple description",
			expected: false,
		},
		{
			name:     "has acceptance criteria header",
			body:     "## Acceptance Criteria\n- Must do X\n- Must do Y",
			expected: true,
		},
		{
			name:     "has requirements header",
			body:     "## Requirements\n- Must do X",
			expected: true,
		},
		{
			name:     "has definition of done",
			body:     "## Definition of Done\nAll tests passing",
			expected: true,
		},
		{
			name:     "has task list",
			body:     "## Tasks\n- [ ] Task 1\n- [ ] Task 2",
			expected: true,
		},
		{
			name:     "has checkboxes without header",
			body:     "Things to do:\n- [ ] Task 1\n- [x] Task 2",
			expected: true,
		},
		{
			name:     "has todo header",
			body:     "## TODO\n- Item 1",
			expected: true,
		},
		{
			name:     "has checklist header",
			body:     "# Checklist\n- Item 1",
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := hasAcceptanceCriteria(tc.body)
			if result != tc.expected {
				t.Errorf("hasAcceptanceCriteria() = %v, want %v", result, tc.expected)
			}
		})
	}
}

func TestFindBlockingLabels(t *testing.T) {
	t.Parallel()

	validator := &IssueValidator{}

	tests := []struct {
		labels   []string
		expected []string
	}{
		{
			labels:   []string{"bug", "enhancement"},
			expected: nil,
		},
		{
			labels:   []string{"blocked"},
			expected: []string{"blocked"},
		},
		{
			labels:   []string{"BLOCKED"}, // case insensitive
			expected: []string{"BLOCKED"},
		},
		{
			labels:   []string{"blocked-by-other"},
			expected: []string{"blocked-by-other"},
		},
		{
			labels:   []string{"needs-info"},
			expected: []string{"needs-info"},
		},
		{
			labels:   []string{"waiting-on-review"},
			expected: []string{"waiting-on-review"},
		},
		{
			labels:   []string{"on-hold"},
			expected: []string{"on-hold"},
		},
		{
			labels:   []string{"paused"},
			expected: []string{"paused"},
		},
		{
			labels:   []string{"dependency"},
			expected: []string{"dependency"},
		},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%v", tc.labels), func(t *testing.T) {
			result := validator.findBlockingLabels(tc.labels)
			if len(result) != len(tc.expected) {
				t.Errorf("findBlockingLabels() = %v, want %v", result, tc.expected)
			}
			for i, v := range result {
				if v != tc.expected[i] {
					t.Errorf("findBlockingLabels()[%d] = %v, want %v", i, v, tc.expected[i])
				}
			}
		})
	}
}

func TestCheckRequiredLabels(t *testing.T) {
	t.Parallel()

	validator := &IssueValidator{
		RequiredLabels: []string{"ralph-ready", "approved"},
	}

	tests := []struct {
		labels          []string
		expectedMissing []string
	}{
		{
			labels:          []string{"ralph-ready", "approved"},
			expectedMissing: nil,
		},
		{
			labels:          []string{"RALPH-READY"}, // case insensitive
			expectedMissing: []string{"approved"},
		},
		{
			labels:          []string{"bug"},
			expectedMissing: []string{"ralph-ready", "approved"},
		},
		{
			labels:          []string{},
			expectedMissing: []string{"ralph-ready", "approved"},
		},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%v", tc.labels), func(t *testing.T) {
			result := validator.checkRequiredLabels(tc.labels)
			if len(result) != len(tc.expectedMissing) {
				t.Errorf("checkRequiredLabels() = %v, want %v", result, tc.expectedMissing)
			}
			for i, v := range result {
				if v != tc.expectedMissing[i] {
					t.Errorf("checkRequiredLabels()[%d] = %v, want %v", i, v, tc.expectedMissing[i])
				}
			}
		})
	}
}

func TestDefaultValidatorRalphReadyIsWarning(t *testing.T) {
	t.Parallel()

	validator := DefaultIssueValidator()
	validator.RunGH = func(ctx context.Context, args ...string) ([]byte, error) {
		json := `{
			"number": 42,
			"title": "Test issue without ralph-ready label",
			"body": "Description with acceptance criteria:\n- [ ] Task 1\n- [ ] Task 2",
			"state": "open",
			"labels": [{"name": "bug"}],
			"url": "https://github.com/misty-step/test/issues/42",
			"closed": false
		}`
		return []byte(json), nil
	}

	result, err := validator.ValidateIssue(context.Background(), 42, "misty-step/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected valid (ralph-ready should be warning, not error), errors: %v", result.Errors)
	}

	hasWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "ralph-ready") {
			hasWarning = true
			break
		}
	}
	if !hasWarning {
		t.Fatalf("expected warning about missing ralph-ready, got warnings: %v", result.Warnings)
	}
}

func TestCheckRecommendedLabels(t *testing.T) {
	t.Parallel()

	v := &IssueValidator{RecommendedLabels: []string{"ralph-ready", "reviewed"}}

	missing := v.checkRecommendedLabels([]string{"bug", "ralph-ready"})
	if len(missing) != 1 || missing[0] != "reviewed" {
		t.Fatalf("expected [reviewed], got %v", missing)
	}

	missing = v.checkRecommendedLabels([]string{"ralph-ready", "reviewed"})
	if len(missing) != 0 {
		t.Fatalf("expected no missing, got %v", missing)
	}
}

func TestValidateIssueFromRequest_NonRalphMode_SuppressesRalphReadyWarning(t *testing.T) {
	t.Parallel()

	// Mock RunGH to return an issue without ralph-ready label
	mockRunGH := func(ctx context.Context, args ...string) ([]byte, error) {
		json := `{
			"number": 200,
			"title": "Test issue without ralph-ready label",
			"body": "Description with acceptance criteria:\n- [ ] Task 1",
			"state": "open",
			"labels": [{"name": "bug"}],
			"url": "https://github.com/misty-step/test/issues/200",
			"closed": false
		}`
		return []byte(json), nil
	}

	// Test non-Ralph mode: should NOT warn about missing ralph-ready label
	nonRalphValidator := IssueValidatorForRalphMode(false)
	nonRalphValidator.RunGH = mockRunGH

	result, err := nonRalphValidator.ValidateIssue(context.Background(), 200, "misty-step/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be valid with no warnings about ralph-ready
	if !result.Valid {
		t.Fatalf("expected valid result, got errors: %v", result.Errors)
	}

	for _, w := range result.Warnings {
		if strings.Contains(w, "ralph-ready") {
			t.Fatalf("non-Ralph mode should not warn about ralph-ready label, got warning: %s", w)
		}
	}

	// Test Ralph mode: should still warn about missing ralph-ready label
	ralphValidator := IssueValidatorForRalphMode(true)
	ralphValidator.RunGH = mockRunGH

	result2, err := ralphValidator.ValidateIssue(context.Background(), 200, "misty-step/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have warning about ralph-ready
	hasRalphWarning := false
	for _, w := range result2.Warnings {
		if strings.Contains(w, "ralph-ready") {
			hasRalphWarning = true
			break
		}
	}
	if !hasRalphWarning {
		t.Fatalf("Ralph mode should warn about missing ralph-ready label, got warnings: %v", result2.Warnings)
	}
}

func TestIssueValidatorForRalphMode(t *testing.T) {
	t.Parallel()

	// Non-Ralph mode should have empty RecommendedLabels
	nonRalphValidator := IssueValidatorForRalphMode(false)
	if len(nonRalphValidator.RecommendedLabels) != 0 {
		t.Fatalf("expected empty RecommendedLabels for non-Ralph mode, got %v", nonRalphValidator.RecommendedLabels)
	}

	// Ralph mode should have ralph-ready in RecommendedLabels
	ralphValidator := IssueValidatorForRalphMode(true)
	if len(ralphValidator.RecommendedLabels) != 1 || ralphValidator.RecommendedLabels[0] != "ralph-ready" {
		t.Fatalf("expected ['ralph-ready'] RecommendedLabels for Ralph mode, got %v", ralphValidator.RecommendedLabels)
	}

	// Both should still have other settings preserved
	if len(nonRalphValidator.RequiredLabels) != 0 {
		t.Fatal("non-Ralph validator should have empty RequiredLabels")
	}
	if !nonRalphValidator.CheckAcceptanceCriteria {
		t.Fatal("non-Ralph validator should check acceptance criteria")
	}
	if !ralphValidator.CheckAcceptanceCriteria {
		t.Fatal("Ralph validator should check acceptance criteria")
	}
}

func TestFormatValidationOutput(t *testing.T) {
	t.Parallel()

	result := &ValidationResult{
		Valid:            false,
		IssueNumber:      157,
		Repo:             "misty-step/bitterblossom",
		Labels:           []string{"bug", "ralph-ready"},
		HasDescription:   true,
		HasBlockingLabel: false,
		Errors:           []string{"missing required labels: approved"},
		Warnings:         []string{"issue description is short"},
	}

	output := result.FormatValidationOutput()

	if !strings.Contains(output, "misty-step/bitterblossom#157") {
		t.Error("expected output to contain issue reference")
	}
	if !strings.Contains(output, "ralph-ready") {
		t.Error("expected output to contain labels")
	}
	if !strings.Contains(output, "Errors:") {
		t.Error("expected output to contain Errors section")
	}
	if !strings.Contains(output, "Warnings:") {
		t.Error("expected output to contain Warnings section")
	}
}
