package monitor

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeExecutor struct {
	listOutput []string
	listErr    error
	execOutput map[string]string
	execErr    map[string]error
}

func (f *fakeExecutor) List(context.Context) ([]string, error) {
	return f.listOutput, f.listErr
}

func (f *fakeExecutor) Exec(_ context.Context, sprite, _ string, _ []byte) (string, error) {
	if err, ok := f.execErr[sprite]; ok {
		return "", err
	}
	if output, ok := f.execOutput[sprite]; ok {
		return output, nil
	}
	return "", nil
}

func TestCheckFleetStates(t *testing.T) {
	fake := &fakeExecutor{
		execOutput: map[string]string{
			"bramble": "__STATUS_JSON__{\"repo\":\"misty-step/heartbeat\",\"issue\":42,\"started\":\"2026-02-07T16:30:00Z\"}\n__AGENT_STATE__alive\n",
			"moss":    "__STATUS_JSON__{\"repo\":\"misty-step/api\",\"issue\":7,\"started\":\"2026-02-07T15:00:00Z\"}\n__AGENT_STATE__dead\n",
			"willow":  "__STATUS_JSON__{}\n__AGENT_STATE__dead\n",
		},
		execErr: map[string]error{
			"thorn": errors.New("sprite offline"),
		},
	}
	svc := NewService(fake)
	svc.now = func() time.Time {
		return time.Date(2026, time.February, 7, 17, 0, 0, 0, time.UTC)
	}

	report, err := svc.CheckFleet(context.Background(), FleetRequest{
		Sprites: []string{"bramble", "moss", "willow", "thorn"},
	})
	if err != nil {
		t.Fatalf("CheckFleet() error = %v", err)
	}
	if got := len(report.Sprites); got != 4 {
		t.Fatalf("expected 4 rows, got %d", got)
	}

	assertRow(t, report.Sprites[0], "bramble", TaskStateRunning, "misty-step/heartbeat#42")
	assertRow(t, report.Sprites[1], "moss", TaskStateDone, "misty-step/api#7")
	assertRow(t, report.Sprites[2], "thorn", TaskStateError, "-")
	assertRow(t, report.Sprites[3], "willow", TaskStateIdle, "-")
}

func TestCheckFleetAll(t *testing.T) {
	fake := &fakeExecutor{
		listOutput: []string{"willow", "bramble"},
		execOutput: map[string]string{
			"bramble": "__STATUS_JSON__{}\n__AGENT_STATE__dead\n",
			"willow":  "__STATUS_JSON__{}\n__AGENT_STATE__dead\n",
		},
	}
	svc := NewService(fake)
	report, err := svc.CheckFleet(context.Background(), FleetRequest{All: true})
	if err != nil {
		t.Fatalf("CheckFleet(All) error = %v", err)
	}
	if got := len(report.Sprites); got != 2 {
		t.Fatalf("expected 2 rows, got %d", got)
	}
	if report.Sprites[0].Sprite != "bramble" || report.Sprites[1].Sprite != "willow" {
		t.Fatalf("unexpected sort order: %#v", report.Sprites)
	}
}

func TestCheckFleetNeedsTargets(t *testing.T) {
	svc := NewService(&fakeExecutor{})
	_, err := svc.CheckFleet(context.Background(), FleetRequest{})
	if err == nil {
		t.Fatal("expected target selection error")
	}
}

func TestCheckFleetAllListError(t *testing.T) {
	svc := NewService(&fakeExecutor{listErr: errors.New("list failed")})
	_, err := svc.CheckFleet(context.Background(), FleetRequest{All: true})
	if err == nil {
		t.Fatal("expected list error")
	}
}

func assertRow(t *testing.T, row TaskStatus, sprite string, state TaskState, task string) {
	t.Helper()
	if row.Sprite != sprite {
		t.Fatalf("row sprite = %q, want %q", row.Sprite, sprite)
	}
	if row.State != state {
		t.Fatalf("row state = %q, want %q", row.State, state)
	}
	if row.Task != task {
		t.Fatalf("row task = %q, want %q", row.Task, task)
	}
}
