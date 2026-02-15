package events

import (
	"context"
	"sort"
	"sync"
	"time"
)

const DefaultGapThreshold = 5 * time.Minute

// AggregatorConfig configures stream aggregation behavior.
type AggregatorConfig struct {
	// Window enables rolling-window snapshots in real-time mode.
	// Zero means aggregate over all retained events.
	Window time.Duration

	// GapThreshold marks inactivity gaps used in uptime/activity analysis.
	GapThreshold time.Duration

	Now func() time.Time
}

// ActivityGap describes an inactivity period.
type ActivityGap struct {
	Sprite   string        `json:"sprite"`
	Start    time.Time     `json:"start"`
	End      time.Time     `json:"end"`
	Duration time.Duration `json:"duration"`
}

// SpriteStats is an aggregation summary for one sprite.
type SpriteStats struct {
	Sprite         string        `json:"sprite"`
	TotalEvents    int           `json:"total_events"`
	ByType         map[Kind]int  `json:"by_type"`
	EventsPerMin   float64       `json:"events_per_min"`
	ErrorRate      float64       `json:"error_rate"`
	Uptime         float64       `json:"uptime"`
	LastEventAt    time.Time     `json:"last_event_at"`
	ActivityGaps   []ActivityGap `json:"activity_gaps,omitempty"`
	MaxActivityGap time.Duration `json:"max_activity_gap,omitempty"`
}

// Snapshot is a full aggregation result for a time window.
type Snapshot struct {
	Start         time.Time              `json:"start"`
	End           time.Time              `json:"end"`
	TotalEvents   int                    `json:"total_events"`
	ByType        map[Kind]int           `json:"by_type"`
	BySprite      map[string]SpriteStats `json:"by_sprite"`
	EventsPerMin  float64                `json:"events_per_min"`
	ErrorRate     float64                `json:"error_rate"`
	Uptime        float64                `json:"uptime"`
	ActivityGaps  []ActivityGap          `json:"activity_gaps,omitempty"`
	UniqueSprites int                    `json:"unique_sprites"`
}

// Aggregator computes per-window event statistics.
type Aggregator struct {
	mu sync.RWMutex

	cfg    AggregatorConfig
	events []Event
}

// NewAggregator constructs an event aggregator.
func NewAggregator(cfg AggregatorConfig) *Aggregator {
	if cfg.GapThreshold <= 0 {
		cfg.GapThreshold = DefaultGapThreshold
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Aggregator{cfg: cfg}
}

// Add ingests one event.
func (a *Aggregator) Add(event Event) {
	if event == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	a.events = append(a.events, event)
	a.pruneLocked(a.cfg.Now())
}

// AddAll ingests a batch of events.
func (a *Aggregator) AddAll(events []Event) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, event := range events {
		if event == nil {
			continue
		}
		a.events = append(a.events, event)
	}
	a.pruneLocked(a.cfg.Now())
}

// Consume ingests events from a channel until context cancellation or channel close.
func (a *Aggregator) Consume(ctx context.Context, in <-chan Event) error {
	for {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.Canceled || ctx.Err() == context.DeadlineExceeded {
				return nil
			}
			return ctx.Err()
		case event, ok := <-in:
			if !ok {
				return nil
			}
			a.Add(event)
		}
	}
}

// Snapshot returns current aggregated statistics.
func (a *Aggregator) Snapshot() Snapshot {
	a.mu.RLock()
	events := append([]Event(nil), a.events...)
	now := a.cfg.Now()
	cfg := a.cfg
	a.mu.RUnlock()

	return Aggregate(events, cfg.Window, cfg.GapThreshold, now)
}

func (a *Aggregator) pruneLocked(now time.Time) {
	if a.cfg.Window <= 0 || len(a.events) == 0 {
		return
	}
	cutoff := now.Add(-a.cfg.Window)
	first := 0
	for first < len(a.events) && a.events[first].Timestamp().Before(cutoff) {
		first++
	}
	if first > 0 {
		a.events = append([]Event(nil), a.events[first:]...)
	}
}

