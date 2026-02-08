package lifecycle

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const defaultSpriteGitHubUser = "misty-step-sprites"

// ghAuthToken returns a token from `gh auth token`, or empty string on failure.
// Package-level var for test isolation.
var ghAuthToken = func() string {
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// GitHubAuth is the resolved git identity + token for one sprite.
type GitHubAuth struct {
	User  string
	Email string
	Token string
}

// ResolveGitHubAuth resolves GitHub identity for a sprite.
// Resolution order per field:
//  1. SPRITE_GITHUB_{FIELD}_{SPRITE_KEY} (per-sprite override)
//  2. SPRITE_GITHUB_DEFAULT_{FIELD} (shared default)
//  3. GITHUB_TOKEN for token fallback
func ResolveGitHubAuth(spriteName string, getenv func(string) string) (GitHubAuth, error) {
	if getenv == nil {
		getenv = os.Getenv
	}

	key := spriteEnvKey(spriteName)
	userVar := "SPRITE_GITHUB_USER_" + key
	emailVar := "SPRITE_GITHUB_EMAIL_" + key
	tokenVar := "SPRITE_GITHUB_TOKEN_" + key

	user := strings.TrimSpace(getenv(userVar))
	if user == "" {
		user = strings.TrimSpace(getenv("SPRITE_GITHUB_DEFAULT_USER"))
	}
	if user == "" {
		user = defaultSpriteGitHubUser
	}

	email := strings.TrimSpace(getenv(emailVar))
	if email == "" {
		email = strings.TrimSpace(getenv("SPRITE_GITHUB_DEFAULT_EMAIL"))
	}
	if email == "" && user != "" {
		email = user + "@users.noreply.github.com"
	}

	token := strings.TrimSpace(getenv(tokenVar))
	if token == "" {
		token = strings.TrimSpace(getenv("SPRITE_GITHUB_DEFAULT_TOKEN"))
	}
	if token == "" {
		token = strings.TrimSpace(getenv("GITHUB_TOKEN"))
	}
	if token == "" {
		token = ghAuthToken()
	}

	if user == "" {
		return GitHubAuth{}, fmt.Errorf("GitHub user missing for sprite %q (set %s or SPRITE_GITHUB_DEFAULT_USER)", spriteName, userVar)
	}
	if email == "" {
		return GitHubAuth{}, fmt.Errorf("GitHub email missing for sprite %q (set %s or SPRITE_GITHUB_DEFAULT_EMAIL)", spriteName, emailVar)
	}
	if token == "" {
		return GitHubAuth{}, fmt.Errorf("GitHub token missing for sprite %q (set %s, SPRITE_GITHUB_DEFAULT_TOKEN, or GITHUB_TOKEN)", spriteName, tokenVar)
	}

	return GitHubAuth{
		User:  user,
		Email: email,
		Token: token,
	}, nil
}

func spriteEnvKey(name string) string {
	normalized := strings.ToUpper(strings.TrimSpace(name))
	return strings.ReplaceAll(normalized, "-", "_")
}
