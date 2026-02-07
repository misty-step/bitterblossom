package logs

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/misty-step/bitterblossom/internal/clients"
)

type fakeSprite struct {
	list []string
}

func (f *fakeSprite) List(context.Context, string) ([]string, error) { return f.list, nil }
func (f *fakeSprite) Exec(_ context.Context, _ string, _ string, cmd string) (string, error) {
	switch {
	case strings.Contains(cmd, "pgrep -la"):
		return "123 claude -p", nil
	case strings.Contains(cmd, "TASK_COMPLETE"):
		return "  TASK_COMPLETE\n", nil
	case strings.Contains(cmd, "tail -"):
		return "line1\nline2\n", nil
	default:
		return "", nil
	}
}
func (f *fakeSprite) API(context.Context, string, string) ([]byte, error) { return nil, nil }
func (f *fakeSprite) SpriteAPI(context.Context, string, string, string) ([]byte, error) {
	return nil, nil
}
func (f *fakeSprite) ListCheckpoints(context.Context, string, string) (string, error) { return "", nil }
func (f *fakeSprite) ListSprites(context.Context, string) ([]clients.SpriteInfo, error) {
	return nil, nil
}

func TestViewerRunSingle(t *testing.T) {
	buf := &bytes.Buffer{}
	v := Viewer{Sprite: &fakeSprite{list: []string{"thorn"}}, Out: buf}
	err := v.Run(context.Background(), Config{Sprite: "thorn", Lines: 10})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !strings.Contains(buf.String(), "thorn") {
		t.Fatalf("expected sprite in output: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "line1") {
		t.Fatalf("expected log lines in output: %s", buf.String())
	}
}

func TestViewerRunAll(t *testing.T) {
	buf := &bytes.Buffer{}
	v := Viewer{Sprite: &fakeSprite{list: []string{"thorn", "fern"}}, Out: buf}
	err := v.Run(context.Background(), Config{All: true, Brief: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "thorn") || !strings.Contains(out, "fern") {
		t.Fatalf("expected both sprite outputs: %s", out)
	}
}

func TestViewerRunMissingSprite(t *testing.T) {
	v := Viewer{Sprite: &fakeSprite{}}
	if err := v.Run(context.Background(), Config{}); err == nil {
		t.Fatal("expected error when sprite missing")
	}
}

var _ clients.SpriteClient = (*fakeSprite)(nil)
