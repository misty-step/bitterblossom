package sprite

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// State is the lifecycle state for a sprite.
type State string

const (
	StateProvisioned State = "provisioned"
	StateIdle        State = "idle"
	StateWorking     State = "working"
	StateDone        State = "done"
	StateBlocked     State = "blocked"
	StateDead        State = "dead"
)

// Valid reports whether the state is part of the supported lifecycle.
func (s State) Valid() bool {
	switch s {
	case StateProvisioned, StateIdle, StateWorking, StateDone, StateBlocked, StateDead:
		return true
	default:
		return false
	}
}

// BusyStrategy configures dispatch behavior when a sprite is already working.
type BusyStrategy string

const (
	BusyQueue BusyStrategy = "queue"
	BusyError BusyStrategy = "error"
)

// Valid reports whether the busy strategy is supported.
func (b BusyStrategy) Valid() bool {
	switch b {
	case BusyQueue, BusyError:
		return true
	default:
		return false
	}
}

// Signal is an external lifecycle signal consumed by a sprite.
type Signal string

const (
	SignalReady   Signal = "ready"
	SignalDone    Signal = "done"
	SignalBlocked Signal = "blocked"
	SignalDead    Signal = "dead"
	SignalRetry   Signal = "retry"
)

// Task is the unit of work assigned to a sprite.
type Task struct {
	ID          string            `json:"id,omitempty"`
	Description string            `json:"description"`
	Repo        string            `json:"repo,omitempty"`
	Branch      string            `json:"branch,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func (t Task) valid() bool {
	return strings.TrimSpace(t.Description) != ""
}

// Persona identifies the behavior profile attached to a sprite.
type Persona struct {
	Name       string   `json:"name"`
	Definition string   `json:"definition"`
	Preference string   `json:"preference,omitempty"`
	Philosophy string   `json:"philosophy,omitempty"`
	Strengths  []string `json:"strengths,omitempty"`
}

// DispatchStatus reports whether a task started immediately or was queued.
type DispatchStatus string

const (
	DispatchStarted DispatchStatus = "started"
	DispatchQueued  DispatchStatus = "queued"
)

// DispatchResult reports dispatch handling details.
type DispatchResult struct {
	Status     DispatchStatus `json:"status"`
	Task       Task           `json:"task"`
	QueueDepth int            `json:"queue_depth"`
}

// Snapshot is the externally visible, VM-agnostic state for a sprite.
type Snapshot struct {
	Name        string  `json:"name"`
	Persona     Persona `json:"persona"`
	State       State   `json:"state"`
	Provisioned bool    `json:"provisioned"`
	Pending     int     `json:"pending"`
	CurrentTask *Task   `json:"current_task,omitempty"`
}

// Runtime hides infrastructure-specific behavior behind a tiny interface.
type Runtime interface {
	EnsureProvisioned(name string, persona Persona) error
	Dispatch(name string, task Task) error
	Teardown(name string) error
}

// Option customizes sprite behavior.
type Option func(*config) error

type config struct {
	runtime      Runtime
	busyStrategy BusyStrategy
	initialState State
	provisioned  bool
}

type noopRuntime struct{}

func (noopRuntime) EnsureProvisioned(string, Persona) error { return nil }
func (noopRuntime) Dispatch(string, Task) error             { return nil }
func (noopRuntime) Teardown(string) error                   { return nil }

// WithRuntime sets the runtime adapter used by the sprite handle.
func WithRuntime(runtime Runtime) Option {
	return func(cfg *config) error {
		if runtime == nil {
			return errors.New("sprite: runtime cannot be nil")
		}
		cfg.runtime = runtime
		return nil
	}
}

// WithBusyStrategy configures busy dispatch behavior.
func WithBusyStrategy(strategy BusyStrategy) Option {
	return func(cfg *config) error {
		if !strategy.Valid() {
			return fmt.Errorf("sprite: invalid busy strategy %q", strategy)
		}
		cfg.busyStrategy = strategy
		return nil
	}
}

// WithInitialState seeds state for handles built from existing fleet data.
func WithInitialState(state State, provisioned bool) Option {
	return func(cfg *config) error {
		if !state.Valid() {
			return fmt.Errorf("%w: %q", ErrInvalidState, state)
		}
		cfg.initialState = state
		cfg.provisioned = provisioned
		return nil
	}
}

// Sprite is an opaque handle for one managed fleet member.
//
// Callers can read high-level state and invoke lifecycle methods; they never
// interact with VM details directly.
type Sprite struct {
	mu          sync.Mutex
	name        string
	persona     Persona
	state       State
	provisioned bool
	current     *Task
	queue       []Task
	runtime     Runtime
	lifecycle   lifecycle
}

// New constructs a sprite handle.
func New(name string, persona Persona, options ...Option) (*Sprite, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("sprite: name is required")
	}

	cfg := config{
		runtime:      noopRuntime{},
		busyStrategy: BusyQueue,
		initialState: StateDead,
		provisioned:  false,
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(&cfg); err != nil {
			return nil, err
		}
	}

	lc, err := newLifecycle(cfg.busyStrategy)
	if err != nil {
		return nil, err
	}

	return &Sprite{
		name:        name,
		persona:     persona,
		state:       cfg.initialState,
		provisioned: cfg.provisioned,
		runtime:     cfg.runtime,
		lifecycle:   lc,
	}, nil
}

// Name returns the sprite identifier.
func (s *Sprite) Name() string {
	return s.name
}

// Persona returns the sprite persona metadata.
func (s *Sprite) Persona() Persona {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persona
}

// State returns the current lifecycle state.
func (s *Sprite) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Snapshot returns a VM-agnostic view of this sprite.
func (s *Sprite) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap := Snapshot{
		Name:        s.name,
		Persona:     s.persona,
		State:       s.state,
		Provisioned: s.provisioned,
		Pending:     len(s.queue),
	}
	if s.current != nil {
		task := *s.current
		snap.CurrentTask = &task
	}
	return snap
}

// Provision ensures the sprite exists. Existing sprites are verified and left
// unchanged so callers can invoke this method safely multiple times.
func (s *Sprite) Provision() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.runtime.EnsureProvisioned(s.name, s.persona); err != nil {
		return err
	}

	if s.provisioned {
		return nil
	}

	s.provisioned = true
	return s.transitionLocked(StateProvisioned)
}

// Dispatch assigns work. Behavior for busy sprites is controlled by strategy:
// queue the task (default) or return ErrBusy.
func (s *Sprite) Dispatch(task Task) (DispatchResult, error) {
	if !task.valid() {
		return DispatchResult{}, errors.New("sprite: task description is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.provisioned {
		return DispatchResult{}, ErrNotProvisioned
	}

	decision, err := s.lifecycle.dispatchDecision(s.state)
	if err != nil {
		return DispatchResult{}, err
	}

	switch decision {
	case dispatchQueue:
		s.queue = append(s.queue, task)
		return DispatchResult{Status: DispatchQueued, Task: task, QueueDepth: len(s.queue)}, nil
	case dispatchStart:
		if err := s.startTaskLocked(task); err != nil {
			return DispatchResult{}, err
		}
		return DispatchResult{Status: DispatchStarted, Task: task, QueueDepth: len(s.queue)}, nil
	default:
		return DispatchResult{}, fmt.Errorf("sprite: unsupported dispatch decision %d", decision)
	}
}

// Signal applies an external lifecycle signal.
func (s *Sprite) Signal(signal Signal) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.provisioned && signal != SignalDead {
		return nil
	}

	next, err := s.lifecycle.signalTransition(s.state, signal)
	if err != nil {
		return err
	}
	if err := s.transitionLocked(next); err != nil {
		return err
	}

	switch signal {
	case SignalDone, SignalBlocked, SignalDead:
		s.current = nil
	}

	if signal == SignalDone && len(s.queue) > 0 {
		if err := s.transitionLocked(StateIdle); err != nil {
			return err
		}
		return s.dispatchNextLocked()
	}
	return nil
}

// Teardown destroys the sprite. Calling teardown for a missing sprite succeeds.
func (s *Sprite) Teardown() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.provisioned {
		return nil
	}

	if err := s.runtime.Teardown(s.name); err != nil {
		return err
	}

	s.provisioned = false
	s.current = nil
	s.queue = nil
	return s.transitionLocked(StateDead)
}

func (s *Sprite) startTaskLocked(task Task) error {
	switch s.state {
	case StateDone, StateBlocked:
		if err := s.transitionLocked(StateIdle); err != nil {
			return err
		}
	}

	if err := s.runtime.Dispatch(s.name, task); err != nil {
		return err
	}
	if err := s.transitionLocked(StateWorking); err != nil {
		return err
	}
	taskCopy := task
	s.current = &taskCopy
	return nil
}

func (s *Sprite) dispatchNextLocked() error {
	if len(s.queue) == 0 || s.state != StateIdle {
		return nil
	}
	next := s.queue[0]
	s.queue = s.queue[1:]
	if err := s.startTaskLocked(next); err != nil {
		s.queue = append([]Task{next}, s.queue...)
		return err
	}
	return nil
}

func (s *Sprite) transitionLocked(next State) error {
	if err := validateTransition(s.state, next); err != nil {
		return err
	}
	s.state = next
	return nil
}
