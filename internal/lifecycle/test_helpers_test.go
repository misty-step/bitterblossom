package lifecycle

import (
	"os"
	"path/filepath"
	"testing"
)

type fixture struct {
	cfg          Config
	settingsPath string
	rootDir      string
}

func newFixture(t *testing.T, spriteName string) fixture {
	t.Helper()

	rootDir := t.TempDir()
	writeFixtureFile(t, filepath.Join(rootDir, "base", "CLAUDE.md"), "# CLAUDE")
	writeFixtureFile(t, filepath.Join(rootDir, "base", "hooks", "hook.sh"), "echo hook")
	writeFixtureFile(t, filepath.Join(rootDir, "base", "skills", "skill.md"), "# skill")
	writeFixtureFile(t, filepath.Join(rootDir, "base", "commands", "command.md"), "# command")
	writeFixtureFile(t, filepath.Join(rootDir, "base", "settings.json"), `{"env":{"EXISTING":"1"}}`)
	writeFixtureFile(t, filepath.Join(rootDir, "sprites", spriteName+".md"), "# persona")
	writeFixtureFile(t, filepath.Join(rootDir, "scripts", "sprite-bootstrap.sh"), "#!/bin/bash\necho boot\n")
	writeFixtureFile(t, filepath.Join(rootDir, "scripts", "sprite-agent.sh"), "#!/bin/bash\necho agent\n")

	cfg := Config{
		Org:        "misty-step",
		RemoteHome: "/home/sprite",
		Workspace:  "/home/sprite/workspace",
		BaseDir:    filepath.Join(rootDir, "base"),
		SpritesDir: filepath.Join(rootDir, "sprites"),
		RootDir:    rootDir,
	}

	return fixture{
		cfg:          cfg,
		settingsPath: filepath.Join(rootDir, "base", "settings.json"),
		rootDir:      rootDir,
	}
}

func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
