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
warning). Values below are the actual fields in use across the six seed
agents (`agents/builder`, `agents/cerberus`, `agents/oracle`,
`agents/orchestrator`, `agents/sweep`, `agents/verifier`).

| Field | Type | What it's for | Actual values in use |
|---|---|---|---|
| `schema_version` | string | Declaration format version | `roster.role.v1` on all six agents |
| `name` | string | Agent id; must match the directory name and be unique across `agents/` | `builder`, `cerberus`, `oracle`, `orchestrator`, `sweep`, `verifier` |
| `description` | string | One-sentence purpose, echoed by `roster list`/`show`/`brief` | see `roster list` output below |
| `model_policy.preferred` | string | Model tier or literal model id (see resolution table below) | `codex-class` (builder, cerberus, verifier), `openrouter-class` (oracle, sweep), `fable-class` (orchestrator) |
| `model_policy.fallbacks` | list\<string\> | Ordered fallback tiers/models | e.g. sweep: `openrouter/moonshotai/kimi-k2.7-code`, `openrouter/deepseek/deepseek-v4-flash`, `openrouter/qwen/qwen3-coder-next` |
| `model_policy.reasoning` | string | Reasoning-effort tier, printed as-is, never parsed | `xhigh` (cerberus), `high` (builder, oracle, verifier), `low` (orchestrator), `medium` (sweep) |
| `permissions.filesystem` | string | Free text; feeds `claude_tools()` (`Write`/`Edit` are added when this contains `"write"`) | `workspace-write` (builder, orchestrator), `read-only` (cerberus, oracle, sweep, verifier) |
| `permissions.commands` | string | Free text; `Bash` is added by `claude_tools()` unless this is `"none"` or `"disabled-by-default"` | `allowed` (builder, orchestrator), `verification-only` (cerberus, oracle, verifier), `read-only` (sweep) |
| `permissions.network` | string | Free text; `WebSearch` is added by `claude_tools()` only when this is exactly `"allowed"` | `allowed` (builder, oracle, orchestrator, sweep, verifier), `disabled-by-default` (cerberus) |
| `permissions.secrets` | string | Free text, documentation only (no code branches on it today) | `env-refs-only` (builder, orchestrator), `none` (cerberus, oracle, sweep, verifier) |
| `permissions.mutations` | string | Free text; `Write`/`Edit` are also added by `claude_tools()` when this is not `"none"` | `with-explicit-scope` (builder, orchestrator), `none` (cerberus, oracle, sweep, verifier) |
| `skills` | list\<{name, path, reason}\> | Skill files the agent should read; `path` is an absolute filesystem path (currently all under `harness-kit`, pending the P3 primitives migration) | orchestrator has 8, builder and verifier have 3 each, cerberus has 3, oracle has 2, sweep has 3 |
| `mcps` | list\<string\> | Bare MCP server names required at dispatch time (rendered as claude/codex/brief's "MCP Servers → Required") | builder, orchestrator: `powder`; sweep: `qmd`; cerberus, oracle, verifier: none |
| `mcps_contextual` | list\<string\>, optional (defaults empty) | MCP server names to bind only when present in the calling harness (rendered as "MCP Servers → Contextual (bind when present)"); not rendered for `bb` (no MCP concept there) | orchestrator: `qmd`, `todoist`, `bitterblossom`, `glass`; oracle: `exa`, `firecrawl`, `context7`; builder, cerberus, sweep, verifier: none |
| `subagent_rights.may_dispatch` / `may_spawn_subagents` / `may_use_peer_harnesses` | bool | What the agent is allowed to fan work out to | builder, orchestrator, cerberus, verifier: all `true`; oracle and sweep: all `false` (leaf lanes) |
| `evidence_expectations` | list\<string\> | Free prose bullets, no fixed vocabulary; printed verbatim under `## Evidence Contract` / `## Evidence Expectations` | see `agents/*/role.yaml` |

### Model tier vocabulary and per-harness resolution

`model_policy.preferred`/`fallbacks` carry an abstract tier string
(`codex-class`, `fable-class`, `openrouter-class`) or a literal model id
(`gpt-5.5`, `claude-opus-4-8`, `openrouter/<provider>/<model>`). What a tier
means operationally is doctrine (`VISION.md`: fable-class → strategy/planning/
review at low-to-medium reasoning, rarely high; codex-class → implementation
lanes on GPT-5.5 at high/xhigh; openrouter-class → cheap OpenRouter sweeps).
`primitives/tiers.yaml` (a NEW file, distinct from the pre-existing
`primitives/providers.yaml` — that file is an unrelated peer-harness-CLI
dispatch table migrated from harness-kit's `agents.yaml` at P0, how to invoke
codex/claude/pi/etc; `tiers.yaml` is how a `model_policy` tier resolves to a
concrete, invocable id when ROSTER renders its own agent declarations) is
where each tier resolves to a concrete id per roster harness;
`roster_core::Providers::load` reads it and every `materialize --harness`
target consults it except `codex`:

- **`--harness claude`** (`render_claude_agent` / `claude_model` in
  `roster-core/src/lib.rs`): looks `preferred` up in `tiers.yaml`'s `tiers`
  table for the `claude` column (e.g. `fable-class` → `inherit`,
  `codex-class` → `sonnet`, `openrouter-class` → `haiku`). If `preferred`
  isn't a known tier, a small conservative literal-id map applies
  (`claude-opus-4-8` → `opus`, etc.); anything still unrecognized falls back
  to `inherit` (the subagent runs on the session's own model) rather than
  guessing. Fixed 2026-07-05 (roster-909): this used to hardcode
  `model: sonnet` for every agent regardless of tier.
- **`--harness codex`** and **`brief`** (`render_brief`): print `preferred`,
  `fallbacks`, and `reasoning` as literal text under `## Model Policy`. No
  table resolution happens here; a human or the orchestrator reads the tier
  and applies the doctrine above to pick a concrete model. `tiers.yaml`'s
  `codex` column exists but nothing consumes it yet — the roster-910
  dispatch-matrix research found this machine's live codex custom-agent
  pattern is `config.toml [agents.<name>]` pointing at a
  `~/.codex/roles/<name>.toml` file with its own `model` field, distinct
  from `render_brief`'s prompt-text output; the `codex` column is reserved
  for a future materializer targeting that file, not for `render_brief`.
- **`--harness bb`** (`render_bb_agent` / `bb_model`): resolves to the first
  `openrouter/`-prefixed value found in `preferred` then `fallbacks`,
  stripping the prefix (unchanged — this is how cerberus and sweep resolve).
  Only if neither is `openrouter/`-prefixed does it fall through to
  `tiers.yaml`'s `bb` column for `preferred` (e.g. `fable-class` →
  `openrouter/moonshotai/kimi-k2.7-code`, prefix stripped the same way). If
  the tier isn't in the table either, `render_bb_agent` returns `Err` — it
  never emits a bare tier string like `model = "fable-class"` into the
  generated TOML. Fixed 2026-07-05 (roster-909): this used to be the gap —
  `orchestrator`'s preferred/fallbacks (`fable-class`, `claude-opus-4-8`,
  `gpt-5.5-pro-browser`) are still all non-`openrouter/`,
  but now resolve through the table instead of leaking the bare tier.

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
`agents/example/role.yaml` (removed afterward — it is not a fourth seed
agent):

