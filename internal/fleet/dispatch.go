package fleet

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/internal/registry"
)

// StatusChecker provides live (remote) status for a machine ID.
//
// It is injected so Fleet.Dispatch stays testable and doesn't depend on any
// particular transport (sprite CLI, Fly exec, etc).
type StatusChecker interface {
	Check(ctx context.Context, machineID string) (LiveStatus, error)
}

// LiveStatus is a best-effort, human-facing snapshot for dispatch routing.
type LiveStatus struct {
	State         string
	Task          string
	Repo          string
	Runtime       string
	BlockedReason string
}

// DispatchRequest describes one dispatch assignment request.
//
// Sprite is optional: when empty, the fleet auto-assigns.
type DispatchRequest struct {
	Sprite string
	Issue  int
	Repo   string
}

// Assignment records the chosen sprite plus the resolved machine ID.
type Assignment struct {
	Sprite    string
	MachineID string

	Issue      int
	Repo       string
	AssignedAt time.Time
}

type DispatchConfig struct {
	RegistryPath     string
	RegistryRequired bool
	Status           StatusChecker
	Now              func() time.Time
	ReservationTTL   time.Duration
}

type dispatchConfig struct {
	registryPath     string
	registryRequired bool
	status           StatusChecker
	now              func() time.Time
	reservationTTL   time.Duration
}

// NewDispatchFleet constructs a Fleet handle configured for registry-backed dispatch.
func NewDispatchFleet(cfg DispatchConfig) (*Fleet, error) {
	if cfg.Status == nil {
		return nil, fmt.Errorf("fleet dispatch: status checker is required")
	}
	regPath := strings.TrimSpace(cfg.RegistryPath)
	if regPath == "" {
		regPath = registry.DefaultPath()
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	ttl := cfg.ReservationTTL
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}

	return &Fleet{
		dispatch: &dispatchConfig{
			registryPath:     regPath,
			registryRequired: cfg.RegistryRequired,
			status:           cfg.Status,
			now:              now,
			reservationTTL:   ttl,
		},
	}, nil
}

// FleetBusyError indicates no eligible sprites are available for dispatch.
type FleetBusyError struct {
	Sprites []BusySpriteStatus
}

type BusySpriteStatus struct {
	Sprite    string
	MachineID string

	State         string
	Task          string
	Repo          string
	Runtime       string
	BlockedReason string

	AssignedIssue int
	AssignedRepo  string
	AssignedAt    time.Time

	CheckError string
}

func (e *FleetBusyError) Error() string {
	if e == nil {
		return "fleet: all sprites busy"
	}
	lines := []string{
		fmt.Sprintf("All %d sprites are busy.", len(e.Sprites)),
	}
	if len(e.Sprites) == 0 {
		return strings.Join(lines, "\n")
	}

	// Keep deterministic output.
	sort.Slice(e.Sprites, func(i, j int) bool { return e.Sprites[i].Sprite < e.Sprites[j].Sprite })

	for _, s := range e.Sprites {
		state := strings.TrimSpace(s.State)
		if state == "" {
			state = "unknown"
		}
		summary := state
		if strings.TrimSpace(s.Runtime) != "" {
			summary += " (" + strings.TrimSpace(s.Runtime) + ")"
		}
		task := strings.TrimSpace(s.Task)
		if task == "" && s.AssignedIssue > 0 {
			task = fmt.Sprintf("issue #%d", s.AssignedIssue)
		}
		if task != "" {
			summary += ": " + task
		}
		if strings.TrimSpace(s.CheckError) != "" {
			summary += " [status error: " + strings.TrimSpace(s.CheckError) + "]"
		}
		lines = append(lines, fmt.Sprintf("  %s: %s", s.Sprite, summary))
	}

	lines = append(lines, "Hint: Wait for a sprite to free up, or expand the fleet.")
	return strings.Join(lines, "\n")
}

var errReserved = errors.New("fleet: sprite reserved")

// PlanDispatch selects a sprite and returns the would-be assignment, with no side effects.
func (f *Fleet) PlanDispatch(ctx context.Context, req DispatchRequest) (*Assignment, error) {
	return f.selectAndMaybeReserve(ctx, req, false)
}

// Dispatch selects a sprite, reserves it atomically in the registry, and returns the assignment.
//
// Selection is deterministic: alphabetical by registry name.
func (f *Fleet) Dispatch(ctx context.Context, req DispatchRequest) (*Assignment, error) {
	return f.selectAndMaybeReserve(ctx, req, true)
}

