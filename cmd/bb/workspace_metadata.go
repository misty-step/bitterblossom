package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"
)

type workspaceMetadata struct {
	SchemaVersion int    `json:"schema_version"`
	Repo          string `json:"repo"`
	RepoDir       string `json:"repo_dir"`
	Sprite        string `json:"sprite"`
	Persona       string `json:"persona"`
	ConfiguredAt  string `json:"configured_at"`
}

func workspaceMetadataPath(workspace string) string {
	return filepath.Join(workspace, workspaceMetadataRelPath)
}

func newWorkspaceMetadata(spriteName, repo, repoDir, persona string, now time.Time) workspaceMetadata {
	return workspaceMetadata{
		SchemaVersion: 1,
		Repo:          repo,
		RepoDir:       repoDir,
		Sprite:        spriteName,
		Persona:       persona,
		ConfiguredAt:  now.UTC().Format(time.RFC3339),
	}
}

func marshalWorkspaceMetadata(meta workspaceMetadata) ([]byte, error) {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal workspace metadata: %w", err)
	}
	return append(data, '\n'), nil
}

func workspaceDiscoveryScript() string {
	return fmt.Sprintf(`
set -euo pipefail

meta=$(ls -dt %s/*/%s 2>/dev/null | head -1 || true)
if [[ -n "$meta" ]]; then
  printf '%%s\n' "${meta%%/%s}"
  exit 0
fi

prompt=$(ls -dt %s/*/%s 2>/dev/null | head -1 || true)
if [[ -n "$prompt" ]]; then
  printf '%%s\n' "${prompt%%/*}"
  exit 0
fi

log=$(ls -dt %s/*/%s 2>/dev/null | head -1 || true)
if [[ -n "$log" ]]; then
  printf '%%s\n' "${log%%/*}"
  exit 0
fi

ws=$(ls -d %s/*/ 2>/dev/null | head -1 || true)
printf '%%s\n' "${ws%%/}"
`,
		spriteWorkspaceRoot,
		workspaceMetadataRelPath,
		workspaceMetadataRelPath,
		spriteWorkspaceRoot,
		dispatchPromptFileName,
		spriteWorkspaceRoot,
		agentLogFileName,
		spriteWorkspaceRoot,
	)
}
