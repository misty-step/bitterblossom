package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommandsExecuteWithFakeSprite(t *testing.T) {
	root := setupRepoFixture(t)
	fakeBin := setupFakeSprite(t)

	cases := []struct {
		name string
		args []string
	}{
		{
			name: "dispatch_dry_run",
			args: []string{"--root", root, "--sprite-cli", fakeBin, "--dry-run", "dispatch", "thorn", "--skip-preflight", "do work"},
		},
		{
			name: "provision_all_dry_run",
			args: []string{"--root", root, "--sprite-cli", fakeBin, "--dry-run", "provision", "--all", "--composition", "compositions/v1.yaml"},
		},
		{
			name: "sync_dry_run",
			args: []string{"--root", root, "--sprite-cli", fakeBin, "--dry-run", "sync"},
		},
		{
			name: "bootstrap_dry_run",
			args: []string{"--root", root, "--sprite-cli", fakeBin, "--dry-run", "bootstrap", "--all"},
		},
		{
			name: "preflight_single",
			args: []string{"--root", root, "--sprite-cli", fakeBin, "preflight", "thorn"},
		},
		{
			name: "preflight_all",
			args: []string{"--root", root, "--sprite-cli", fakeBin, "preflight", "--all"},
		},
		{
			name: "teardown_force_dry_run",
			args: []string{"--root", root, "--sprite-cli", fakeBin, "--dry-run", "teardown", "--force", "thorn"},
		},
		{
			name: "dispatch_status",
			args: []string{"--root", root, "--sprite-cli", fakeBin, "dispatch", "thorn", "--status"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
		})
	}
}

func TestCommandValidationErrors(t *testing.T) {
	root := setupRepoFixture(t)
	fakeBin := setupFakeSprite(t)

	cases := []struct {
		name string
		args []string
	}{
		{
			name: "dispatch_missing_sprite",
			args: []string{"--root", root, "--sprite-cli", fakeBin, "dispatch"},
		},
		{
			name: "preflight_usage_error",
			args: []string{"--root", root, "--sprite-cli", fakeBin, "preflight"},
		},
		{
			name: "provision_all_and_targets",
			args: []string{"--root", root, "--sprite-cli", fakeBin, "--dry-run", "provision", "--all", "thorn"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestHelpers(t *testing.T) {
	if got := envOrDefault("BB_TEST_ENV", "fallback"); got != "fallback" {
		t.Fatalf("unexpected env default: %s", got)
	}
	if got := envInt("BB_TEST_INT", 12); got != 12 {
		t.Fatalf("unexpected env int default: %d", got)
	}
	t.Setenv("BB_TEST_ENV", "value")
	if got := envOrDefault("BB_TEST_ENV", "fallback"); got != "value" {
		t.Fatalf("unexpected env value: %s", got)
	}
	t.Setenv("BB_TEST_INT", "41")
	if got := envInt("BB_TEST_INT", 12); got != 41 {
		t.Fatalf("unexpected env int: %d", got)
	}
	t.Setenv("BB_TEST_INT", "bad")
	if got := envInt("BB_TEST_INT", 12); got != 12 {
		t.Fatalf("unexpected parsed fallback: %d", got)
	}
}

func setupRepoFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	mustMkdir := func(path string) {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}
	mustWrite := func(path string, content string) {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	mustMkdir(filepath.Join(root, "base", "hooks"))
	mustMkdir(filepath.Join(root, "base", "skills"))
	mustMkdir(filepath.Join(root, "base", "commands"))
	mustMkdir(filepath.Join(root, "sprites"))
	mustMkdir(filepath.Join(root, "scripts"))
	mustMkdir(filepath.Join(root, "compositions"))
	mustMkdir(filepath.Join(root, "observations", "archives"))

	mustWrite(filepath.Join(root, "base", "CLAUDE.md"), "base")
	mustWrite(filepath.Join(root, "base", "hooks", "h.py"), "print('x')")
	mustWrite(filepath.Join(root, "base", "skills", "s.md"), "skill")
	mustWrite(filepath.Join(root, "base", "commands", "c.md"), "cmd")
	mustWrite(filepath.Join(root, "base", "settings.json"), `{"env":{"ANTHROPIC_AUTH_TOKEN":"placeholder"}}`)
	mustWrite(filepath.Join(root, "sprites", "thorn.md"), "persona")
	mustWrite(filepath.Join(root, "scripts", "ralph-prompt-template.md"), "{{TASK_DESCRIPTION}} {{REPO}} {{SPRITE_NAME}}")
	mustWrite(filepath.Join(root, "compositions", "v1.yaml"), "sprites:\n  thorn:\n    preference: x\n")

	return root
}

func setupFakeSprite(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "sprite")
	script := `#!/usr/bin/env bash
set -euo pipefail
cmd="${1:-}"
shift || true
if [[ "$cmd" == "list" ]]; then
  echo "thorn"
  exit 0
fi
if [[ "$cmd" == "checkpoint" ]]; then
  exit 0
fi
if [[ "$cmd" == "destroy" || "$cmd" == "create" ]]; then
  exit 0
fi
if [[ "$cmd" == "exec" ]]; then
  while [[ $# -gt 0 && "$1" != "--" ]]; do shift; done
  if [[ $# -gt 0 ]]; then shift; fi
  op="${1:-}"
  case "$op" in
    echo)
      shift
      echo "${*:-}"
      ;;
    cat)
      target="${2:-}"
      if [[ "$target" == *"settings.json" ]]; then
        echo '{"env":{"ANTHROPIC_AUTH_TOKEN":"secret"}}'
      elif [[ "$target" == *"MEMORY.md" ]]; then
        echo "memory"
      else
        echo "claude"
      fi
      ;;
    tail)
      echo "tail"
      ;;
    bash)
      body="${3:-}"
      if [[ "$body" == *"claude --version"* ]]; then
        echo "claude 1.0"
      elif [[ "$body" == *"credential.helper"* ]]; then
        echo "store"
      elif [[ "$body" == *".git-credentials"* ]]; then
        echo "EXISTS"
      elif [[ "$body" == *"preflight-test"* ]]; then
        echo "PASS"
      elif [[ "$body" == *"test -f /home/sprite/workspace/CLAUDE.md"* ]]; then
        echo "YES"
      elif [[ "$body" == *"df -h"* ]]; then
        echo "10G"
      elif [[ "$body" == *"user.name"* ]]; then
        echo "sprite"
      elif [[ "$body" == *"pgrep -c claude"* ]]; then
        echo "0"
      elif [[ "$body" == *"ralph.pid"* && "$body" == *"kill -0"* ]]; then
        echo "NOT RUNNING"
      elif [[ "$body" == *"STATUS:"* ]]; then
        echo "STATUS: Working"
      else
        echo "ok"
      fi
      ;;
    *)
      echo "ok"
      ;;
  esac
  exit 0
fi
exit 0
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sprite: %v", err)
	}
	return bin
}
