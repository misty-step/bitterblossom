# roster

Roster is the agent declaration repository for Misty Step Factory. It keeps
agent identities, prompts, model policy, primitive references, and materializers
in one plain-file tree.

## Install

```sh
cargo install --locked --path crates/roster-cli   # installs `roster` to ~/.cargo/bin
cargo install --locked --path crates/roster-mcp   # installs `roster-mcp` to ~/.cargo/bin
```

Both binaries read the roster checkout via `--root <path>` (CLI, per
invocation) or the `ROSTER_ROOT` env var (MCP server); either defaults to the
current directory otherwise. See `SKILL.md` for the MCP tool contract and the
skill-facing operating rules.

P0 provides:

- `agents/<name>/role.yaml` and `instructions.md` declarations.
- `roster list`, `roster show <agent>`, `roster materialize <agent> --harness <target>`, and `roster brief <agent>`.
- Reference-only primitive indexes for skills and MCP servers.

```sh
roster --root . list
roster --root . show cerberus
roster --root . materialize cerberus --harness codex
roster --root . brief cerberus
```

(Working from inside the checkout, `--root .` is the default and can be
omitted. During development against uncommitted source, use
`cargo run -p roster-cli --` in place of `roster`.)

P2 adds an opt-in workstation sync for the default orchestrator agent:

```sh
roster --root . sync
```

`roster sync` installs roster-managed orchestrator artifacts under
`.roster/orchestrator/` and harness-native agent files at
`.codex/agents/orchestrator.md`, `.claude/agents/orchestrator.md`, and
`.pi/agents/orchestrator.md` beneath the target home directory. The curated
primitive subset is reference-only: skill bodies stay in harness-kit until the
P3 primitives migration. Existing harness-kit bootstrap globals such as
`.codex/AGENTS.md`, `.claude/CLAUDE.md`, and `.pi/settings.json` are not
overwritten during the parallel run.

Rollback is manifest-driven:

```sh
roster --root . sync --disable
```

For tests or staged installs, pass `--home <path>` to either command. Disable
removes only files recorded in `.roster/orchestrator/manifest.json`, and
harness-agent files outside `.roster/orchestrator/` are removed only when they
still carry the roster sync marker.

## `role.yaml` field reference

Every declaration lives at `agents/<name>/role.yaml` + `agents/<name>/instructions.md`
and is validated by `roster_core::Roster::load` (`deny_unknown_fields`: an
unrecognized key or a missing `instructions.md` fails the load, not just a lint
warning). Values below are the actual fields in use across the eight seed
agents (`agents/builder`, `agents/cerberus`, `agents/designer`,
`agents/incident-hound`, `agents/oracle`, `agents/orchestrator`,
`agents/sweep`, `agents/verifier`).

