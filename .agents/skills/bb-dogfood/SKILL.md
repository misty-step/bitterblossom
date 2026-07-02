---
name: bb-dogfood
description: |
  Dogfood Bitterblossom by using `bb` itself to deliver a Bitterblossom
  backlog PR, then critique the build, submission, gate, ledger, and operator
  UX. Use when the user asks to "dogfood Bitterblossom", "bb-dogfood",
  "work through the Bitterblossom backlog with bb", "use bb on itself",
  "capture Bitterblossom friction", "run the bb backlog loop", or "test the
  event plane as its primary user". Trigger: bb-dogfood.
argument-hint: "[backlog-id|backlog-path] [--dry-run]"
---

# bb-dogfood

Use Bitterblossom as the operating surface for Bitterblossom work. You are the
primary user. `bb run build` is the authoring surface under test; local edits
afterward are for review, dogfood notes, and synthesized backlog.

## Load First

Also load `../../../VISION.md` for the product boundary and
`../../../skills/bitterblossom/SKILL.md` for the base `bb` command contract. Use
`references/session-notes-template.md` when starting or updating a dogfood notes
artifact. Use `references/ux-review-card.md` before reflecting into backlog.

## Preflight

Work from `/Users/phaedrus/Development/bitterblossom` unless the user says
otherwise.

Run:

```bash
export BB_RUNTIME_PLANE="${BB_RUNTIME_PLANE:-/path/to/private/plane}"
git status --short --branch --untracked-files=all
flyctl orgs list
sprite org list
sprite use -o misty-step lane-1
sprite org list
sprite exec -- whoami
./target/debug/bb --config "$BB_RUNTIME_PLANE" check
./target/debug/bb --config "$BB_RUNTIME_PLANE" status --json
./target/debug/bb --config "$BB_RUNTIME_PLANE" task list --json
./target/debug/bb --config "$BB_RUNTIME_PLANE" runs list --json
./target/debug/bb --config "$BB_RUNTIME_PLANE" dlq list --json
```

Hard requirements:

- Sprite org must be `misty-step` before any remote lane or `bb` dispatch.
  `sprite org list` can show another selected org; fix with
  `sprite use -o misty-step lane-1` and verify with `sprite exec -- whoami`.
- The production plane is instance data, not checked into this repo. Do not
  assume `--config plane`; require `BB_RUNTIME_PLANE` or the service's
  `BB_PLANE_DIR` runtime environment.
- Do not unpark a task just to make a gate run. Read the parked reason and
  record what condition would make unpark safe.
- Do not run verdict tasks directly with arbitrary payloads. They need a
  submission id.
- Keep the tree clean between dogfood slices; every visible path is an action.

## Backlog Loop

1. Read active `backlog.d/` and choose the highest leverage ready item unless
   the user named one.
2. Create or update a dogfood notes artifact under `docs/plans/` using the
   template. Notes are part of the deliverable, not a side channel.
3. Dispatch the builder through Bitterblossom. This is the product surface
   being tested:

```bash
GH_TOKEN=$(gh auth token) ./target/debug/bb --config "$BB_RUNTIME_PLANE" run build \
  --payload '{"repo":"misty-step/bitterblossom","backlog":"<backlog-id-or-path>","branch_slug":"<slug>","dry_run":false}' \
  --json
```

4. Read the run bundle and artifact `REPORT.json`. If status is `blocked` or
   `failed`, do not quietly implement locally. Record the blocker as dogfood
   evidence, decide whether it is backlog-worthy, and stop unless the user
   explicitly asks for a fallback.
5. Fetch and check out the pushed `bb/build/<slug>` branch locally. Review the
   diff, run the local gate, and add only dogfood notes/backlog emissions as a
   separate commit when they are part of the run:

```bash
git fetch origin
git checkout <branch-from-report>
./scripts/verify.sh
```

6. Open or update a draft PR. The PR is reviewable product evidence, not a
   merge request by default:

```bash
gh pr create --draft --base master --head "<branch-from-report>" \
  --title "<title>" --body-file "<body-file>"
```

If the PR already exists, update its body with the dogfood evidence instead of
opening a duplicate.

7. Open a submission and run the gate through `bb`:

```bash
./target/debug/bb --config "$BB_RUNTIME_PLANE" submit open \
  --change "<pr-url-or-branch>" \
  --rev "<git-sha>" \
  --context "<short dogfood context>" \
  --json
```

Then run required members with the returned submission id. Use `GH_TOKEN`
for the whole storm unless you have checked the task specs and know a member
does not need it; even command-backed verifier tasks may shell out to GitHub:

```bash
GH_TOKEN=$(gh auth token) ./target/debug/bb --config "$BB_RUNTIME_PLANE" run verify \
  --idempotency-key "storm:<submission>:verify" \
  --payload '{"submission":"<submission>","repo":"misty-step/bitterblossom","rev":"<git-sha>","change":"<change-key>"}' \
  --json
```

Repeat for `correctness`, `security`, `simplification`, and `product` only
when those tasks are unparked and the plane is healthy.

Evaluate:

```bash
./target/debug/bb --config "$BB_RUNTIME_PLANE" gate --submission <submission> --json
```

If a required member is parked, blocked, or unavailable, record that as
dogfood evidence and use the local gate plus available members. Do not hide the
gap.

## UX Critique

Every command is part of the product surface. Take notes while the run is hot:

- `Good:` what reduced effort, improved confidence, or made state visible.
- `Bad:` awkward but recoverable friction, unclear ordering, extra steps.
- `Ugly:` wrong behavior, misleading output, hidden state, dead ends.
- `Friction:` exact step that slowed you down.
- `Bug:` behavior that contradicts the documented contract.
- `Delight:` useful result or interaction worth preserving.

Answer the `references/ux-review-card.md` questions explicitly enough that a
future implementer can improve the product without replaying chat context.

Reflect into backlog only after the delivery evidence is read. Backlog-worthy
means: the issue affects the event-plane vision in `VISION.md`, repeats beyond
one operator mistake, and has an oracle. Prefer one small `backlog.d/NNN-*.md`
per product improvement. Do not create backlog for taste notes with no fix; put
those under "No action".

## Notes Contract

For every dogfood run, capture:

- exact `bb` binary and plane path;
- Sprite org/account state and sprite name;
- backlog item, build run id, PR URL, commit, and submission id;
- commands run, run ids, status output, gate output, costs, parked tasks, and DLQ state;
- good, bad, ugly, friction, bugs, missing affordances, and delightful moments;
- whether each observation becomes backlog, docs, or no action.

Prefer concrete phrasing:

- `Friction:` what slowed or confused you.
- `Bug:` wrong behavior or misleading output.
- `Delight:` something that made the workflow better.
- `Lean in:` a positive pattern to preserve or expand.
- `Mitigate:` a concrete fix or backlog reference.
- `Reflect into backlog:` exact new or updated backlog item, or "no action"
  with a reason.

## Closeout

End with:

```bash
git status --short --branch --untracked-files=all
git rev-list --left-right --count master...origin/master
./target/debug/bb --config "$BB_RUNTIME_PLANE" task list --json
./target/debug/bb --config "$BB_RUNTIME_PLANE" dlq list --json
```

Report final verification, dogfood findings, residual parked/DLQ state, and
the next backlog item.
