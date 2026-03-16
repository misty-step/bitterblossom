package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type preflightOptions struct {
	RepoRoot  string
	EnvFile   string
	FleetFile string
}

type localCommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type preflightSpriteAuth struct {
	Token  string
	Source string
}

type preflightWorkerProbe struct {
	Name      string
	Reachable bool
	GHAuth    bool
	Detail    string
}

type preflightDeps struct {
	getenv            func(string) string
	getwd             func() (string, error)
	readFile          func(string) ([]byte, error)
	runCommand        func(context.Context, string, string, ...string) (localCommandResult, error)
	mkdirAll          func(string, os.FileMode) error
	writeFile         func(string, []byte, os.FileMode) error
	remove            func(string) error
	resolveSpriteAuth func(context.Context) (preflightSpriteAuth, error)
	probeWorkers      func(context.Context, string, []string) ([]preflightWorkerProbe, error)
}

type preflightStatus string

const (
	preflightPass preflightStatus = "PASS"
	preflightWarn preflightStatus = "WARN"
	preflightFail preflightStatus = "FAIL"
)

type preflightCheckResult struct {
	Status   preflightStatus
	Label    string
	Detail   string
	FixHint  string
	Critical bool
}

type preflightRunner struct {
	opts        preflightOptions
	deps        preflightDeps
	spriteToken string
}

