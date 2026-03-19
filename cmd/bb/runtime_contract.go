package main

// spriteModel is the canonical sprite runtime model identifier.
//
// This is the single authoritative Go reference for the model string.
// It must stay in sync with:
//   - base/settings.json  model profile alias (canonical deployed config)
//   - scripts/lib.sh      openrouter-claude default fallback
//   - README.md           documented runtime profile
//
// To update the sprite model: change this constant and base/settings.json
// together in the same commit, then run:
//
//	python3 -m pytest -q scripts/test_runtime_contract.py
const spriteModel = "anthropic/claude-sonnet-4-6"
