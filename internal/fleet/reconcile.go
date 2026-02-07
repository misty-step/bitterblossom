package fleet

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/misty-step/bitterblossom/internal/sprite"
)

// SpriteStatus represents one observed sprite in the actual fleet.
type SpriteStatus struct {
	Name          string       `json:"name"`
	MachineID     string       `json:"machine_id,omitempty"`
	Persona       string       `json:"persona,omitempty"`
	ConfigVersion string       `json:"config_version,omitempty"`
	State         sprite.State `json:"state"`
}

// SpriteDrift captures desired-vs-actual differences for one sprite.
type SpriteDrift struct {
	Name                 string       `json:"name"`
	State                sprite.State `json:"state"`
	DesiredPersona       string       `json:"desired_persona"`
	ActualPersona        string       `json:"actual_persona"`
	DesiredConfigVersion string       `json:"desired_config_version"`
	ActualConfigVersion  string       `json:"actual_config_version"`
	PersonaMismatch      bool         `json:"persona_mismatch"`
	ConfigMismatch       bool         `json:"config_mismatch"`
}

// ReconcilePlan contains a full diff plus execution-ready actions.
type ReconcilePlan struct {
	Missing []SpriteSpec   `json:"missing"`
	Extra   []SpriteStatus `json:"extra"`
	Drift   []SpriteDrift  `json:"drift"`
	Actions []Action       `json:"-"`
}

// Reconcile computes pure reconciliation actions. It has no side effects.
func Reconcile(desired Composition, actual []SpriteStatus) []Action {
	return BuildPlan(desired, actual).Actions
}

// BuildPlan computes rich desired-vs-actual diff details and actions.
func BuildPlan(desired Composition, actual []SpriteStatus) ReconcilePlan {
	actualByName := make(map[string]SpriteStatus, len(actual))
	duplicates := make([]SpriteStatus, 0)
	for _, status := range actual {
		if existing, exists := actualByName[status.Name]; exists {
			_ = existing
			duplicates = append(duplicates, status)
			continue
		}
		actualByName[status.Name] = status
	}

	desiredConfigVersion := desiredVersion(desired)
	desiredSprites := append([]SpriteSpec(nil), desired.Sprites...)
	sort.Slice(desiredSprites, func(i, j int) bool {
		return desiredSprites[i].Name < desiredSprites[j].Name
	})

	desiredByName := make(map[string]SpriteSpec, len(desiredSprites))
	missing := make([]SpriteSpec, 0)
	drift := make([]SpriteDrift, 0)
	actions := make([]Action, 0)

	for _, spec := range desiredSprites {
		desiredByName[spec.Name] = spec

		status, exists := actualByName[spec.Name]
		if !exists {
			missing = append(missing, spec)
			actions = append(actions, &ProvisionAction{
				Sprite:        spec,
				ConfigVersion: desiredConfigVersion,
				Reason:        "missing from actual fleet",
			})
			continue
		}

		if status.State == sprite.StateDead {
			actions = append(actions, &ProvisionAction{
				Sprite:        spec,
				ConfigVersion: desiredConfigVersion,
				Reason:        "sprite reported dead",
			})
			continue
		}

		personaMismatch := strings.TrimSpace(status.Persona) != strings.TrimSpace(spec.Persona.Name)
		configMismatch := strings.TrimSpace(status.ConfigVersion) != strings.TrimSpace(desiredConfigVersion)
		if !personaMismatch && !configMismatch {
			continue
		}

		changeSummary := make([]string, 0, 2)
		if personaMismatch {
			changeSummary = append(changeSummary, fmt.Sprintf("persona %q -> %q", valueOrUnknown(status.Persona), spec.Persona.Name))
		}
		if configMismatch {
			changeSummary = append(changeSummary, fmt.Sprintf("config %q -> %q", valueOrUnknown(status.ConfigVersion), valueOrUnknown(desiredConfigVersion)))
		}

		drift = append(drift, SpriteDrift{
			Name:                 spec.Name,
			State:                status.State,
			DesiredPersona:       spec.Persona.Name,
			ActualPersona:        status.Persona,
			DesiredConfigVersion: desiredConfigVersion,
			ActualConfigVersion:  status.ConfigVersion,
			PersonaMismatch:      personaMismatch,
			ConfigMismatch:       configMismatch,
		})

		actions = append(actions, &UpdateAction{
			Desired:       spec,
			DesiredConfig: desiredConfigVersion,
			Current:       status,
			Changes:       changeSummary,
			Reason:        "desired state drift detected",
		})

		if isActiveState(status.State) {
			actions = append(actions, &RedispatchAction{
				Name:   spec.Name,
				Reason: fmt.Sprintf("sprite state is %q during update", status.State),
			})
		}
	}

	extra := make([]SpriteStatus, 0)
	actualNames := sortedKeys(actualByName)
	for _, name := range actualNames {
		status := actualByName[name]
		if _, exists := desiredByName[name]; exists {
			continue
		}
		extra = append(extra, status)
		actions = append(actions, &TeardownAction{
			Name:      status.Name,
			MachineID: status.MachineID,
			Reason:    "not present in desired composition",
		})
	}

	sort.Slice(duplicates, func(i, j int) bool {
		if duplicates[i].Name != duplicates[j].Name {
			return duplicates[i].Name < duplicates[j].Name
		}
		return duplicates[i].MachineID < duplicates[j].MachineID
	})
	for _, duplicate := range duplicates {
		extra = append(extra, duplicate)
		actions = append(actions, &TeardownAction{
			Name:      duplicate.Name,
			MachineID: duplicate.MachineID,
			Reason:    "duplicate sprite instance",
		})
	}

	sort.Slice(extra, func(i, j int) bool {
		if extra[i].Name != extra[j].Name {
			return extra[i].Name < extra[j].Name
		}
		return extra[i].MachineID < extra[j].MachineID
	})
	sort.Slice(drift, func(i, j int) bool {
		return drift[i].Name < drift[j].Name
	})

	return ReconcilePlan{
		Missing: missing,
		Extra:   extra,
		Drift:   drift,
		Actions: SortActions(actions),
	}
}

func isActiveState(state sprite.State) bool {
	switch state {
	case sprite.StateWorking, sprite.StateBlocked:
		return true
	default:
		return false
	}
}

func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "<unknown>"
	}
	return value
}

func desiredVersion(composition Composition) string {
	if composition.Version <= 0 {
		return ""
	}
	return strconv.Itoa(composition.Version)
}
