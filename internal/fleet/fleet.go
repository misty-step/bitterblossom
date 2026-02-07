package fleet

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/misty-step/bitterblossom/internal/clients"
)

// FleetConfig controls event-log-based fleet status checks.
type FleetConfig struct {
	Org        string
	EventsFile string
	MaxAge     time.Duration
	JSONOutput bool
}

// FleetRow is one sprite status row.
type FleetRow struct {
	Sprite    string `json:"sprite"`
	Status    string `json:"status"`
	Source    string `json:"source"`
	LastEvent string `json:"last_event,omitempty"`
	Age       string `json:"age,omitempty"`
}

// FleetService checks fleet status from event logs with exec/ssh fallback.
type FleetService struct {
	Sprite clients.SpriteClient
	Fly    clients.FlyClient
	Out    io.Writer
	Now    func() time.Time
}

// Run prints fleet status.
func (f *FleetService) Run(ctx context.Context, cfg FleetConfig) ([]FleetRow, error) {
	if f.Sprite == nil {
		return nil, fmt.Errorf("sprite client required")
	}
	if f.Out == nil {
		f.Out = os.Stdout
	}
	if f.Now == nil {
		f.Now = time.Now
	}
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = 20 * time.Minute
	}

	events, _ := loadLatestEvents(cfg.EventsFile)
	sprites, err := f.Sprite.List(ctx, cfg.Org)
	if err != nil {
		return nil, err
	}

	rows := make([]FleetRow, 0, len(sprites))
	for _, sprite := range sprites {
		if event, ok := events[sprite]; ok {
			age := f.Now().Sub(event.Time)
			if age <= cfg.MaxAge {
				rows = append(rows, FleetRow{
					Sprite:    sprite,
					Status:    statusFromEvent(event.Event),
					Source:    "event-log",
					LastEvent: event.Event,
					Age:       age.Truncate(time.Second).String(),
				})
				continue
			}
		}

		status, source := f.fallbackStatus(ctx, cfg.Org, sprite)
		rows = append(rows, FleetRow{Sprite: sprite, Status: status, Source: source})
	}

	if cfg.JSONOutput {
		enc := json.NewEncoder(f.Out)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rows)
	} else {
		tw := tabwriter.NewWriter(f.Out, 2, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "SPRITE\tSTATUS\tSOURCE\tLAST EVENT\tAGE")
		for _, row := range rows {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", row.Sprite, row.Status, row.Source, row.LastEvent, row.Age)
		}
		_ = tw.Flush()
	}

	return rows, nil
}

func (f *FleetService) fallbackStatus(ctx context.Context, org, sprite string) (string, string) {
	cmd := `if [ -f /home/sprite/workspace/TASK_COMPLETE ]; then echo COMPLETE;
elif [ -f /home/sprite/workspace/BLOCKED.md ]; then echo BLOCKED;
elif pgrep -f "claude -p" >/dev/null 2>&1; then echo RUNNING;
else echo IDLE; fi`
	out, err := f.Sprite.Exec(ctx, org, sprite, cmd)
	if err == nil {
		return normalizeStatus(out), "sprite-exec"
	}
	if f.Fly != nil {
		sshOut, sshErr := f.Fly.SSHRun(ctx, org, sprite, cmd)
		if sshErr == nil {
			return normalizeStatus(sshOut), "fly-ssh"
		}
	}
	return "unknown", "unavailable"
}

type eventLine struct {
	Sprite    string         `json:"sprite"`
	Event     string         `json:"event"`
	Timestamp string         `json:"timestamp"`
	Metadata  map[string]any `json:"metadata"`
}

type latestEvent struct {
	Event string
	Time  time.Time
}

func loadLatestEvents(path string) (map[string]latestEvent, error) {
	m := map[string]latestEvent{}
	if strings.TrimSpace(path) == "" {
		return m, nil
	}
	fh, err := os.Open(path)
	if err != nil {
		return m, err
	}
	defer func() { _ = fh.Close() }()

	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev eventLine
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if ev.Sprite == "" || ev.Timestamp == "" {
			continue
		}
		ts, err := time.Parse(time.RFC3339, ev.Timestamp)
		if err != nil {
			continue
		}
		old, ok := m[ev.Sprite]
		if !ok || ts.After(old.Time) {
			m[ev.Sprite] = latestEvent{Event: ev.Event, Time: ts}
		}
	}
	return m, nil
}

func statusFromEvent(event string) string {
	event = strings.ToLower(strings.TrimSpace(event))
	switch {
	case strings.Contains(event, "complete"):
		return "completed"
	case strings.Contains(event, "block"):
		return "blocked"
	case strings.Contains(event, "shutdown"):
		return "stopped"
	case strings.Contains(event, "error"):
		return "error"
	default:
		return "active"
	}
}

func normalizeStatus(out string) string {
	value := strings.ToLower(strings.TrimSpace(out))
	switch {
	case strings.Contains(value, "complete"):
		return "completed"
	case strings.Contains(value, "blocked"):
		return "blocked"
	case strings.Contains(value, "running"):
		return "running"
	case strings.Contains(value, "idle"):
		return "idle"
	default:
		return "unknown"
	}
}
