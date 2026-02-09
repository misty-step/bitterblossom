package lifecycle

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/misty-step/bitterblossom/internal/sprite"
)

func TestFleetOverviewCompositionAndOrphans(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, "bramble")
	compositionPath := filepath.Join(fx.rootDir, "compositions", "v1.yaml")
	writeFixtureFile(t, compositionPath, `version: 1
name: "test"
sprites:
  bramble:
    definition: sprites/bramble.md
`)

	cli := &sprite.MockSpriteCLI{
		APIFn: func(context.Context, string, string) (string, error) {
			return `{"sprites":[{"name":"bramble","status":"running","url":"https://bramble"},{"name":"thorn","status":"stopped","url":"https://thorn"}]}`, nil
		},
		CheckpointListFn: func(_ context.Context, name, _ string) (string, error) {
			if name == "bramble" {
				return "checkpoint-a", nil
			}
			return "", nil
		},
	}

	status, err := FleetOverview(context.Background(), cli, fx.cfg, compositionPath, FleetOverviewOpts{
		IncludeCheckpoints: true,
	})
	if err != nil {
		t.Fatalf("FleetOverview() error = %v", err)
	}
	if len(status.Sprites) != 2 {
		t.Fatalf("len(Sprites) = %d, want 2", len(status.Sprites))
	}
	if len(status.Composition) != 1 || !status.Composition[0].Provisioned {
		t.Fatalf("unexpected composition entries: %#v", status.Composition)
	}
	if len(status.Orphans) != 1 || status.Orphans[0].Name != "thorn" {
		t.Fatalf("unexpected orphans: %#v", status.Orphans)
	}
	if status.Checkpoints["bramble"] != "checkpoint-a" {
		t.Fatalf("checkpoint for bramble = %q", status.Checkpoints["bramble"])
	}
	if status.Checkpoints["thorn"] != "(none)" {
		t.Fatalf("checkpoint for thorn = %q", status.Checkpoints["thorn"])
	}
	if !status.CheckpointsIncluded {
		t.Fatalf("CheckpointsIncluded = false, want true")
	}
}

func TestFleetOverviewSkipsCheckpointsByDefault(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, "bramble")
	compositionPath := filepath.Join(fx.rootDir, "compositions", "v1.yaml")
	writeFixtureFile(t, compositionPath, `version: 1
name: "test"
sprites:
  bramble:
    definition: sprites/bramble.md
`)

	var checkpointCalls int
	cli := &sprite.MockSpriteCLI{
		APIFn: func(context.Context, string, string) (string, error) {
			return `{"sprites":[{"name":"bramble","status":"running","url":"https://bramble"}]}`, nil
		},
		CheckpointListFn: func(_ context.Context, _ string, _ string) (string, error) {
			checkpointCalls++
			return "checkpoint-a", nil
		},
	}

	status, err := FleetOverview(context.Background(), cli, fx.cfg, compositionPath, FleetOverviewOpts{})
	if err != nil {
		t.Fatalf("FleetOverview() error = %v", err)
	}
	if checkpointCalls != 0 {
		t.Fatalf("checkpoint calls = %d, want 0", checkpointCalls)
	}
	if status.CheckpointsIncluded {
		t.Fatalf("CheckpointsIncluded = true, want false")
	}
	if len(status.Checkpoints) != 0 {
		t.Fatalf("len(Checkpoints) = %d, want 0", len(status.Checkpoints))
	}
}

func TestSpriteDetail(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, "bramble")
	cli := &sprite.MockSpriteCLI{
		APISpriteFn: func(context.Context, string, string, string) (string, error) {
			return `{"name":"bramble","status":"running"}`, nil
		},
		ExecFn: func(_ context.Context, _ string, command string, _ []byte) (string, error) {
			switch command {
			case "ls -la '/home/sprite/workspace'":
				return "workspace listing", nil
			case "head -20 '/home/sprite/workspace/MEMORY.md'":
				return "memory lines", nil
			default:
				return "", nil
			}
		},
		CheckpointListFn: func(context.Context, string, string) (string, error) {
			return "checkpoint-1", nil
		},
	}

	result, err := SpriteDetail(context.Background(), cli, fx.cfg, "bramble")
	if err != nil {
		t.Fatalf("SpriteDetail() error = %v", err)
	}
	if result.Name != "bramble" {
		t.Fatalf("name = %q, want bramble", result.Name)
	}
	if result.Workspace != "workspace listing" {
		t.Fatalf("workspace = %q", result.Workspace)
	}
	if result.Memory != "memory lines" {
		t.Fatalf("memory = %q", result.Memory)
	}
	if result.Checkpoints != "checkpoint-1" {
		t.Fatalf("checkpoints = %q", result.Checkpoints)
	}
	if result.API["status"] != "running" {
		t.Fatalf("api = %#v", result.API)
	}
}
