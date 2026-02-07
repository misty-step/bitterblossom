package fleet

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ActionKind identifies the reconciliation action type.
type ActionKind string

const (
	ActionProvision  ActionKind = "provision"
	ActionTeardown   ActionKind = "teardown"
	ActionUpdate     ActionKind = "update"
	ActionRedispatch ActionKind = "redispatch"
)

var (
	// ErrActionRuntimeNotBound indicates Execute was invoked without a runtime.
	ErrActionRuntimeNotBound = errors.New("fleet: action runtime not bound")
)

// Action is a pure instruction produced by reconciliation.
//
// Execute is optional and only works when the action is bound to a runtime.
type Action interface {
	Kind() ActionKind
	SpriteName() string
	Description() string
	DryRun() string
	Execute(ctx context.Context) error
	bind(runtime ActionRuntime)
	toView() ActionView
}

// ActionRuntime performs the side effects for action execution.
type ActionRuntime interface {
	Provision(ctx context.Context, action ProvisionAction) error
	Teardown(ctx context.Context, action TeardownAction) error
	Update(ctx context.Context, action UpdateAction) error
	Redispatch(ctx context.Context, action RedispatchAction) error
}

// ActionView is a stable JSON-friendly representation.
type ActionView struct {
	Kind        ActionKind `json:"kind"`
	Sprite      string     `json:"sprite"`
	Description string     `json:"description"`
	DryRun      string     `json:"dry_run"`
}

// ProvisionAction provisions a missing (or dead) sprite.
type ProvisionAction struct {
	Sprite        SpriteSpec `json:"sprite"`
	ConfigVersion string     `json:"config_version,omitempty"`
	Reason        string     `json:"reason,omitempty"`

	runtime ActionRuntime
}

func (a *ProvisionAction) Kind() ActionKind           { return ActionProvision }
func (a *ProvisionAction) SpriteName() string         { return a.Sprite.Name }
func (a *ProvisionAction) bind(runtime ActionRuntime) { a.runtime = runtime }
func (a *ProvisionAction) toView() ActionView {
	return ActionView{Kind: a.Kind(), Sprite: a.SpriteName(), Description: a.Description(), DryRun: a.DryRun()}
}

func (a *ProvisionAction) Description() string {
	parts := []string{fmt.Sprintf("provision sprite %q", a.Sprite.Name)}
	if persona := strings.TrimSpace(a.Sprite.Persona.Name); persona != "" {
		parts = append(parts, fmt.Sprintf("persona=%q", persona))
	}
	if version := strings.TrimSpace(a.ConfigVersion); version != "" {
		parts = append(parts, fmt.Sprintf("config_version=%q", version))
	}
	detail := strings.Join(parts, " ")
	if a.Reason == "" {
		return detail
	}
	return detail + " (" + a.Reason + ")"
}

func (a *ProvisionAction) DryRun() string { return "[dry-run] " + a.Description() }

func (a *ProvisionAction) Execute(ctx context.Context) error {
	if a.runtime == nil {
		return ErrActionRuntimeNotBound
	}
	return a.runtime.Provision(ctx, *a)
}

// TeardownAction tears down an extra sprite.
type TeardownAction struct {
	Name      string `json:"name"`
	MachineID string `json:"machine_id,omitempty"`
	Reason    string `json:"reason,omitempty"`

	runtime ActionRuntime
}

func (a *TeardownAction) Kind() ActionKind           { return ActionTeardown }
func (a *TeardownAction) SpriteName() string         { return a.Name }
func (a *TeardownAction) bind(runtime ActionRuntime) { a.runtime = runtime }
func (a *TeardownAction) toView() ActionView {
	return ActionView{Kind: a.Kind(), Sprite: a.SpriteName(), Description: a.Description(), DryRun: a.DryRun()}
}

func (a *TeardownAction) Description() string {
	detail := fmt.Sprintf("teardown sprite %q", a.Name)
	if a.Reason == "" {
		return detail
	}
	return detail + " (" + a.Reason + ")"
}

func (a *TeardownAction) DryRun() string { return "[dry-run] " + a.Description() }

func (a *TeardownAction) Execute(ctx context.Context) error {
	if a.runtime == nil {
		return ErrActionRuntimeNotBound
	}
	return a.runtime.Teardown(ctx, *a)
}

