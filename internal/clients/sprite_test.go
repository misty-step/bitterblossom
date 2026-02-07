package clients

import (
	"context"
	"reflect"
	"testing"
)

type fakeRunner struct {
	out string
	err error
}

func (f fakeRunner) Run(context.Context, string, ...string) (string, int, error) {
	if f.err != nil {
		return f.out, 1, f.err
	}
	return f.out, 0, nil
}

func TestSpriteCLIList(t *testing.T) {
	cli := NewSpriteCLI(fakeRunner{out: "thorn\nfern\n"}, "sprite")
	names, err := cli.List(context.Background(), "misty-step")
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	want := []string{"thorn", "fern"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("got %v want %v", names, want)
	}
}

func TestListSpritesDecode(t *testing.T) {
	payload := `{"sprites":[{"name":"thorn","status":"running","url":"https://x"}]}`
	cli := NewSpriteCLI(fakeRunner{out: payload}, "sprite")
	sprites, err := cli.ListSprites(context.Background(), "misty-step")
	if err != nil {
		t.Fatalf("ListSprites returned error: %v", err)
	}
	if len(sprites) != 1 || sprites[0].Name != "thorn" {
		t.Fatalf("unexpected sprites: %+v", sprites)
	}
}

func TestSpriteCLIExec(t *testing.T) {
	cli := NewSpriteCLI(fakeRunner{out: "ok"}, "sprite")
	out, err := cli.Exec(context.Background(), "misty-step", "thorn", "echo hi")
	if err != nil {
		t.Fatalf("Exec returned error: %v", err)
	}
	if out != "ok" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestSpriteCLISpriteAPIAndCheckpoints(t *testing.T) {
	cli := NewSpriteCLI(fakeRunner{out: `{"k":"v"}`}, "sprite")
	if _, err := cli.SpriteAPI(context.Background(), "misty-step", "thorn", "/"); err != nil {
		t.Fatalf("SpriteAPI error: %v", err)
	}
	if _, err := cli.ListCheckpoints(context.Background(), "misty-step", "thorn"); err != nil {
		t.Fatalf("ListCheckpoints error: %v", err)
	}
}
