package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseExportedEnv(t *testing.T) {
	t.Parallel()

	exports := parseExportedEnv(`
# comment
export FLY_ORG=misty-step
 export SPRITES_ORG="sprites-org"
export EMPTY=''
NOT_EXPORTED=value
export OPENROUTER_API_KEY="${TOKEN}"
`)

	if exports["FLY_ORG"] != "misty-step" {
		t.Fatalf("FLY_ORG = %q, want %q", exports["FLY_ORG"], "misty-step")
	}
	if exports["SPRITES_ORG"] != "sprites-org" {
		t.Fatalf("SPRITES_ORG = %q, want %q", exports["SPRITES_ORG"], "sprites-org")
	}
	if exports["EMPTY"] != "" {
		t.Fatalf("EMPTY = %q, want empty string", exports["EMPTY"])
	}
	if exports["OPENROUTER_API_KEY"] != "${TOKEN}" {
		t.Fatalf("OPENROUTER_API_KEY = %q, want %q", exports["OPENROUTER_API_KEY"], "${TOKEN}")
	}
	if _, ok := exports["NOT_EXPORTED"]; ok {
		t.Fatalf("NOT_EXPORTED should be ignored, got %+v", exports)
	}
}

func TestParseFleetSpriteNames(t *testing.T) {
	t.Parallel()

	names, err := parseFleetSpriteNames(`
[defaults]
repo = "misty-step/bitterblossom"

[[sprite]]
name = "bb-builder"
role = "builder"

[[sprite]]
role = "fixer"
name = "bb-fixer"
`)
	if err != nil {
		t.Fatalf("parseFleetSpriteNames() error = %v", err)
	}

	want := []string{"bb-builder", "bb-fixer"}
	if len(names) != len(want) {
		t.Fatalf("len(names) = %d, want %d (%v)", len(names), len(want), names)
	}
	for i, name := range want {
		if names[i] != name {
			t.Fatalf("names[%d] = %q, want %q", i, names[i], name)
		}
	}
}

func TestParseFleetSpriteNamesHandlesQuotedHashesAndEscapes(t *testing.T) {
	t.Parallel()

	names, err := parseFleetSpriteNames(`
[[sprite]]
name = "bb-\"builder\" #1" # comment
`)
	if err != nil {
		t.Fatalf("parseFleetSpriteNames() error = %v", err)
	}
	if len(names) != 1 {
		t.Fatalf("len(names) = %d, want 1", len(names))
	}
	if names[0] != `bb-"builder" #1` {
		t.Fatalf("names[0] = %q, want %q", names[0], `bb-"builder" #1`)
	}
}

