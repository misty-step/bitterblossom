package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/misty-step/bitterblossom/internal/clients"
)

// StatusConfig controls status command behavior.
type StatusConfig struct {
	Org             string
	CompositionPath string
	SpritesDir      string
	TargetSprite    string
}

// StatusService provides fleet overview and per-sprite detail.
type StatusService struct {
	Sprite clients.SpriteClient
	Out    io.Writer
}

// Run renders either fleet overview or sprite detail.
func (s *StatusService) Run(ctx context.Context, cfg StatusConfig) error {
	if s.Sprite == nil {
		return fmt.Errorf("sprite client required")
	}
	if s.Out == nil {
		s.Out = os.Stdout
	}
	if cfg.TargetSprite != "" {
		return s.renderSpriteDetail(ctx, cfg.Org, cfg.TargetSprite)
	}
	return s.renderFleetOverview(ctx, cfg)
}

func (s *StatusService) renderFleetOverview(ctx context.Context, cfg StatusConfig) error {
	_, _ = fmt.Fprintln(s.Out, "=== Bitterblossom Fleet Status ===")
	_, _ = fmt.Fprintln(s.Out)

	liveSprites, liveErr := s.Sprite.ListSprites(ctx, cfg.Org)
	if liveErr != nil {
		_, _ = fmt.Fprintf(s.Out, "No sprites found (or API call failed): %v\n", liveErr)
	}

	composition := s.compositionOrFallback(cfg)

	if len(liveSprites) > 0 {
		tw := tabwriter.NewWriter(s.Out, 2, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "SPRITE\tSTATUS\tURL")
		_, _ = fmt.Fprintln(tw, "------\t------\t---")
		for _, sprite := range liveSprites {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", sprite.Name, sprite.Status, emptyOr(sprite.URL, "n/a"))
		}
		_ = tw.Flush()
	}

	if len(composition) > 0 {
		_, _ = fmt.Fprintln(s.Out)
		_, _ = fmt.Fprintf(s.Out, "Composition sprites (%s):\n", cfg.CompositionPath)
		liveSet := map[string]bool{}
		for _, sprite := range liveSprites {
			liveSet[sprite.Name] = true
		}
		for _, name := range composition {
			if liveSet[name] {
				_, _ = fmt.Fprintf(s.Out, "  ✓ %s (provisioned)\n", name)
			} else {
				_, _ = fmt.Fprintf(s.Out, "  ○ %s (not provisioned)\n", name)
			}
		}

		orphans := collectOrphans(liveSprites, composition)
		if len(orphans) > 0 {
			_, _ = fmt.Fprintln(s.Out)
			_, _ = fmt.Fprintln(s.Out, "Orphan sprites (live but not in composition):")
			for _, orphan := range orphans {
				_, _ = fmt.Fprintf(s.Out, "  ? %s\n", orphan)
			}
		}
	}

	if len(liveSprites) > 0 {
		_, _ = fmt.Fprintln(s.Out)
		_, _ = fmt.Fprintln(s.Out, "Checkpoints:")
		for _, sprite := range liveSprites {
			cp, err := s.Sprite.ListCheckpoints(ctx, cfg.Org, sprite.Name)
			if err != nil || strings.TrimSpace(cp) == "" {
				cp = "(none)"
			}
			_, _ = fmt.Fprintf(s.Out, "  %s: %s\n", sprite.Name, strings.TrimSpace(cp))
		}
	}

	if liveErr != nil {
		return liveErr
	}
	return nil
}

func (s *StatusService) renderSpriteDetail(ctx context.Context, org, name string) error {
	_, _ = fmt.Fprintf(s.Out, "=== Sprite: %s ===\n\n", name)

	payload, err := s.Sprite.SpriteAPI(ctx, org, name, "/")
	if err != nil {
		_, _ = fmt.Fprintf(s.Out, "(API call failed): %v\n", err)
	} else {
		renderAPIObject(s.Out, payload)
	}

	_, _ = fmt.Fprintln(s.Out)
	_, _ = fmt.Fprintln(s.Out, "Workspace:")
	workspaceOut, err := s.Sprite.Exec(ctx, org, name, "ls -la /home/sprite/workspace/")
	if err != nil {
		_, _ = fmt.Fprintln(s.Out, "  (no workspace)")
	} else {
		_, _ = fmt.Fprint(s.Out, workspaceOut)
	}

	_, _ = fmt.Fprintln(s.Out)
	_, _ = fmt.Fprintln(s.Out, "MEMORY.md (first 20 lines):")
	memoryOut, err := s.Sprite.Exec(ctx, org, name, "head -20 /home/sprite/workspace/MEMORY.md")
	if err != nil {
		_, _ = fmt.Fprintln(s.Out, "  (no MEMORY.md)")
	} else {
		_, _ = fmt.Fprint(s.Out, memoryOut)
	}

	_, _ = fmt.Fprintln(s.Out)
	_, _ = fmt.Fprintln(s.Out, "Checkpoints:")
	checkpoints, err := s.Sprite.ListCheckpoints(ctx, org, name)
	if err != nil || strings.TrimSpace(checkpoints) == "" {
		_, _ = fmt.Fprintln(s.Out, "  (none)")
	} else {
		_, _ = fmt.Fprint(s.Out, checkpoints)
	}
	return nil
}

func (s *StatusService) compositionOrFallback(cfg StatusConfig) []string {
	if cfg.CompositionPath != "" {
		names, err := CompositionSprites(cfg.CompositionPath)
		if err == nil {
			return names
		}
	}
	if cfg.SpritesDir != "" {
		names, err := FallbackSpriteNames(cfg.SpritesDir)
		if err == nil {
			return names
		}
	}
	return nil
}

func renderAPIObject(w io.Writer, payload []byte) {
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		_, _ = fmt.Fprintf(w, "%s\n", strings.TrimSpace(string(payload)))
		return
	}
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := data[key]
		switch typed := value.(type) {
		case map[string]any:
			_, _ = fmt.Fprintf(w, "%s:\n", key)
			sub := sortedKeys(typed)
			for _, k := range sub {
				_, _ = fmt.Fprintf(w, "  %s: %v\n", k, typed[k])
			}
		default:
			_, _ = fmt.Fprintf(w, "%s: %v\n", key, typed)
		}
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func emptyOr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func collectOrphans(live []clients.SpriteInfo, composition []string) []string {
	inComp := map[string]bool{}
	for _, name := range composition {
		inComp[name] = true
	}
	orphans := make([]string, 0)
	for _, sprite := range live {
		if !inComp[sprite.Name] {
			orphans = append(orphans, fmt.Sprintf("%s (%s, not in composition)", sprite.Name, sprite.Status))
		}
	}
	return orphans
}
