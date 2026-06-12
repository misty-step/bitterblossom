# Let target repos own their workload config (.bb/ discovery)

Priority: P2 · Status: done · Estimate: M

## Goal

A repo declares its own reflexes — task spec + lane card in a `.bb/`
directory versioned with the code they govern — and the plane discovers
and runs them within plane-granted bounds, so adding a workload to a
project is a commit to that project, not an edit to the plane.

## Oracle

- [x] A repo containing `.bb/tasks/<name>/{task.toml,card.md}` gets its
      task loaded by a plane that lists that repo in an explicit
      allowlist; `bb check` shows the discovered task with its source
      repo
- [x] The trust boundary is enforced in code: agent bindings, model/auth
      policy, budget ceilings, and substrate choice remain plane-owned —
      a repo-side task.toml that names an unknown agent, exceeds a
      plane-set budget cap, or requests `substrate = "local"` fails
      `bb check` with an error naming the repo
- [x] Repo config changes take effect without plane restarts (refresh on
      dispatch or a bounded cache), and the ledger records which repo+ref
      supplied the config for every run
- [x] Removing the repo from the plane allowlist removes its tasks —
      no orphaned triggers

## Notes

**Why:** the strongest steal from Ona (2026-06-11 research) — their
`automations.yml` lives in the target repo, which is what makes "agent
fleets across your codebase" scale past one project. Bitterblossom
centralizes everything in `plane/tasks/`, which is right for one repo
and wrong for twenty. **The load-bearing design issue is the trust
split**: the repo owns *what to do* (cards, triggers, filters); the
plane owns *what it may spend and as whom it runs* (agents, models,
auth, budgets, substrate). A repo must never be able to raise its own
budget or bind a subscription agent — that inverts the policy-as-code
arc (036). Depends loosely on 040 (multi-repo is where this pays);
shapeable independently.

## Evidence (2026-06-12)

- `docs/spine.md` documents `[[workload_repo]]`: local config checkout
  `path`, optional workspace `repo_url`, plane-owned agent/substrate/
  workspace/budget caps, and repo-owned `.bb/tasks/*`.
- `tests/repo_workloads.rs` covers discovery, `bb check` source output,
  `repo_url` workspace override, trust-boundary failures, config refresh,
  allowlist removal, success-run source recording, and budget-blocked-run
  source recording.
- Live CLI smoke: temp repo under `target/repo-workload-qa.hu92DF`, run
  `01666c450528` succeeded; `bb check` showed `target/review` with
  `source=.../target@main`; `bb runs show` printed the same source.
- Gate: `./scripts/verify.sh` green; final `src LOC: 4952`.
- Fresh cross-model review: Grok found URL checkout hot-path blockers;
  fixed by rejecting config `url` in v1 and adding `repo_url` for
  workspace clones. Final Grok re-review reported no remaining blockers
  or important findings.
