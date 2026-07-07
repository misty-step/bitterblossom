---
name: ci
description: |
  Audit, design, and run repo-owned CI gates. Host-agnostic by default:
  local, GitHub Actions, Azure, or another runner should call the same
  repo-owned contract. Roster's own gate is fmt/clippy/test plus
  `cargo run --locked -p roster-cli -- check`; consumer repos
  keep their own native gate. Use when: "run ci", "check ci", "fix ci",
  "audit ci", "design CI", "host-agnostic CI", "Dagger", "is ci passing",
  "run the gates", "why is ci failing", "strengthen ci", "tighten ci",
  "ci is red", "gates failing", "feedback loop is slow", "run gates less often".
  Trigger: /ci, /gates.
argument-hint: "[--audit-only|--run-only]"
---

# /ci

Confidence in correctness without turning local work into a provider or Docker
tax.

Roster's own source-repo gate is:

```sh
cargo fmt --all -- --check
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace
cargo run --locked -p roster-cli -- check
```

The Rust checks run in `.github/workflows/ci.yml`; `roster check` (deterministic
frontmatter/path/index/conflict-marker gate over `primitives/`) is implemented
at `crates/roster-cli/src/check.rs`. This is roster's own maintenance plumbing,
not a CI framework to project into every consumer repo.

When `/ci` runs in a consumer repo, do not assume roster's Rust gate is
installed there. Read that repo's root instructions, package manifests, CI
workflows, hook config, and shipped scripts, then strengthen the repo-owned
gate. Roster supplies the agent judgment for CI design; the consumer repo
owns the implementation.

For CI architecture or Dagger decisions, load
`references/host-agnostic-ci.md`. The invariant is repo-owned contract first,
execution substrate second.

Consumer repos should have a two-tier gate unless live evidence proves one
loop is both strong and fast:

- **Fast local gate:** pre-commit/pre-push should run deterministic checks an
  agent will tolerate during amend/push cycles: formatting, changed-path lint,
  typecheck, focused or changed tests, shell syntax, no-local-ticket/backlog
  bans, and cheap secret scans when available.
- **Full ship gate:** expensive Docker, Dagger, browser, network, mutation,
  provider, full-coverage, and live-readiness checks stay required at PR/main,
  deploy, or explicit `ship-check` time.

If a consumer repo's behavioral gate needs supported third-party APIs, prefer
an `emulate.dev`-backed local/CI lane over weakening the gate, hand-written
mocks, or live networked sandboxes. Keep truly networked provider checks in the
full ship gate only when real provider behavior is the point. Usage details:
https://emulate.dev/docs.

What to gate on — not just where each gate runs — follows the standing quality
floor in `primitives/shared/references/quality-gates.md`: gate the diff not the
legacy baseline, hard-block the Goodhart-resistant behavioral set (tests,
diff-coverage, mutation, supply-chain, secrets), ratchet structural debt
(god-files, duplication, dead code) so legacy only improves, and keep gameable
metrics as reports. Every gate names the real failure it catches; default to
free/OSS or a homebrew tripwire, never a paid SaaS forced on a consumer.

Moving work out of pre-push is only valid when the same invariant remains
required before merge or deploy. If a required GitHub check is path-filtered,
add a sentinel/split-check design; skipped required workflows can leave PRs
stuck pending.

Each gate should leave useful context, not just an exit code: run duration,
critical path, cache behavior where available, test/coverage reports, perf
benchmarks when relevant, security findings, artifact hashes, and residual
unverified paths. CI output is agent context for the next run.

## Modes

- Default: audit the gate surface, fix mechanical gaps, then run the repo-owned
  gate.
- `--audit-only`: produce audit report and gap proposals; do not run gates.
- `--run-only`: skip audit, just drive the repo-owned gate green.

## Stance

1. **Repo-owned contract first.** The gate is a command, script, Dagger
   function, Make/Just/Task target, or build-system target the repo owns. Hosted
   providers call it; they do not define it.
