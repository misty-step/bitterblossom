package dispatch

import (
	"errors"
	"fmt"
	"os"

	"github.com/misty-step/bitterblossom/internal/registry"
)

// ErrSpriteNotInRegistry indicates the sprite name was not found in the registry.
type ErrSpriteNotInRegistry struct {
	Name string
}

func (e *ErrSpriteNotInRegistry) Error() string {
	return fmt.Sprintf("sprite %q not found in registry — run 'bb init' to set up your fleet", e.Name)
}

// ErrRegistryNotFound indicates the registry file does not exist.
type ErrRegistryNotFound struct {
	Path string
}

func (e *ErrRegistryNotFound) Error() string {
	return fmt.Sprintf("registry file not found at %q — run 'bb init' to create it", e.Path)
}

// ResolveSprite looks up a sprite name in the registry and returns its Fly.io machine ID.
// If registryPath is empty, the default path (~/.config/bb/registry.toml) is used.
// Returns ErrRegistryNotFound if the registry file does not exist.
func ResolveSprite(name string, registryPath string) (string, error) {
	if registryPath == "" {
		registryPath = registry.DefaultPath()
	}

	// Check if registry file exists before loading
	if _, err := os.Stat(registryPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", &ErrRegistryNotFound{Path: registryPath}
		}
		return "", fmt.Errorf("check registry file: %w", err)
	}

	reg, err := registry.Load(registryPath)
	if err != nil {
		return "", fmt.Errorf("load registry: %w", err)
	}

	machineID, ok := reg.LookupMachine(name)
	if !ok {
		return "", &ErrSpriteNotInRegistry{Name: name}
	}

	return machineID, nil
}

// IssuePrompt generates the default dispatch prompt for a GitHub issue number.
func IssuePrompt(issue int, repo string) string {
	repoClause := ""
	if repo != "" {
		repoClause = fmt.Sprintf(" in %s", repo)
	}
	return fmt.Sprintf(
		"Implement GitHub issue #%d%s. Read the issue for the full spec. "+
			"Run tests to verify your changes. Commit with a descriptive message "+
			"referencing the issue number. Push your branch and open a PR.\n\n"+
			"When complete, write a summary to a file named exactly TASK_COMPLETE with no file extension "+
			"(e.g. echo 'Done: fixed X' > TASK_COMPLETE). Do NOT use TASK_COMPLETE.md.\n"+
			"If blocked, write the reason to BLOCKED.md and stop.",
		issue, repoClause,
	)
}
