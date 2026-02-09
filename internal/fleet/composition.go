package fleet

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/misty-step/bitterblossom/internal/provider"
	"github.com/misty-step/bitterblossom/internal/sprite"
)

// Composition describes desired fleet state loaded from YAML.
type Composition struct {
	Version int          `json:"version"`
	Name    string       `json:"name"`
	Source  string       `json:"source"`
	Sprites []SpriteSpec `json:"sprites"`
}

// SpriteSpec describes one desired sprite from composition input.
type SpriteSpec struct {
	Name       string         `json:"name"`
	Persona    sprite.Persona `json:"persona"`
	Definition string         `json:"definition"`
	Fallback   bool           `json:"fallback"`
	// Provider configuration for this sprite (optional, inherits from base if empty)
	Provider provider.Config `json:"provider,omitempty"`
}

type rawComposition struct {
	Version int
	Name    string
	Sprites map[string]rawSpriteSpec
}

type rawSpriteSpec struct {
	Definition     string
	Preference     string
	Philosophy     string
	Fallback       bool
	Provider       string // Provider identifier (e.g., "moonshot", "openrouter-claude")
	Model          string // Model identifier (e.g., "kimi-k2.5", "anthropic/claude-opus-4")
	ProviderConfig map[string]string // Additional provider env vars
}

var errInvalidComposition = errors.New("fleet: invalid composition")

// LoadCompositions loads all .yaml/.yml compositions from a directory.
func LoadCompositions(compositionsDir string) ([]Composition, error) {
	paths, err := compositionPaths(compositionsDir)
	if err != nil {
		return nil, err
	}

	result := make([]Composition, 0, len(paths))
	for _, path := range paths {
		composition, err := LoadComposition(path)
		if err != nil {
			return nil, err
		}
		result = append(result, composition)
	}
	return result, nil
}

// LoadComposition loads a composition and validates sprite personas against
// the sibling sprites directory.
func LoadComposition(path string) (Composition, error) {
	spritesDir := filepath.Join(filepath.Dir(filepath.Dir(path)), "sprites")
	return LoadCompositionWithSprites(path, spritesDir)
}

// ParseComposition is an alias for LoadComposition.
func ParseComposition(path string) (Composition, error) {
	return LoadComposition(path)
}

// LoadCompositionWithSprites loads one composition using an explicit personas directory.
func LoadCompositionWithSprites(path, spritesDir string) (Composition, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Composition{}, err
	}

	raw, err := parseCompositionYAML(string(content))
	if err != nil {
		return Composition{}, err
	}

	if raw.Name == "" {
		return Composition{}, fmt.Errorf("%w: missing name", errInvalidComposition)
	}
	if raw.Version <= 0 {
		return Composition{}, fmt.Errorf("%w: invalid version %d", errInvalidComposition, raw.Version)
	}
	if len(raw.Sprites) == 0 {
		return Composition{}, fmt.Errorf("%w: at least one sprite is required", errInvalidComposition)
	}

	personas, err := discoverPersonas(spritesDir)
	if err != nil {
		return Composition{}, err
	}

	names := sortedRawSpriteSpecKeys(raw.Sprites)
	specs := make([]SpriteSpec, 0, len(names))
	for _, spriteName := range names {
		rawSpec := raw.Sprites[spriteName]
		if strings.TrimSpace(rawSpec.Definition) == "" {
			return Composition{}, fmt.Errorf("%w: sprite %q missing definition", errInvalidComposition, spriteName)
		}

		resolved := resolveDefinitionPath(path, rawSpec.Definition)
		if _, err := os.Stat(resolved); err != nil {
			return Composition{}, fmt.Errorf("%w: sprite %q definition path %q missing", errInvalidComposition, spriteName, rawSpec.Definition)
		}

		personaName := strings.TrimSuffix(filepath.Base(rawSpec.Definition), filepath.Ext(rawSpec.Definition))
		if _, ok := personas[personaName]; !ok {
			return Composition{}, fmt.Errorf("%w: sprite %q references unknown persona %q", errInvalidComposition, spriteName, personaName)
		}

		// Parse provider configuration
		providerCfg, err := parseProviderConfig(rawSpec)
		if err != nil {
			return Composition{}, fmt.Errorf("%w: sprite %q has invalid provider config: %w", errInvalidComposition, spriteName, err)
		}

		specs = append(specs, SpriteSpec{
			Name: spriteName,
			Persona: sprite.Persona{
				Name:       personaName,
				Definition: resolved,
				Preference: rawSpec.Preference,
				Philosophy: rawSpec.Philosophy,
			},
			Definition: resolved,
			Fallback:   rawSpec.Fallback,
			Provider:   providerCfg,
		})
	}

	return Composition{
		Version: raw.Version,
		Name:    raw.Name,
		Source:  path,
		Sprites: specs,
	}, nil
}

