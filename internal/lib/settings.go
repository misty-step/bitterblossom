package lib

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type renderedSettings struct {
	path string
}

func (r renderedSettings) Path() string {
	return r.path
}

func (r renderedSettings) Cleanup() error {
	if r.path == "" {
		return nil
	}
	if err := os.Remove(r.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func PrepareSettings(baseSettingsPath, token string) (renderedSettings, error) {
	trimmedToken := strings.TrimSpace(token)
	if trimmedToken == "" {
		return renderedSettings{}, &ValidationError{Field: "ANTHROPIC_AUTH_TOKEN", Message: "is required"}
	}

	payload, err := os.ReadFile(baseSettingsPath)
	if err != nil {
		return renderedSettings{}, fmt.Errorf("read settings template %s: %w", baseSettingsPath, err)
	}

	var settings map[string]any
	if err := json.Unmarshal(payload, &settings); err != nil {
		return renderedSettings{}, fmt.Errorf("parse settings template %s: %w", baseSettingsPath, err)
	}

	envMap, ok := settings["env"].(map[string]any)
	if !ok || envMap == nil {
		envMap = map[string]any{}
		settings["env"] = envMap
	}
	envMap["ANTHROPIC_AUTH_TOKEN"] = trimmedToken

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return renderedSettings{}, fmt.Errorf("render settings: %w", err)
	}
	out = append(out, '\n')

	tmp, err := os.CreateTemp("", "bb-settings-*.json")
	if err != nil {
		return renderedSettings{}, fmt.Errorf("create temp settings file: %w", err)
	}
	path := tmp.Name()
	defer func() {
		_ = tmp.Close()
	}()

	if err := tmp.Chmod(0o600); err != nil {
		_ = os.Remove(path)
		return renderedSettings{}, fmt.Errorf("chmod temp settings file: %w", err)
	}
	if _, err := tmp.Write(out); err != nil {
		_ = os.Remove(path)
		return renderedSettings{}, fmt.Errorf("write temp settings file: %w", err)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return renderedSettings{path: abs}, nil
}
