package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/misty-step/bitterblossom/internal/clients"
)

type fakeRunner struct {
	responses map[string]string
}

func (f fakeRunner) Run(_ context.Context, name string, args ...string) (string, int, error) {
	key := name + " " + joinArgs(args)
	if out, ok := f.responses[key]; ok {
		return out, 0, nil
	}
	return "", 1, assertErr{}
}

type assertErr struct{}

func (assertErr) Error() string { return "error" }

func joinArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	out := args[0]
	for i := 1; i < len(args); i++ {
		out += " " + args[i]
	}
	return out
}

func TestSystemHealthCollectorCollect(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := fakeRunner{responses: map[string]string{
		"ps -A -o %cpu=": "1.0\n1.0\n",
		"ps -A -o %mem=": "2.0\n",
	}}

	collector := SystemHealthCollector{
		Workspace: tmp,
		Runner:    r,
		CPUCores:  2,
	}
	snap := collector.Collect(context.Background(), 3, true)

	if snap.CPUPercent != "1%" {
		t.Fatalf("cpu percent: got %q", snap.CPUPercent)
	}
	if snap.MemoryPercent != "2%" {
		t.Fatalf("memory percent: got %q", snap.MemoryPercent)
	}
	if snap.WorkspaceBytes == 0 {
		t.Fatal("expected workspace bytes > 0")
	}
	if !snap.ClaudeRunning {
		t.Fatal("expected claude running")
	}
	if snap.LoopIteration != 3 {
		t.Fatalf("iteration mismatch: %d", snap.LoopIteration)
	}
}

func TestDiskPercentUnknown(t *testing.T) {
	if got := diskPercent("/path/does/not/exist"); got == "" {
		t.Fatal("expected a non-empty disk percentage")
	}
}

var _ clients.Runner = fakeRunner{}