// parseProviderConfig converts raw sprite spec to provider configuration.
func parseProviderConfig(raw rawSpriteSpec) (provider.Config, error) {
	// Handle the deprecated "model: inherit" pattern
	if raw.Model == "inherit" {
		raw.Model = ""
	}

	cfg := provider.Config{
		Model:       raw.Model,
		Environment: raw.ProviderConfig,
	}

	// Parse provider string if set
	if raw.Provider != "" {
		p, err := provider.ParseProvider(raw.Provider)
		if err != nil {
			return cfg, err
		}
		cfg.Provider = p
	} else if raw.Model != "" && !strings.EqualFold(raw.Model, "inherit") {
		// If model is specified without provider, try to infer provider from model
		cfg.Provider = inferProviderFromModel(raw.Model)
	}

	if err := cfg.Validate(); err != nil {
		return cfg, err
	}

	return cfg, nil
}

// inferProviderFromModel attempts to determine the provider based on model name.
func inferProviderFromModel(model string) provider.Provider {
	model = strings.ToLower(model)

	// Check for Kimi models
	if strings.Contains(model, "kimi") {
		// Check if it looks like an OpenRouter format
		if strings.Contains(model, "/") {
			return provider.ProviderOpenRouterKimi
		}
		return provider.ProviderMoonshot
	}

	// Check for Claude models
	if strings.Contains(model, "claude") {
		return provider.ProviderOpenRouterClaude
	}

	// Default to inherit for unknown models
	return provider.ProviderInherit
}

func parseCompositionYAML(input string) (rawComposition, error) {
	result := rawComposition{Sprites: map[string]rawSpriteSpec{}}

	scanner := bufio.NewScanner(strings.NewReader(input))
	inSprites := false
	currentSprite := ""
	inProviderConfig := false

	for lineNo := 1; scanner.Scan(); lineNo++ {
		rawLine := stripInlineComment(scanner.Text())
		if strings.TrimSpace(rawLine) == "" {
			continue
		}

		indent := leadingSpaces(rawLine)
		trimmed := strings.TrimSpace(rawLine)

		if indent == 0 {
			currentSprite = ""
			inSprites = false
			inProviderConfig = false

			key, value, ok := splitYAMLKeyValue(trimmed)
			if !ok {
				continue
			}

			switch key {
			case "version":
				version, err := strconv.Atoi(parseYAMLString(value))
				if err != nil {
					return rawComposition{}, fmt.Errorf("%w: invalid version at line %d", errInvalidComposition, lineNo)
				}
				result.Version = version
			case "name":
				result.Name = parseYAMLString(value)
			case "sprites":
				if strings.TrimSpace(value) != "" {
					return rawComposition{}, fmt.Errorf("%w: sprites must be a mapping", errInvalidComposition)
				}
				inSprites = true
			}
			continue
		}

		if !inSprites {
			continue
		}

		if indent == 2 && strings.HasSuffix(trimmed, ":") {
			spriteName := strings.TrimSpace(strings.TrimSuffix(trimmed, ":"))
			if spriteName == "" {
				return rawComposition{}, fmt.Errorf("%w: empty sprite name at line %d", errInvalidComposition, lineNo)
			}
			if _, exists := result.Sprites[spriteName]; exists {
				return rawComposition{}, fmt.Errorf("%w: duplicate sprite name %q", errInvalidComposition, spriteName)
			}
			result.Sprites[spriteName] = rawSpriteSpec{ProviderConfig: map[string]string{}}
			currentSprite = spriteName
			inProviderConfig = false
			continue
		}

		if currentSprite == "" || indent < 4 {
			continue
		}

		// Check for list items (skills, strengths, etc.) - skip these
		if strings.HasPrefix(strings.TrimSpace(rawLine), "-") {
			continue
		}

		key, value, ok := splitYAMLKeyValue(trimmed)
		if !ok {
			continue
		}

		spec := result.Sprites[currentSprite]

		// Handle provider config block
		if key == "provider" && value == "" {
			inProviderConfig = true
			result.Sprites[currentSprite] = spec
			continue
		}

		// Handle provider config nested keys
		if inProviderConfig && indent >= 6 {
			switch key {
			case "name", "provider":
				spec.Provider = parseYAMLString(value)
			case "model":
				spec.Model = parseYAMLString(value)
			default:
				// Additional env vars
				if spec.ProviderConfig == nil {
					spec.ProviderConfig = map[string]string{}
				}
				spec.ProviderConfig[key] = parseYAMLString(value)
			}
			result.Sprites[currentSprite] = spec
			continue
		}

		// Handle top-level sprite fields
		if indent >= 4 {
			switch key {
			case "definition":
				spec.Definition = parseYAMLString(value)
			case "preference":
				spec.Preference = parseYAMLString(value)
			case "philosophy":
				spec.Philosophy = parseYAMLString(value)
			case "fallback":
				boolValue, err := strconv.ParseBool(strings.ToLower(parseYAMLString(value)))
				if err != nil {
					return rawComposition{}, fmt.Errorf("%w: invalid fallback boolean at line %d", errInvalidComposition, lineNo)
				}
				spec.Fallback = boolValue
			case "provider":
				// Inline provider: "provider-name" format
				spec.Provider = parseYAMLString(value)
			case "model":
				spec.Model = parseYAMLString(value)
			}
			result.Sprites[currentSprite] = spec
		}
	}
	if err := scanner.Err(); err != nil {
		return rawComposition{}, err
	}

	return result, nil
}