func newPreflightCmd() *cobra.Command {
	opts := preflightOptions{}
	cmd := &cobra.Command{
		Use:   "preflight",
		Short: "Validate local Bitterblossom control-plane readiness",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPreflight(cmd.Context(), opts, preflightDeps{}, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&opts.RepoRoot, "repo-root", "", "Repo root to inspect (defaults to auto-detected root)")
	cmd.Flags().StringVar(&opts.EnvFile, "env-file", "", "Env file to inspect (defaults to .env.bb in the repo root)")
	cmd.Flags().StringVar(&opts.FleetFile, "fleet", "", "Fleet file to inspect (defaults to fleet.toml in the repo root)")

	return cmd
}

func runPreflight(ctx context.Context, opts preflightOptions, deps preflightDeps, out io.Writer) error {
	deps = withDefaultPreflightDeps(deps)

	repoRoot := opts.RepoRoot
	if repoRoot == "" {
		cwd, err := deps.getwd()
		if err != nil {
			return err
		}
		repoRoot, err = findRepoRoot(cwd)
		if err != nil {
			return err
		}
	}

	runner := &preflightRunner{
		opts: preflightOptions{
			RepoRoot:  repoRoot,
			EnvFile:   resolveRepoPath(repoRoot, firstNonEmpty(opts.EnvFile, ".env.bb")),
			FleetFile: resolveRepoPath(repoRoot, firstNonEmpty(opts.FleetFile, "fleet.toml")),
		},
		deps: deps,
	}

	results := []preflightCheckResult{
		runner.checkElixirRuntime(ctx),
		runner.checkCLI(ctx, "gh", "gh CLI installed", true, "Install GitHub CLI, then verify `gh --version` works."),
		runner.checkCLI(ctx, "sprite", "sprite CLI installed", true, "Install the `sprite` CLI and confirm it is on PATH."),
		runner.checkCLI(ctx, "fly", "fly CLI installed", false, "Install `flyctl` if you want fleet and token-management tooling on this machine."),
		runner.checkEnvFile(),
		runner.checkGitHubToken(),
		runner.checkSpriteAuth(ctx),
		runner.checkWorkers(ctx),
		runner.checkConductorCompile(ctx),
		runner.checkDBWritable(),
	}

	failures := 0
	warnings := 0
	for _, result := range results {
		if _, err := fmt.Fprintf(out, "%s %s", result.Status, result.Label); err != nil {
			return err
		}
		if result.Detail != "" {
			if _, err := fmt.Fprintf(out, ": %s", result.Detail); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
		if result.Status != preflightPass && result.FixHint != "" {
			if _, err := fmt.Fprintf(out, "  fix: %s\n", result.FixHint); err != nil {
				return err
			}
		}
		if result.Status == preflightFail && result.Critical {
			failures++
		}
		if result.Status == preflightWarn {
			warnings++
		}
	}

	if failures > 0 {
		if _, err := fmt.Fprintf(out, "Preflight failed: %d critical failure(s), %d warning(s).\n", failures, warnings); err != nil {
			return err
		}
		return &exitError{Code: 1}
	}

	_, err := fmt.Fprintf(out, "Preflight passed: %d warning(s).\n", warnings)
	return err
}

func withDefaultPreflightDeps(deps preflightDeps) preflightDeps {
	if deps.getenv == nil {
		deps.getenv = os.Getenv
	}
	if deps.getwd == nil {
		deps.getwd = os.Getwd
	}
	if deps.readFile == nil {
		deps.readFile = os.ReadFile
	}
	if deps.runCommand == nil {
		deps.runCommand = defaultRunLocalCommand
	}
	if deps.mkdirAll == nil {
		deps.mkdirAll = os.MkdirAll
	}
	if deps.writeFile == nil {
		deps.writeFile = os.WriteFile
	}
	if deps.remove == nil {
		deps.remove = os.Remove
	}
	if deps.resolveSpriteAuth == nil {
		deps.resolveSpriteAuth = func(context.Context) (preflightSpriteAuth, error) {
			token, source, err := resolveSpriteToken(io.Discard)
			if err != nil {
				return preflightSpriteAuth{}, err
			}
			return preflightSpriteAuth{Token: token, Source: source}, nil
		}
	}
	if deps.probeWorkers == nil {
		deps.probeWorkers = defaultProbeWorkers
	}
	return deps
}

func defaultRunLocalCommand(ctx context.Context, dir, name string, args ...string) (localCommandResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := localCommandResult{
		Stdout: strings.TrimSpace(stdout.String()),
		Stderr: strings.TrimSpace(stderr.String()),
	}

	if err == nil {
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}

	return localCommandResult{}, err
}

func defaultProbeWorkers(ctx context.Context, token string, names []string) ([]preflightWorkerProbe, error) {
	client := newSpritesClient(token, spriteClientOptions{disableControl: true})
	defer func() { _ = client.Close() }()

	results := make([]preflightWorkerProbe, 0, len(names))
	for _, name := range names {
		sprite := client.Sprite(name)
		probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := probeSprite(probeCtx, sprite, name, 10*time.Second)
		cancel()
		if err != nil {
			results = append(results, preflightWorkerProbe{
				Name:      name,
				Reachable: false,
				GHAuth:    false,
				Detail:    err.Error(),
			})
			continue
		}

		authCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		authState, authErr := ghAuthStateWithRunner(authCtx, spriteBashRunner(sprite))
		cancel()
		if authErr != nil {
			results = append(results, preflightWorkerProbe{
				Name:      name,
				Reachable: true,
				GHAuth:    false,
				Detail:    fmt.Sprintf("gh auth check failed: %v", authErr),
			})
			continue
		}

		results = append(results, preflightWorkerProbe{
			Name:      name,
			Reachable: true,
			GHAuth:    authState == "ok",
			Detail:    fmt.Sprintf("gh auth %s", authState),
		})
	}

	return results, nil
}

func (r *preflightRunner) checkElixirRuntime(ctx context.Context) preflightCheckResult {
	elixirRes, err := r.deps.runCommand(ctx, "", "elixir", "--version")
	if err != nil {
		return preflightCheckResult{
			Status:   preflightFail,
			Label:    "Elixir + Erlang",
			Detail:   err.Error(),
			FixHint:  "Install Elixir 1.16+ (with Erlang/OTP) so `elixir`, `erl`, and `mix` are all available.",
			Critical: true,
		}
	}
	if elixirRes.ExitCode != 0 {
		return preflightCheckResult{
			Status:   preflightFail,
			Label:    "Elixir + Erlang",
			Detail:   summarizeCommandFailure(elixirRes),
			FixHint:  "Install Elixir 1.16+ (with Erlang/OTP) so `elixir`, `erl`, and `mix` are all available.",
			Critical: true,
		}
	}

	erlRes, err := r.deps.runCommand(ctx, "", "erl", "-noshell", "-eval", "erlang:display(erlang:system_info(otp_release)), halt().")
	if err != nil || erlRes.ExitCode != 0 {
		detail := errString(err)
		if detail == "" {
			detail = summarizeCommandFailure(erlRes)
		}
		return preflightCheckResult{
			Status:   preflightFail,
			Label:    "Elixir + Erlang",
			Detail:   detail,
			FixHint:  "Install Erlang/OTP alongside Elixir so `erl` starts cleanly.",
			Critical: true,
		}
	}

	mixRes, err := r.deps.runCommand(ctx, "", "mix", "--version")
	if err != nil || mixRes.ExitCode != 0 {
		detail := errString(err)
		if detail == "" {
			detail = summarizeCommandFailure(mixRes)
		}
		return preflightCheckResult{
			Status:   preflightFail,
			Label:    "Elixir + Erlang",
			Detail:   detail,
			FixHint:  "Install Elixir 1.16+ and verify `mix --version` succeeds.",
			Critical: true,
		}
	}

	elixirLine := findLineContaining(elixirRes.Stdout, "Elixir ")
	if elixirLine == "" {
		elixirLine = firstNonEmptyLine(elixirRes.Stdout)
	}
	otpRelease := strings.Trim(firstNonEmptyLine(erlRes.Stdout), "\"")
	detail := elixirLine
	if detail != "" && otpRelease != "" {
		detail = fmt.Sprintf("%s / OTP %s", detail, otpRelease)
	}
	return preflightCheckResult{
		Status:   preflightPass,
		Label:    "Elixir + Erlang",
		Detail:   detail,
		Critical: true,
	}
}

func (r *preflightRunner) checkCLI(ctx context.Context, binary, label string, critical bool, fixHint string) preflightCheckResult {
	res, err := r.deps.runCommand(ctx, "", binary, "--version")
	if err != nil {
		return preflightCheckResult{
			Status:   choosePreflightFailureStatus(critical),
			Label:    label,
			Detail:   err.Error(),
			FixHint:  fixHint,
			Critical: critical,
		}
	}
	if res.ExitCode != 0 {
		return preflightCheckResult{
			Status:   choosePreflightFailureStatus(critical),
			Label:    label,
			Detail:   summarizeCommandFailure(res),
			FixHint:  fixHint,
			Critical: critical,
		}
	}
	return preflightCheckResult{
		Status:   preflightPass,
		Label:    label,
		Detail:   firstNonEmptyLine(res.Stdout),
		Critical: critical,
	}
}

func (r *preflightRunner) checkEnvFile() preflightCheckResult {
	data, err := r.deps.readFile(r.opts.EnvFile)
	if err != nil {
		return preflightCheckResult{
			Status:   preflightFail,
			Label:    ".env.bb exports",
			Detail:   err.Error(),
			FixHint:  "Generate it with `./scripts/onboard.sh --write .env.bb`, then `source .env.bb`.",
			Critical: true,
		}
	}

	exports := parseExportedEnv(string(data))
	orgVars := exportedVars(exports, "SPRITES_ORG", "FLY_ORG")
	if len(orgVars) == 0 {
		return preflightCheckResult{
			Status:   preflightFail,
			Label:    ".env.bb exports",
			Detail:   "missing SPRITES_ORG/FLY_ORG export",
			FixHint:  "Add `export SPRITES_ORG=...` or `export FLY_ORG=...` to .env.bb.",
			Critical: true,
		}
	}

	return preflightCheckResult{
		Status:   preflightPass,
		Label:    ".env.bb exports",
		Detail:   fmt.Sprintf("found %s", strings.Join(orgVars, ", ")),
		Critical: true,
	}
}

func (r *preflightRunner) checkGitHubToken() preflightCheckResult {
	if strings.TrimSpace(r.deps.getenv("GITHUB_TOKEN")) == "" {
		return preflightCheckResult{
			Status:   preflightFail,
			Label:    "GITHUB_TOKEN set",
			Detail:   "missing from environment",
			FixHint:  `export GITHUB_TOKEN="$(gh auth token)"`,
			Critical: true,
		}
	}
	return preflightCheckResult{
		Status:   preflightPass,
		Label:    "GITHUB_TOKEN set",
		Detail:   "present in environment",
		Critical: true,
	}
}

func (r *preflightRunner) checkSpriteAuth(ctx context.Context) preflightCheckResult {
	auth, err := r.deps.resolveSpriteAuth(ctx)
	if err != nil {
		return preflightCheckResult{
			Status:   preflightFail,
			Label:    "sprite auth usable",
			Detail:   err.Error(),
			FixHint:  "Set SPRITE_TOKEN, set a fresh FLY_API_TOKEN, or log in with `sprite auth login`.",
			Critical: true,
		}
	}

	r.spriteToken = auth.Token
	return preflightCheckResult{
		Status:   preflightPass,
		Label:    "sprite auth usable",
		Detail:   auth.Source,
		Critical: true,
	}
}

func (r *preflightRunner) checkWorkers(ctx context.Context) preflightCheckResult {
	data, err := r.deps.readFile(r.opts.FleetFile)
	if err != nil {
		return preflightCheckResult{
			Status:   preflightFail,
			Label:    "worker reachability + GH auth",
			Detail:   err.Error(),
			FixHint:  "Create `fleet.toml` with at least one worker sprite entry or pass `--fleet`.",
			Critical: true,
		}
	}

	names, err := parseFleetSpriteNames(string(data))
	if err != nil {
		return preflightCheckResult{
			Status:   preflightFail,
			Label:    "worker reachability + GH auth",
			Detail:   err.Error(),
			FixHint:  "Add at least one `[[sprite]] name = \"...\"` entry to fleet.toml.",
			Critical: true,
		}
	}

	if r.spriteToken == "" {
		auth, authErr := r.deps.resolveSpriteAuth(ctx)
		if authErr != nil {
			return preflightCheckResult{
				Status:   preflightFail,
				Label:    "worker reachability + GH auth",
				Detail:   authErr.Error(),
				FixHint:  "Repair local sprite auth before probing workers.",
				Critical: true,
			}
		}
		r.spriteToken = auth.Token
	}

	probes, err := r.deps.probeWorkers(ctx, r.spriteToken, names)
	if err != nil {
		return preflightCheckResult{
			Status:   preflightFail,
			Label:    "worker reachability + GH auth",
			Detail:   err.Error(),
			FixHint:  "Repair sprite connectivity or credentials, then rerun `bb preflight`.",
			Critical: true,
		}
	}

	summaries := make([]string, 0, len(probes))
	for _, probe := range probes {
		switch {
		case probe.Reachable && probe.GHAuth:
			summaries = append(summaries, fmt.Sprintf("%s ok", probe.Name))
			return preflightCheckResult{
				Status:   preflightPass,
				Label:    "worker reachability + GH auth",
				Detail:   strings.Join(summaries, ", "),
				Critical: true,
			}
		case probe.Reachable:
			summaries = append(summaries, fmt.Sprintf("%s reachable, gh auth missing", probe.Name))
		default:
			summaries = append(summaries, fmt.Sprintf("%s unreachable", probe.Name))
		}
	}

	return preflightCheckResult{
		Status:   preflightFail,
		Label:    "worker reachability + GH auth",
		Detail:   strings.Join(summaries, "; "),
		FixHint:  "Run `bb setup <worker> --repo <owner/repo>` or repair worker GitHub auth and reachability.",
		Critical: true,
	}
}

func (r *preflightRunner) checkConductorCompile(ctx context.Context) preflightCheckResult {
	conductorDir := filepath.Join(r.opts.RepoRoot, "conductor")
	res, err := r.deps.runCommand(ctx, conductorDir, "mix", "compile")
	if err != nil {
		return preflightCheckResult{
			Status:   preflightFail,
			Label:    "conductor compile",
			Detail:   err.Error(),
			FixHint:  "Install Elixir deps with `cd conductor && mix deps.get`, then rerun `mix compile`.",
			Critical: true,
		}
	}
	if res.ExitCode != 0 {
		return preflightCheckResult{
			Status:   preflightFail,
			Label:    "conductor compile",
			Detail:   summarizeCommandFailure(res),
			FixHint:  "Run `cd conductor && mix deps.get && mix compile` and fix any compile errors.",
			Critical: true,
		}
	}
	return preflightCheckResult{
		Status:   preflightPass,
		Label:    "conductor compile",
		Detail:   firstNonEmptyLine(firstNonEmpty(res.Stdout, res.Stderr)),
		Critical: true,
	}
}

func (r *preflightRunner) checkDBWritable() preflightCheckResult {
	dbDir := filepath.Join(r.opts.RepoRoot, ".bb")
	if err := r.deps.mkdirAll(dbDir, 0o755); err != nil {
		return preflightCheckResult{
			Status:   preflightFail,
			Label:    "conductor DB writable",
			Detail:   err.Error(),
			FixHint:  "Create `.bb/` and ensure the current user can write there.",
			Critical: true,
		}
	}

	probePath := filepath.Join(dbDir, fmt.Sprintf(".preflight-write-%d", time.Now().UnixNano()))
	if err := r.deps.writeFile(probePath, []byte("ok\n"), 0o644); err != nil {
		return preflightCheckResult{
			Status:   preflightFail,
			Label:    "conductor DB writable",
			Detail:   err.Error(),
			FixHint:  "Fix permissions for `.bb/` so Bitterblossom can write `.bb/conductor.db`.",
			Critical: true,
		}
	}
	_ = r.deps.remove(probePath)

	return preflightCheckResult{
		Status:   preflightPass,
		Label:    "conductor DB writable",
		Detail:   filepath.Join(".bb", "conductor.db"),
		Critical: true,
	}
}

func parseExportedEnv(content string) map[string]string {
	exports := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "export ") {
			continue
		}
		body := strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, ok := strings.Cut(body, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch {
		case len(value) >= 2 && strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\""):
			value = strings.Trim(value, "\"")
		case len(value) >= 2 && strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'"):
			value = strings.Trim(value, "'")
		}
		exports[key] = value
	}
	return exports
}

