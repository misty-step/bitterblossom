package config

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// Composition represents a composition YAML file.
type Composition struct {
	Version int                      `yaml:"version"`
	Name    string                   `yaml:"name"`
	Sprites map[string]SpriteProfile `yaml:"sprites"`
}

// SpriteProfile includes only fields the control plane needs.
type SpriteProfile struct {
	Definition string `yaml:"definition"`
}

// LoadComposition parses one composition YAML file from disk.
func LoadComposition(path string) (Composition, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Composition{}, fmt.Errorf("reading composition %q: %w", path, err)
	}

	var composition Composition
	if err := yaml.Unmarshal(raw, &composition); err != nil {
		return Composition{}, fmt.Errorf("parsing composition %q: %w", path, err)
	}
	if len(composition.Sprites) == 0 {
		return Composition{}, fmt.Errorf("composition %q has no sprites", path)
	}
	return composition, nil
}

// SpriteNames returns sorted sprite keys.
func SpriteNames(composition Composition) []string {
	names := make([]string, 0, len(composition.Sprites))
	for name := range composition.Sprites {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