func discoverPersonas(spritesDir string) (map[string]string, error) {
	entries, err := os.ReadDir(spritesDir)
	if err != nil {
		return nil, err
	}

	personas := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".md" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ext)
		personas[name] = filepath.Join(spritesDir, entry.Name())
	}
	if len(personas) == 0 {
		return nil, fmt.Errorf("%w: no personas in %s", errInvalidComposition, spritesDir)
	}
	return personas, nil
}

func compositionPaths(dir string) ([]string, error) {
	yamlMatches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	ymlMatches, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		return nil, err
	}

	paths := append(yamlMatches, ymlMatches...)
	sort.Strings(paths)
	return paths, nil
}

func splitYAMLKeyValue(line string) (key string, value string, ok bool) {
	index := strings.Index(line, ":")
	if index <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:index])
	value = strings.TrimSpace(line[index+1:])
	return key, value, true
}

func parseYAMLString(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		unquoted, err := strconv.Unquote(value)
		if err == nil {
			return unquoted
		}
	}
	if len(value) >= 2 && strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return value[1 : len(value)-1]
	}
	return value
}

func stripInlineComment(line string) string {
	inSingle := false
	inDouble := false

	for idx, r := range line {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return strings.TrimRight(line[:idx], " \t")
			}
		}
	}

	return strings.TrimRight(line, " \t")
}

func leadingSpaces(s string) int {
	count := 0
	for _, r := range s {
		if r != ' ' {
			break
		}
		count++
	}
	return count
}

func resolveDefinitionPath(compositionPath, definition string) string {
	if filepath.IsAbs(definition) {
		return filepath.Clean(definition)
	}

	compositionRelative := filepath.Clean(filepath.Join(filepath.Dir(compositionPath), definition))
	if _, err := os.Stat(compositionRelative); err == nil {
		return compositionRelative
	}

	rootRelative := filepath.Clean(filepath.Join(filepath.Dir(filepath.Dir(compositionPath)), definition))
	return rootRelative
}

// sortedKeys returns sorted keys from a rawSpriteSpec map.
// This is a non-generic version to avoid conflict with the generic version in fleet.go
func sortedRawSpriteSpecKeys(m map[string]rawSpriteSpec) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
