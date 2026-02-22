package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	sprites "github.com/superfly/sprites-go"

	"github.com/spf13/cobra"
)

func newLogsCmd() *cobra.Command {
	var (
		follow   bool
		lines    int
		jsonMode bool
	)

	cmd := &cobra.Command{
		Use:   "logs <sprite>",
		Short: "Stream sprite agent logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()

			return runLogs(ctx, cmd.OutOrStdout(), cmd.ErrOrStderr(), args[0], follow, lines, jsonMode)
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	cmd.Flags().IntVarP(&lines, "lines", "n", 0, "Last N lines (0 = all)")
	cmd.Flags().BoolVar(&jsonMode, "json", false, "Show raw stream-json events")

	return cmd
}

func runLogs(ctx context.Context, stdout, stderr io.Writer, spriteName string, follow bool, lines int, jsonMode bool) error {
	if lines < 0 {
		return fmt.Errorf("--lines must be >= 0")
	}

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

	workspace, err := findSpriteWorkspace(ctx, s)
	if err != nil {
		return fmt.Errorf("find workspace: %w", err)
	}
	if workspace == "" {
		return fmt.Errorf("sprite %q has no workspace repo (run: bb setup %s --repo owner/repo)", spriteName, spriteName)
	}

	logPath := workspace + "/ralph.log"

	active := spriteHasRunningAgent(ctx, s)
	hasLog := spriteFileHasContent(ctx, s, logPath)
	if !active && !hasLog {
		if err := writeLogsNoTaskMsg(stderr); err != nil {
			return fmt.Errorf("logs: %w", err)
		}
		return nil
	}

	remoteCmd := logsRemoteCommand(logPath, follow, lines)

	logWriter := newStreamJSONWriter(stdout, jsonMode)
	defer func() {
		if err := logWriter.Flush(); err != nil {
			_, _ = fmt.Fprintf(stderr, "logs: flush: %v\n", err)
		}
	}()

	remote := s.CommandContext(ctx, "bash", "-c", remoteCmd)
	remote.Stdout = logWriter
	remote.Stderr = stderr

	if err := remote.Run(); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("logs failed: %w", err)
	}
	return nil
}

// writeLogsNoTaskMsg writes the "No active task" status message to stderr.
// This is an operational message, not log data, so it must never appear on
// stdout — stdout must remain parseable JSON in --json mode.
func writeLogsNoTaskMsg(stderr io.Writer) error {
	_, err := fmt.Fprintf(stderr, "No active task\n")
	return err
}

func logsRemoteCommand(logPath string, follow bool, lines int) string {
	if follow {
		n := lines
		if n == 0 {
			n = 50
		}
		return fmt.Sprintf("touch %q && tail -n %d -f %q", logPath, n, logPath)
	}

	if lines > 0 {
		return fmt.Sprintf("touch %q && tail -n %d %q", logPath, lines, logPath)
	}
	return fmt.Sprintf("touch %q && cat %q", logPath, logPath)
}

func spriteHasRunningAgent(ctx context.Context, s *sprites.Sprite) bool {
	check := `pgrep -f '[r]alph\.sh|[c]laude|[o]pencode' >/dev/null 2>&1`
	return s.CommandContext(ctx, "bash", "-c", check).Run() == nil
}

func spriteFileHasContent(ctx context.Context, s *sprites.Sprite, path string) bool {
	check := fmt.Sprintf("test -s %q", path)
	return s.CommandContext(ctx, "bash", "-c", check).Run() == nil
}