func TestParseFleetSpriteNamesFailsWithoutSprites(t *testing.T) {
	t.Parallel()

	_, err := parseFleetSpriteNames("[defaults]\nrepo = \"misty-step/bitterblossom\"\n")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFindRepoRootAscends(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "conductor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "nested", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "conductor", "mix.exs"), []byte("mix"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := findRepoRoot(filepath.Join(root, "nested", "deep"))
	if err != nil {
		t.Fatalf("findRepoRoot() error = %v", err)
	}
	if found != root {
		t.Fatalf("repo root = %q, want %q", found, root)
	}
}

func TestRunPreflightReturnsExitErrorForCriticalFailures(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "conductor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "conductor", "mix.exs"), []byte("mix"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	deps := preflightDeps{
		getenv: func(name string) string {
			if name == "GITHUB_TOKEN" {
				return ""
			}
			return ""
		},
		getwd: func() (string, error) { return repoRoot, nil },
		readFile: func(path string) ([]byte, error) {
			switch filepath.Base(path) {
			case ".env.bb":
				return []byte("export FLY_ORG=misty-step\n"), nil
			case "fleet.toml":
				return []byte("[[sprite]]\nname = \"bb-builder\"\n"), nil
			default:
				return nil, os.ErrNotExist
			}
		},
		runCommand: func(_ context.Context, dir, name string, args ...string) (localCommandResult, error) {
			switch {
			case name == "elixir":
				return localCommandResult{Stdout: "Erlang/OTP 27\nElixir 1.16.2\n", ExitCode: 0}, nil
			case name == "erl":
				return localCommandResult{Stdout: "27\n", ExitCode: 0}, nil
			case name == "mix" && len(args) == 1 && args[0] == "--version":
				return localCommandResult{Stdout: "Erlang/OTP 27\nMix 1.16.2\n", ExitCode: 0}, nil
			case name == "gh" && len(args) == 1 && args[0] == "--version":
				return localCommandResult{Stdout: "gh version 2.50.0\n", ExitCode: 0}, nil
			case name == "sprite" && len(args) == 1 && args[0] == "--version":
				return localCommandResult{Stdout: "sprite 1.2.3\n", ExitCode: 0}, nil
			case name == "mix" && len(args) == 1 && args[0] == "compile":
				if dir != filepath.Join(repoRoot, "conductor") {
					t.Fatalf("mix compile dir = %q, want %q", dir, filepath.Join(repoRoot, "conductor"))
				}
				return localCommandResult{Stdout: "Compiled conductor\n", ExitCode: 0}, nil
			default:
				return localCommandResult{}, errors.New("unexpected command")
			}
		},
		mkdirAll:  os.MkdirAll,
		writeFile: os.WriteFile,
		remove:    os.Remove,
		resolveSpriteAuth: func(_ context.Context, _ func(string) string) (preflightSpriteAuth, error) {
			return preflightSpriteAuth{Token: "sprite-token", Source: "SPRITE_TOKEN"}, nil
		},
		probeWorkers: func(_ context.Context, token string, names []string) ([]preflightWorkerProbe, error) {
			if token != "sprite-token" {
				t.Fatalf("probe token = %q, want sprite-token", token)
			}
			if len(names) != 1 || names[0] != "bb-builder" {
				t.Fatalf("probe names = %v, want [bb-builder]", names)
			}
			return []preflightWorkerProbe{
				{Name: "bb-builder", Reachable: true, GHAuth: true, Detail: "reachable"},
			}, nil
		},
	}

	err := runPreflight(context.Background(), preflightOptions{}, deps, &out)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var coded *exitError
	if !errors.As(err, &coded) {
		t.Fatalf("expected exitError, got %T", err)
	}
	if coded.Code != 1 {
		t.Fatalf("exit code = %d, want 1", coded.Code)
	}

	text := out.String()
	if !strings.Contains(text, "FAIL GITHUB_TOKEN set") {
		t.Fatalf("output = %q, want GITHUB_TOKEN failure", text)
	}
	if !strings.Contains(text, "fix: export GITHUB_TOKEN") {
		t.Fatalf("output = %q, want fix hint", text)
	}
	if !strings.Contains(text, "WARN fly CLI installed") {
		t.Fatalf("output = %q, want fly warning", text)
	}
}

func TestRunPreflightPassesWhenAllCriticalChecksPass(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "conductor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "conductor", "mix.exs"), []byte("mix"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	deps := preflightDeps{
		getenv: func(name string) string {
			if name == "GITHUB_TOKEN" {
				return "ghp_test"
			}
			return ""
		},
		getwd: func() (string, error) { return repoRoot, nil },
		readFile: func(path string) ([]byte, error) {
			switch filepath.Base(path) {
			case ".env.bb":
				return []byte("export FLY_ORG=misty-step\n"), nil
			case "fleet.toml":
				return []byte("[[sprite]]\nname = \"bb-builder\"\n"), nil
			default:
				return nil, os.ErrNotExist
			}
		},
		runCommand: func(_ context.Context, _ string, name string, args ...string) (localCommandResult, error) {
			switch {
			case name == "elixir":
				return localCommandResult{Stdout: "Elixir 1.16.2\n", ExitCode: 0}, nil
			case name == "erl":
				return localCommandResult{Stdout: "27\n", ExitCode: 0}, nil
			case name == "mix" && len(args) == 1 && args[0] == "--version":
				return localCommandResult{Stdout: "Mix 1.16.2\n", ExitCode: 0}, nil
			case name == "gh" && len(args) == 1 && args[0] == "--version":
				return localCommandResult{Stdout: "gh version 2.50.0\n", ExitCode: 0}, nil
			case name == "sprite" && len(args) == 1 && args[0] == "--version":
				return localCommandResult{Stdout: "sprite 1.2.3\n", ExitCode: 0}, nil
			case name == "fly" && len(args) == 1 && args[0] == "--version":
				return localCommandResult{Stdout: "flyctl v0.2.0\n", ExitCode: 0}, nil
			case name == "mix" && len(args) == 1 && args[0] == "compile":
				return localCommandResult{Stdout: "Compiled\n", ExitCode: 0}, nil
			default:
				return localCommandResult{}, errors.New("unexpected command")
			}
		},
		mkdirAll:  os.MkdirAll,
		writeFile: os.WriteFile,
		remove:    os.Remove,
		resolveSpriteAuth: func(_ context.Context, _ func(string) string) (preflightSpriteAuth, error) {
			return preflightSpriteAuth{Token: "sprite-token", Source: "SPRITE_TOKEN"}, nil
		},
		probeWorkers: func(_ context.Context, _ string, _ []string) ([]preflightWorkerProbe, error) {
			return []preflightWorkerProbe{
				{Name: "bb-builder", Reachable: true, GHAuth: true, Detail: "reachable"},
				{Name: "bb-fixer", Reachable: false, GHAuth: false, Detail: "unreachable"},
			}, nil
		},
	}

	if err := runPreflight(context.Background(), preflightOptions{}, deps, &out); err != nil {
		t.Fatalf("runPreflight() error = %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "PASS worker reachability + GH auth") {
		t.Fatalf("output = %q, want worker pass", text)
	}
	if !strings.Contains(text, "bb-builder ok, bb-fixer unreachable") {
		t.Fatalf("output = %q, want complete worker summary", text)
	}
	if !strings.Contains(text, "Preflight passed") {
		t.Fatalf("output = %q, want passing summary", text)
	}
}