func (f *Fleet) selectAndMaybeReserve(ctx context.Context, req DispatchRequest, reserve bool) (*Assignment, error) {
	if f == nil || f.dispatch == nil {
		return nil, fmt.Errorf("fleet dispatch: not configured (use NewDispatchFleet)")
	}
	cfg := f.dispatch

	reg, err := registry.Load(cfg.registryPath)
	if err != nil {
		return nil, fmt.Errorf("fleet dispatch: load registry: %w", err)
	}

	var candidates []string
	if strings.TrimSpace(req.Sprite) != "" {
		candidates = []string{strings.TrimSpace(req.Sprite)}
	} else {
		candidates = reg.Names()
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("fleet dispatch: no sprites in registry (%s) — run 'bb init'", cfg.registryPath)
	}

	busy := make([]BusySpriteStatus, 0, len(candidates))
	for _, name := range candidates {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		machineID, ok := reg.LookupMachine(name)
		if !ok || strings.TrimSpace(machineID) == "" {
			if cfg.registryRequired || strings.TrimSpace(req.Sprite) != "" {
				return nil, fmt.Errorf("fleet dispatch: sprite %q not found in registry (%s) — run 'bb init'", name, cfg.registryPath)
			}
			continue
		}

		live, liveErr := cfg.status.Check(ctx, machineID)
		if isBusyState(live.State) || liveErr != nil {
			entry := reg.Sprites[name]
			busy = append(busy, BusySpriteStatus{
				Sprite:        name,
				MachineID:     machineID,
				State:         strings.TrimSpace(live.State),
				Task:          strings.TrimSpace(live.Task),
				Repo:          strings.TrimSpace(live.Repo),
				Runtime:       strings.TrimSpace(live.Runtime),
				BlockedReason: strings.TrimSpace(live.BlockedReason),
				AssignedIssue: entry.AssignedIssue,
				AssignedRepo:  strings.TrimSpace(entry.AssignedRepo),
				AssignedAt:    entry.AssignedAt,
				CheckError:    errString(liveErr),
			})
			continue
		}

		entry := reg.Sprites[name]
		now := cfg.now().UTC()
		if !entry.AssignedAt.IsZero() && now.Sub(entry.AssignedAt) < cfg.reservationTTL {
			busy = append(busy, BusySpriteStatus{
				Sprite:        name,
				MachineID:     machineID,
				State:         "reserved",
				Task:          strings.TrimSpace(live.Task),
				Repo:          strings.TrimSpace(live.Repo),
				Runtime:       strings.TrimSpace(live.Runtime),
				BlockedReason: strings.TrimSpace(live.BlockedReason),
				AssignedIssue: entry.AssignedIssue,
				AssignedRepo:  strings.TrimSpace(entry.AssignedRepo),
				AssignedAt:    entry.AssignedAt,
			})
			if strings.TrimSpace(req.Sprite) != "" {
				return nil, &FleetBusyError{Sprites: busy}
			}
			continue
		}

		if !reserve {
			return &Assignment{
				Sprite:     name,
				MachineID:  machineID,
				Issue:      req.Issue,
				Repo:       strings.TrimSpace(req.Repo),
				AssignedAt: now,
			}, nil
		}

		assignment, reserveErr := cfg.reserve(ctx, name, req)
		if reserveErr == nil {
			return assignment, nil
		}
		if errors.Is(reserveErr, errReserved) {
			busy = append(busy, BusySpriteStatus{
				Sprite:        name,
				MachineID:     machineID,
				State:         "reserved",
				Task:          strings.TrimSpace(live.Task),
				Repo:          strings.TrimSpace(live.Repo),
				Runtime:       strings.TrimSpace(live.Runtime),
				BlockedReason: strings.TrimSpace(live.BlockedReason),
			})
			if strings.TrimSpace(req.Sprite) != "" {
				return nil, &FleetBusyError{Sprites: busy}
			}
			continue
		}
		return nil, reserveErr
	}

	return nil, &FleetBusyError{Sprites: busy}
}

func (cfg *dispatchConfig) reserve(ctx context.Context, sprite string, req DispatchRequest) (*Assignment, error) {
	var out Assignment
	err := registry.WithLockedRegistry(ctx, cfg.registryPath, func(reg *registry.Registry) error {
		machineID, ok := reg.LookupMachine(sprite)
		if !ok || strings.TrimSpace(machineID) == "" {
			return fmt.Errorf("fleet dispatch: sprite %q not found in registry (%s) — run 'bb init'", sprite, cfg.registryPath)
		}

		entry := reg.Sprites[sprite]
		now := cfg.now().UTC()
		if !entry.AssignedAt.IsZero() && now.Sub(entry.AssignedAt) < cfg.reservationTTL {
			return errReserved
		}

		entry.AssignedIssue = req.Issue
		entry.AssignedRepo = strings.TrimSpace(req.Repo)
		entry.AssignedAt = now
		reg.Sprites[sprite] = entry

		out = Assignment{
			Sprite:     sprite,
			MachineID:  machineID,
			Issue:      req.Issue,
			Repo:       strings.TrimSpace(req.Repo),
			AssignedAt: now,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func isBusyState(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "running", "blocked":
		return true
	default:
		return false
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
