# Context Packet: Agent-First Fleet CLI

## Why This Exists

Bitterblossom has already pushed most workflow judgment into sprite personas and
skills, but the runtime surface still behaves like a coordinator product:

- `mix conductor start` assumes an always-on local control process.
- `Conductor.Application` owns fleet launch, health monitoring, and restart
  policy.
- `Conductor.Launcher` resets workspaces and re-dispatches loops from a central
  process.
- `fleet.toml` models concrete instances, which makes "run three Weavers" a
  file-editing exercise instead of a first-class operation.

The next step is not "move the conductor to a remote box." The next step is to
finish the architectural turn that the repo already started: sprites own their
loops, and Bitterblossom becomes the opinionated CLI that creates, inspects,
starts, pauses, stops, clones, and destroys those sprites.

## Product Exploration

### Approach 1: Keep the conductor, just host it remotely

Run the current Elixir app on an always-on Fly Machine or VM and keep the rest
of the design intact.

Why reject it:

- Solves laptop uptime, not the architectural mismatch.
- Preserves central restart/health logic that should belong to each sprite.
- Leaves the operator surface oriented around `start`ing a manager instead of
  managing sprites directly.

### Approach 2: Drop Bitterblossom down to raw `sprite` commands

Tell operators to use the upstream `sprite` CLI directly plus ad hoc shell
scripts.

Why reject it:

- `sprite` gives transport primitives, not Bitterblossom policy.
- No repo-owned template catalog, no persona-aware provisioning, no truthful
  capacity surface, no standard pause/start semantics.
- Pushes the important semantics back into operator lore.

### Approach 3: Make Bitterblossom a thin fleet CLI over self-managing sprites

Keep the deep sprite primitives, delete the coordinator assumptions, and expose
an operator surface for:

- fleet audit and capacity
- sprite lifecycle control
- template-based create/clone/scale
- persona-aware provisioning and loop launch

Recommendation: this is the right direction.

It preserves the useful Bitterblossom opinion layer while deleting the central
runtime that now mostly fights the architecture.

## Product Shape

### Core model

There are two concepts:

1. **Template**: a repo-owned catalog entry that defines an agent type
   (`weaver`, `thorn`, `fern`, future `canary-triage`), including persona,
   role, repo, harness, model, and naming convention.
2. **Instance**: a live sprite created from a template (`bb-builder`,
   `bb-builder-2`, `bb-polisher-3`).

`fleet.toml` should describe templates, not the currently running set of
instances.

### Operator jobs the CLI must own

- Show current fleet health and spare capacity without requiring a background
  process.
- Audit which sprites exist, which are reachable, which are actively running a
  loop, which are paused, and which are broken.
- Create a new instance from a template.
- Clone an existing instance's template/config into a sibling instance.
- Start, pause, resume, stop, and destroy instances.
- Scale a template up or down imperatively.
- Tail logs and inspect remote state from one consistent surface.

### Semantics

- `start` launches an autonomous loop on a sprite and returns immediately.
- `pause` marks a sprite unschedulable for future loop starts.
- `pause --wait` is the maintenance form: mark paused, then wait for or stop the
  current loop before returning.
- `resume` removes the pause marker.
- `stop` stops the current loop without changing pause state.
- `scale <template> <n>` is imperative sugar over create/destroy. It changes the
  live fleet now; it does not smuggle a background controller back in.

This is intentionally closer to `kubectl scale` plus `cordon`/`drain`
semantics than to a long-running control plane.

## Recommended CLI Surface

Primary commands:

```bash
bitterblossom fleet status [--json]
bitterblossom fleet audit [--json]
bitterblossom fleet scale <template> <count>

bitterblossom sprite create --template weaver --name bb-builder-2
bitterblossom sprite clone bb-builder --name bb-builder-3
bitterblossom sprite start bb-builder
bitterblossom sprite pause bb-builder [--wait]
bitterblossom sprite resume bb-builder
bitterblossom sprite stop bb-builder
bitterblossom sprite destroy bb-builder-3
bitterblossom sprite logs bb-builder [--follow] [--lines N]
bitterblossom sprite status bb-builder [--json]
```

Compatibility:

- Keep `mix conductor ...` as a temporary compatibility wrapper.
- Move the documented operator surface to `bitterblossom ...`.

## Technical Exploration

### Option A: Thin Elixir CLI over existing deep modules plus remote state files

Reuse:

- `Conductor.Sprite` for transport, exec, provision, auth sync, logs, probe
- `Conductor.Fleet.Loader` as the basis for template loading

Add:

