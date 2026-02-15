package dispatch

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// ValidationProfile controls how policy validations are treated.
// Safety validations are always enforced regardless of profile.
type ValidationProfile string

const (
	// ValidationProfileAdvisory emits policy violations as warnings (default).
	ValidationProfileAdvisory ValidationProfile = "advisory"
	// ValidationProfileStrict converts policy warnings to hard errors.
	ValidationProfileStrict ValidationProfile = "strict"
	// ValidationProfileOff skips policy layer entirely; safety checks remain.
	ValidationProfileOff ValidationProfile = "off"
)

// IsValid returns true if the profile is a known value.
func (p ValidationProfile) IsValid() bool {
	switch p {
	case ValidationProfileAdvisory, ValidationProfileStrict, ValidationProfileOff:
		return true
	}
	return false
}

// String returns the string representation.
func (p ValidationProfile) String() string {
	return string(p)
}

// ParseValidationProfile parses a string into a ValidationProfile.
// Returns ValidationProfileAdvisory as default for empty/unknown values.
func ParseValidationProfile(s string) ValidationProfile {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "strict":
		return ValidationProfileStrict
	case "off":
		return ValidationProfileOff
	case "advisory", "":
		return ValidationProfileAdvisory
	default:
		return ValidationProfileAdvisory
	}
}

// SafetyCheckResult contains the result of safety layer validation.
// These checks cannot be bypassed by any validation profile.
type SafetyCheckResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
	Sprite string   `json:"sprite,omitempty"`
	Repo   string   `json:"repo,omitempty"`
}

// PolicyCheckResult contains the result of policy layer validation.
// These checks are controlled by the validation profile.
type PolicyCheckResult struct {
	Warnings         []string `json:"warnings,omitempty"`
	Errors           []string `json:"errors,omitempty"`
	HasBlockingLabel bool     `json:"has_blocking_label"`
	HasDescription   bool     `json:"has_description"`
	Labels           []string `json:"labels,omitempty"`
}

// CombinedValidationResult contains both safety and policy results.
type CombinedValidationResult struct {
	Safety SafetyCheckResult `json:"safety"`
	Policy PolicyCheckResult `json:"policy"`
	// Legacy fields for backward compatibility
	Valid       bool     `json:"valid"`
	Warnings    []string `json:"warnings,omitempty"`
	Errors      []string `json:"errors,omitempty"`
	IssueNumber int      `json:"issue_number,omitempty"`
	Repo        string   `json:"repo,omitempty"`
	Labels      []string `json:"labels,omitempty"`
}

// IsSafe returns true if all safety checks passed.
func (r *CombinedValidationResult) IsSafe() bool {
	return r.Safety.Valid && len(r.Safety.Errors) == 0
}

// HasIssues returns true if there are any safety errors, policy errors, or policy warnings.
func (r *CombinedValidationResult) HasIssues() bool {
	return len(r.Safety.Errors) > 0 || len(r.Policy.Errors) > 0 || len(r.Policy.Warnings) > 0
}

// IsPolicyCompliant returns true based on the validation profile.
func (r *CombinedValidationResult) IsPolicyCompliant(profile ValidationProfile) bool {
	switch profile {
	case ValidationProfileOff:
		return true
	case ValidationProfileStrict:
		return len(r.Policy.Errors) == 0 && len(r.Policy.Warnings) == 0
	case ValidationProfileAdvisory:
		return len(r.Policy.Errors) == 0
	}
	return len(r.Policy.Errors) == 0
}

// ToError returns an error if validation failed according to the profile.
func (r *CombinedValidationResult) ToError(profile ValidationProfile) error {
	if r.IsSafe() && r.IsPolicyCompliant(profile) {
		return nil
	}

	var parts []string
	if !r.IsSafe() {
		parts = append(parts, "Safety errors (cannot be bypassed):")
		for _, e := range r.Safety.Errors {
			parts = append(parts, "  ✗ "+e)
		}
	}
	if !r.IsPolicyCompliant(profile) {
		parts = append(parts, "Policy violations:")
		for _, e := range r.Policy.Errors {
			parts = append(parts, "  ✗ "+e)
		}
		if profile == ValidationProfileStrict {
			for _, w := range r.Policy.Warnings {
				parts = append(parts, "  ✗ "+w)
			}
		}
	}

	return &ErrIssueNotReady{
		Issue:  r.IssueNumber,
		Repo:   r.Repo,
		Reason: strings.Join(parts, "\n"),
	}
}

// FormatReport returns a human-readable validation report for CLI output.
func (r *CombinedValidationResult) FormatReport(profile ValidationProfile) string {
	var lines []string

	if !r.IsSafe() {
		lines = append(lines, "Safety validation failed (cannot be bypassed):")
		for _, e := range r.Safety.Errors {
			lines = append(lines, "  ✗ "+e)
		}
	}

	if len(r.Policy.Errors) > 0 {
		lines = append(lines, "Policy validation failed:")
		for _, e := range r.Policy.Errors {
			lines = append(lines, "  ✗ "+e)
		}
	}

	if len(r.Policy.Warnings) > 0 {
		if profile == ValidationProfileStrict {
			lines = append(lines, "Policy warnings (strict mode — treated as errors):")
		} else {
			lines = append(lines, "Policy warnings:")
		}
		for _, w := range r.Policy.Warnings {
			if profile == ValidationProfileStrict {
				lines = append(lines, "  ✗ "+w)
			} else {
				lines = append(lines, "  ⚠ "+w)
			}
		}
	}

	return strings.Join(lines, "\n")
}

