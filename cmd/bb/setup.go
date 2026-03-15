package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	sprites "github.com/superfly/sprites-go"

	"github.com/spf13/cobra"
)

func newSetupCmd() *cobra.Command {
	var (
		repo    string
		force   bool
		persona string
	)

	cmd := &cobra.Command{
		Use:   "setup <sprite>",
		Short: "Configure a sprite with base configs, persona, and ralph loop",
		Long: `Configure a sprite with base configs, persona, and ralph loop.

If no persona file exists for the sprite name, use --persona to specify one:
  bb setup worker-1 --persona bramble      # use sprites/bramble.md
  bb setup worker-1 --persona sprites/bramble.md  # explicit path

Without --persona, bb falls back to the first available file in sprites/.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup(cmd.Context(), args[0], repo, force, persona)
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repo to clone (owner/repo)")
	cmd.Flags().BoolVar(&force, "force", false, "Re-clone repo and overwrite configs")
	cmd.Flags().StringVar(&persona, "persona", "", "Persona sprite name or file path (falls back to first available in sprites/ if unset)")

	return cmd
}

func runSetup(ctx context.Context, spriteName, repo string, force bool, persona string) error {
	openrouterKey, err := requireEnv("OPENROUTER_API_KEY")
	if err != nil {
		return err
	}

	// 1. Probe
	_, _ = fmt.Fprintf(os.Stderr, "probing %s...\n", spriteName)
	session, err := newSpriteSession(ctx, spriteName, spriteSessionOptions{probeTimeout: 15 * time.Second})
	if err != nil {
		return err
	}
	defer func() { _ = session.close() }()
	s := session.sprite

	// 2. Create remote directories
	dirs := []string{
		spriteClaudeDir,
		spriteClaudeDir + "/hooks",
		spriteClaudeDir + "/skills",
		spriteClaudeDir + "/commands",
		spriteClaudeDir + "/prompts",
		spriteCodexDir,
		spriteWorkspaceRoot,
	}
	mkdirScript := "mkdir -p " + strings.Join(dirs, " ")
	if _, err := s.CommandContext(ctx, "bash", "-c", mkdirScript).Output(); err != nil {
		return fmt.Errorf("create directories: %w", err)
	}

	// 3. Upload base configs
	_, _ = fmt.Fprintf(os.Stderr, "uploading base configs...\n")

	if err := uploadPatchedSettings(ctx, s, openrouterKey); err != nil {
		return fmt.Errorf("upload settings.json: %w", err)
	}

	configMap, err := buildBaseConfigMap(".")
	if err != nil {
		return fmt.Errorf("collect base configs: %w", err)
	}

	for local, remote := range configMap {
		if err := uploadFile(ctx, s, local, remote); err != nil {
			return fmt.Errorf("upload %s: %w", local, err)
		}
	}

	// 4. Install Codex CLI and upload config
	_, _ = fmt.Fprintf(os.Stderr, "installing codex...\n")
	codexInstall := s.CommandContext(ctx, "bash", "-c", "npm i -g @openai/codex 2>&1")
	codexInstall.Stdout = os.Stderr
	codexInstall.Stderr = os.Stderr
	if err := codexInstall.Run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: codex install failed (non-fatal): %v\n", err)
	}

	if err := uploadFile(ctx, s, "base/codex-config.toml", spriteCodexDir+"/config.toml"); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: codex config upload failed (non-fatal): %v\n", err)
	}

	if err := uploadFile(ctx, s, "base/codex-instructions.md", spriteCodexDir+"/instructions.md"); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: codex instructions upload failed (non-fatal): %v\n", err)
	}

	// 5. Upload persona
	personaFile, err := resolvePersona(spriteName, persona)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(os.Stderr, "uploading persona from %s...\n", personaFile)
	if err := uploadFile(ctx, s, personaFile, spritePersonaPath); err != nil {
		return fmt.Errorf("upload persona: %w", err)
	}

	// 6. Upload ralph script + prompt template
	if err := uploadFile(ctx, s, "scripts/ralph.sh", spriteRalphScriptPath); err != nil {
		return fmt.Errorf("upload ralph.sh: %w", err)
	}
	// Make executable
	if _, err := s.CommandContext(ctx, "chmod", "+x", spriteRalphScriptPath).Output(); err != nil {
		return fmt.Errorf("chmod ralph.sh: %w", err)
	}

	if err := uploadFile(ctx, s, "scripts/builder-prompt-template.md", spriteRalphPromptTemplatePath); err != nil {
		return fmt.Errorf("upload prompt template: %w", err)
	}

	// 7. Git auth
	_, _ = fmt.Fprintf(os.Stderr, "configuring git auth...\n")
	gitAuthScript := `
git config --global credential.helper '!f() { echo "username=x-access-token"; echo "password=$GH_TOKEN"; }; f'
git config --global user.name "bitterblossom[bot]"
git config --global user.email "bitterblossom@misty-step.dev"
git config --global --add safe.directory '*'
`
	if _, err := s.CommandContext(ctx, "bash", "-c", gitAuthScript).Output(); err != nil {
		return fmt.Errorf("git auth: %w", err)
	}

	// 8. Clone repo
	if repo != "" {
		_, _ = fmt.Fprintf(os.Stderr, "setting up repo %s...\n", repo)

		ghToken := os.Getenv("GITHUB_TOKEN")
		if ghToken == "" {
			return fmt.Errorf("GITHUB_TOKEN must be set to clone repo")
		}

		repoDir := spriteRepoWorkspace(repo)

		var cloneScript string
		if force {
			cloneScript = fmt.Sprintf(
				`rm -rf %s && cd %s && git clone https://github.com/%s.git`,
				repoDir, spriteWorkspaceRoot, repo,
			)
		} else {
			cloneScript = fmt.Sprintf(
				`if [ -d %s ]; then cd %s && git checkout master 2>/dev/null || git checkout main 2>/dev/null && git pull --ff-only; else cd %s && git clone https://github.com/%s.git; fi`,
				repoDir, repoDir, spriteWorkspaceRoot, repo,
			)
		}

		cloneScript = fmt.Sprintf("export GH_TOKEN=%q && %s", ghToken, cloneScript)
		cloneCmd := s.CommandContext(ctx, "bash", "-c", cloneScript)
		cloneCmd.Stdout = os.Stderr
		cloneCmd.Stderr = os.Stderr
		if err := cloneCmd.Run(); err != nil {
			return fmt.Errorf("repo setup: %w", err)
		}

		// Create skill directories in repo workspace
		skillDirs := fmt.Sprintf("mkdir -p %s/.claude/skills %s/.claude/commands", repoDir, repoDir)
		_, _ = s.CommandContext(ctx, "bash", "-c", skillDirs).Output()

		meta := newWorkspaceMetadata(spriteName, repo, repoDir, personaFile, time.Now())
		metaBytes, err := marshalWorkspaceMetadata(meta)
		if err != nil {
			return err
		}
		if err := s.Filesystem().WriteFileContext(ctx, workspaceMetadataPath(repoDir), metaBytes, 0644); err != nil {
			return fmt.Errorf("write workspace metadata: %w", err)
		}
	}

	_, _ = fmt.Fprintf(os.Stderr, "setup complete: %s\n", spriteName)
	return nil
}