- detached loop launch primitive
- remote runtime state files under `/home/sprite/.bitterblossom/`
- template-oriented fleet schema
- imperative lifecycle subcommands

Delete or retire:

- `Conductor.Application`
- `Conductor.Fleet.HealthMonitor`
- `Conductor.Launcher` as the supported runtime path
- dashboard-first operator assumptions

Why this is best:

- Keeps the working Elixir code and tests.
- Deletes coordinator policy instead of re-platforming it.
- Matches the repo's recent direction: thin infrastructure, agent-owned
  judgment.

### Option B: Reintroduce a separate Go `bb` binary

Why reject it:

- The repo just deleted the Go CLI and consolidated on Elixir.
- Reintroduces two implementation languages for the same surface.
- Buys no architectural advantage for this problem.

### Option C: Shell wrappers over `sprite`

Why reject it:

- Repeats the earlier process-handling and testability mistakes.
- Makes detached lifecycle semantics and truthful status harder, not easier.

## Recommended Technical Design

### 1. Convert `fleet.toml` from instance inventory to template catalog

Replace `[[sprite]]` entries with template-oriented entries such as
`[[template]]`.

Each template defines:

- `key`
- `role`
- `name_prefix`
- `repo`
- `persona` or `persona_ref`
- `harness`
- `model`
- `reasoning_effort`
- `capability_tags`

Example:

```toml
[defaults]
org = "misty-step"
repo = "misty-step/bitterblossom"
harness = "codex"
model = "gpt-5.4"
reasoning_effort = "medium"

[personas]
fern = "You are Fern..."

[[template]]
key = "weaver"
role = "builder"
name_prefix = "bb-builder"
persona = "You are Weaver..."

[[template]]
key = "thorn"
role = "fixer"
name_prefix = "bb-fixer"
persona = "You are Thorn..."

[[template]]
key = "fern"
role = "polisher"
name_prefix = "bb-polisher"
persona_ref = "fern"
reasoning_effort = "high"
```

### 2. Add a remote instance contract

Each sprite should carry its own Bitterblossom metadata under
`/home/sprite/.bitterblossom/`.

At minimum:

- `instance.json`: static metadata written at create/provision time
- `state.json`: mutable lifecycle state
- `paused`: marker file for pause state
- `loop.pid`: pid of the active loop wrapper
- `loop.log`: canonical log path

Suggested `state.json` fields:

- `template_key`
- `role`
- `repo`
- `status` (`running`, `idle`, `paused`, `stopped`, `failed`)
- `started_at`
- `last_exit_at`
- `last_exit_reason`
- `pid`
- `workspace`

Truth rule:

- CLI status must derive from both the remote state files and a live `pgrep`
  check so stale pid files do not lie.

### 3. Introduce a detached loop wrapper on the sprite

Current `Conductor.Sprite.dispatch/4` is synchronous. That is wrong for a CLI
whose `start` command should return immediately.

Add a sprite-local wrapper that:

- writes and updates runtime state
- launches the codex loop detached
- records pid and log path
- exits cleanly when paused/stopped

This wrapper should be deliberately small. It is not a new conductor. It is the
minimum self-management needed to make each sprite independently operable.

### 4. Split lifecycle primitives from old conductor boot logic

Keep deep primitives:

- `probe`
- `status`
- `provision`
- `wake`
- `logs`
- `kill`

Add new primitives:

- `create`
- `clone`
- `start_loop`
- `stop_loop`
- `pause`
- `resume`
- `inspect_instance`
- `discover_instances`

Retire unsupported primitives:

- app boot
- fleet-wide restart supervision
- background health monitor
- dashboard as the primary operator surface

### 5. Make capacity a computed surface, not a daemon feature

`fleet status` should aggregate discovered instances by template/role:

- total
- reachable
- healthy
- paused
- running
- idle
- broken

Available capacity is computed as:

- reachable
- healthy
- not paused
- not currently running a loop

No background service is required to answer this.

## Recommendation Against Overreach

Do not build these in the first slice:

- a new central API server
- a web dashboard replacement
- multi-operator distributed state
- workflow judgment about which issues to work
- Canary-specific behavior in the CLI
- interactive template authoring from the CLI

New agent types should be added by editing `fleet.toml`. The CLI's first job is
to materialize and manage instances from those templates cleanly.

## Context Packet

## Goal

Turn Bitterblossom into a thin, agent-first CLI that manages live sprite
instances for a repo-owned template catalog, while pushing loop ownership and
runtime truth down onto each sprite.

## Non-Goals

