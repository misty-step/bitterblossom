package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupSpritesDir creates a temp directory with sprite persona files and
// changes the working directory to it so resolvePersona can find sprites/.
func setupSpritesDir(t *testing.T, files []string) string {
	t.Helper()
	dir := t.TempDir()
	spritesDir := filepath.Join(dir, "sprites")
	if err := os.MkdirAll(spritesDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		path := filepath.Join(spritesDir, f)
		if err := os.WriteFile(path, []byte("# persona: "+f), 0644); err != nil {
			t.Fatal(err)
		}
	}
	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	return dir
}

func TestResolvePersonaExactMatch(t *testing.T) {
	setupSpritesDir(t, []string{"bramble.md", "willow.md"})

	got, err := resolvePersona("bramble", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "sprites/bramble.md" {
		t.Errorf("got %q, want %q", got, "sprites/bramble.md")
	}
}

func TestResolvePersonaFallbackWhenNoExactMatch(t *testing.T) {
	setupSpritesDir(t, []string{"bramble.md", "willow.md"})

	got, err := resolvePersona("e2e-0219123628", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fall back to first available (alphabetical: bramble.md)
	if got != "sprites/bramble.md" {
		t.Errorf("got %q, want %q", got, "sprites/bramble.md")
	}
}

func TestResolvePersonaExplicitPersonaFlag(t *testing.T) {
	setupSpritesDir(t, []string{"bramble.md", "willow.md"})

	got, err := resolvePersona("e2e-0219123628", "willow")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "sprites/willow.md" {
		t.Errorf("got %q, want %q", got, "sprites/willow.md")
	}
}

func TestResolvePersonaExplicitPersonaWithExtension(t *testing.T) {
	setupSpritesDir(t, []string{"bramble.md"})

	got, err := resolvePersona("worker-1", "bramble.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "sprites/bramble.md" {
		t.Errorf("got %q, want %q", got, "sprites/bramble.md")
	}
}

func TestResolvePersonaExplicitDirectPath(t *testing.T) {
	dir := t.TempDir()
	personaPath := filepath.Join(dir, "custom-persona.md")
	if err := os.WriteFile(personaPath, []byte("# custom"), 0644); err != nil {
		t.Fatal(err)
	}
	// No sprites/ dir needed — direct path resolves first
	setupSpritesDir(t, nil)

	got, err := resolvePersona("worker-1", personaPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != personaPath {
		t.Errorf("got %q, want %q", got, personaPath)
	}
}

func TestResolvePersonaErrorWhenNoPersonasAvailable(t *testing.T) {
	setupSpritesDir(t, nil) // empty sprites/

	_, err := resolvePersona("e2e-0219123628", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "--persona") {
		t.Errorf("error should mention --persona flag, got: %v", err)
	}
}

func TestResolvePersonaErrorWhenExplicitPersonaNotFound(t *testing.T) {
	setupSpritesDir(t, []string{"bramble.md"})

	_, err := resolvePersona("worker-1", "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention the missing persona name, got: %v", err)
	}
}

func TestBuildBaseConfigMapIncludesImportedSkillFilesRecursively(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(rel string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(rel), 0644); err != nil {
			t.Fatal(err)
		}
	}

	mustWrite("base/CLAUDE.md")
	mustWrite("base/hooks/test-hook.py")
	mustWrite("base/commands/commit.md")
	mustWrite("base/prompts/orientation-phase.md")
	mustWrite("base/skills/shape/SKILL.md")
	mustWrite("base/skills/shape/references/breadboarding.md")

	got, err := buildBaseConfigMap(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertRemote := func(local, wantRemote string) {
		t.Helper()
		gotRemote, ok := got[filepath.Join(dir, local)]
		if !ok {
			t.Fatalf("missing local path %q in config map", local)
		}
		if gotRemote != wantRemote {
			t.Fatalf("remote for %q = %q, want %q", local, gotRemote, wantRemote)
		}
	}

	assertRemote("base/CLAUDE.md", "/home/sprite/.claude/CLAUDE.md")
	assertRemote("base/hooks/test-hook.py", "/home/sprite/.claude/hooks/test-hook.py")
	assertRemote("base/commands/commit.md", "/home/sprite/.claude/commands/commit.md")
	assertRemote("base/prompts/orientation-phase.md", "/home/sprite/.claude/prompts/orientation-phase.md")
	assertRemote("base/skills/shape/SKILL.md", "/home/sprite/.claude/skills/shape/SKILL.md")
	assertRemote("base/skills/shape/references/breadboarding.md", "/home/sprite/.claude/skills/shape/references/breadboarding.md")
}

func TestPersistGitHubAuthScriptUsesGhCredentialHelper(t *testing.T) {
	t.Parallel()

	script := persistGitHubAuthScript("/tmp/bb-gh-token")

	if !strings.Contains(script, "gh auth login --with-token") {
		t.Fatalf("script = %q, want gh auth login", script)
	}
	if !strings.Contains(script, "credential.helper '!gh auth git-credential'") {
		t.Fatalf("script = %q, want gh auth git-credential helper", script)
	}
	if strings.Contains(script, "password=$GH_TOKEN") {
		t.Fatalf("script = %q, should not use env-backed git credential helper", script)
	}
	if !strings.Contains(script, "trap 'rm -f") {
		t.Fatalf("script = %q, want cleanup trap for token file", script)
	}
	if !strings.Contains(script, `test "$(git config --global --get credential.helper)" = "!gh auth git-credential"`) {
		t.Fatalf("script = %q, want credential helper verification", script)
	}
	if !strings.Contains(script, `git config --global pull.rebase true`) {
		t.Fatalf("script = %q, want pull.rebase configuration", script)
	}
}

func TestRepoSetupScriptDoesNotExportGHToken(t *testing.T) {
	t.Parallel()

	script := repoSetupScript("/home/sprite/workspace/repo", "misty-step/bitterblossom", false)

	if strings.Contains(script, "GH_TOKEN") {
		t.Fatalf("script = %q, should not export GH_TOKEN during repo setup", script)
	}
	if !strings.Contains(script, "git clone https://github.com/misty-step/bitterblossom.git") {
		t.Fatalf("script = %q, want clone command", script)
	}
	if !strings.Contains(script, "git pull --rebase") {
		t.Fatalf("script = %q, want rebase-based pull for existing repos", script)
	}
}

func TestRenderSpriteRuntimeEnvMirrorsOpenAIIntoCodexWithoutGitHubToken(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-openai-test")
	t.Setenv("CODEX_API_KEY", "") // unset so fallback kicks in
	t.Setenv("EXA_API_KEY", "exa-test-key")
	t.Setenv("GITHUB_TOKEN", "ghp-should-not-leak")

	rendered := renderSpriteRuntimeEnv()

	if !strings.Contains(rendered, "export OPENAI_API_KEY='sk-openai-test'") {
		t.Fatalf("runtime env = %q, want OPENAI_API_KEY export", rendered)
	}
	if !strings.Contains(rendered, "export CODEX_API_KEY='sk-openai-test'") {
		t.Fatalf("runtime env = %q, want CODEX_API_KEY fallback from OPENAI_API_KEY", rendered)
	}
	if !strings.Contains(rendered, "export EXA_API_KEY='exa-test-key'") {
		t.Fatalf("runtime env = %q, want EXA_API_KEY export", rendered)
	}
	if strings.Contains(rendered, "GITHUB_TOKEN") {
		t.Fatalf("runtime env = %q, should not include GITHUB_TOKEN", rendered)
	}
}

func TestRenderSpriteRuntimeEnvExplicitCodexAPIKeyTakesPrecedence(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-openai-test")
	t.Setenv("CODEX_API_KEY", "sk-codex-explicit")

	rendered := renderSpriteRuntimeEnv()

	if !strings.Contains(rendered, "export OPENAI_API_KEY='sk-openai-test'") {
		t.Fatalf("runtime env = %q, want OPENAI_API_KEY export", rendered)
	}
	if !strings.Contains(rendered, "export CODEX_API_KEY='sk-codex-explicit'") {
		t.Fatalf("runtime env = %q, want explicit CODEX_API_KEY", rendered)
	}
	if strings.Count(rendered, "CODEX_API_KEY") != 1 {
		t.Fatalf("runtime env = %q, want exactly one CODEX_API_KEY entry", rendered)
	}
}

func TestRenderSpriteRuntimeEnvCodexOnlyWithoutOpenAI(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("CODEX_API_KEY", "sk-codex-only")
	t.Setenv("EXA_API_KEY", "")

	rendered := renderSpriteRuntimeEnv()

	if strings.Contains(rendered, "OPENAI_API_KEY") {
		t.Fatalf("runtime env = %q, should not include OPENAI_API_KEY when unset", rendered)
	}
	if !strings.Contains(rendered, "export CODEX_API_KEY='sk-codex-only'") {
		t.Fatalf("runtime env = %q, want CODEX_API_KEY export", rendered)
	}
}

func TestRenderSpriteRuntimeEnvEmptyWhenNoKeysSet(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("CODEX_API_KEY", "")
	t.Setenv("EXA_API_KEY", "")

	rendered := renderSpriteRuntimeEnv()

	if strings.Contains(rendered, "export ") {
		t.Fatalf("runtime env = %q, should have no exports when no keys set", rendered)
	}
	if !strings.Contains(rendered, "# Managed by Bitterblossom") {
		t.Fatalf("runtime env = %q, want header comment", rendered)
	}
}

func TestShellQuoteEscapesSingleQuotes(t *testing.T) {
	t.Parallel()

	got := shellQuote("sk'key")
	want := "'sk'\"'\"'key'"
	if got != want {
		t.Fatalf("shellQuote(%q) = %q, want %q", "sk'key", got, want)
	}
}

func TestRuntimeEnvSourceCommandUsesFilePathNotInlineSecrets(t *testing.T) {
	t.Parallel()

	command := runtimeEnvSourceCommand(spriteRuntimeEnvPath)

	if !strings.Contains(command, spriteRuntimeEnvPath) {
		t.Fatalf("command = %q, want runtime env path", command)
	}
	if strings.Contains(command, "OPENAI_API_KEY=") || strings.Contains(command, "CODEX_API_KEY=") {
		t.Fatalf("command = %q, should not inline API keys", command)
	}
	if !strings.Contains(command, ". ") {
		t.Fatalf("command = %q, want shell source invocation", command)
	}
}
