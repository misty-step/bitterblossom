package dispatch

import (
	"fmt"

	"github.com/misty-step/bitterblossom/internal/registry"
)

// ErrSpriteNotInRegistry indicates the sprite name was not found in the registry.
type ErrSpriteNotInRegistry struct {
	Name string
}

func (e *ErrSpriteNotInRegistry) Error() string {
	return fmt.Sprintf("sprite %q not found in registry — run 'bb init' to set up your fleet", e.Name)
}

// ResolveSprite looks up a sprite name in the registry and returns its Fly.io machine ID.
// If registryPath is empty, the default path (~/.config/bb/registry.toml) is used.
func ResolveSprite(name string, registryPath string) (string, error) {
	if registryPath == "" {
		registryPath = registry.DefaultPath()
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
			"referencing the issue number.",
		issue, repoClause,
	)
}