```
$ cargo run -q -p roster-cli -- list
builder	codex-class	high	Delivery lane that takes one ticket from a stated goal to a working, gated change — red-green-refactor discipline, live proof, gates never lowered to reach green.
cerberus	codex-class	xhigh	Code-review master agent that turns available change context into grounded findings, verdicts, and a review artifact without overstating inspected evidence.
example	openrouter-class	medium	Minimal placeholder agent used by the README quickstart to demonstrate adding a new agent.
oracle	openrouter-class	high	AI-awareness sidekick — current on state-of-the-art models, harnesses, and agent tooling; advises routing (local vs open vs frontier), critiques deterministic-where-a-model-belongs, and pushes the fleet to use AI more than instinct suggests.
orchestrator	fable-class	low	Master orchestrator — frames factory work, grooms and shapes the board, composes and dispatches lanes, compares evidence, verifies outcomes, and closes the workspace cleanly.
sweep	openrouter-class	medium	Cheap read-only research and repository sweep lane for broad scanning, source collection, and concise discrepancy reports.
verifier	codex-class	high	Adversarial verification lane that reproduces a claim live before trusting it — read-only, findings-only, never fixes what it verifies.

$ cargo test --workspace
...
test loads_seed_agents_from_repo ... FAILED
thread 'loads_seed_agents_from_repo' panicked at crates/roster-core/tests/loader.rs:27:5:
assertion `left == right` failed
  left: ["builder", "cerberus", "example", "oracle", "orchestrator", "sweep", "verifier"]
 right: ["builder", "cerberus", "oracle", "orchestrator", "sweep", "verifier"]
```

