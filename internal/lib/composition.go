package lib

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func ResolveCompositionPath(rootDir, input string) (string, error) {
	candidate := strings.TrimSpace(input)
	if candidate == "" {
		return "", &ValidationError{Field: "composition", Message: "path is required"}
	}

	resolvedRoot, err := ResolveRoot(rootDir)
	if err != nil {
		return "", err
	}

	allowedRoot, err := filepath.EvalSymlinks(filepath.Join(resolvedRoot, "compositions"))
	if err != nil {
		return "", fmt.Errorf("unable to resolve compositions directory under %s: %w", resolvedRoot, err)
	}

	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(resolvedRoot, candidate)
	}

	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(candidate))
	if err != nil {
		return "", fmt.Errorf("invalid composition path %q: %w", input, err)
	}
	resolvedPath := filepath.Join(resolvedParent, filepath.Base(candidate))

	if !isWithin(allowedRoot, resolvedPath) {
		return "", &ValidationError{Field: "composition", Message: fmt.Sprintf("must be within %s", allowedRoot)}
	}

	return resolvedPath, nil
}

func CompositionSprites(paths Paths, compositionPath string, strict bool) ([]string, string, error) {
	resolvedPath, err := ResolveCompositionPath(paths.Root, compositionPath)
	if err != nil {
		return fallbackOrFail(paths, strict, err)
	}

	file, err := os.Open(resolvedPath)
	if err != nil {
		return fallbackOrFail(paths, strict, fmt.Errorf("composition file not found: %s", resolvedPath))
	}
	defer func() {
		_ = file.Close()
	}()

	sprites, err := ParseCompositionSprites(file)
	if err != nil {
		return fallbackOrFail(paths, strict, fmt.Errorf("failed to parse composition %s: %w", resolvedPath, err))
	}

	return sprites, resolvedPath, nil
}

func fallbackOrFail(paths Paths, strict bool, cause error) ([]string, string, error) {
	if strict {
		return nil, "", cause
	}
	sprites, err := FallbackSpriteNames(paths)
	if err != nil {
		return nil, "", errors.Join(cause, err)
	}
	return sprites, "", nil
}

func FallbackSpriteNames(paths Paths) ([]string, error) {
	entries, err := os.ReadDir(paths.SpritesDir)
	if err != nil {
		return nil, fmt.Errorf("read sprites dir %s: %w", paths.SpritesDir, err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no sprite definitions found in %s", paths.SpritesDir)
	}
	sort.Strings(names)
	return names, nil
}

// ParseCompositionSprites extracts sprite names from top-level "sprites:" map.
func ParseCompositionSprites(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	inSpritesSection := false
	sectionIndent := -1
	seen := make(map[string]struct{})
	var names []string

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := leadingSpaces(line)
		if !inSpritesSection {
			if trimmed == "sprites:" {
				inSpritesSection = true
				sectionIndent = indent
			}
			continue
		}

		if indent <= sectionIndent {
			break
		}
		if indent != sectionIndent+2 {
			continue
		}
		if strings.HasPrefix(trimmed, "-") {
			continue
		}

		colon := strings.Index(trimmed, ":")
		if colon <= 0 {
			continue
		}
		name := strings.TrimSpace(trimmed[:colon])
		name = strings.Trim(name, `"'`)
		if err := ValidateSpriteName(name); err != nil {
			continue
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("duplicate sprite name %q", name)
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !inSpritesSection {
		return nil, errors.New("missing sprites section")
	}
	if len(names) == 0 {
		return nil, errors.New("no sprites found in composition")
	}
	return names, nil
}

func leadingSpaces(line string) int {
	count := 0
	for _, r := range line {
		if r != ' ' {
			break
		}
		count++
	}
	return count
}

func isWithin(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if strings.HasPrefix(rel, "..") {
		return false
	}
	return true
}
