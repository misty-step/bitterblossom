package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	sprites "github.com/superfly/sprites-go"

	"github.com/spf13/cobra"
)

func newKillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "kill <sprite>",
		Short: "Clean up stale ralph/agent processes on a sprite",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKill(cmd.Context(), cmd.OutOrStdout(), args[0])
		},
	}
}

func runKill(ctx context.Context, out io.Writer, spriteName string) error {
	token, err := spriteToken()
	if err != nil {
		return err
	}

	client := sprites.New(token)
	defer func() { _ = client.Close() }()
	s := client.Sprite(spriteName)

	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if _, err := s.CommandContext(probeCtx, "echo", "ok").Output(); err != nil {
		return fmt.Errorf("sprite %q unreachable: %w", spriteName, err)
	}

	killCtx, killCancel := context.WithTimeout(ctx, 10*time.Second)
	defer killCancel()
	outBytes, err := s.CommandContext(killCtx, "bash", "-c", killAgentProcessesScript).CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(outBytes)); msg != "" {
			return fmt.Errorf("failed to cleanup sprite %q: %w (%s)", spriteName, err, msg)
		}
		return fmt.Errorf("failed to cleanup sprite %q: %w", spriteName, err)
	}

	if len(outBytes) == 0 {
		_, _ = fmt.Fprintln(out, "no stale agent processes found")
		return nil
	}
	_, _ = fmt.Fprint(out, string(outBytes))
	return nil
}

const killAgentProcessesScript = `
if ! command -v pgrep >/dev/null 2>&1; then
  echo "required process tools unavailable on sprite: pgrep missing" >&2
  exit 1
fi

if ! command -v pkill >/dev/null 2>&1; then
  echo "required process tools unavailable on sprite: pkill missing" >&2
  exit 1
fi

agents='/home/sprite/workspace/\.[r]alph\.sh|[c]laude|[o]pencode'

match=$(pgrep -af "$agents" 2>&1 || true)
if [ -z "$match" ]; then
  echo "no stale agent processes found"
  exit 0
fi

echo "found agent processes:"
echo "$match"

pkill -9 -f "$agents" 2>/dev/null || true
sleep 1

remaining=$(pgrep -af "$agents" 2>&1 || true)
if [ -n "$remaining" ]; then
  echo "cleanup verification failed; process still running:" >&2
  echo "$remaining" >&2
  exit 1
fi

echo "stale agent processes terminated"
`
