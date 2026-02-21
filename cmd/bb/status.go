package main

import (
	"context"
	"fmt"
	"os"
	"sync"
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

	// Probe all sprites concurrently to avoid O(n) sequential latency.
	type probeResult struct {
		name   string
		status string
		reach  string
		avail  string
		note   string
	}

	results := make([]probeResult, len(all))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // Bound concurrent sprite probes

	for i, sprite := range all {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, s *sprites.Sprite) {
			defer func() { <-sem }()
			defer wg.Done()
			r := probeResult{name: s.Name(), status: s.Status, reach: "?", avail: "-"}

			probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			_, probeErr := s.CommandContext(probeCtx, "echo", "ok").Output()
			cancel()

			if probeErr != nil {
				r.reach = "no"
				r.note = "unreachable"
			} else {
				r.reach = "ok"
				busy, busyErr := isDispatchLoopActive(ctx, s)
				if busyErr != nil {
					r.avail = "?"
					r.note = "busy-check failed"
				} else if busy {
					r.avail = "busy"
				} else {
					r.avail = "idle"
				}
			}

			results[idx] = r
		}(i, sprite)
	}

	wg.Wait()

	fmt.Printf("%-15s %-10s %-8s %-6s %s\n", "SPRITE", "STATUS", "REACH", "AVAIL", "NOTE")
	fmt.Printf("%-15s %-10s %-8s %-6s %s\n", "------", "------", "-----", "-----", "----")
	for _, r := range results {
		fmt.Printf("%-15s %-10s %-8s %-6s %s\n", r.name, r.status, r.reach, r.avail, r.note)
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