- Rehost the existing conductor on a remote always-on machine.
- Preserve `start` as "boot a coordinator that manages the fleet for you."
- Reintroduce a second Go control plane.
- Build a generic fleet-management framework outside Bitterblossom's sprite
  domain.
- Make the CLI responsible for issue routing, review judgment, or merge policy.
- Add a new central database or API server just to preserve desired counts.

## Constraints / Invariants

- Sprites own their own loops; Bitterblossom owns CRUD and observability.
- The supported operator surface must be CLI-first.
- Status must be truthful without requiring `start` to have been run earlier in
  the same shell or VM.
- Pause/resume/start/stop semantics must be explicit and non-overlapping.
- Existing deep sprite transport/provisioning logic should be reused where it is
  already correct.
- Repo-owned template definitions remain the source of persona and harness
  policy.
- Multiple instances of the same template must be a normal case, not a special
  branch.

## Authority Order

tests > type system > code > docs > lore

## Repo Anchors

- `conductor/lib/conductor/sprite.ex` — existing deep sprite transport and
  provisioning boundary worth keeping
- `conductor/lib/conductor/fleet/loader.ex` — current fleet schema/parser that
  should become template-oriented
- `conductor/lib/conductor/cli.ex` — current operator surface to reshape around
  imperative fleet/sprite commands
- `conductor/lib/conductor/application.ex` — central boot/restart behavior to
  retire
- `conductor/lib/conductor/launcher.ex` — synchronous launch model and
  coordinator-era workspace reset behavior to replace
- `fleet.toml` — current static instance inventory to convert into a template
  catalog
- `docs/archive/SPRITES.md` — prior art for per-sprite pid/state/log files

## Prior Art

- Current repo: `Conductor.Sprite` already proves the useful deep module is the
  sprite boundary, not the application supervisor.
- Current repo: archive `bb agent` material shows a viable per-sprite pid/state
  contract without a central control loop.
- Kubernetes command model: `scale` for instance count, `cordon`/`drain` style
  pause semantics for maintenance rather than a monolithic "stop everything"
  command.
- Underlying transport: local `sprite` CLI already exposes `create`, `list`,
  `exec`, `checkpoint`, `restore`, and `destroy`; Bitterblossom should layer
  policy and truth surfaces on top of that, not duplicate it.

## Oracle (Definition of Done)

This context packet describes the full direction. The first shippable slice is
smaller: truthful fleet inspection plus detached lifecycle control for declared
`[[sprite]]` entries. Template catalog, clone/create/destroy, and scale remain
follow-up work.

- [ ] `cd conductor && mix test test/conductor/cli_fleet_test.exs test/conductor/sprite_test.exs test/conductor/fleet/loader_test.exs test/conductor/sprite_agent_test.exs`
- [ ] `bitterblossom fleet status --json` exits 0 without any prior `start`
      command and reports truthful `reachable`, `healthy`, `paused`, and
      `running` fields for each discovered instance.
- [ ] `bitterblossom sprite start <instance>` returns after launching a detached
      remote loop, and `bitterblossom sprite status <instance> --json` reports
      `status=running`.
- [ ] `bitterblossom sprite pause <instance> --wait` leaves the instance
      `paused=true` and `status!=running`.
- [ ] `bitterblossom sprite stop <instance>` stops the current loop without
      changing pause state.

## Implementation Sequence

1. Add truthful inspection fields in `Conductor.Sprite`: `paused`, `busy`,
   `loop_pid`, lifecycle status.
2. Implement detached loop lifecycle primitives for declared sprites: start,
   stop, pause, resume.
3. Rework the CLI around `fleet status|audit` and `sprite
   status|start|stop|pause|resume|logs`, while keeping compatibility aliases for
   the current surface.
4. Update docs so the root architectural story matches this additive sprint-1
   slice.
5. Follow with template catalog, create/clone/destroy/scale, then retire the
   coordinator-first `start` story.

## Risk + Rollout

- Biggest risk: detached remote lifecycle becomes another brittle PID-file
  system.
  Mitigation: keep one tested wrapper, derive truth from live process checks,
  and avoid scattered shell snippets.
- Biggest product risk: fuzzy pause/stop semantics confuse operators.
  Mitigation: define `pause` as unschedulable, `pause --wait` as maintenance,
  and `stop` as immediate loop termination.
- Biggest migration risk: existing docs and tests still describe a
  conductor-first system.
  Mitigation: ship the new CLI surface behind compatibility wrappers first, then
  delete old docs once parity is proven.
- Rollout: land the new sprite lifecycle and status surfaces first, then move
  scale/create/clone, then retire the old `start` story.
