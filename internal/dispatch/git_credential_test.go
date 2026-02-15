package dispatch

import (
	"strings"
	"testing"
)

func TestBuildGitConfigScript_ConfiguresCredentialHelper(t *testing.T) {
	t.Parallel()

	script := buildGitConfigScript("/home/sprite/workspace")

	// Must configure credential helper
	if !strings.Contains(script, "git config --global credential.helper") {
		t.Error("script must configure git credential helper")
	}

	// Must use store helper for non-interactive operation
	if !strings.Contains(script, "store") {
		t.Error("credential helper should use 'store' mode")
	}
}

func TestBuildGitConfigScript_HandlesGitHubToken(t *testing.T) {
	t.Parallel()

	script := buildGitConfigScript("/home/sprite/workspace")

	// Must check for GITHUB_TOKEN or GH_TOKEN
	if !strings.Contains(script, "GITHUB_TOKEN") && !strings.Contains(script, "GH_TOKEN") {
		t.Error("script must reference GitHub token environment variables")
	}

	// Must configure credential store when token is present
	if !strings.Contains(script, "git credential-store") {
		t.Error("script must use git credential-store to cache credentials")
	}
}

func TestBuildGitConfigScript_HandlesMissingToken(t *testing.T) {
	t.Parallel()

	script := buildGitConfigScript("/home/sprite/workspace")

	// Should skip gracefully if no token is available
	if !strings.Contains(script, "if") || !strings.Contains(script, "then") {
		t.Error("script should check for token presence before configuring")
	}

	// Should not fail when token is missing
	if !strings.Contains(script, "echo") {
		t.Error("script should provide feedback about missing token")
	}
}

func TestBuildSetupRepoScript_WithGitConfig(t *testing.T) {
	t.Parallel()

	// Test the combined script that includes git config
	script := buildSetupRepoScriptWithGitConfig("/home/sprite/workspace", "https://github.com/misty-step/bb.git", "bb")

	// Must include git credential configuration
	if !strings.Contains(script, "credential.helper") {
		t.Error("combined script must include git credential configuration")
	}

	// Must set up git config BEFORE any git operations that might need auth
	credentialIdx := strings.Index(script, "credential.helper")
	pushIdx := strings.Index(script, "git push")

	// If there's a push operation, credential setup must come before it
	if pushIdx != -1 && credentialIdx > pushIdx {
		t.Error("git credential configuration must come before git push operations")
	}

	// Must include the original setup repo functionality
	if !strings.Contains(script, "git clone") && !strings.Contains(script, "gh repo clone") {
		t.Error("script must include repository cloning capability")
	}
}

func TestBuildGitConfigScript_CreatesCredentialStore(t *testing.T) {
	t.Parallel()

	script := buildGitConfigScript("/home/sprite/workspace")

	// Must ensure .git-credentials directory/file exists
	if !strings.Contains(script, "mkdir -p") {
		t.Error("script should ensure credential store path exists")
	}

	// Should configure credential store file
	if !strings.Contains(script, ".git-credentials") {
		t.Error("credential store should reference .git-credentials file")
	}

	// Should use git credential-store protocol
	if !strings.Contains(script, "protocol=https") {
		t.Error("credential store should use https protocol")
	}

	// Should target github.com
	if !strings.Contains(script, "host=github.com") {
		t.Error("credential store should target github.com")
	}
}