// SafetyValidator performs safety checks that cannot be bypassed.
type SafetyValidator struct {
	// SecretChecker validates commands don't contain secrets
	SecretChecker func(command, context string) error
}

// DefaultSafetyValidator returns a safety validator with standard checks.
func DefaultSafetyValidator() *SafetyValidator {
	return &SafetyValidator{
		SecretChecker: ValidateCommandNoSecrets,
	}
}

// ValidateSafety performs safety checks on a dispatch request.
// These checks are always enforced regardless of validation profile.
func (v *SafetyValidator) ValidateSafety(req Request) *SafetyCheckResult {
	result := &SafetyCheckResult{
		Valid:  true,
		Sprite: req.Sprite,
		Repo:   req.Repo,
	}

	// Validate sprite name is not empty
	if strings.TrimSpace(req.Sprite) == "" {
		result.Errors = append(result.Errors, "sprite name is required")
		result.Valid = false
	}

	// Validate repo format if provided
	if req.Repo != "" {
		normalized := normalizeRepoSlug(req.Repo)
		if normalized == "" {
			result.Errors = append(result.Errors, "invalid repository format (expected owner/repo)")
			result.Valid = false
		}
	}

	// Validate prompt doesn't contain secrets (basic check)
	if req.Prompt != "" && ContainsSecret(req.Prompt) {
		result.Errors = append(result.Errors, "prompt contains potential secret (API key pattern detected)")
		result.Valid = false
	}

	return result
}

// ValidateSafetyWithEnv performs safety checks including environment validation.
func (v *SafetyValidator) ValidateSafetyWithEnv(ctx context.Context, req Request, env map[string]string, allowDirect bool) *SafetyCheckResult {
	result := v.ValidateSafety(req)

	// Check for direct Anthropic key (bypasses proxy)
	if err := ValidateNoDirectAnthropic(env, allowDirect); err != nil {
		result.Errors = append(result.Errors, err.Error())
		result.Valid = false
	}

	return result
}

// secretPatterns matches known credential prefixes that must never appear
// in command-line arguments (visible via ps aux / Fly dashboard).
//
// Each pattern requires the prefix plus enough key-like characters to
// distinguish real keys from innocent strings like "sk-ants-are-cool".
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-ant-api[a-zA-Z0-9]`), // Anthropic API keys
	regexp.MustCompile(`sk-or-v1-[a-f0-9]{8}`),  // OpenRouter API keys
	regexp.MustCompile(`ghp_[A-Za-z0-9]{4}`),    // GitHub personal access tokens
}

// ContainsSecret reports whether s contains a string matching known API key
// or token patterns. Use this to validate that constructed shell commands
// do not leak credentials into process argument lists.
func ContainsSecret(s string) bool {
	for _, pat := range secretPatterns {
		if pat.MatchString(s) {
			return true
		}
	}
	return false
}

// ErrSecretInCommand indicates that a constructed command contains what looks
// like a credential, which would be visible in process listings.
type ErrSecretInCommand struct {
	Context string
}

func (e *ErrSecretInCommand) Error() string {
	return fmt.Sprintf("dispatch: credential detected in %s — secrets must be passed via env files, not command arguments", e.Context)
}

// ErrDirectAnthropicKey indicates that a real Anthropic API key was detected
// in the dispatch environment, which would bypass the proxy and use expensive
// Anthropic credits directly.
type ErrDirectAnthropicKey struct {
	KeyPrefix string
}

func (e *ErrDirectAnthropicKey) Error() string {
	return fmt.Sprintf(
		"ANTHROPIC_API_KEY contains a real key (%s...) — this would bypass the proxy "+
			"and use expensive Anthropic credits. Set ANTHROPIC_API_KEY=\"\" and use "+
			"ANTHROPIC_AUTH_TOKEN for OpenRouter instead",
		e.KeyPrefix,
	)
}

// ValidateNoDirectAnthropic checks that the dispatch environment does not contain
// a real Anthropic API key that would bypass the proxy.
//
// Acceptable values for ANTHROPIC_API_KEY: empty string, "proxy-mode", unset.
// If the key starts with "sk-ant-", dispatch is blocked unless allowDirect is true.
func ValidateNoDirectAnthropic(env map[string]string, allowDirect bool) error {
	key, exists := env["ANTHROPIC_API_KEY"]
	if !exists {
		return nil
	}
	if key == "" || key == "proxy-mode" {
		return nil
	}
	if strings.HasPrefix(key, "sk-ant-") && !allowDirect {
		prefix := key
		if len(prefix) > 12 {
			prefix = prefix[:12]
		}
		return &ErrDirectAnthropicKey{KeyPrefix: prefix}
	}
	return nil
}

// ValidateCommandNoSecrets checks that a constructed command string does not
// contain credentials. Returns an error if secrets are detected.
func ValidateCommandNoSecrets(command, context string) error {
	if ContainsSecret(command) {
		return &ErrSecretInCommand{Context: context}
	}
	return nil
}

// ErrOrphanSprite indicates dispatch was attempted to a sprite that exists
// remotely but is not in the loaded composition. Orphan sprites lack persistent
// workspace volumes, so dispatches run in void and produce no durable work.
type ErrOrphanSprite struct {
	Sprite      string
	Composition []string
}

func (e *ErrOrphanSprite) Error() string {
	return fmt.Sprintf(
		"sprite %q is not in the loaded composition — it may lack a persistent workspace volume. "+
			"Dispatch to orphan sprites runs in void and produces no durable work. "+
			"Use a composition sprite (%s) or pass --allow-orphan to override",
		e.Sprite, strings.Join(e.Composition, ", "),
	)
}