2. **Fast enough to use.** A default local gate that routinely takes many
   minutes for harness/docs changes is a design failure. Keep expensive,
   networked, mutation, browser, provider, and experimental checks opt-in or
   path-scoped; use local emulation when it preserves provider-shaped behavior
   without network.
3. **Dagger earns its place.** Keep Dagger when portability, containerized
   dependencies, caching, service orchestration, or traceability outweighs its
   startup/debug cost. Do not use Dagger merely to wrap ordinary lint,
   typecheck, unit test, and build commands in the inner loop.
4. **No quality lowering.** Removing, splitting, or moving Dagger is not
   permission to remove the invariant. Preserve checks in the fast gate, full
   gate, or explicit ship gate.
5. **Act, do not propose.** Mechanical strengthenings are applied directly.
   Escalate only when the choice is product scope, not CI plumbing.
6. **Fix-until-green on self-healable failures.** Formatting drift, stale
   generated docs/index, and trivial lints get fixed. Logic failures get a
   precise diagnosis.
7. **Security floor is part of CI.** A credible repo gate prevents or fails on
   secret leaks in source files, generated artifacts, logs, and Git/PR
   metadata. Commit subjects/bodies, PR titles/bodies, release notes, and
   agent-generated summaries are in scope. Prefer server-side push protection
   or pre-receive hooks when available; otherwise require repo hooks plus CI.
8. **Reports are product.** A strong gate emits reviewable artifacts a future
   agent can use: run digest, test reports, coverage/diff-coverage, mutation or
   fuzz survivors where relevant, perf deltas, security findings, and produced
   artifact checksums.

## Delegation Judgment

Delegate per the shared Roster contract (shared AGENTS.md: Roster).

Local lane guidance: Each lane states responsibilities, context boundary,
output evidence, and lead verification. Direct work is limited to mechanical
repair and emergency preservation; the lead owns synthesis.

## Audit

Check the live gate surface:

- Roster only: root contract names
  `cargo fmt --all -- --check && cargo clippy --workspace --all-targets -- -D
  warnings && cargo test --workspace && cargo run --locked -p roster-cli --
  check`; `.github/workflows/ci.yml` runs the Rust checks; `check.rs` contains
  the `roster check` lane list; `skills-index.yaml` is current after
  skill/primitive changes.
- Secret scanning covers both committed content and metadata that never appears
  in the working tree: commit message file, outbound commit range, PR title/body,
  and release/changelog text. The report must redact matched values.

For non-roster repos, replace the roster-specific bullets above with
that repo's equivalent gate contract, then apply the same security floor.
Also check:

- Local hooks run the fast gate, not the full ship gate, unless the full gate is
  proven fast enough for repeated pushes.
- The full ship gate is still required in CI/merge/deploy protection.
- There is an explicit command for humans/agents to run the full gate locally
  before marking a PR ready or merging.
- The same repo-owned contract can run locally and from GitHub Actions, Azure,
  or another runner without provider-specific rewrites.
- CI cancels stale PR runs where safe, but deploy/main runs do not get
  interrupted mid-release.
- Reports/artifacts are durable enough for a later agent to diagnose failures
  without chat context.

## Run

For roster, run:

```sh
cargo fmt --all -- --check
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace
cargo run --locked -p roster-cli -- check
```

For consumer repos, run the repo-owned gate discovered in the audit. If none
exists, the finding is `high`: design the smallest native gate before claiming
CI is meaningful.

If red:

- Fix deterministic generated drift.
- Run focused tests for the failing Rust module.
- Re-run the aggregate gate.
- Stop after three self-heal attempts per gate and report the exact failing
  command, file/path, and likely cause.

## Completion Gate

See `primitives/shared/AGENTS.md` (Completion Evidence) for the shared core.
`/ci` adds:

- **Audit:** gaps found, severity, substrate choice, what was strengthened,
  what was deferred.
- **Run:** gate command, pass/fail, self-heals, escalations.
- **Evidence:** reports/artifacts generated or missing: test, coverage,
  performance, security, build artifacts, traces/logs.

Never claim green from a provider status alone. Name the repo-owned command,
function, target, or artifact that proved the behavior.
