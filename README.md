# Bitterblossom

Declarative sprite factory for provisioning and managing a fleet of Fly.io Sprites running Claude Code.

## What This Is

Bitterblossom is how OpenClaw (Kaylee) brainstorms, provisions, observes, and iterates on team compositions. The specific team composition is a **hypothesis**, not a fixed answer. Bitterblossom makes compositions cheap to change: edit YAML, re-provision, observe, iterate.

**v1:** Declarative config + provisioning + observation journal.
**Not yet:** Automated experimentation framework. That comes after manual observation reveals patterns.

## Architecture

```
bitterblossom/
├── base/                  # Shared config all sprites inherit
│   ├── CLAUDE.md          # Base engineering philosophy
│   ├── skills/            # Portable reference skills
│   ├── hooks/             # Safety hooks (Linux-compatible)
│   ├── commands/          # Shared command workflows
│   └── settings.json      # Base Claude Code settings
├── compositions/          # Team hypotheses (YAML)
│   ├── v1.yaml            # Current active composition
│   └── archive/           # Previous compositions
├── sprites/               # Individual sprite definitions
├── observations/          # Kaylee's learning journal
├── scripts/               # Provisioning + lifecycle
└── openclaw/              # Routing config for Kaylee
```

## How Kaylee Uses Bitterblossom

1. **Provision:** `./scripts/provision.sh <sprite-name>` creates a Fly.io machine from a sprite definition
2. **Route:** Kaylee reads `openclaw/agents.yaml` to match tasks to the best sprite
3. **Observe:** After tasks complete, Kaylee logs patterns in `observations/OBSERVATIONS.md`
4. **Iterate:** Edit `compositions/v1.yaml`, re-provision, observe again

## Constraints

- PR review handled by separate GitHub Action (multi-model council)
- All sprites are full-stack; specialization is preference, not constraint
- OpenClaw coordinates and routes tasks to sprites
- Fae/fairy naming convention throughout
- Sprites operate under shared GitHub account

## Quick Start

```bash
# Provision all sprites from current composition
./scripts/provision.sh --all

# Provision a single sprite
./scripts/provision.sh bramble

# Sync config updates to running fleet
./scripts/sync.sh

# Decommission a sprite
./scripts/teardown.sh bramble
```

## Composition Philosophy

5 full-stack sprites, each with a specialization preference. OpenClaw routes to the most appropriate sprite per task. All can handle any work.

| Sprite | Preference | Routes When |
|--------|-----------|-------------|
| **Bramble** | Systems & Data | DB, APIs, performance, server logic |
| **Willow** | Interface & Experience | UI, components, accessibility, design |
| **Thorn** | Quality & Security | Tests, security, bugs, error handling |
| **Fern** | Platform & Operations | CI/CD, deploy, Docker, environments |
| **Moss** | Architecture & Evolution | Refactoring, tech debt, design, docs |

See `compositions/v1.yaml` for full details.
