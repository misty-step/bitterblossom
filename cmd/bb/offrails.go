package main

import (
	"context"
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

	key := normalizeError(errText)
	if key == "" {
		return
	}

	d.mu.Lock()
	d.errorCounts[key]++
	count := d.errorCounts[key]
	d.mu.Unlock()

	if count >= d.errorRepeatN {
		msg := fmt.Sprintf("same error repeated %d times", count)
		_, _ = fmt.Fprintf(d.alert, "[off-rails] %s: %s\n", msg, truncateStr(key, 120))
		d.cancel(fmt.Errorf("off-rails: error loop — %s", msg))
	}
}

func (d *offRailsDetector) start() {
	go d.loop()
}

func (d *offRailsDetector) stop() {
	d.stopOnce.Do(func() { close(d.stopCh) })
}

func (d *offRailsDetector) loop() {
	if d.silenceAbort <= 0 {
		// Silence abort disabled; still run for warnings
		d.warnLoop()
		return
	}

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
				remaining := d.silenceAbort - silent
				if remaining < 0 {
					remaining = 0
				}
				_, _ = fmt.Fprintf(d.alert, "[off-rails] no output for %s (abort in %s)\n",
					silent.Round(time.Second), remaining.Round(time.Second))
				warned = true
			}

			if silent >= d.silenceAbort {
				_, _ = fmt.Fprintf(d.alert, "[off-rails] aborting: no output for %s (threshold %s)\n",
					silent.Round(time.Second), d.silenceAbort)
				d.cancel(fmt.Errorf("off-rails: no output for %s", silent.Round(time.Second)))
				return
			}

			if silent < d.silenceWarn {
				warned = false
			}
		}
	}
}

// warnLoop runs when silence abort is disabled but we still want periodic warnings.
func (d *offRailsDetector) warnLoop() {
	ticker := time.NewTicker(d.silenceWarn)
	defer ticker.Stop()

	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			last := time.Unix(0, d.lastActivityNano.Load())
			silent := time.Since(last)
			if silent >= d.silenceWarn {
				_, _ = fmt.Fprintf(d.alert, "[off-rails] no output for %s; still running...\n",
					silent.Round(time.Second))
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

// normalizeError strips whitespace and truncates for grouping logically similar errors.
func normalizeError(s string) string {
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
