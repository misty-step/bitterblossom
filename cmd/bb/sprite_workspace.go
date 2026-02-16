package main

import (
	"context"
	"strings"

	sprites "github.com/superfly/sprites-go"
)

func findSpriteWorkspace(ctx context.Context, s *sprites.Sprite) (string, error) {
	findWS := `ls -d /home/sprite/workspace/*/ 2>/dev/null | head -1 | tr -d '\n'`
	wsOut, err := s.CommandContext(ctx, "bash", "-c", findWS).Output()
	if err != nil {
		return "", err
	}
	workspace := strings.TrimSpace(string(wsOut))
	return strings.TrimRight(workspace, "/"), nil
}