// Aggregate computes a historical or point-in-time snapshot.
func Aggregate(input []Event, window, gapThreshold time.Duration, now time.Time) Snapshot {
	if gapThreshold <= 0 {
		gapThreshold = DefaultGapThreshold
	}
	events := make([]Event, 0, len(input))
	for _, event := range input {
		if event != nil {
			events = append(events, event)
		}
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp().Before(events[j].Timestamp())
	})

	if len(events) == 0 {
		return Snapshot{
			Start:        now.UTC(),
			End:          now.UTC(),
			ByType:       map[Kind]int{},
			BySprite:     map[string]SpriteStats{},
			ActivityGaps: []ActivityGap{},
		}
	}

	end := now.UTC()
	if end.IsZero() {
		end = events[len(events)-1].Timestamp().UTC()
	}
	start := events[0].Timestamp().UTC()
	if window > 0 {
		start = end.Add(-window)
	}

	filtered := make([]Event, 0, len(events))
	for _, event := range events {
		ts := event.Timestamp()
		if ts.Before(start) || ts.After(end) {
			continue
		}
		filtered = append(filtered, event)
	}

	byType := make(map[Kind]int)
	spriteEvents := make(map[string][]Event)
	errorCount := 0
	for _, event := range filtered {
		byType[event.Kind()]++
		spriteEvents[event.Sprite()] = append(spriteEvents[event.Sprite()], event)
		if event.Kind() == KindError {
			errorCount++
		}
	}

	durationMinutes := end.Sub(start).Minutes()
	if durationMinutes <= 0 {
		durationMinutes = 1
	}

	bySprite := make(map[string]SpriteStats, len(spriteEvents))
	allGaps := make([]ActivityGap, 0)
	totalUptime := 0.0
	for sprite, items := range spriteEvents {
		sort.Slice(items, func(i, j int) bool { return items[i].Timestamp().Before(items[j].Timestamp()) })

		spriteByType := make(map[Kind]int)
		spriteErr := 0
		for _, event := range items {
			spriteByType[event.Kind()]++
			if event.Kind() == KindError {
				spriteErr++
			}
		}

		gaps := detectActivityGaps(sprite, items, start, end, gapThreshold)
		maxGap := time.Duration(0)
		downtime := time.Duration(0)
		for _, gap := range gaps {
			if gap.Duration > maxGap {
				maxGap = gap.Duration
			}
			downtime += gap.Duration
			allGaps = append(allGaps, gap)
		}
		windowDuration := end.Sub(start)
		uptime := 1.0
		if windowDuration > 0 {
			uptime = 1 - (float64(downtime) / float64(windowDuration))
			if uptime < 0 {
				uptime = 0
			}
		}
		totalUptime += uptime

		stats := SpriteStats{
			Sprite:         sprite,
			TotalEvents:    len(items),
			ByType:         spriteByType,
			EventsPerMin:   float64(len(items)) / durationMinutes,
			ErrorRate:      ratio(spriteErr, len(items)),
			Uptime:         uptime,
			LastEventAt:    items[len(items)-1].Timestamp().UTC(),
			ActivityGaps:   gaps,
			MaxActivityGap: maxGap,
		}
		bySprite[sprite] = stats
	}

	sort.Slice(allGaps, func(i, j int) bool {
		if allGaps[i].Start.Equal(allGaps[j].Start) {
			return allGaps[i].Sprite < allGaps[j].Sprite
		}
		return allGaps[i].Start.Before(allGaps[j].Start)
	})

	return Snapshot{
		Start:         start,
		End:           end,
		TotalEvents:   len(filtered),
		ByType:        byType,
		BySprite:      bySprite,
		EventsPerMin:  float64(len(filtered)) / durationMinutes,
		ErrorRate:     ratio(errorCount, len(filtered)),
		Uptime:        ratioFloat(totalUptime, float64(len(spriteEvents))),
		ActivityGaps:  allGaps,
		UniqueSprites: len(spriteEvents),
	}
}

func detectActivityGaps(sprite string, events []Event, start, end time.Time, threshold time.Duration) []ActivityGap {
	if len(events) == 0 {
		return nil
	}
	gaps := make([]ActivityGap, 0)

	// Gap from window start to first event.
	first := events[0].Timestamp().UTC()
	if first.After(start) {
		gap := first.Sub(start)
		if gap >= threshold {
			gaps = append(gaps, ActivityGap{Sprite: sprite, Start: start, End: first, Duration: gap})
		}
	}

	// Gaps between events.
	for i := 1; i < len(events); i++ {
		prev := events[i-1].Timestamp().UTC()
		curr := events[i].Timestamp().UTC()
		gap := curr.Sub(prev)
		if gap >= threshold {
			gaps = append(gaps, ActivityGap{Sprite: sprite, Start: prev, End: curr, Duration: gap})
		}
	}

	// Gap from last event to window end.
	last := events[len(events)-1].Timestamp().UTC()
	if end.After(last) {
		gap := end.Sub(last)
		if gap >= threshold {
			gaps = append(gaps, ActivityGap{Sprite: sprite, Start: last, End: end, Duration: gap})
		}
	}

	return gaps
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func ratioFloat(numerator, denominator float64) float64 {
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}
