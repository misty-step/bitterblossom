package lifecycle

import (
	"strings"
	"testing"
)

func TestResolveGitHubAuthPerSpriteOverride(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"SPRITE_GITHUB_USER_BRAMBLE":  "sprite-user",
		"SPRITE_GITHUB_EMAIL_BRAMBLE": "sprite@example.com",
		"SPRITE_GITHUB_TOKEN_BRAMBLE": "sprite-token",
	}

	auth, err := ResolveGitHubAuth("bramble", envLookup(env))
	if err != nil {
		t.Fatalf("ResolveGitHubAuth() error = %v", err)
	}
	if auth.User != "sprite-user" || auth.Email != "sprite@example.com" || auth.Token != "sprite-token" {
		t.Fatalf("unexpected auth: %+v", auth)
	}
}

func TestResolveGitHubAuthDefaultFallback(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"SPRITE_GITHUB_DEFAULT_USER":  "default-user",
		"SPRITE_GITHUB_DEFAULT_EMAIL": "default@example.com",
		"SPRITE_GITHUB_DEFAULT_TOKEN": "default-token",
	}

	auth, err := ResolveGitHubAuth("thorn", envLookup(env))
	if err != nil {
		t.Fatalf("ResolveGitHubAuth() error = %v", err)
	}
	if auth.User != "default-user" || auth.Email != "default@example.com" || auth.Token != "default-token" {
		t.Fatalf("unexpected auth: %+v", auth)
	}
}

func TestResolveGitHubAuthGitHubTokenFallback(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"SPRITE_GITHUB_DEFAULT_USER": "default-user",
		"GITHUB_TOKEN":               "legacy-token",
	}

	auth, err := ResolveGitHubAuth("fern", envLookup(env))
	if err != nil {
		t.Fatalf("ResolveGitHubAuth() error = %v", err)
	}
	if auth.Token != "legacy-token" {
		t.Fatalf("token = %q, want legacy-token", auth.Token)
	}
}

func TestResolveGitHubAuthMissingCredentials(t *testing.T) {
	orig := ghAuthToken
	ghAuthToken = func() string { return "" }
	t.Cleanup(func() { ghAuthToken = orig })

	_, err := ResolveGitHubAuth("moss", envLookup(map[string]string{}))
	if err == nil {
		t.Fatal("expected error for missing token")
	}
	if !strings.Contains(err.Error(), "GitHub token missing") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveGitHubAuthGhCLIFallback(t *testing.T) {
	orig := ghAuthToken
	ghAuthToken = func() string { return "gh-cli-token" }
	t.Cleanup(func() { ghAuthToken = orig })

	env := map[string]string{
		"SPRITE_GITHUB_DEFAULT_USER": "default-user",
	}

	auth, err := ResolveGitHubAuth("vine", envLookup(env))
	if err != nil {
		t.Fatalf("ResolveGitHubAuth() error = %v", err)
	}
	if auth.Token != "gh-cli-token" {
		t.Fatalf("token = %q, want gh-cli-token", auth.Token)
	}
}

func envLookup(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}
