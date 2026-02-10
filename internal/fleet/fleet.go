package fleet

import (
	"sort"
	"strconv"

	"github.com/misty-step/bitterblossom/internal/sprite"
)

// SpriteReport summarizes one actual sprite in the fleet.
type SpriteReport struct {
	Name        string       `json:"name"`
	Persona     string       `json:"persona"`
	State       sprite.State `json:"state"`
	Provisioned bool         `json:"provisioned"`
	Pending     int          `json:"pending"`
}

// FleetReport summarizes desired vs actual fleet state.
type FleetReport struct {
	Composition        string               `json:"composition"`
	Desired            int                  `json:"desired"`
	Actual             int                  `json:"actual"`
	States             map[sprite.State]int `json:"states"`
	Sprites            []SpriteReport       `json:"sprites"`
	Missing            []string             `json:"missing"`
	Extra              []string             `json:"extra"`
	PersonaMismatches  []string             `json:"persona_mismatches"`
	UnprovisionedNames []string             `json:"unprovisioned"`
}

// Fleet holds desired state from composition and actual sprite handles.
type Fleet struct {
	Desired Composition
	Sprites []*sprite.Sprite

	dispatch *dispatchConfig
}

// New constructs a fleet handle.
func New(desired Composition, sprites []*sprite.Sprite) *Fleet {
	return &Fleet{
		Desired: desired,
		Sprites: sprites,
	}
}

// Reconcile returns the actions required to converge actual state to desired state.
func (f *Fleet) Reconcile() []Action {
	configVersion := ""
	if f.Desired.Version > 0 {
		configVersion = strconv.Itoa(f.Desired.Version)
	}

	actual := make([]SpriteStatus, 0, len(f.Sprites))
	for _, handle := range f.Sprites {
		if handle == nil {
			continue
		}
		snap := handle.Snapshot()
		actual = append(actual, SpriteStatus{
			Name:          snap.Name,
			Persona:       snap.Persona.Name,
			ConfigVersion: configVersion,
			State:         snap.State,
		})
	}

	return Reconcile(f.Desired, actual)
}

// Status returns a current-state summary for reporting and API consumers.
func (f *Fleet) Status() FleetReport {
	stateCounts := map[sprite.State]int{
		sprite.StateProvisioned: 0,
		sprite.StateIdle:        0,
		sprite.StateWorking:     0,
		sprite.StateDone:        0,
		sprite.StateBlocked:     0,
		sprite.StateDead:        0,
	}

	desired := make(map[string]SpriteSpec, len(f.Desired.Sprites))
	for _, spec := range f.Desired.Sprites {
		desired[spec.Name] = spec
	}

	reports := make([]SpriteReport, 0, len(f.Sprites))
	extra := make([]string, 0)
	mismatch := make([]string, 0)
	unprovisioned := make([]string, 0)
	seen := make(map[string]struct{}, len(f.Sprites))

	for _, handle := range f.Sprites {
		if handle == nil {
			continue
		}
		snap := handle.Snapshot()
		seen[snap.Name] = struct{}{}
		stateCounts[snap.State]++

		reports = append(reports, SpriteReport{
			Name:        snap.Name,
			Persona:     snap.Persona.Name,
			State:       snap.State,
			Provisioned: snap.Provisioned,
			Pending:     snap.Pending,
		})

		spec, ok := desired[snap.Name]
		if !ok {
			extra = append(extra, snap.Name)
			continue
		}
		if !snap.Provisioned {
			unprovisioned = append(unprovisioned, snap.Name)
		}
		if snap.Persona.Name != spec.Persona.Name || snap.Persona.Definition != spec.Persona.Definition {
			mismatch = append(mismatch, snap.Name)
		}
	}

	missing := make([]string, 0)
	for _, spec := range f.Desired.Sprites {
		if _, ok := seen[spec.Name]; !ok {
			missing = append(missing, spec.Name)
		}
	}

	sort.Slice(reports, func(i, j int) bool {
		return reports[i].Name < reports[j].Name
	})
	sort.Strings(missing)
	sort.Strings(extra)
	sort.Strings(mismatch)
	sort.Strings(unprovisioned)

	return FleetReport{
		Composition:        f.Desired.Name,
		Desired:            len(f.Desired.Sprites),
		Actual:             len(reports),
		States:             stateCounts,
		Sprites:            reports,
		Missing:            missing,
		Extra:              extra,
		PersonaMismatches:  mismatch,
		UnprovisionedNames: unprovisioned,
	}
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