// UpdateAction updates a drifted sprite.
type UpdateAction struct {
	Desired       SpriteSpec   `json:"desired"`
	DesiredConfig string       `json:"desired_config_version,omitempty"`
	Current       SpriteStatus `json:"current"`
	Changes       []string     `json:"changes,omitempty"`
	Reason        string       `json:"reason,omitempty"`

	runtime ActionRuntime
}

func (a *UpdateAction) Kind() ActionKind           { return ActionUpdate }
func (a *UpdateAction) SpriteName() string         { return a.Desired.Name }
func (a *UpdateAction) bind(runtime ActionRuntime) { a.runtime = runtime }
func (a *UpdateAction) toView() ActionView {
	return ActionView{Kind: a.Kind(), Sprite: a.SpriteName(), Description: a.Description(), DryRun: a.DryRun()}
}

func (a *UpdateAction) Description() string {
	parts := append([]string(nil), a.Changes...)
	if a.Reason != "" {
		parts = append(parts, a.Reason)
	}
	detail := fmt.Sprintf("update sprite %q", a.Desired.Name)
	if len(parts) == 0 {
		return detail
	}
	return detail + " (" + strings.Join(parts, "; ") + ")"
}

func (a *UpdateAction) DryRun() string { return "[dry-run] " + a.Description() }

func (a *UpdateAction) Execute(ctx context.Context) error {
	if a.runtime == nil {
		return ErrActionRuntimeNotBound
	}
	return a.runtime.Update(ctx, *a)
}

// RedispatchAction re-routes work while reconciliation is in progress.
type RedispatchAction struct {
	Name   string `json:"name"`
	Reason string `json:"reason,omitempty"`

	runtime ActionRuntime
}

func (a *RedispatchAction) Kind() ActionKind           { return ActionRedispatch }
func (a *RedispatchAction) SpriteName() string         { return a.Name }
func (a *RedispatchAction) bind(runtime ActionRuntime) { a.runtime = runtime }
func (a *RedispatchAction) toView() ActionView {
	return ActionView{Kind: a.Kind(), Sprite: a.SpriteName(), Description: a.Description(), DryRun: a.DryRun()}
}

func (a *RedispatchAction) Description() string {
	detail := fmt.Sprintf("redispatch workload for sprite %q", a.Name)
	if a.Reason == "" {
		return detail
	}
	return detail + " (" + a.Reason + ")"
}

func (a *RedispatchAction) DryRun() string { return "[dry-run] " + a.Description() }

func (a *RedispatchAction) Execute(ctx context.Context) error {
	if a.runtime == nil {
		return ErrActionRuntimeNotBound
	}
	return a.runtime.Redispatch(ctx, *a)
}

// SortActions returns actions in deterministic execution order.
func SortActions(actions []Action) []Action {
	ordered := make([]Action, len(actions))
	copy(ordered, actions)
	sort.SliceStable(ordered, func(i, j int) bool {
		left := ordered[i]
		right := ordered[j]
		leftPriority := actionPriority(left.Kind())
		rightPriority := actionPriority(right.Kind())
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		if left.SpriteName() != right.SpriteName() {
			return left.SpriteName() < right.SpriteName()
		}
		return left.Description() < right.Description()
	})
	return ordered
}

func actionPriority(kind ActionKind) int {
	switch kind {
	case ActionTeardown:
		return 0
	case ActionUpdate:
		return 1
	case ActionProvision:
		return 2
	case ActionRedispatch:
		return 3
	default:
		return 4
	}
}

// ActionsView converts actions to stable machine-readable output.
func ActionsView(actions []Action) []ActionView {
	view := make([]ActionView, 0, len(actions))
	for _, action := range SortActions(actions) {
		view = append(view, action.toView())
	}
	return view
}

// Executor runs actions using a runtime.
type Executor struct {
	Runtime ActionRuntime
}

// Execute runs actions in deterministic order.
func (e Executor) Execute(ctx context.Context, actions []Action) error {
	for _, action := range SortActions(actions) {
		action.bind(e.Runtime)
		if err := action.Execute(ctx); err != nil {
			return fmt.Errorf("%s: %w", action.Description(), err)
		}
	}
	return nil
}

// DryRun renders dry-run descriptions in execution order.
func (e Executor) DryRun(actions []Action) []string {
	lines := make([]string, 0, len(actions))
	for _, action := range SortActions(actions) {
		lines = append(lines, action.DryRun())
	}
	return lines
}
