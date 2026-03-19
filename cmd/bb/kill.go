package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newKillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "kill <sprite>",
		Short: "Clean up stale agent processes on a sprite",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKill(cmd.Context(), cmd.OutOrStdout(), args[0])
		},
	}
}

func runKill(ctx context.Context, out io.Writer, spriteName string) error {
	session, err := newSpriteSession(ctx, spriteName, spriteSessionOptions{probeTimeout: 10 * time.Second})
	if err != nil {
		return err
	}
	defer func() { _ = session.close() }()
	s := session.sprite

	workspace, err := findSpriteWorkspace(ctx, s)
	if err != nil {
		return fmt.Errorf("find workspace: %w", err)
	}
	if workspace == "" {
		_, _ = fmt.Fprintln(out, "could not determine workspace; stale agents may still exist")
		return nil
	}

	killCtx, killCancel := context.WithTimeout(ctx, 10*time.Second)
	defer killCancel()
	outBytes, err := s.CommandContext(killCtx, "bash", "-c", killDispatchProcessScriptFor(workspace)).CombinedOutput()
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
