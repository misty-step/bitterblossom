package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// SpriteInfo contains basic metadata from sprite API endpoints.
type SpriteInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	URL    string `json:"url,omitempty"`
}

// SpriteClient wraps sprite CLI operations.
type SpriteClient interface {
	List(ctx context.Context, org string) ([]string, error)
	Exec(ctx context.Context, org, sprite, command string) (string, error)
	API(ctx context.Context, org, path string) ([]byte, error)
	SpriteAPI(ctx context.Context, org, sprite, path string) ([]byte, error)
	ListCheckpoints(ctx context.Context, org, sprite string) (string, error)
	ListSprites(ctx context.Context, org string) ([]SpriteInfo, error)
}

// SpriteCLI implements SpriteClient using the sprite binary.
type SpriteCLI struct {
	Bin    string
	Runner Runner
}

// NewSpriteCLI builds a SpriteCLI with sane defaults.
func NewSpriteCLI(r Runner, binary string) *SpriteCLI {
	if binary == "" {
		binary = "sprite"
	}
	return &SpriteCLI{Bin: binary, Runner: r}
}

func (s *SpriteCLI) run(ctx context.Context, args ...string) (string, error) {
	out, _, err := s.Runner.Run(ctx, s.Bin, args...)
	if err != nil {
		return out, err
	}
	return out, nil
}

// List returns sprite names for an org.
func (s *SpriteCLI) List(ctx context.Context, org string) ([]string, error) {
	args := []string{"list"}
	if org != "" {
		args = append(args, "-o", org)
	}
	out, err := s.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		names = append(names, line)
	}
	return names, nil
}

// Exec runs a shell command on a sprite.
func (s *SpriteCLI) Exec(ctx context.Context, org, sprite, command string) (string, error) {
	if sprite == "" {
		return "", fmt.Errorf("sprite name required")
	}
	args := []string{"exec"}
	if org != "" {
		args = append(args, "-o", org)
	}
	args = append(args, "-s", sprite, "--", "bash", "-lc", command)
	return s.run(ctx, args...)
}

// API calls sprite api for org-scoped endpoints.
func (s *SpriteCLI) API(ctx context.Context, org, path string) ([]byte, error) {
	if path == "" {
		path = "/"
	}
	args := []string{"api"}
	if org != "" {
		args = append(args, "-o", org)
	}
	args = append(args, path)
	out, err := s.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}

// SpriteAPI calls sprite api for a specific sprite endpoint.
func (s *SpriteCLI) SpriteAPI(ctx context.Context, org, sprite, path string) ([]byte, error) {
	if sprite == "" {
		return nil, fmt.Errorf("sprite name required")
	}
	if path == "" {
		path = "/"
	}
	args := []string{"api"}
	if org != "" {
		args = append(args, "-o", org)
	}
	args = append(args, "-s", sprite, path)
	out, err := s.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}

// ListCheckpoints returns raw checkpoint output.
func (s *SpriteCLI) ListCheckpoints(ctx context.Context, org, sprite string) (string, error) {
	if sprite == "" {
		return "", fmt.Errorf("sprite name required")
	}
	args := []string{"checkpoint", "list"}
	if org != "" {
		args = append(args, "-o", org)
	}
	args = append(args, "-s", sprite)
	return s.run(ctx, args...)
}

// ListSprites fetches /sprites API list and decodes it.
func (s *SpriteCLI) ListSprites(ctx context.Context, org string) ([]SpriteInfo, error) {
	payload, err := s.API(ctx, org, "/sprites")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Sprites []SpriteInfo `json:"sprites"`
	}
	if err := json.Unmarshal(payload, &resp); err != nil {
		return nil, fmt.Errorf("decode sprite list: %w", err)
	}
	return resp.Sprites, nil
}
