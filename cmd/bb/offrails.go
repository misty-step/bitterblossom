package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultSilenceAbortThreshold = 5 * time.Minute
	defaultSilenceWarnThreshold  = 45 * time.Second
	defaultErrorRepeatThreshold  = 3
	defaultCheckInterval         = 30 * time.Second
)

// errOffRails is the sentinel error used as context.CancelCause when the
// detector kills a dispatch. Check with errors.Is, not string matching.
var errOffRails = errors.New("off-rails")

// offRailsConfig configures the off-rails detector.
type offRailsConfig struct {
	SilenceAbort  time.Duration          // no output for this long → cancel (0 = disable)
	SilenceWarn   time.Duration          // no output for this long → log warning
	ErrorRepeatN  int                    // same error N times → cancel (0 = disable)
	CheckInterval time.Duration          // how often to check silence
	Cancel        context.CancelCauseFunc // called when off-rails detected
	Alert         io.Writer              // where to write alerts
}

// offRailsDetector monitors dispatch output for signs the agent is off-rails:
//   - No output for an extended period (hung agent)
//   - Same error repeated multiple times (error loop)
type offRailsDetector struct {
	silenceAbort  time.Duration
	silenceWarn   time.Duration
	errorRepeatN  int
	checkInterval time.Duration

	cancel context.CancelCauseFunc
	alert  io.Writer

	lastActivityNano atomic.Int64

	// mu guards errorCounts. Lock ordering: streamJSONWriter.mu → offRailsDetector.mu
	// (recordError is called from streamJSONWriter.writeLine under its lock).
	mu          sync.Mutex
	errorCounts map[string]int

	stopCh   chan struct{}
	stopOnce sync.Once
}

func newOffRailsDetector(cfg offRailsConfig) *offRailsDetector {
	if cfg.SilenceWarn <= 0 {
		cfg.SilenceWarn = defaultSilenceWarnThreshold
	}
	if cfg.ErrorRepeatN <= 0 {
		cfg.ErrorRepeatN = defaultErrorRepeatThreshold
	}
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = defaultCheckInterval
	}

	d := &offRailsDetector{
		silenceAbort:  cfg.SilenceAbort,
		silenceWarn:   cfg.SilenceWarn,
		errorRepeatN:  cfg.ErrorRepeatN,
		checkInterval: cfg.CheckInterval,
		cancel:        cfg.Cancel,
		alert:         cfg.Alert,
		errorCounts:   make(map[string]int),
		stopCh:        make(chan struct{}),
	}
	d.lastActivityNano.Store(time.Now().UnixNano())
	return d
}

// wrap returns a writer that marks activity on every successful write.
func (d *offRailsDetector) wrap(w io.Writer) io.Writer {
	return &activityWriter{out: w, mark: d.markActivity}
}

func (d *offRailsDetector) markActivity() {
	d.lastActivityNano.Store(time.Now().UnixNano())
}

// recordError tracks a tool error. If the same error appears errorRepeatN times, cancels the dispatch.
func (d *offRailsDetector) recordError(errText string) {
	if d.errorRepeatN <= 0 {
		return
	}

	// Truncate for map key grouping (exact match only; smarter normalization in #398).
	key := truncateError(errText)
	if key == "" {
		return
	}

	d.mu.Lock()
	d.errorCounts[key]++
	count := d.errorCounts[key]
	d.mu.Unlock()

	if count >= d.errorRepeatN {
		msg := fmt.Sprintf("same error repeated %d times", count)
		// Truncate shorter for display in alert output.
		_, _ = fmt.Fprintf(d.alert, "[off-rails] %s: %s\n", msg, truncateStr(key, 120))
		d.cancel(fmt.Errorf("%w: error loop — %s", errOffRails, msg))
	}
}

func (d *offRailsDetector) start() {
	go d.loop()
}

func (d *offRailsDetector) stop() {
	d.stopOnce.Do(func() { close(d.stopCh) })
}

func (d *offRailsDetector) loop() {
	ticker := time.NewTicker(d.checkInterval)
	defer ticker.Stop()

	warned := false
	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			last := time.Unix(0, d.lastActivityNano.Load())
			silent := time.Since(last)

			if !warned && silent >= d.silenceWarn {
				if d.silenceAbort > 0 {
					remaining := d.silenceAbort - silent
					if remaining < 0 {
						remaining = 0
					}
					_, _ = fmt.Fprintf(d.alert, "[off-rails] no output for %s (abort in %s)\n",
						silent.Round(time.Second), remaining.Round(time.Second))
				} else {
					_, _ = fmt.Fprintf(d.alert, "[off-rails] no output for %s; still running...\n",
						silent.Round(time.Second))
				}
				warned = true
			}

			if d.silenceAbort > 0 && silent >= d.silenceAbort {
				_, _ = fmt.Fprintf(d.alert, "[off-rails] aborting: no output for %s (threshold %s)\n",
					silent.Round(time.Second), d.silenceAbort)
				d.cancel(fmt.Errorf("%w: no output for %s", errOffRails, silent.Round(time.Second)))
				return
			}

			if silent < d.silenceWarn {
				warned = false
			}
		}
	}
}

// activityWriter wraps an io.Writer, calling mark on every successful write.
type activityWriter struct {
	out  io.Writer
	mark func()
}

func (w *activityWriter) Write(p []byte) (int, error) {
	n, err := w.out.Write(p)
	if n > 0 && w.mark != nil {
		w.mark()
	}
	return n, err
}

// truncateError trims whitespace and caps length for exact-match error grouping.
// Does not strip paths, line numbers, or timestamps — real normalization is a
// follow-up (see issue tracking smarter error deduplication).
func truncateError(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
