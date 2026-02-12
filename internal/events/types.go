package events

import (
	"os"
	"path/filepath"
	"time"

	pkgevents "github.com/misty-step/bitterblossom/pkg/events"
)

const dailyLayout = "2006-01-02"

// Event aliases the shared fleet event schema.
type Event = pkgevents.Event

// Filter aliases the shared event filter type.
type Filter = pkgevents.Filter

// QueryOptions controls event history queries.
type QueryOptions struct {
	Filter Filter
	Since  time.Time
	Until  time.Time
	Issue  int
}

// DefaultDir returns the canonical on-operator event store directory.
func DefaultDir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil || home == "" {
			return filepath.Join(".config", "bb", "events")
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "bb", "events")
}

