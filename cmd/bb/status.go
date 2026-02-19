package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	sprites "github.com/superfly/sprites-go"

	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [sprite]",
		Short: "Show sprite or fleet status",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fleetStatus(cmd.Context())
			}
			return spriteStatus(cmd.Context(), args[0])
		},
	}
}

func statusClient(token string) *sprites.Client {
	client := sprites.New(token, sprites.WithDisableControl())
	return client
}

func fleetStatus(ctx context.Context) error {
	token, err := spriteToken()
	if err != nil {
		return err
	}

	client := statusClient(token)
	defer func() { _ = client.Close() }()

	all, err := client.ListAllSprites(ctx, "")
	if err != nil {
		return fmt.Errorf("list sprites: %w", err)
	}

	if len(all) == 0 {
		fmt.Println("no sprites found")
		return nil
	}

	fmt.Printf("%-15s %-10s %-8s %s\n", "SPRITE", "STATUS", "REACH", "NOTE")
	fmt.Printf("%-15s %-10s %-8s %s\n", "------", "------", "-----", "----")

	for _, sprite := range all {
		reach := "?"
		note := ""

		// Quick probe (3s timeout)
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		if _, probeErr := sprite.CommandContext(probeCtx, "echo", "ok").Output(); probeErr == nil {
			reach = "ok"
		} else {
			reach = "no"
			note = "unreachable"
		}
		cancel()

		// Check for active dispatch loop (busy state)
		status := sprite.Status
		busyCtx, busyCancel := context.WithTimeout(ctx, 5*time.Second)
		busy, busyErr := checkActiveDispatchLoop(busyCtx, spriteBashRunnerForStatus(sprite))
		busyCancel()
		if busyErr == nil && busy {
			status = "busy"
			note = "active dispatch loop"
		}

		fmt.Printf("%-15s %-10s %-8s %s\n", sprite.Name(), status, reach, note)
	}

	return nil
}

func spriteStatus(ctx context.Context, spriteName string) error {
	token, err := spriteToken()
	if err != nil {
		return err
	}

	client := statusClient(token)
	defer func() { _ = client.Close() }()
	s := client.Sprite(spriteName)

	// Probe
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if _, err := s.CommandContext(probeCtx, "echo", "ok").Output(); err != nil {
		return fmt.Errorf("sprite %q unreachable: %w", spriteName, err)
	}

	// Gather status info
	statusScript := `
echo "=== signals ==="
for f in TASK_COMPLETE TASK_COMPLETE.md BLOCKED.md; do
  [ -f "$WS/$f" ] && echo "$f: present" || echo "$f: absent"
done

echo ""
echo "=== git ==="
if [ -d "$WS" ]; then
  cd "$WS"
  echo "branch: $(git branch --show-current 2>/dev/null || echo 'n/a')"
  echo "status: $(git status --porcelain 2>/dev/null | wc -l | tr -d ' ') dirty files"
  echo ""
  echo "recent commits:"
  git log --oneline -5 2>/dev/null || echo "(no commits)"
  echo ""
  echo "=== PRs ==="
  gh pr list --json url,title,state --jq '.[] | "\(.state): \(.title) \(.url)"' 2>/dev/null || echo "(gh not available)"
else
  echo "(no repo found at $WS)"
fi
`

	workspace, err := findSpriteWorkspace(ctx, s)
	if err != nil {
		return fmt.Errorf("find workspace: %w", err)
	}

	if workspace == "" {
		fmt.Printf("sprite: %s\nstatus: reachable\nworkspace: empty (run bb setup)\n", spriteName)
		return nil
	}

	fmt.Printf("sprite: %s\nworkspace: %s\n\n", spriteName, workspace)

	fullScript := fmt.Sprintf("export WS=%q\n%s", workspace, statusScript)
	cmd := s.CommandContext(ctx, "bash", "-c", fullScript)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func spriteBashRunnerForStatus(s *sprites.Sprite) func(ctx context.Context, script string) ([]byte, int, error) {
	return func(ctx context.Context, script string) ([]byte, int, error) {
		out, err := s.CommandContext(ctx, "bash", "-c", script).CombinedOutput()
		if err == nil {
			return out, 0, nil
		}

		// Try to get exit code
		var exitErr *sprites.ExitError
		if errors.As(err, &exitErr) {
			return out, exitErr.ExitCode(), nil
		}

		return out, 0, err
	}
}

func checkActiveDispatchLoop(ctx context.Context, run func(ctx context.Context, script string) ([]byte, int, error)) (bool, error) {
	_, exitCode, err := run(ctx, activeRalphLoopCheckScript)
	if err != nil {
		return false, fmt.Errorf("check dispatch loop: %w", err)
	}

	switch exitCode {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, nil
	}
}
