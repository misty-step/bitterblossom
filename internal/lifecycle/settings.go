package lifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// RenderSettings reads settings from settingsPath, injects the auth token,
// and returns the path to a rendered temp file. Caller must clean up.
func RenderSettings(settingsPath, authToken string) (string, error) {
	token := strings.TrimSpace(authToken)
	if token == "" {
		return "", fmt.Errorf("ANTHROPIC_AUTH_TOKEN is required")
	}

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		return "", fmt.Errorf("read settings %q: %w", settingsPath, err)
	}

	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		return "", fmt.Errorf("parse settings %q: %w", settingsPath, err)
	}

	envAny, ok := settings["env"]
	if !ok {
		settings["env"] = map[string]any{}
		envAny = settings["env"]
	}
	env, ok := envAny.(map[string]any)
	if !ok {
		return "", fmt.Errorf("settings %q has invalid env object", settingsPath)
	}
	env["ANTHROPIC_AUTH_TOKEN"] = token

	encoded, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode rendered settings: %w", err)
	}
	encoded = append(encoded, '\n')

	file, err := os.CreateTemp("", "bb-settings-*.json")
	if err != nil {
		return "", fmt.Errorf("create rendered settings tempfile: %w", err)
	}
	path := file.Name()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("chmod rendered settings tempfile: %w", err)
	}
	if _, err := file.Write(encoded); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write rendered settings tempfile: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close rendered settings tempfile: %w", err)
	}
	return path, nil
}
