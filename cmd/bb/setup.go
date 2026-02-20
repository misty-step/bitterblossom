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
		repo  string
		force bool
	)

	cmd := &cobra.Command{
		Use:   "setup <sprite>",
		Short: "Configure a sprite with base configs, persona, and ralph loop",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup(cmd.Context(), args[0], repo, force)
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repo to clone (owner/repo)")
	cmd.Flags().BoolVar(&force, "force", false, "Re-clone repo and overwrite configs")

	return cmd
}

func runSetup(ctx context.Context, spriteName, repo string, force bool) error {
	token, err := spriteToken()
	if err != nil {
		return err
	}
	openrouterKey, err := requireEnv("OPENROUTER_API_KEY")
	if err != nil {
		return err
	}

	client := sprites.New(token)
	defer func() { _ = client.Close() }()
	s := client.Sprite(spriteName)

	// 1. Probe
	_, _ = fmt.Fprintf(os.Stderr, "probing %s...\n", spriteName)
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if _, err := s.CommandContext(probeCtx, "echo", "ok").Output(); err != nil {
		return fmt.Errorf("sprite %q unreachable: %w", spriteName, err)
	}

	// 2. Create remote directories
	dirs := []string{
		"/home/sprite/.claude",
		"/home/sprite/.claude/hooks",
		"/home/sprite/.claude/skills",
		"/home/sprite/.claude/commands",
		"/home/sprite/.claude/prompts",
		"/home/sprite/workspace",
	}
	mkdirScript := "mkdir -p " + strings.Join(dirs, " ")
	if _, err := s.CommandContext(ctx, "bash", "-c", mkdirScript).Output(); err != nil {
		return fmt.Errorf("create directories: %w", err)
	}

	// 3. Upload base configs
	_, _ = fmt.Fprintf(os.Stderr, "uploading base configs...\n")

	configMap := map[string]string{
		"base/CLAUDE.md": "/home/sprite/.claude/CLAUDE.md",
	}

	if err := uploadPatchedSettings(ctx, s, openrouterKey); err != nil {
		return fmt.Errorf("upload settings.json: %w", err)
	}

	// hooks
	hookFiles, _ := filepath.Glob("base/hooks/*.py")
	for _, f := range hookFiles {
		configMap[f] = "/home/sprite/.claude/hooks/" + filepath.Base(f)
	}

	// commands
	cmdFiles, _ := filepath.Glob("base/commands/*.md")
	for _, f := range cmdFiles {
		configMap[f] = "/home/sprite/.claude/commands/" + filepath.Base(f)
	}

	// prompts
	promptFiles, _ := filepath.Glob("base/prompts/*.md")
	for _, f := range promptFiles {
		configMap[f] = "/home/sprite/.claude/prompts/" + filepath.Base(f)
	}

	// skills — each skill is a directory with SKILL.md
	_ = filepath.WalkDir("base/skills", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel("base/skills", p)
		configMap[p] = "/home/sprite/.claude/skills/" + rel
		return nil
	})

	for local, remote := range configMap {
		if err := uploadFile(ctx, s, local, remote); err != nil {
			return fmt.Errorf("upload %s: %w", local, err)
		}
	}

	// 4. Upload persona
	personaFile := "sprites/" + spriteName + ".md"
	if _, err := os.Stat(personaFile); err != nil {
		return fmt.Errorf("persona not found: %s", personaFile)
	}
	if err := uploadFile(ctx, s, personaFile, "/home/sprite/workspace/PERSONA.md"); err != nil {
		return fmt.Errorf("upload persona: %w", err)
	}

	// 5. Upload ralph script + prompt template
	if err := uploadFile(ctx, s, "scripts/ralph.sh", "/home/sprite/workspace/.ralph.sh"); err != nil {
		return fmt.Errorf("upload ralph.sh: %w", err)
	}
	// Make executable
	if _, err := s.CommandContext(ctx, "chmod", "+x", "/home/sprite/workspace/.ralph.sh").Output(); err != nil {
		return fmt.Errorf("chmod ralph.sh: %w", err)
	}

	if err := uploadFile(ctx, s, "scripts/ralph-prompt-template.md", "/home/sprite/workspace/.ralph-prompt-template.md"); err != nil {
		return fmt.Errorf("upload prompt template: %w", err)
	}

	// 6. Git auth
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

	// 7. Clone repo
	if repo != "" {
		_, _ = fmt.Fprintf(os.Stderr, "setting up repo %s...\n", repo)

		ghToken := os.Getenv("GITHUB_TOKEN")
		if ghToken == "" {
			return fmt.Errorf("GITHUB_TOKEN must be set to clone repo")
		}

		repoName := filepath.Base(repo)
		repoDir := "/home/sprite/workspace/" + repoName

		var cloneScript string
		if force {
			cloneScript = fmt.Sprintf(
				`rm -rf %s && cd /home/sprite/workspace && git clone https://github.com/%s.git`,
				repoDir, repo,
			)
		} else {
			cloneScript = fmt.Sprintf(
				`if [ -d %s ]; then cd %s && git checkout master 2>/dev/null || git checkout main 2>/dev/null && git pull --ff-only; else cd /home/sprite/workspace && git clone https://github.com/%s.git; fi`,
				repoDir, repoDir, repo,
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
	}

	_, _ = fmt.Fprintf(os.Stderr, "setup complete: %s\n", spriteName)
	return nil
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

	return s.Filesystem().WriteFileContext(ctx, "/home/sprite/.claude/settings.json", []byte(patched), 0644)
}
