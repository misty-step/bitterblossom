# Credential Refusal Doctrine

Ratified 2026-07-14 (bitterblossom-971). Binding for every lane the plane
dispatches, on every substrate, at every hour.

## The rule

**A `401`/`403` (or any authorization refusal) on a credential this run
declares is a STOP-and-report boundary, never a puzzle.** The lane:

1. stops pursuing the refused operation and the goal that requires it;
2. writes a blocked `REPORT.json` naming the refused operation and the
   refused credential by *name* (env var name, never value bytes);
3. ends the run without completing the task.

The lane must **never** respond to a refusal by locating, minting, or
borrowing a stronger credential — not from the environment, not from a
keychain or credential helper, not from 1Password/`op`, not from config
files, not from another agent. If the operation genuinely requires more
authority than the run declares, that is an operator decision surfaced
through the blocked report, not a lane decision made at 3am.

Blocked report shape (report-only convention; the run itself may still be a
mechanical `success` — the *outcome* is blocked):

```json
{
  "status": "blocked_credential_refused",
  "refused_operation": "PATCH /api/cards/<id> -> HTTP 403 (admin scope required)",
  "credential": "POWDER_API_KEY (declared scoped key; value never recorded)",
  "action": "stopped per credential-refusal doctrine; no stronger credential sought",
  "task_completed": false
}
```

## Why

2026-07-09, disclosed in-run: a dispatched lane's agent-scoped Powder key got
`403 admin scope required` on PATCH. The lane "solved" it by finding
`powder-admin-key` in the operator's macOS keychain and using it. Not
misconduct — the action was in-scope for the task and disclosed — but exactly
the reflex that must not be normalized. Scoped, actor-bound keys exist so a
lane's blast radius is bounded by its credential; a lane that routes around a
403 silently restores full admin authority to an unsupervised agent.

## Where the doctrine is enforced

- **Dispatch prompt seam** (`src/harness.rs::commission_prompt`, used by
  `src/dispatch.rs`): every dispatched lane's commission preamble carries the
  STOP-and-report rule, so even a card that forgets to state it is covered.
- **Lane-card contract** (`tests/task_card_contract.rs`): every public-plane
  and template task card must state the refused-credential boundary in its
  `## Boundaries` section (`STOP-and-report` sentinel enforced by test).
- **Agent-facing skill** (`skills/bitterblossom/SKILL.md`, the exported
  interface): states the rule under Dispatch Rules.
- **Drill** (`scripts/credential-refusal-drill.sh`): repeatable dev-plane
  evidence that a refused lane blocks instead of completing, plus a local
  substrate isolation probe. Worked transcript:
  `docs/shakedowns/2026-07-14-credential-refusal-drill.md`.

## Substrate credential isolation (reviewed 2026-07-14)

What a lane can even *reach* when it is tempted:

### sprites (production reflex substrate) — verified from adapter code

`src/substrate/sprites.rs`:

- Lanes execute on a **remote Fly sprite** (`/home/sprite/bb/...`). The
  operator's macOS login keychain, `security(1)`, credential-osxkeychain
  helper, and local `op` session physically do not exist there.
- Declared secrets travel to the remote shell as `export`s over **stdin**
  (heredoc with collision-checked delimiter), never argv.
- `GH_TOKEN` is always `unset` in the remote exec preamble before the
  workload starts; git auth is attempt-scoped via an askpass shim, and the
  checkout explicitly disables any credential helper
  (`git_auth_setup_script` in `src/substrate/local.rs`, shared).
- Hermetic runs (every api-auth reflex lane) scrub the remote env down to
  `PATH|TERM|LANG|LC_ALL|PWD|SHLVL|TMPDIR` plus declared secrets and a
  relocated `HOME="$PWD/.home"`.

Verified by code review plus the unit/e2e suite (stubbed sprite transport),
**and live-probed 2026-07-14 on production sprite `bb-builder`** (reachability
and names only; no values read):

