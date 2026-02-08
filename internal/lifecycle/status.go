package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/misty-step/bitterblossom/internal/fleet"
	"github.com/misty-step/bitterblossom/internal/sprite"
)

// FleetStatus contains fleet and composition state.
type FleetStatus struct {
	Sprites     []SpriteStatus     `json:"sprites"`
	Composition []CompositionEntry `json:"composition"`
	Orphans     []SpriteStatus     `json:"orphans"`
	Checkpoints map[string]string  `json:"checkpoints"`
}

// SpriteStatus describes one live sprite from the Sprite API.
type SpriteStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	URL    string `json:"url,omitempty"`
}

// CompositionEntry maps composition membership to provisioning state.
type CompositionEntry struct {
	Name        string `json:"name"`
	Provisioned bool   `json:"provisioned"`
}

// SpriteDetailResult captures detailed status for one sprite.
type SpriteDetailResult struct {
	Name        string         `json:"name"`
	API         map[string]any `json:"api,omitempty"`
	Workspace   string         `json:"workspace"`
	Memory      string         `json:"memory"`
	Checkpoints string         `json:"checkpoints"`
}

type spriteAPIListResponse struct {
	Sprites []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		URL    string `json:"url"`
	} `json:"sprites"`
}

// FleetOverview returns live fleet status + composition coverage + checkpoint summaries.
func FleetOverview(ctx context.Context, cli sprite.SpriteCLI, cfg Config, compositionPath string) (FleetStatus, error) {
	if err := requireConfig(cfg); err != nil {
		return FleetStatus{}, err
	}
	composition, err := fleet.ParseComposition(compositionPath)
	if err != nil {
		return FleetStatus{}, err
	}

	live, err := fetchLiveSprites(ctx, cli, cfg)
	if err != nil {
		return FleetStatus{}, err
	}

	provisioned := make(map[string]struct{}, len(live))
	for _, item := range live {
		provisioned[item.Name] = struct{}{}
	}

	entries := make([]CompositionEntry, 0, len(composition.Sprites))
	compositionNames := make([]string, 0, len(composition.Sprites))
	for _, spec := range composition.Sprites {
		compositionNames = append(compositionNames, spec.Name)
		_, ok := provisioned[spec.Name]
		entries = append(entries, CompositionEntry{Name: spec.Name, Provisioned: ok})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	sort.Strings(compositionNames)

	orphans := make([]SpriteStatus, 0, len(live))
	for _, item := range live {
		if !slices.Contains(compositionNames, item.Name) {
			orphans = append(orphans, item)
		}
	}
	sort.Slice(orphans, func(i, j int) bool {
		return orphans[i].Name < orphans[j].Name
	})

	checkpoints := make(map[string]string, len(live))
	for _, item := range live {
		value, err := cli.CheckpointList(ctx, item.Name, cfg.Org)
		if err != nil {
			checkpoints[item.Name] = "(none)"
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			value = "(none)"
		}
		checkpoints[item.Name] = value
	}

	return FleetStatus{
		Sprites:     live,
		Composition: entries,
		Orphans:     orphans,
		Checkpoints: checkpoints,
	}, nil
}

// SpriteDetail returns API + workspace + memory + checkpoint status for one sprite.
func SpriteDetail(ctx context.Context, cli sprite.SpriteCLI, cfg Config, name string) (SpriteDetailResult, error) {
	if err := requireConfig(cfg); err != nil {
		return SpriteDetailResult{}, err
	}
	if err := ValidateSpriteName(name); err != nil {
		return SpriteDetailResult{}, err
	}

	result := SpriteDetailResult{Name: name}

	apiRaw, err := cli.APISprite(ctx, cfg.Org, name, "/")
	if err == nil {
		var payload map[string]any
		if decodeErr := json.Unmarshal([]byte(apiRaw), &payload); decodeErr == nil {
			result.API = payload
		}
	}

	workspaceCommand := "ls -la " + shellQuote(path.Join(cfg.Workspace, "/"))
	workspaceOutput, workspaceErr := cli.Exec(ctx, name, workspaceCommand, nil)
	if workspaceErr != nil {
		result.Workspace = "(no workspace)"
	} else {
		result.Workspace = strings.TrimSpace(workspaceOutput)
	}

	memoryCommand := "head -20 " + shellQuote(path.Join(cfg.Workspace, "MEMORY.md"))
	memoryOutput, memoryErr := cli.Exec(ctx, name, memoryCommand, nil)
	if memoryErr != nil {
		result.Memory = "(no MEMORY.md)"
	} else {
		result.Memory = strings.TrimSpace(memoryOutput)
	}

	checkpoints, err := cli.CheckpointList(ctx, name, cfg.Org)
	if err != nil {
		result.Checkpoints = "(none)"
	} else {
		result.Checkpoints = strings.TrimSpace(checkpoints)
		if result.Checkpoints == "" {
			result.Checkpoints = "(none)"
		}
	}

	return result, nil
}

func fetchLiveSprites(ctx context.Context, cli sprite.SpriteCLI, cfg Config) ([]SpriteStatus, error) {
	raw, err := cli.API(ctx, cfg.Org, "/sprites")
	if err != nil {
		return nil, err
	}

	var decoded spriteAPIListResponse
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, fmt.Errorf("decode sprite api response: %w", err)
	}

	result := make([]SpriteStatus, 0, len(decoded.Sprites))
	for _, item := range decoded.Sprites {
		if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Status) == "" {
			continue
		}
		result = append(result, SpriteStatus{
			Name:   item.Name,
			Status: item.Status,
			URL:    item.URL,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}