func buildBaseConfigMap(root string) (map[string]string, error) {
	configMap := map[string]string{
		filepath.Join(root, "base/CLAUDE.md"): spriteClaudeDir + "/CLAUDE.md",
	}

	hookFiles, err := filepath.Glob(filepath.Join(root, "base/hooks/*.py"))
	if err != nil {
		return nil, err
	}
	for _, f := range hookFiles {
		configMap[f] = spriteClaudeDir + "/hooks/" + filepath.Base(f)
	}

	commandFiles, err := filepath.Glob(filepath.Join(root, "base/commands/*.md"))
	if err != nil {
		return nil, err
	}
	for _, f := range commandFiles {
		configMap[f] = spriteClaudeDir + "/commands/" + filepath.Base(f)
	}

	promptFiles, err := filepath.Glob(filepath.Join(root, "base/prompts/*.md"))
	if err != nil {
		return nil, err
	}
	for _, f := range promptFiles {
		configMap[f] = spriteClaudeDir + "/prompts/" + filepath.Base(f)
	}

	skillsRoot := filepath.Join(root, "base/skills")
	if err := filepath.WalkDir(skillsRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(skillsRoot, p)
		if err != nil {
			return err
		}
		configMap[p] = spriteClaudeDir + "/skills/" + rel
		return nil
	}); err != nil {
		return nil, err
	}

	return configMap, nil
}

// resolvePersona returns the local path to the persona file to use for setup.
// Resolution order:
//  1. Explicit --persona flag: treat as sprite name (sprites/<persona>.md) or direct path
//  2. Matching sprite name: sprites/<spriteName>.md
//  3. Fallback: first .md file found in sprites/ (alphabetical)
//
// Returns an actionable error if no persona can be resolved.
func resolvePersona(spriteName, persona string) (string, error) {
	// Explicit persona flag takes priority
	if persona != "" {
		// Check if it's a direct file path
		if _, err := os.Stat(persona); err == nil {
			return persona, nil
		}
		// Treat as sprite name: sprites/<persona>.md
		candidate := "sprites/" + strings.TrimSuffix(persona, ".md") + ".md"
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		return "", fmt.Errorf("persona %q not found (tried %s); run 'ls sprites/' to see available personas", persona, candidate)
	}

	// Try exact match for sprite name
	exact := "sprites/" + spriteName + ".md"
	if _, err := os.Stat(exact); err == nil {
		return exact, nil
	}

	// Fallback: first available persona in sprites/
	entries, err := filepath.Glob("sprites/*.md")
	if err != nil || len(entries) == 0 {
		return "", fmt.Errorf("no persona file found for %q and no fallback available in sprites/; use --persona <name>", spriteName)
	}
	fallback := entries[0]
	_, _ = fmt.Fprintf(os.Stderr, "warning: no persona for %q, using fallback %s (use --persona to override)\n", spriteName, fallback)
	return fallback, nil
}

// uploadFile reads a local file and writes it to the sprite filesystem.
func uploadFile(ctx context.Context, s *sprites.Sprite, localPath, remotePath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}

	sfs := s.Filesystem()

	// Ensure parent directory exists
	parentDir := filepath.Dir(remotePath)
	_ = sfs.MkdirAll(parentDir, 0755)

	return sfs.WriteFileContext(ctx, remotePath, data, 0644)
}

// uploadPatchedSettings reads base/settings.json, replaces secret placeholders, and uploads.
func uploadPatchedSettings(ctx context.Context, s *sprites.Sprite, openrouterKey string) error {
	data, err := os.ReadFile("base/settings.json")
	if err != nil {
		return err
	}

	patched := strings.ReplaceAll(string(data), "__SET_VIA_OPENROUTER_API_KEY_ENV__", openrouterKey)

	return s.Filesystem().WriteFileContext(ctx, spriteClaudeDir+"/settings.json", []byte(patched), 0644)
}
