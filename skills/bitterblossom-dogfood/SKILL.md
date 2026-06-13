---
name: bitterblossom-dogfood
description: |
  Dogfood Bitterblossom by using `bb` itself to work through the
  Bitterblossom backlog, especially backlog.d delivery, submission storms,
  verdict gates, Fly Sprite execution, and operator UX notes. Use when the
  user asks to "dogfood Bitterblossom", "work through the Bitterblossom
  backlog with bb", "use bb on itself", "capture Bitterblossom friction",
  "run the bb backlog loop", or "test the event plane as its primary user".
---

# Bitterblossom Dogfood

Use Bitterblossom as the operating surface for Bitterblossom work. You are the
primary user. Capture delight, friction, bugs, and missing affordances while
you work, then fold the useful observations into backlog or docs.

## Load First

Also load `../bitterblossom/SKILL.md` for the base `bb` command contract.
Use `references/session-notes-template.md` when starting or updating a dogfood
notes artifact.

## Preflight

Work from `/Users/phaedrus/Development/bitterblossom` unless the user says
otherwise.

Run:

```bash
git status --short --branch --untracked-files=all
flyctl orgs list
sprite org list
sprite use -o misty-step lane-1
sprite org list
sprite exec -- whoami
./target/debug/bb --config plane check
./target/debug/bb --config plane status --json
./target/debug/bb --config plane task list --json
./target/debug/bb --config plane runs list --json
./target/debug/bb --config plane dlq list --json
```

Hard requirements:

- Sprite org must be `misty-step` before any remote lane or `bb` dispatch.
  `sprite org list` can show another selected org; fix with
  `sprite use -o misty-step lane-1` and verify with `sprite exec -- whoami`.
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
3. Deliver the slice in the local checkout.
4. Run the local gate:

```bash
./scripts/verify.sh
```

5. Commit and push the branch or master change as appropriate for the run.
6. Open a submission and run the gate through `bb`:

```bash
./target/debug/bb --config plane submit open \
  --change "<change-key>" \
  --rev "<git-sha>" \
  --context "<short dogfood context>" \
  --json
```

Then run required members with the returned submission id. Use `GH_TOKEN`
for the whole storm unless you have checked the task specs and know a member
does not need it; even command-backed verifier tasks may shell out to GitHub:

```bash
GH_TOKEN=$(gh auth token) ./target/debug/bb --config plane run verify \
  --idempotency-key "storm:<submission>:verify" \
  --payload '{"submission":"<submission>","repo":"misty-step/bitterblossom","rev":"<git-sha>","change":"<change-key>"}' \
  --json
```

Repeat for `correctness`, `security`, `simplification`, and `product` only
when those tasks are unparked and the plane is healthy.

Evaluate:

```bash
./target/debug/bb --config plane gate --submission <submission> --json
```

If a required member is parked, blocked, or unavailable, record that as
dogfood evidence and use the local gate plus available members. Do not hide the
gap.

## Notes Contract

For every dogfood run, capture:

- exact `bb` binary and plane path;
- Sprite org/account state and sprite name;
- backlog item, commit, and submission id;
- commands run, run ids, status output, gate output, costs, parked tasks, and DLQ state;
- friction, bugs, missing affordances, and delightful moments;
- whether each observation becomes backlog, docs, or no action.

Prefer concrete phrasing:

- `Friction:` what slowed or confused you.
- `Bug:` wrong behavior or misleading output.
- `Delight:` something that made the workflow better.
- `Lean in:` a positive pattern to preserve or expand.
- `Mitigate:` a concrete fix or backlog reference.

## Closeout

End with:

```bash
git status --short --branch --untracked-files=all
git rev-list --left-right --count master...origin/master
./target/debug/bb --config plane task list --json
./target/debug/bb --config plane dlq list --json
```

Report final verification, dogfood findings, residual parked/DLQ state, and
the next backlog item.
