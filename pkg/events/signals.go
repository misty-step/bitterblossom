package events

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Severity classifies signal impact.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

const (
	SignalSpriteStalled = "sprite_stalled"
	SignalBuildFailure  = "build_failure"
	SignalRepeatedError = "repeated_error"
)

// Signal is a structured alert emitted by stream detectors.
type Signal struct {
	Name            string         `json:"name"`
	Severity        Severity       `json:"severity"`
	Source          string         `json:"source"`
	Description     string         `json:"description"`
	SuggestedAction string         `json:"suggested_action,omitempty"`
	At              time.Time      `json:"at"`
	Count           int            `json:"count,omitempty"`
	Window          time.Duration  `json:"window,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// StallSignalConfig controls no-activity stall detection.
type StallSignalConfig struct {
	Enabled         bool
	Threshold       time.Duration
	Severity        Severity
	SuggestedAction string
}

// BuildFailureSignalConfig controls build-failure matching.
type BuildFailureSignalConfig struct {
	Enabled         bool
	Severity        Severity
	SuggestedAction string
	ErrorCodes      []string
	MessageContains []string
}

// RepeatedErrorSignalConfig controls repeated-error detection.
type RepeatedErrorSignalConfig struct {
	Enabled         bool
	Window          time.Duration
	Threshold       int
	Severity        Severity
	SuggestedAction string
}

// SignalConfig configures the default detector chain.
type SignalConfig struct {
	Now func() time.Time

	Stall         StallSignalConfig
	BuildFailure  BuildFailureSignalConfig
	RepeatedError RepeatedErrorSignalConfig
}

// DefaultSignalConfig returns a practical baseline detector configuration.
func DefaultSignalConfig() SignalConfig {
	return SignalConfig{
		Now: func() time.Time { return time.Now().UTC() },
		Stall: StallSignalConfig{
			Enabled:         true,
			Threshold:       10 * time.Minute,
			Severity:        SeverityWarning,
			SuggestedAction: "check sprite health and recent logs",
		},
		BuildFailure: BuildFailureSignalConfig{
			Enabled:         true,
			Severity:        SeverityCritical,
			SuggestedAction: "inspect build logs, fix compile/test failures, then re-dispatch",
			ErrorCodes:      []string{"build_failed", "build"},
			MessageContains: []string{"build failed", "compile failed", "test failed"},
		},
		RepeatedError: RepeatedErrorSignalConfig{
			Enabled:         true,
			Window:          5 * time.Minute,
			Threshold:       3,
			Severity:        SeverityWarning,
			SuggestedAction: "investigate recurring runtime failures",
		},
	}
}

// SignalDetector is a composable stream detector.
type SignalDetector interface {
	Observe(Event) []Signal
	Tick(time.Time) []Signal
}

// SignalEngine chains multiple detectors over an event stream.
type SignalEngine struct {
	mu        sync.Mutex
	now       func() time.Time
	detectors []SignalDetector
}

// NewSignalEngine builds a detector chain with explicit detectors.
func NewSignalEngine(now func() time.Time, detectors ...SignalDetector) *SignalEngine {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &SignalEngine{
		now:       now,
		detectors: append([]SignalDetector(nil), detectors...),
	}
}

// NewConfiguredSignalEngine builds a detector chain from SignalConfig.
func NewConfiguredSignalEngine(cfg SignalConfig) *SignalEngine {
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}

	detectors := make([]SignalDetector, 0, 3)
	if cfg.Stall.Enabled {
		detectors = append(detectors, NewStallDetector(cfg.Stall, cfg.Now))
	}
	if cfg.BuildFailure.Enabled {
		detectors = append(detectors, NewBuildFailureDetector(cfg.BuildFailure))
	}
	if cfg.RepeatedError.Enabled {
		detectors = append(detectors, NewRepeatedErrorDetector(cfg.RepeatedError))
	}

	return NewSignalEngine(cfg.Now, detectors...)
}

// Observe processes one event through the detector chain.
func (e *SignalEngine) Observe(event Event) []Signal {
	e.mu.Lock()
	defer e.mu.Unlock()

	out := make([]Signal, 0)
	for _, detector := range e.detectors {
		out = append(out, detector.Observe(event)...)
	}
	return out
}

// Tick evaluates time-based detectors (like stalls) at current time.
func (e *SignalEngine) Tick() []Signal {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := e.now()
	out := make([]Signal, 0)
	for _, detector := range e.detectors {
		out = append(out, detector.Tick(now)...)
	}
	return out
}

// StallDetector triggers when a sprite emits no events for the configured threshold.
type StallDetector struct {
	cfg StallSignalConfig
	now func() time.Time

	lastEventAt map[string]time.Time
	alerted     map[string]bool
}

// NewStallDetector constructs a stall detector.
func NewStallDetector(cfg StallSignalConfig, now func() time.Time) *StallDetector {
	if cfg.Threshold <= 0 {
		cfg.Threshold = 10 * time.Minute
	}
	if cfg.Severity == "" {
		cfg.Severity = SeverityWarning
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &StallDetector{
		cfg:         cfg,
		now:         now,
		lastEventAt: make(map[string]time.Time),
		alerted:     make(map[string]bool),
	}
}

// Observe records per-sprite last-seen activity.
func (d *StallDetector) Observe(event Event) []Signal {
	if event == nil {
		return nil
	}
	sprite := strings.TrimSpace(event.Sprite())
	if sprite == "" {
		return nil
	}

	ts := event.Timestamp()
	if ts.IsZero() {
		ts = d.now()
	}
	d.lastEventAt[sprite] = ts
	d.alerted[sprite] = false
	return nil
}

// Tick checks for stalled sprites.
func (d *StallDetector) Tick(now time.Time) []Signal {
	if now.IsZero() {
		now = d.now()
	}
	out := make([]Signal, 0)
	for sprite, last := range d.lastEventAt {
		if last.IsZero() || d.alerted[sprite] {
			continue
		}
		idle := now.Sub(last)
		if idle < d.cfg.Threshold {
			continue
		}

		d.alerted[sprite] = true
		out = append(out, Signal{
			Name:            SignalSpriteStalled,
			Severity:        d.cfg.Severity,
			Source:          sprite,
			Description:     fmt.Sprintf("sprite %s has no events for %s", sprite, idle.Round(time.Second)),
			SuggestedAction: d.cfg.SuggestedAction,
			At:              now.UTC(),
			Window:          d.cfg.Threshold,
			Metadata: map[string]any{
				"last_event_at": last.UTC(),
			},
		})
	}
	return out
}

// BuildFailureDetector emits signals when an error event looks like a build failure.
type BuildFailureDetector struct {
	cfg           BuildFailureSignalConfig
	errorCodes    map[string]struct{}
	msgSubstrings []string
}

// NewBuildFailureDetector constructs a build failure detector.
func NewBuildFailureDetector(cfg BuildFailureSignalConfig) *BuildFailureDetector {
	if cfg.Severity == "" {
		cfg.Severity = SeverityCritical
	}
	codes := make(map[string]struct{}, len(cfg.ErrorCodes))
	for _, code := range cfg.ErrorCodes {
		code = strings.TrimSpace(strings.ToLower(code))
		if code != "" {
			codes[code] = struct{}{}
		}
	}
	substrings := make([]string, 0, len(cfg.MessageContains))
	for _, item := range cfg.MessageContains {
		item = strings.TrimSpace(strings.ToLower(item))
		if item != "" {
			substrings = append(substrings, item)
		}
	}
	return &BuildFailureDetector{
		cfg:           cfg,
		errorCodes:    codes,
		msgSubstrings: substrings,
	}
}

// Observe checks error events for build-failure characteristics.
func (d *BuildFailureDetector) Observe(event Event) []Signal {
	errEvent, ok := event.(ErrorEvent)
	if !ok {
		ptr, ok := event.(*ErrorEvent)
		if !ok {
			return nil
		}
		errEvent = *ptr
	}

	code := strings.TrimSpace(strings.ToLower(errEvent.Code))
	message := strings.TrimSpace(strings.ToLower(errEvent.Message))
	matched := false

	if _, ok := d.errorCodes[code]; ok && code != "" {
		matched = true
	}
	if !matched {
		for _, needle := range d.msgSubstrings {
			if strings.Contains(message, needle) {
				matched = true
				break
			}
		}
	}
	if !matched {
		return nil
	}

	ts := errEvent.Timestamp()
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	return []Signal{{
		Name:            SignalBuildFailure,
		Severity:        d.cfg.Severity,
		Source:          errEvent.Sprite(),
		Description:     fmt.Sprintf("build failure detected for sprite %s: %s", errEvent.Sprite(), errEvent.Message),
		SuggestedAction: d.cfg.SuggestedAction,
		At:              ts.UTC(),
		Metadata: map[string]any{
			"code":    errEvent.Code,
			"message": errEvent.Message,
		},
	}}
}

// Tick is a no-op for build-failure detection.
func (d *BuildFailureDetector) Tick(time.Time) []Signal { return nil }

// RepeatedErrorDetector tracks recurring errors inside a time window.
type RepeatedErrorDetector struct {
	cfg RepeatedErrorSignalConfig

	perSprite map[string][]time.Time
	lastFired map[string]time.Time
}

// NewRepeatedErrorDetector constructs a repeated-error detector.
func NewRepeatedErrorDetector(cfg RepeatedErrorSignalConfig) *RepeatedErrorDetector {
	if cfg.Window <= 0 {
		cfg.Window = 5 * time.Minute
	}
	if cfg.Threshold <= 0 {
		cfg.Threshold = 3
	}
	if cfg.Severity == "" {
		cfg.Severity = SeverityWarning
	}
	return &RepeatedErrorDetector{
		cfg:       cfg,
		perSprite: make(map[string][]time.Time),
		lastFired: make(map[string]time.Time),
	}
}

// Observe records errors and emits when threshold is crossed in-window.
func (d *RepeatedErrorDetector) Observe(event Event) []Signal {
	if event == nil || event.Kind() != KindError {
		return nil
	}
	sprite := strings.TrimSpace(event.Sprite())
	if sprite == "" {
		return nil
	}

	ts := event.Timestamp()
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	windowStart := ts.Add(-d.cfg.Window)
	samples := d.perSprite[sprite]
	pruned := samples[:0]
	for _, sample := range samples {
		if !sample.Before(windowStart) {
			pruned = append(pruned, sample)
		}
	}
	pruned = append(pruned, ts)
	d.perSprite[sprite] = pruned

	if len(pruned) < d.cfg.Threshold {
		return nil
	}

	if last, ok := d.lastFired[sprite]; ok && ts.Sub(last) < d.cfg.Window {
		return nil
	}
	d.lastFired[sprite] = ts

	return []Signal{{
		Name:            SignalRepeatedError,
		Severity:        d.cfg.Severity,
		Source:          sprite,
		Description:     fmt.Sprintf("sprite %s emitted %d errors in %s", sprite, len(pruned), d.cfg.Window),
		SuggestedAction: d.cfg.SuggestedAction,
		At:              ts.UTC(),
		Count:           len(pruned),
		Window:          d.cfg.Window,
	}}
}

// Tick is a no-op for repeated-error detection.
func (d *RepeatedErrorDetector) Tick(time.Time) []Signal { return nil }

