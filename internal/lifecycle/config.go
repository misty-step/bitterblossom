package lifecycle

// Config holds shared lifecycle configuration.
type Config struct {
	Org        string // Sprites org (default "misty-step")
	RemoteHome string // /home/sprite
	Workspace  string // /home/sprite/workspace
	BaseDir    string // local path to base/ directory
	SpritesDir string // local path to sprites/ directory
	RootDir    string // local repo root
}