func TestCheckElixirRuntimeFailures(t *testing.T) {
	tests := []struct {
		name       string
		runCommand func(context.Context, string, string, ...string) (localCommandResult, error)
		want       string
	}{
		{
			name: "elixir missing",
			runCommand: func(_ context.Context, _ string, name string, _ ...string) (localCommandResult, error) {
				if name == "elixir" {
					return localCommandResult{}, errors.New("exec: elixir not found")
				}
				return localCommandResult{}, nil
			},
			want: "exec: elixir not found",
		},
		{
			name: "erl exits nonzero",
			runCommand: func(_ context.Context, _ string, name string, _ ...string) (localCommandResult, error) {
				switch name {
				case "elixir":
					return localCommandResult{Stdout: "Elixir 1.16.2", ExitCode: 0}, nil
				case "erl":
					return localCommandResult{Stderr: "erl failed", ExitCode: 1}, nil
				default:
					return localCommandResult{}, nil
				}
			},
			want: "erl failed",
		},
		{
			name: "mix exits nonzero",
			runCommand: func(_ context.Context, _ string, name string, _ ...string) (localCommandResult, error) {
				switch name {
				case "elixir":
					return localCommandResult{Stdout: "Elixir 1.16.2", ExitCode: 0}, nil
				case "erl":
					return localCommandResult{Stdout: "27", ExitCode: 0}, nil
				case "mix":
					return localCommandResult{Stderr: "mix failed", ExitCode: 1}, nil
				default:
					return localCommandResult{}, nil
				}
			},
			want: "mix failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &preflightRunner{deps: withDefaultPreflightDeps(preflightDeps{runCommand: tt.runCommand})}
			result := runner.checkElixirRuntime(context.Background())
			if result.Status != preflightFail {
				t.Fatalf("status = %s, want FAIL", result.Status)
			}
			if !strings.Contains(result.Detail, tt.want) {
				t.Fatalf("detail = %q, want %q", result.Detail, tt.want)
			}
		})
	}
}

func TestCheckEnvFileFailures(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		runner := &preflightRunner{
			opts: preflightOptions{EnvFile: ".env.bb"},
			deps: withDefaultPreflightDeps(preflightDeps{
				readFile: func(string) ([]byte, error) { return nil, os.ErrNotExist },
			}),
		}
		result := runner.checkEnvFile()
		if result.Status != preflightFail {
			t.Fatalf("status = %s, want FAIL", result.Status)
		}
	})

	t.Run("missing org exports", func(t *testing.T) {
		runner := &preflightRunner{
			opts: preflightOptions{EnvFile: ".env.bb"},
			deps: withDefaultPreflightDeps(preflightDeps{
				readFile: func(string) ([]byte, error) { return []byte("export OPENROUTER_API_KEY=token\n"), nil },
			}),
		}
		result := runner.checkEnvFile()
		if result.Status != preflightFail {
			t.Fatalf("status = %s, want FAIL", result.Status)
		}
		if !strings.Contains(result.Detail, "SPRITES_ORG/FLY_ORG") {
			t.Fatalf("detail = %q, want org-export failure", result.Detail)
		}
	})
}