| Field | Type | What it's for | Actual values in use |
|---|---|---|---|
| `schema_version` | string | Declaration format version | `roster.role.v1` on all eight agents |
| `name` | string | Agent id; must match the directory name and be unique across `agents/` | `builder`, `cerberus`, `designer`, `incident-hound`, `oracle`, `orchestrator`, `sweep`, `verifier` |
| `description` | string | One-sentence purpose, echoed by `roster list`/`show`/`brief` | see `roster list` output below |
| `model_policy.preferred` | {model, reasoning} | Concrete, invocable model id + its reasoning effort (see concrete-id resolution below) | `gpt-5.5`/`high` (builder, verifier), `gpt-5.5`/`xhigh` (cerberus, incident-hound), `claude-fable-5`/`high` (orchestrator), `claude-fable-5`/`medium` (designer), `openrouter/deepseek/deepseek-v4-flash`/`high` (oracle), `openrouter/deepseek/deepseek-v4-flash`/`medium` (sweep) |
| `model_policy.fallbacks` | list\<{model, reasoning}\> | Ordered fallback ids, each with its own reasoning | e.g. sweep: `openrouter/moonshotai/kimi-k2.7-code`, `openrouter/qwen/qwen3-coder-next` (both `medium`) |
| `permissions.filesystem` | string | Free text; feeds `claude_tools()` (`Write`/`Edit` are added when this contains `"write"`) | `workspace-write` (builder, designer, orchestrator), `read-only` (cerberus, incident-hound, oracle, sweep, verifier) |
| `permissions.commands` | string | Free text; `Bash` is added by `claude_tools()` unless this is `"none"` or `"disabled-by-default"` | `allowed` (builder, designer, orchestrator), `verification-only` (cerberus, incident-hound, oracle, verifier), `read-only` (sweep) |
| `permissions.network` | string | Free text; `WebSearch` is added by `claude_tools()` only when this is exactly `"allowed"` | `allowed` (builder, designer, incident-hound, oracle, orchestrator, sweep, verifier), `disabled-by-default` (cerberus) |
| `permissions.secrets` | string | Free text, documentation only (no code branches on it today) | `env-refs-only` (builder, orchestrator), `none` (cerberus, designer, incident-hound, oracle, sweep, verifier) |
| `permissions.mutations` | string | Free text; `Write`/`Edit` are also added by `claude_tools()` when this is not `"none"` | `with-explicit-scope` (builder, orchestrator), `styling-and-markup-scope` (designer), `none` (cerberus, incident-hound, oracle, sweep, verifier) |
| `skills` | list\<{name, path, reason}\> | Skill files the agent should read; `path` is an absolute filesystem path (mostly under `primitives/skills`, a few still pending migration from `harness-kit`) | orchestrator has 8, builder/verifier/sweep have 3 each, cerberus/designer/incident-hound have 3 each, oracle has 2 |
| `mcps` | list\<string\> | Bare MCP server names required at dispatch time (rendered as claude/codex/brief's "MCP Servers → Required") | builder, orchestrator: `powder`; sweep: `qmd`; cerberus, designer, incident-hound, oracle, verifier: none |
| `mcps_contextual` | list\<string\>, optional (defaults empty) | MCP server names to bind only when present in the calling harness (rendered as "MCP Servers → Contextual (bind when present)"); not rendered for `bb` (no MCP concept there) | orchestrator: `qmd`, `todoist`, `bitterblossom`, `glass`; oracle: `exa`, `firecrawl`, `context7`; the rest: none |
| `subagent_rights.may_dispatch` / `may_spawn_subagents` / `may_use_peer_harnesses` | bool | What the agent is allowed to fan work out to | `may_spawn_subagents` is `true` on all eight (roster-924); `may_dispatch`/`may_use_peer_harnesses` stay agent-specific (`false` for designer, incident-hound, oracle, sweep) |
| `evidence_expectations` | list\<string\> | Free prose bullets, no fixed vocabulary; printed verbatim under `## Evidence Contract` / `## Evidence Expectations` | see `agents/*/role.yaml` |

### Concrete model ids and per-harness resolution

Model policy v2 (roster-924) retired the abstract tier vocabulary
(`codex-class`/`fable-class`/`openrouter-class`) that `model_policy` used to
carry. `preferred` and every `fallbacks` entry are now always a CONCRETE,
invocable model id (`gpt-5.5`, `claude-fable-5`, `claude-opus-4-8`,
`openrouter/<provider>/<model>`) paired with its own `reasoning` value — no
role-level `reasoning` field, no symbol a human has to decode against
doctrine. What each concrete id is *for* is still doctrine (`VISION.md`:
`claude-fable-5` → strategy/planning/review, rarely high reasoning;
`gpt-5.5` → implementation lanes at high/xhigh; the OpenRouter ids → cheap
sweeps), but rendering never needs to consult that doctrine — the id itself
is what gets invoked.

Most concrete ids are already the token their target harness needs
(`openrouter/`-prefixed ids for `bb`; `sonnet`/`opus`/`haiku`/`inherit` for
`claude`) and resolve with no lookup at all. The handful that need real
per-harness translation — `claude-fable-5`, `gpt-5.5`, and
`openrouter/deepseek/deepseek-v4-flash` — go through
`primitives/models.yaml` (retires `primitives/tiers.yaml`; still a distinct
concept from the pre-existing `primitives/providers.yaml`, which is an
unrelated peer-harness-CLI dispatch table migrated from harness-kit's
`agents.yaml` at P0). `roster_core::Models::load` reads it, consulted only
for `model_policy.preferred` — never for fallbacks:

- **`--harness claude`** (`render_claude_agent` / `claude_model` in
  `roster-core/src/lib.rs`): looks `preferred.model` up in `models.yaml`'s
  `models` table for the `claude` column (e.g. `claude-fable-5` → `inherit`,
  `gpt-5.5` → `sonnet`, `openrouter/deepseek/deepseek-v4-flash` → `haiku`).
  If `preferred.model` isn't in the table, a small conservative literal-id
  map applies (`claude-opus-4-8` → `opus`, etc.); anything still unrecognized
  falls back to `inherit` (the subagent runs on the session's own model)
  rather than guessing.
- **`--harness codex`** and **`brief`** (`render_brief`): print
  `preferred`/`fallbacks` as literal `model (reasoning: x)` text under
  `## Model Policy`. No table resolution happens here — codex materializes
  straight from the declaration; nothing in this repo yet targets the
  `~/.codex/roles/<name>.toml` shape the roster-910 dispatch-matrix research
  found on this machine.
- **`--harness bb`** (`render_bb_agent` / `bb_model`): resolves to the first
  `openrouter/`-prefixed model found in `preferred` then `fallbacks`,
  stripping the prefix (unchanged — this is how oracle and sweep resolve,
  since their `preferred` id is already `openrouter/`-prefixed). Only if
  neither is `openrouter/`-prefixed does it fall through to `models.yaml`'s
  `bb` column for `preferred.model` (e.g. `claude-fable-5` and `gpt-5.5` both
  → `openrouter/moonshotai/kimi-k2.7-code`, prefix stripped the same way —
  this is how orchestrator and incident-hound resolve, since neither has an
  `openrouter/`-prefixed fallback). If the id isn't in the table either,
  `render_bb_agent` returns `Err` — it never emits an uninvokable id like
  `model = "gpt-5.5"` into the generated TOML.

### Default ad hoc subagent pool

`primitives/subagent-pool.yaml` declares, once, the models every roster
agent should reach for when it spawns an ad hoc subagent mid-task (distinct
from the declared `agents/*` identities dispatched through `roster brief` —
this is the fallback pool for one-off fan-out a lane decides it needs).
Every agent's `subagent_rights.may_spawn_subagents` is `true` (roster-924),
and each `instructions.md` carries one line pointing at the file rather than
re-listing the pool per agent: "Dispatch ad hoc subagents where useful;
favor the pool declared in `primitives/subagent-pool.yaml`." The pool itself
is operator-curated and extensible — add rows on capability-ledger evidence,
not vibes.

## Add a new agent (quickstart)

1. Create `agents/<name>/role.yaml` and `agents/<name>/instructions.md`
   following the field reference above.
2. Run `cargo run -p roster-cli -- list` and confirm the new name appears.
3. Run `cargo test --workspace`. `roster_core::Roster::load` will accept the
   new agent, but note `crates/roster-core/tests/loader.rs`'s
   `loads_seed_agents_from_repo` test hardcodes the exact seed-agent name list
   — adding a real fourth seed agent means updating that assertion too, or the
   test fails on the list mismatch (not a validation error).

This was run for real while authoring this section, with a throwaway
`agents/example/role.yaml` (removed afterward — it is not a ninth seed
agent):

```
$ cargo run -q -p roster-cli -- list
builder	gpt-5.5	high	Delivery lane that takes one ticket from a stated goal to a working, gated change — red-green-refactor discipline, live proof, gates never lowered to reach green.
cerberus	gpt-5.5	xhigh	Code-review master agent that turns available change context into grounded findings, verdicts, and a review artifact without overstating inspected evidence.
designer	claude-fable-5	medium	Visible-artifact critique and polish across UI, design-system primitives, docs pages, and diagrams.
example	openrouter/deepseek/deepseek-v4-flash	medium	Minimal placeholder agent used by the README quickstart to demonstrate adding a new agent.
incident-hound	gpt-5.5	xhigh	Live-system incident root cause — production failures, not diff review.
oracle	openrouter/deepseek/deepseek-v4-flash	high	AI-awareness sidekick — current on state-of-the-art models, harnesses, and agent tooling; advises routing (local vs open vs frontier), critiques deterministic-where-a-model-belongs, and pushes the fleet to use AI more than instinct suggests.
orchestrator	claude-fable-5	high	Master orchestrator — frames factory work, grooms and shapes the board, composes and dispatches lanes, compares evidence, verifies outcomes, and closes the workspace cleanly.
sweep	openrouter/deepseek/deepseek-v4-flash	medium	Cheap read-only research and repository sweep lane for broad scanning, source collection, and concise discrepancy reports.
verifier	gpt-5.5	high	Adversarial verification lane that reproduces a claim live before trusting it — read-only, findings-only, never fixes what it verifies.

$ cargo test --workspace
...
test loads_seed_agents_from_repo ... FAILED
thread 'loads_seed_agents_from_repo' panicked at crates/roster-core/tests/loader.rs:29:5:
assertion `left == right` failed
  left: ["builder", "cerberus", "designer", "example", "incident-hound", "oracle", "orchestrator", "sweep", "verifier"]
 right: ["builder", "cerberus", "designer", "incident-hound", "oracle", "orchestrator", "sweep", "verifier"]
```

The `list` step proves the loader picked up the new declaration; the `test`
step proves it validated correctly (schema-wise) and only failed on the
seed-count assertion, exactly as described above.

## `roster brief --card` transcript (real, live)

This is the actual output of `roster brief sweep --card roster-924`, run
against roster-924 — the live Powder card that shipped model policy v2 —
with `POWDER_API_BASE_URL`/`POWDER_API_KEY` set from `~/.secrets`:

```
$ cargo run -q -p roster-cli -- brief sweep --card roster-924
# Roster Brief: sweep

## Role

Cheap read-only research and repository sweep lane for broad scanning, source collection, and concise discrepancy reports.

## Model Policy

- Preferred: openrouter/deepseek/deepseek-v4-flash (reasoning: medium)
- Fallbacks: openrouter/moonshotai/kimi-k2.7-code (reasoning: medium), openrouter/qwen/qwen3-coder-next (reasoning: medium)

## Instructions

Read: ./agents/sweep/instructions.md

# Sweep Lane

You are a cheap read-only sweep lane. Search broadly, cite exactly what you
inspected, and keep the output compact enough for the orchestrator to act on.

Use repository files, command output, and current external sources when allowed.
Separate confirmed evidence from inference. Do not edit files, update trackers,
push branches, send messages, or perform any mutating action.

Return a report with: objective, sources checked, high-signal findings,
discrepancies or gaps, and the next one or two checks that would most improve
confidence.

Dispatch ad hoc subagents where useful; favor the pool declared in
`primitives/subagent-pool.yaml`.

## Skills To Read

- research: /Users/phaedrus/Development/roster/primitives/skills/research/SKILL.md (source-backed web and repository research)
- orient: /Users/phaedrus/Development/roster/primitives/skills/orient/SKILL.md (fast repository orientation from live evidence)
- diagnose: /Users/phaedrus/Development/roster/primitives/skills/diagnose/SKILL.md (structured triage when the sweep finds a contradiction)

## MCP Servers

### Required

- qmd

### Contextual (bind when present)

- none

## Permissions

- Filesystem: read-only
- Commands: read-only
- Network: allowed
- Secrets: none
- Mutations: none

## Subagent Rights

- May dispatch: false
- May spawn subagents: true
- May use peer harnesses: false

## Evidence Contract

- Report sources, commands, and searched paths.
- Separate confirmed facts from plausible inferences.
- Return a concise packet with findings, gaps, and next checks.
- Do not mutate files, trackers, remotes, or external systems.

## Powder Card

- ID: roster-924
- Title: Model policy v2: concrete models + per-entry reasoning; kill the class tiers; declared subagent pool

### Acceptance

- *-class strings gone from every role.yaml and from the loader schema; concrete model ids + per-entry reasoning everywhere
- orchestrator: claude-fable-5/high with gpt-5.5/xhigh fallback (ruled binding)
- tiers.yaml retired; harness-token translation (if needed) lives in primitives/models.yaml keyed by concrete id
- subagent pool declared in primitives; all 8 agents may_spawn_subagents true with the dispatch-encouragement instructions line
- materialize green for all 8 agents on claude+bb; agents page regenerated + republished; gates green

### Body

Operator ruling 2026-07-06 (voice): the *-class notations make no sense -- the real axes are (1) primitives/config/tools/prompt (the identity), (2) the harness it runs in, (3) the model and reasoning. Redesign: model_policy becomes CONCRETE -- preferred {model, reasoning} and fallbacks [{model, reasoning}] (per-entry reasoning; schema change, loader deny_unknown_fields updated). RULED BINDINGS: orchestrator = claude-fable-5 reasoning HIGH, fallback gpt-5.5 reasoning XHIGH. [...]
```

This is exactly the "dynamic composition" seam `VISION.md` calls out as the
critical design consideration: one declaration (`agents/sweep/role.yaml` +
`instructions.md`), one CLI invocation, and the brief carries role prompt,
skill paths, required/contextual MCP servers, permissions, subagent rights,
evidence contract, and — when `--card` is passed — the live Powder card
context, ready for any harness to consume as prompt-native text.
