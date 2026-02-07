package logs

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/misty-step/bitterblossom/internal/clients"
)

// Config controls tail logs behavior.
type Config struct {
	Org    string
	Sprite string
	All    bool
	Lines  int
	Brief  bool
}

// Viewer tails remote sprite logs.
type Viewer struct {
	Sprite clients.SpriteClient
	Out    io.Writer
}

// Run executes log tailing.
func (v *Viewer) Run(ctx context.Context, cfg Config) error {
	if v.Sprite == nil {
		return fmt.Errorf("sprite client required")
	}
	if v.Out == nil {
		v.Out = os.Stdout
	}
	if cfg.Lines <= 0 {
		cfg.Lines = 50
	}
	if cfg.Brief {
		cfg.Lines = 5
	}

	if cfg.All {
		sprites, err := v.Sprite.List(ctx, cfg.Org)
		if err != nil {
			return err
		}
		for _, name := range sprites {
			v.tailOne(ctx, cfg.Org, name, cfg.Lines)
		}
		return nil
	}
	if strings.TrimSpace(cfg.Sprite) == "" {
		return fmt.Errorf("sprite name required unless --all")
	}
	v.tailOne(ctx, cfg.Org, cfg.Sprite, cfg.Lines)
	return nil
}

func (v *Viewer) tailOne(ctx context.Context, org, sprite string, lines int) {
	_, _ = fmt.Fprintln(v.Out, "╔══════════════════════════════════════════════════════════╗")
	_, _ = fmt.Fprintf(v.Out, "║  %s — last %d lines\n", sprite, lines)
	_, _ = fmt.Fprintln(v.Out, "╚══════════════════════════════════════════════════════════╝")

	claudePID, err := v.Sprite.Exec(ctx, org, sprite, "pgrep -la claude")
	if err != nil || strings.TrimSpace(claudePID) == "" {
		claudePID = "NOT RUNNING"
	}
	_, _ = fmt.Fprintf(v.Out, "  Claude: %s\n", strings.TrimSpace(claudePID))

	signalCmd := `[ -f /home/sprite/workspace/TASK_COMPLETE ] && echo '  TASK_COMPLETE';
[ -f /home/sprite/workspace/BLOCKED.md ] && echo '  BLOCKED';
[ -f /home/sprite/workspace/LEARNINGS.md ] && echo '  Has LEARNINGS.md'`
	signals, err := v.Sprite.Exec(ctx, org, sprite, signalCmd)
	if err == nil && strings.TrimSpace(signals) != "" {
		_, _ = fmt.Fprint(v.Out, signals)
	}
	_, _ = fmt.Fprintln(v.Out)

	tailCmd := fmt.Sprintf("tail -%d /home/sprite/workspace/ralph.log", lines)
	logOut, err := v.Sprite.Exec(ctx, org, sprite, tailCmd)
	if err != nil || strings.TrimSpace(logOut) == "" {
		_, _ = fmt.Fprintln(v.Out, "  (no ralph.log)")
	} else {
		_, _ = fmt.Fprint(v.Out, logOut)
	}
	_, _ = fmt.Fprintln(v.Out)
}
