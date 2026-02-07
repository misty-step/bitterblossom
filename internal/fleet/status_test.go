package fleet

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/misty-step/bitterblossom/internal/clients"
)

type fakeSprite struct {
	sprites     []clients.SpriteInfo
	workspace   string
	memory      string
	checkpoints string
	apiPayload  []byte
}

func (f *fakeSprite) List(context.Context, string) ([]string, error) {
	names := make([]string, 0, len(f.sprites))
	for _, s := range f.sprites {
		names = append(names, s.Name)
	}
	return names, nil
}

func (f *fakeSprite) Exec(_ context.Context, _ string, _ string, cmd string) (string, error) {
	switch {
	case strings.Contains(cmd, "ls -la"):
		return f.workspace, nil
	case strings.Contains(cmd, "head -20"):
		return f.memory, nil
	default:
		return "", errors.New("unsupported")
	}
}

func (f *fakeSprite) API(context.Context, string, string) ([]byte, error) { return nil, nil }
func (f *fakeSprite) SpriteAPI(context.Context, string, string, string) ([]byte, error) {
	return f.apiPayload, nil
}
func (f *fakeSprite) ListCheckpoints(context.Context, string, string) (string, error) {
	return f.checkpoints, nil
}
func (f *fakeSprite) ListSprites(context.Context, string) ([]clients.SpriteInfo, error) {
	return f.sprites, nil
}

func TestStatusServiceFleetOverview(t *testing.T) {
	buf := &bytes.Buffer{}
	service := StatusService{
		Sprite: &fakeSprite{sprites: []clients.SpriteInfo{{Name: "thorn", Status: "running", URL: "x"}}},
		Out:    buf,
	}

	err := service.Run(context.Background(), StatusConfig{CompositionPath: "", SpritesDir: ""})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !strings.Contains(buf.String(), "thorn") {
		t.Fatalf("expected output to contain sprite name, got: %s", buf.String())
	}
}

func TestStatusServiceSpriteDetail(t *testing.T) {
	buf := &bytes.Buffer{}
	service := StatusService{
		Sprite: &fakeSprite{
			sprites:     []clients.SpriteInfo{{Name: "thorn", Status: "running"}},
			workspace:   "workspace files\n",
			memory:      "memory lines\n",
			checkpoints: "checkpoint1\n",
			apiPayload:  []byte(`{"status":"running","meta":{"a":1}}`),
		},
		Out: buf,
	}

	err := service.Run(context.Background(), StatusConfig{TargetSprite: "thorn"})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Sprite: thorn") {
		t.Fatalf("missing detail header: %s", out)
	}
	if !strings.Contains(out, "workspace files") {
		t.Fatalf("missing workspace output: %s", out)
	}
}

var _ clients.SpriteClient = (*fakeSprite)(nil)