The `list` step proves the loader picked up the new declaration; the `test`
step proves it validated correctly (schema-wise) and only failed on the
seed-count assertion, exactly as described above.

## `roster brief --card` transcript (real, live)

This is the actual output of `roster brief sweep --card roster-012`, run
against roster-012 — the live Powder card tracking this documentation work —
with `POWDER_API_BASE_URL`/`POWDER_API_KEY` set from `~/.secrets`:

```
$ cargo run -q -p roster-cli -- brief sweep --card roster-012
# Roster Brief: sweep

## Role

Cheap read-only research and repository sweep lane for broad scanning, source collection, and concise discrepancy reports.

## Model Policy

- Preferred: openrouter-class
- Fallbacks: openrouter/moonshotai/kimi-k2.7-code, openrouter/deepseek/deepseek-v4-flash, openrouter/qwen/qwen3-coder-next
- Reasoning: medium

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

## Skills To Read

- research: /Users/phaedrus/Development/harness-kit/skills/research/SKILL.md (source-backed web and repository research)
- orient: /Users/phaedrus/Development/harness-kit/skills/orient/SKILL.md (fast repository orientation from live evidence)
- diagnose: /Users/phaedrus/Development/harness-kit/skills/diagnose/SKILL.md (structured triage when the sweep finds a contradiction)

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
- May spawn subagents: false
- May use peer harnesses: false

## Evidence Contract

- Report sources, commands, and searched paths.
- Separate confirmed facts from plausible inferences.
- Return a concise packet with findings, gaps, and next checks.
- Do not mutate files, trackers, remotes, or external systems.

## Powder Card

- ID: roster-012
- Title: Documentation floor: role.yaml field reference, a real brief --card transcript, and an add-a-new-agent quickstart

### Acceptance

- README (or a linked docs/ page) documents every role.yaml field -- schema_version, model_policy (including the tier vocabulary: codex-class/fable-class/openrouter-class and what each resolves to per harness), permissions vocabulary, skills, mcps, subagent_rights, evidence_expectations -- against the actual values used by the three seed agents.
- README includes one real, copy-pasteable transcript of `roster brief <agent> --card <id>` run against a live Powder card, showing the actual composed output -- not a hypothetical example -- since this is VISION.md's stated critical design consideration.
- README includes an add-a-new-agent quickstart: create agents/<name>/{role.yaml,instructions.md}, run `roster list` to confirm registration, run cargo test --workspace to confirm the loader validates it.
- Every command shown in the README/docs is actually run during authoring and its real output pasted in, not written from memory.

### Body

Current README is 18 lines: three bare `cargo run` invocations with no explanation of role.yaml semantics, no worked brief --card example, and no path for adding a new agent -- despite VISION.md's own 3-month bar being 'new agents are a directory, not a project.' Marketing site is explicitly out of scope (misty-step-910 owns it); this card is documentation-floor only (application-floor.md item 2).
```

This is exactly the "dynamic composition" seam `VISION.md` calls out as the
critical design consideration: one declaration (`agents/sweep/role.yaml` +
`instructions.md`), one CLI invocation, and the brief carries role prompt,
skill paths, required/contextual MCP servers, permissions, subagent rights,
evidence contract, and — when `--card` is passed — the live Powder card
context, ready for any harness to consume as prompt-native text.
