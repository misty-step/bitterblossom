package main

import (
	"context"
	"strings"

	sprites "github.com/superfly/sprites-go"
)

func findSpriteWorkspace(ctx context.Context, s *sprites.Sprite) (string, error) {
	findWS := `
set -euo pipefail

prompt=$(ls -dt /home/sprite/workspace/*/.dispatch-prompt.md 2>/dev/null | head -1 || true)
if [[ -n "$prompt" ]]; then
  printf '%s\n' "${prompt%/*}"
  exit 0
fi

log=$(ls -dt /home/sprite/workspace/*/ralph.log 2>/dev/null | head -1 || true)
if [[ -n "$log" ]]; then
  printf '%s\n' "${log%/*}"
  exit 0
fi

ws=$(ls -d /home/sprite/workspace/*/ 2>/dev/null | head -1 || true)
printf '%s\n' "${ws%/}"
`
	wsOut, err := s.CommandContext(ctx, "bash", "-c", findWS).Output()
	if err != nil {
		return "", err
	}
	workspace := strings.TrimSpace(string(wsOut))
	return strings.TrimRight(workspace, "/"), nil
}
