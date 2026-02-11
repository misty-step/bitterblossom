package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/internal/contracts"
	"github.com/misty-step/bitterblossom/internal/lifecycle"
	"github.com/misty-step/bitterblossom/internal/names"
	"github.com/misty-step/bitterblossom/internal/registry"
	"github.com/misty-step/bitterblossom/internal/sprite"
	"github.com/spf13/cobra"
)

type addOptions struct {
	Count       int
	Name        string
	Org         string
	SpriteCLI   string
	Composition string
	Timeout     time.Duration
}

type addDeps struct {
	getwd              func() (string, error)
	getenv             func(string) string
	newCLI             func(binary, org string) sprite.SpriteCLI
	resolveGitHubAuth  func(spriteName string, getenv func(string) string) (lifecycle.GitHubAuth, error)
	renderSettings     func(settingsPath, authToken string) (string, error)
	provision          func(ctx context.Context, cli sprite.SpriteCLI, cfg lifecycle.Config, opts lifecycle.ProvisionOpts) (lifecycle.ProvisionResult, error)
	registryPath       func() string
}

func defaultAddDeps() addDeps {
	return addDeps{
		getwd:  os.Getwd,
		getenv: os.Getenv,
		newCLI: func(binary, org string) sprite.SpriteCLI {
			return sprite.NewCLIWithOrg(binary, org)
		},
		resolveGitHubAuth: lifecycle.ResolveGitHubAuth,
		renderSettings:    lifecycle.RenderSettings,
		provision:         lifecycle.Provision,
		registryPath:      registry.DefaultPath,
	}
}

func newAddCmd() *cobra.Command {
	return newAddCmdWithDeps(defaultAddDeps())
}

func newAddCmdWithDeps(deps addDeps) *cobra.Command {
	opts := addOptions{
		Count:       1,
		Org:         defaultOrg(),
		SpriteCLI:   defaultSpriteCLIPath(),
		Composition: defaultLifecycleComposition,
		Timeout:     30 * time.Minute,
	}

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add sprites to the fleet",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAdd(cmd, opts, deps)
		},
	}

	cmd.Flags().IntVar(&opts.Count, "count", opts.Count, "Number of sprites to add")
	cmd.Flags().StringVar(&opts.Name, "name", "", "Custom sprite name (mutually exclusive with --count > 1)")
	cmd.Flags().StringVar(&opts.Org, "org", opts.Org, "Sprites organization")
	cmd.Flags().StringVar(&opts.SpriteCLI, "sprite-cli", opts.SpriteCLI, "Path to sprite CLI")
	cmd.Flags().StringVar(&opts.Composition, "composition", opts.Composition, "Path to composition YAML")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", opts.Timeout, "Command timeout")

	return cmd
}

func runAdd(cmd *cobra.Command, opts addOptions, deps addDeps) error {
	if opts.Name != "" && opts.Count > 1 {
		return errors.New("cannot use --name with --count > 1")
	}
	if opts.Count < 1 {
		return errors.New("--count must be at least 1")
	}

	regPath := deps.registryPath()
	reg, err := registry.Load(regPath)
	if err != nil {
		return fmt.Errorf("load registry: %w", err)
	}

	// Pick names
	spritesToAdd, err := pickNewNames(reg, opts.Name, opts.Count)
	if err != nil {
		return err
	}

	rootDir, err := deps.getwd()
	if err != nil {
		return err
	}
	cfg := defaultLifecycleConfig(rootDir, opts.Org)

	settingsPath := filepath.Join(cfg.BaseDir, "settings.json")
	authToken := resolveLifecycleAuthToken(deps.getenv)
	if authToken == "" {
		return errors.New("add: OPENROUTER_API_KEY is required (ANTHROPIC_AUTH_TOKEN is accepted as a legacy fallback)")
	}
	renderedSettings, err := deps.renderSettings(settingsPath, authToken)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(renderedSettings) }()

	runCtx, cancel := context.WithTimeout(cmd.Context(), opts.Timeout)
	defer cancel()

	cli := deps.newCLI(opts.SpriteCLI, opts.Org)
	compositionLabel := strings.TrimSuffix(filepath.Base(opts.Composition), filepath.Ext(opts.Composition))
	stderr := cmd.ErrOrStderr()

	_, _ = fmt.Fprintf(stderr, "Provisioning %d sprite(s)...\n", len(spritesToAdd))

	results := make([]addResult, 0, len(spritesToAdd))
	for _, name := range spritesToAdd {
		auth, err := deps.resolveGitHubAuth(name, deps.getenv)
		if err != nil {
			return err
		}

		_, _ = fmt.Fprintf(stderr, "  %s ", name)
		provResult, err := deps.provision(runCtx, cli, cfg, lifecycle.ProvisionOpts{
			Name:             name,
			CompositionLabel: compositionLabel,
			SettingsPath:     renderedSettings,
			GitHubAuth:       auth,
			BootstrapScript:  filepath.Join(cfg.RootDir, "scripts", "sprite-bootstrap.sh"),
			AgentScript:      filepath.Join(cfg.RootDir, "scripts", "sprite-agent.sh"),
		})
		if err != nil {
			writeStatus(stderr, false)
			return fmt.Errorf("provision %q: %w", name, err)
		}
		writeStatus(stderr, true)
		results = append(results, addResult{Name: name, Created: provResult.Created})
	}

	// Register all new sprites atomically
	if err := registry.WithLockedRegistry(regPath, func(reg *registry.Registry) error {
		for _, r := range results {
			reg.Register(r.Name, "")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("update registry: %w", err)
	}

	_, _ = fmt.Fprintf(stderr, "\nRegistry updated (%d sprites total).\n", reg.Count()+len(results))

	return contracts.WriteJSON(cmd.OutOrStdout(), "add", map[string]any{
		"added": results,
		"total": reg.Count() + len(results),
	})
}

type addResult struct {
	Name    string `json:"name"`
	Created bool   `json:"created"`
}

func pickNewNames(reg *registry.Registry, customName string, count int) ([]string, error) {
	if customName != "" {
		name := strings.TrimSpace(customName)
		if _, exists := reg.LookupMachine(name); exists {
			return nil, fmt.Errorf("sprite %q already exists in registry", name)
		}
		return []string{name}, nil
	}

	existing := make(map[string]struct{}, reg.Count())
	for _, name := range reg.Names() {
		existing[name] = struct{}{}
	}

	picked := make([]string, 0, count)
	// Scan through the name pool to find available names
	for i := 0; len(picked) < count; i++ {
		candidate := names.PickName(i)
		if _, taken := existing[candidate]; !taken {
			picked = append(picked, candidate)
			existing[candidate] = struct{}{}
		}
		// Safety: don't loop forever if pool is exhausted
		if i > names.Count()*10 {
			return nil, fmt.Errorf("cannot find %d available names (pool exhausted)", count)
		}
	}
	return picked, nil
}

func writeStatus(w io.Writer, ok bool) {
	if ok {
		_, _ = fmt.Fprintln(w, "ok")
	} else {
		_, _ = fmt.Fprintln(w, "FAILED")
	}
}
