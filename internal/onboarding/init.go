// Package onboarding implements the `bb init` one-time setup flow.
// It discovers existing sprites, maps them to botanical names, and writes
// the registry TOML file.
package onboarding

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/misty-step/bitterblossom/internal/names"
	"github.com/misty-step/bitterblossom/internal/registry"
)

// Machine represents a Fly.io machine discovered during init.
type Machine struct {
	ID   string
	Name string
}

// MachineDiscoverer lists machines from the Fly.io API.
type MachineDiscoverer interface {
	ListMachines(ctx context.Context, app string) ([]Machine, error)
}

// InitConfig holds the configuration for the init flow.
type InitConfig struct {
	// App is the Fly.io app name.
	App string

	// RegistryPath overrides the default registry path (for testing).
	RegistryPath string

	// MaxSprites limits how many sprites to register. 0 = all discovered.
	MaxSprites int
}

// InitResult contains the outcome of the init flow.
type InitResult struct {
	RegistryPath string
	SpritesFound int
	Registered   int
	Names        []string
}

// Init discovers existing machines, assigns botanical names, and writes the registry.
// It is idempotent — re-running detects existing registrations.
func Init(ctx context.Context, disco MachineDiscoverer, cfg InitConfig) (*InitResult, error) {
	if cfg.App == "" {
		return nil, fmt.Errorf("onboarding: app name is required")
	}

	// 1. Discover machines.
	machines, err := disco.ListMachines(ctx, cfg.App)
	if err != nil {
		return nil, fmt.Errorf("onboarding: discover machines: %w", err)
	}

	if len(machines) == 0 {
		return nil, fmt.Errorf("onboarding: no machines found in app %q — provision sprites first or check FLY_API_TOKEN", cfg.App)
	}

	// Sort machines by ID for deterministic name assignment.
	sort.Slice(machines, func(i, j int) bool {
		return machines[i].ID < machines[j].ID
	})

	// 2. Limit if requested.
	limit := len(machines)
	if cfg.MaxSprites > 0 && cfg.MaxSprites < limit {
		limit = cfg.MaxSprites
	}
	if limit > names.Count() {
		limit = names.Count()
	}
	machines = machines[:limit]

	// 3. Build registry.
	regPath := cfg.RegistryPath
	if regPath == "" {
		regPath = registry.DefaultPath()
	}

	// Load existing registry or start fresh.
	reg, err := registry.Load(regPath)
	if err != nil {
		// If load fails (corrupt file), log and start fresh.
		reg = &registry.Registry{
			Sprites: make(map[string]registry.SpriteEntry),
		}
	}

	// Set metadata.
	reg.Meta.App = cfg.App
	reg.Meta.InitAt = time.Now().UTC()

	for i, m := range machines {
		name, err := names.PickName(i)
		if err != nil {
			return nil, fmt.Errorf("onboarding: assign name at index %d: %w", i, err)
		}
		reg.Register(name, m.ID)
	}

	// 4. Write registry.
	if err := reg.Save(regPath); err != nil {
		return nil, fmt.Errorf("onboarding: write registry: %w", err)
	}

	// 5. Collect results.
	result := &InitResult{
		RegistryPath: regPath,
		SpritesFound: len(machines),
		Registered:   limit,
		Names:        reg.Names(),
	}

	return result, nil
}
