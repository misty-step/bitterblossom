package fleet

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var spriteKeyRe = regexp.MustCompile(`^\s{2}([a-z][a-z0-9-]*):\s*$`)

// CompositionSprites parses sprite names from compositions YAML without external deps.
func CompositionSprites(path string) ([]string, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open composition: %w", err)
	}
	defer func() { _ = fh.Close() }()

	var out []string
	sc := bufio.NewScanner(fh)
	inSprites := false
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "sprites:" {
			inSprites = true
			continue
		}
		if !inSprites {
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, "  ") {
			break
		}
		if strings.HasPrefix(line, "    ") {
			continue
		}
		match := spriteKeyRe.FindStringSubmatch(line)
		if len(match) == 2 {
			out = append(out, match[1])
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read composition: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no sprites in composition")
	}
	return out, nil
}

// FallbackSpriteNames loads sprite names from sprites/*.md.
func FallbackSpriteNames(spritesDir string) ([]string, error) {
	entries, err := os.ReadDir(spritesDir)
	if err != nil {
		return nil, fmt.Errorf("read sprites dir: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		names = append(names, strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no sprite definitions")
	}
	return names, nil
}