func TestCheckWorkersFailures(t *testing.T) {
	t.Run("missing fleet file", func(t *testing.T) {
		runner := &preflightRunner{
			opts: preflightOptions{FleetFile: "fleet.toml"},
			deps: withDefaultPreflightDeps(preflightDeps{
				readFile: func(string) ([]byte, error) { return nil, os.ErrNotExist },
			}),
		}
		result := runner.checkWorkers(context.Background())
		if result.Status != preflightFail {
			t.Fatalf("status = %s, want FAIL", result.Status)
		}
	})

	t.Run("no sprite names", func(t *testing.T) {
		runner := &preflightRunner{
			opts: preflightOptions{FleetFile: "fleet.toml"},
			deps: withDefaultPreflightDeps(preflightDeps{
				readFile: func(string) ([]byte, error) { return []byte("[defaults]\nrepo = \"misty-step/bitterblossom\"\n"), nil },
			}),
		}
		result := runner.checkWorkers(context.Background())
		if result.Status != preflightFail {
			t.Fatalf("status = %s, want FAIL", result.Status)
		}
	})

	t.Run("sprite auth error", func(t *testing.T) {
		runner := &preflightRunner{
			opts: preflightOptions{FleetFile: "fleet.toml"},
			deps: withDefaultPreflightDeps(preflightDeps{
				readFile: func(string) ([]byte, error) { return []byte("[[sprite]]\nname = \"bb-builder\"\n"), nil },
				resolveSpriteAuth: func(_ context.Context, _ func(string) string) (preflightSpriteAuth, error) {
					return preflightSpriteAuth{}, errors.New("auth failed")
				},
			}),
		}
		result := runner.checkWorkers(context.Background())
		if result.Status != preflightFail {
			t.Fatalf("status = %s, want FAIL", result.Status)
		}
	})

	t.Run("probe error", func(t *testing.T) {
		runner := &preflightRunner{
			opts:        preflightOptions{FleetFile: "fleet.toml"},
			spriteToken: "sprite-token",
			deps: withDefaultPreflightDeps(preflightDeps{
				readFile: func(string) ([]byte, error) { return []byte("[[sprite]]\nname = \"bb-builder\"\n"), nil },
				probeWorkers: func(context.Context, string, []string) ([]preflightWorkerProbe, error) {
					return nil, errors.New("probe failed")
				},
			}),
		}
		result := runner.checkWorkers(context.Background())
		if result.Status != preflightFail {
			t.Fatalf("status = %s, want FAIL", result.Status)
		}
	})

	t.Run("workers unhealthy", func(t *testing.T) {
		runner := &preflightRunner{
			opts:        preflightOptions{FleetFile: "fleet.toml"},
			spriteToken: "sprite-token",
			deps: withDefaultPreflightDeps(preflightDeps{
				readFile: func(string) ([]byte, error) { return []byte("[[sprite]]\nname = \"bb-builder\"\n"), nil },
				probeWorkers: func(context.Context, string, []string) ([]preflightWorkerProbe, error) {
					return []preflightWorkerProbe{{Name: "bb-builder", Reachable: true, GHAuth: false}}, nil
				},
			}),
		}
		result := runner.checkWorkers(context.Background())
		if result.Status != preflightFail {
			t.Fatalf("status = %s, want FAIL", result.Status)
		}
	})
}

func TestEnvLookupFallsBackToEnvFileExports(t *testing.T) {
	t.Parallel()

	runner := &preflightRunner{
		envExports: map[string]string{"FLY_ORG": "misty-step"},
		deps: withDefaultPreflightDeps(preflightDeps{
			getenv: func(string) string { return "" },
		}),
	}

	if got := runner.envLookup("FLY_ORG"); got != "misty-step" {
		t.Fatalf("envLookup(FLY_ORG) = %q, want %q", got, "misty-step")
	}
}

func TestCheckSpriteAuthUsesEnvFileExports(t *testing.T) {
	t.Parallel()

	runner := &preflightRunner{
		envExports: map[string]string{"FLY_ORG": "misty-step"},
		deps: withDefaultPreflightDeps(preflightDeps{
			getenv: func(string) string { return "" },
			resolveSpriteAuth: func(_ context.Context, getenv func(string) string) (preflightSpriteAuth, error) {
				if got := getenv("FLY_ORG"); got != "misty-step" {
					t.Fatalf("getenv(FLY_ORG) = %q, want %q", got, "misty-step")
				}
				return preflightSpriteAuth{Token: "sprite-token", Source: "FLY_API_TOKEN"}, nil
			},
		}),
	}

	result := runner.checkSpriteAuth(context.Background())
	if result.Status != preflightPass {
		t.Fatalf("status = %s, want PASS", result.Status)
	}
}
