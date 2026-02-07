package fleet

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/internal/clients"
)

type fleetSprite struct {
	list    []string
	execOut map[string]string
	execErr map[string]error
}

func (f *fleetSprite) List(context.Context, string) ([]string, error) { return f.list, nil }
func (f *fleetSprite) Exec(_ context.Context, _ string, sprite, cmd string) (string, error) {
	if err, ok := f.execErr[sprite]; ok {
		return "", err
	}
	if out, ok := f.execOut[sprite]; ok {
		return out, nil
	}
	_ = cmd
	return "IDLE", nil
}
func (f *fleetSprite) API(context.Context, string, string) ([]byte, error) { return nil, nil }
func (f *fleetSprite) SpriteAPI(context.Context, string, string, string) ([]byte, error) {
	return nil, nil
}
func (f *fleetSprite) ListCheckpoints(context.Context, string, string) (string, error) {
	return "", nil
}
func (f *fleetSprite) ListSprites(context.Context, string) ([]clients.SpriteInfo, error) {
	return nil, nil
}

type fleetFly struct {
	out string
	err error
}

func (f fleetFly) SSHRun(context.Context, string, string, string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.out, nil
}

func TestFleetServiceRunEventAndFallback(t *testing.T) {
	events := filepath.Join(t.TempDir(), "events.ndjson")
	if err := os.WriteFile(events, []byte("{\"sprite\":\"thorn\",\"event\":\"heartbeat\",\"timestamp\":\"2026-01-01T00:00:00Z\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	buf := &bytes.Buffer{}
	service := FleetService{
		Sprite: &fleetSprite{list: []string{"thorn", "fern"}, execOut: map[string]string{"fern": "RUNNING"}, execErr: map[string]error{}},
		Out:    buf,
		Now:    func() time.Time { return time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC) },
	}

	rows, err := service.Run(context.Background(), FleetConfig{EventsFile: events, MaxAge: 10 * time.Minute})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Source != "event-log" {
		t.Fatalf("expected event-log source for thorn: %+v", rows[0])
	}
	if rows[1].Source != "sprite-exec" {
		t.Fatalf("expected sprite-exec source for fern: %+v", rows[1])
	}
	if !strings.Contains(buf.String(), "SPRITE") {
		t.Fatalf("expected tabular output header: %s", buf.String())
	}
}

func TestFleetServiceFallbackToFly(t *testing.T) {
	buf := &bytes.Buffer{}
	service := FleetService{
		Sprite: &fleetSprite{list: []string{"thorn"}, execOut: map[string]string{}, execErr: map[string]error{"thorn": errors.New("exec fail")}},
		Fly:    fleetFly{out: "BLOCKED"},
		Out:    buf,
		Now:    func() time.Time { return time.Now() },
	}

	rows, err := service.Run(context.Background(), FleetConfig{EventsFile: "", MaxAge: time.Minute, JSONOutput: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if rows[0].Source != "fly-ssh" {
		t.Fatalf("expected fly-ssh source, got %+v", rows[0])
	}
	if rows[0].Status != "blocked" {
		t.Fatalf("expected blocked status, got %s", rows[0].Status)
	}
}

func TestCollectOrphans(t *testing.T) {
	live := []clients.SpriteInfo{{Name: "thorn", Status: "running"}, {Name: "ghost", Status: "stopped"}}
	orphans := collectOrphans(live, []string{"thorn"})
	if len(orphans) != 1 || !strings.Contains(orphans[0], "ghost") {
		t.Fatalf("unexpected orphans: %v", orphans)
	}
}

var _ clients.SpriteClient = (*fleetSprite)(nil)
var _ clients.FlyClient = fleetFly{}