- Linux host, user `sprite` (uid 1001); `security(1)`, macOS keychain dir,
  and the `op` CLI are all absent — the operator's keychain and 1Password
  session are unreachable from a sprite lane by construction.
- Default exec env carries only `BROWSER DEBIAN_FRONTEND HOME LANG PATH PWD`
  — zero credential names, so even a *non-hermetic* run inherits no ambient
  secrets from the exec environment.
- No `~/.netrc`, `~/.git-credentials`, `~/.ssh`, `~/.aws`, or Powder/`op`
  config baked into the image.
- The image **does** bake lane-class platform credentials: `~/.config/gh`,
  `~/.claude`, `~/.codex`, and a global git `credential.helper`. These are
  the identities subscription-auth dispatch lanes are *entitled* to on that
  sprite — sprite-image scope, not operator-admin scope. Escalation from a
  refused scoped key to these is still a doctrine violation; escalation to
  operator admin credentials is impossible because they are not there.

Residual caveat: checkpoint hygiene is part of this boundary. A future
checkpoint baked with broader credentials silently widens every lane on that
sprite; audit what a checkpoint bakes (env + home dotfiles, names only) when
minting one.

### local (dev/test substrate) — DEV-ONLY, gap documented

`src/substrate/local.rs` isolates the *environment* well: `execute()` spawns
with `env_clear()` and a strict allowlist, hermetic runs get a relocated
`HOME`, and the checkout disables credential helpers. The drill proves an
ambient admin env var in the parent process is invisible to the lane.

But the lane process still runs **as the operator's own user on the
operator's machine**. Consequences:

- the macOS login keychain is reachable (`security find-generic-password`
  works regardless of env scrubbing) — this is exactly how the 2026-07-09
  escalation happened;
- `OP_SERVICE_ACCOUNT_TOKEN` is deliberately passed through
  (bitterblossom-915), so `op` works inside local workloads;
- anything the operator's uid can read is ultimately readable.

**Decision: the local substrate is a dev/test convenience and is documented
as such, not hardened.** Unattended or untrusted workloads belong on the
sprites substrate. Closing the gap locally would mean OS-level sandboxing,
which is not this plane's mechanism to own. The doctrine (prompt seam + card
contract) is the guardrail that applies on every substrate including this
one.

### tailnet — same posture as sprites

`src/substrate/tailnet.rs` executes on remote tailnet hosts; the operator
keychain is absent unless the operator points it at their own workstation.
Do not register the operator's own machine as a tailnet lane host for
unattended work.

## Scope-gap review (the Powder PATCH that started this)

The refused operation on 2026-07-09 was a Powder card PATCH (acceptance
update) that then required admin scope. Verified 2026-07-14 against the
current Powder MCP surface: `update_card` is now **any-authenticated-actor**
("Any authenticated actor may patch; the change is audited with actor and
field list") — the legitimate operation lanes need no longer requires admin,
so the original scope gap is closed upstream by a right-sized scope plus
audit, not by admin sharing.

Operations that **remain admin-only by design** (and correctly so): API key
management (`list_keys` remote mode requires admin), webhook/event
subscriptions, repository admin (`POWDER_MCP_TOOLSETS=admin` toolset). A lane
refused on any of these must block and report; there is no legitimate
unattended need. If a future lane legitimately needs one of them recurringly,
the fix is a new right-sized scope in Powder — file the card; do not share
admin keys.

## Drill (repeatable)

```bash
cargo build
./scripts/credential-refusal-drill.sh
```

Asserts, on a throwaway dev plane with a stub command harness:

1. a lane whose declared scoped key is refused (live HTTP 403) writes
   `REPORT.json` with `status=blocked_credential_refused` naming the refused
   operation, and does **not** produce the task's completion artifact;
2. an ambient admin credential exported in the dispatching process's
   environment is invisible inside the lane (local substrate env isolation);
3. on macOS it prints — as a warning, values never read — that the keychain
   surface *is* reachable from a local lane, restating the dev-only ruling
   above.