func parseFleetSpriteNames(content string) ([]string, error) {
	var names []string
	inSprite := false
	currentName := ""

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(stripInlineComment(scanner.Text()))
		if line == "" {
			continue
		}

		switch {
		case line == "[[sprite]]":
			if inSprite && currentName != "" {
				names = append(names, currentName)
			}
			inSprite = true
			currentName = ""
		case strings.HasPrefix(line, "[[") || (strings.HasPrefix(line, "[") && line != "[[sprite]]"):
			if inSprite && currentName != "" {
				names = append(names, currentName)
			}
			inSprite = false
			currentName = ""
		case inSprite && strings.HasPrefix(line, "name"):
			if name, ok := parseQuotedAssignment(line, "name"); ok {
				currentName = name
			}
		}
	}
	if inSprite && currentName != "" {
		names = append(names, currentName)
	}

	if len(names) == 0 {
		return nil, errors.New("no sprite names found in fleet.toml")
	}

	return names, nil
}

func findRepoRoot(start string) (string, error) {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, "conductor", "mix.exs")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not find repo root containing conductor/mix.exs")
		}
		dir = parent
	}
}

func resolveRepoPath(repoRoot, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(repoRoot, path)
}

func choosePreflightFailureStatus(critical bool) preflightStatus {
	if critical {
		return preflightFail
	}
	return preflightWarn
}

func summarizeCommandFailure(res localCommandResult) string {
	if res.Stderr != "" {
		return firstNonEmptyLine(res.Stderr)
	}
	if res.Stdout != "" {
		return firstNonEmptyLine(res.Stdout)
	}
	return fmt.Sprintf("exit code %d", res.ExitCode)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptyLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func findLineContaining(value, needle string) string {
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func stripInlineComment(line string) string {
	var b strings.Builder
	inSingle := false
	inDouble := false
	for i, r := range line {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return line[:i]
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}

func parseQuotedAssignment(line, key string) (string, bool) {
	left, right, ok := strings.Cut(line, "=")
	if !ok || strings.TrimSpace(left) != key {
		return "", false
	}
	value := strings.TrimSpace(right)
	if len(value) < 2 || !strings.HasPrefix(value, "\"") {
		return "", false
	}
	end := strings.LastIndex(value, "\"")
	if end <= 0 {
		return "", false
	}
	return value[1:end], true
}

func exportedVars(exports map[string]string, keys ...string) []string {
	var present []string
	for _, key := range keys {
		if strings.TrimSpace(exports[key]) != "" {
			present = append(present, key)
		}
	}
	sort.Strings(present)
	return present
}
