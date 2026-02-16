package main

import (
	"context"
	"strings"

	sprites "github.com/superfly/sprites-go"
)

func findSpriteWorkspace(ctx context.Context, s *sprites.Sprite) string {
	findWS := `ls -d /home/sprite/workspace/*/ 2>/dev/null | head -1 | tr -d '\n'`
	wsOut, _ := s.CommandContext(ctx, "bash", "-c", findWS).Output()
	workspace := strings.TrimSpace(string(wsOut))
	return strings.TrimRight(workspace, "/")
}
