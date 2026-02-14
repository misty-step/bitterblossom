package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/misty-step/bitterblossom/internal/fleet"
	"github.com/misty-step/bitterblossom/internal/lifecycle"
	"github.com/misty-step/bitterblossom/internal/provider"
)

const (
	defaultLifecycleComposition = "compositions/v1.yaml"
	defaultLifecycleOrg         = "misty-step"
	defaultSpriteCLIBinary      = "sprite"
)

func defaultOrg() string {
	if value := strings.TrimSpace(os.Getenv("FLY_ORG")); value != "" {
		return value
	}
	return defaultLifecycleOrg
}

func defaultSpriteCLIPath() string {
	if value := strings.TrimSpace(os.Getenv("SPRITE_CLI")); value != "" {
		return value
	}
	return defaultSpriteCLIBinary
}

func defaultLifecycleConfig(rootDir, org string) lifecycle.Config {
	return lifecycle.Config{
		Org:        strings.TrimSpace(org),
		RemoteHome: "/home/sprite",
		Workspace:  "/home/sprite/workspace",
		BaseDir:    filepath.Join(rootDir, "base"),
		SpritesDir: filepath.Join(rootDir, "sprites"),
		RootDir:    rootDir,
	}
}

func resolveCompositionSprites(path string) ([]string, error) {
	composition, err := fleet.ParseComposition(path)
	if err != nil {
		return nil, err
	}
	if len(composition.Sprites) == 0 {
		return nil, fmt.Errorf("no sprites found in composition %q", path)
	}
	names := make([]string, 0, len(composition.Sprites))
	for _, sprite := range composition.Sprites {
		names = append(names, sprite.Name)
	}
	return names, nil
}

func resolveLifecycleAuthToken(getenv func(string) string) string {
	return provider.ResolveAuthToken(provider.DefaultProvider, getenv)
}

func defaultFlyToken() string {
	if token := strings.TrimSpace(os.Getenv("FLY_API_TOKEN")); token != "" {
		return token
	}
	return strings.TrimSpace(os.Getenv("FLY_TOKEN"))
}

// resolveFlyToken returns the token from the flag if set, otherwise from environment.
func resolveFlyToken(flagToken string) string {
	if token := strings.TrimSpace(flagToken); token != "" {
		return token
	}
	return defaultFlyToken()
}
