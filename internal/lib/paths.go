package lib

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultOrg          = "misty-step"
	DefaultSpriteCLI    = "sprite"
	DefaultRemoteHome   = "/home/sprite"
	DefaultComposition  = "compositions/v1.yaml"
	DefaultSettingsPath = "base/settings.json"
)

// Paths centralizes all repo-relative locations used by bb.
type Paths struct {
	Root         string
	BaseDir      string
	SpritesDir   string
	ScriptsDir   string
	CompsDir     string
	Observations string
	ArchivesDir  string
}

func ResolveRoot(root string) (string, error) {
	candidate := strings.TrimSpace(root)
	if candidate == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve cwd: %w", err)
		}
		candidate = cwd
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve root %q: %w", candidate, err)
	}
	return resolved, nil
}

func NewPaths(root string) (Paths, error) {
	resolved, err := ResolveRoot(root)
	if err != nil {
		return Paths{}, err
	}
	return Paths{
		Root:         resolved,
		BaseDir:      filepath.Join(resolved, "base"),
		SpritesDir:   filepath.Join(resolved, "sprites"),
		ScriptsDir:   filepath.Join(resolved, "scripts"),
		CompsDir:     filepath.Join(resolved, "compositions"),
		Observations: filepath.Join(resolved, "observations"),
		ArchivesDir:  filepath.Join(resolved, "observations", "archives"),
	}, nil
}

func (p Paths) BaseSettingsPath() string {
	return filepath.Join(p.Root, DefaultSettingsPath)
}

func (p Paths) RalphTemplatePath() string {
	return filepath.Join(p.ScriptsDir, "ralph-prompt-template.md")
}
