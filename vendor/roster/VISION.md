# VISION — roster

Roster makes agents the first-class citizens of the factory. It is the single
place where every agent we run is DECLARED — its role, system prompt,
capabilities, model policy, permitted skills and MCP servers, evidence
expectations — and the machinery that turns a declaration into a running
agent on any plane: this laptop's harnesses, bitterblossom's remote runners,
or whatever substrate comes next.

It succeeds harness-kit by inverting the sync model. Harness-kit synced the
entire pack of primitives to every harness on every machine — overkill that
made every session carry every skill. Roster syncs ONE default orchestrator
agent (the lead) with a curated subset of primitives; the rest of the roster
is exposed through the five faces (CLI, MCP, API, SDK, skill) and discovered
ad hoc. The lead sees the other agents, and dispatches them with exactly the
context they need — no more, no less.

## The shape (operator-ratified 2026-07-04)

- **`agents/<name>/`** — one directory per declared agent (the Eve
  convention, without the Vercel runtime): `role.yaml` (description, model
  policy with a concrete preferred {model, reasoning} + fallback
  {model, reasoning} entries, permissions, skills list, mcps list, subagent
  rights, evidence expectations) and `instructions.md` (the system prompt).
  Optional `tools/` for bespoke tooling. The declaration is data + prose,
  never framework code.
- **`primitives/`** — the curated library beneath the agents: `skills/`
  (first-party skills migrated from harness-kit over time, vendored external
  skills — Anthropic official, OpenAI official, Matt Pocock's — under
  `skills/.external/`), `mcps/` (MCP server registry: name, launch, env
  refs), `providers.yaml` (invocation tables per brain: the harness-kit
  agents.yaml lineage — how to actually invoke codex/claude/pi/etc),
  `models.yaml` (per-harness token translation for the handful of concrete
  model ids that need one), `subagent-pool.yaml` (the default ad hoc
  subagent pool every agent favors).
- **`roster` CLI (Rust)** — the operational face: `list`, `show <agent>`,
  `materialize <agent> --harness <claude|codex|pi|bb>` (emit the
  harness-native form of a declaration), `brief <agent> [--card <id>]
  [--add-skill X] [--add-mcp Y]` (emit a ready lane-brief header: role
  prompt + skill file paths to read + MCP selection + evidence contract —
  the DYNAMIC COMPOSITION seam), `sync` (install the default agent + its
  curated subset to this machine's harnesses — the harness-kit bootstrap
  successor, initially opt-in and parallel).

## Dynamic composition (the critical design consideration)

A dispatch must be able to compose an agent's context at dispatch time:
which skills it reads, which MCP servers it reaches, which model tier runs
it. The mechanism is prompt-native, because every harness ultimately accepts
text and file paths: `roster brief` renders the declaration plus overrides
into a brief header any harness consumes — Claude Code subagents read the
named skill files and ToolSearch the named MCPs; codex lanes get the same
header prepended to their task brief; bitterblossom materializes the same
declaration into its runner config. One declaration, one composition seam,
every plane. Rigid schema exists only where deterministic code branches
(role.yaml); everything the model consumes rides as prose.

## Model policy (operator doctrine, encoded in role.yaml)

Fable (`claude-fable-5`) identities are reserved for strategy, planning,
review, and visual intelligence, typically at low-to-medium reasoning,
rarely high — and spawned sparingly. Implementation lanes default to
GPT-5.5 at high/xhigh. Claude Code subagents materialize as Sonnet 5 (a
harness-level translation, not a role choice). Cheap sweeps ride OpenRouter
lanes. `role.yaml`'s `model_policy` carries this as a concrete preferred
model id + reasoning, and concrete fallback ids each with their own
reasoning — no abstract tier symbol to decode (model policy v2, roster-924
retired the `*-class` vocabulary). Every agent may also spawn ad hoc
subagents from the pool declared once in `primitives/subagent-pool.yaml`.
The declaration is where routing doctrine lives from now on (crucible's
routing bench feeds it evidence over time).

## Phases (this repo's own backlog; P0 is the founding lane)

P0 — repo, VISION, CLI core (list/show/materialize/brief), three seed
agents, providers.yaml migrated. P1 — bitterblossom consumes roster
(vendored pin) for one role end-to-end. P2 — `roster sync` initializes the
lead on this machine, parallel with harness-kit. P3 — primitives migration;
harness-kit archives. P4 — Cerberus's identity fully in roster; repo
archives. Parallel-run discipline throughout: harness-kit keeps working
until each phase proves its replacement.

## Non-goals

- Not an execution runtime: roster declares and materializes; planes run.
- Not a framework: no SDK-lock, no build step for agent definitions.
- No secret values in declarations — env refs only.
- No big-bang migration: nothing existing breaks in any phase.

## What excellent looks like (3 months)

Every agent in the fleet — the lead session, local subagents, bb runners,
review lanes — is a `roster show` away from full legibility: one file tree
answers what it is, what it may do, what it reads, and what runs it. New
agents are a directory, not a project.
