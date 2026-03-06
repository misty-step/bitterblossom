package main

import (
	"context"
	"strings"

	sprites "github.com/superfly/sprites-go"
)

func findSpriteWorkspace(ctx context.Context, s *sprites.Sprite) (string, error) {
	wsOut, err := s.CommandContext(ctx, "bash", "-c", workspaceDiscoveryScript()).Output()
	if err != nil {
		return "", err
	}
	workspace := strings.TrimSpace(string(wsOut))
	return strings.TrimRight(workspace, "/"), nil
}
