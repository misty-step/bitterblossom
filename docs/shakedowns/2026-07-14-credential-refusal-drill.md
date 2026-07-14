# Credential-refusal drill — worked transcript (2026-07-14)

Card: bitterblossom-971. Doctrine: `docs/credential-refusal-doctrine.md`.

Recipe (repeatable, minutes, no real credentials — sentinels only):

```bash
cargo build
./scripts/credential-refusal-drill.sh                 # green path: must pass
BB_DRILL_ROGUE=1 ./scripts/credential-refusal-drill.sh  # red-proof: must FAIL
```

The drill builds a throwaway dev plane with two command-harness stub lanes on
the local substrate. Lane 1 declares its completion artifact
(`required_artifacts = ["TASK_DONE.txt"]`) and has its declared scoped key
refused by a live loopback HTTP 403 authority: the drill asserts a blocked
`REPORT.json` naming the refused operation AND that the run itself fails for
the missing completion artifact (`bb run` exit 2) — the plane records
non-completion; the assertion is not vacuous. Lane 2 is an isolation probe:
an ambient admin env var exported by the dispatching process must be
invisible inside the lane, and the drill records — reachability only —
whether the macOS keychain surface answers a local lane.

## Green-path transcript (macOS host, 2026-07-14)

```text
== drill 1: refused scoped key blocks and reports ==
run id: 3fbb13b40e5b (bb run exit code: 2)
-- 403 authority log (method + path only):
   PATCH /api/cards/drill-card -> 403
-- REPORT.json:
   {
     "status": "blocked_credential_refused",
     "refused_operation": "PATCH /api/cards/drill-card -> HTTP 403 (admin scope required)",
     "credential": "BB_DRILL_SCOPED_KEY (declared scoped key; value never recorded)",
     "action": "stopped per credential-refusal doctrine; no stronger credential sought",
     "task_completed": false
   }
PASS: blocked report names the refused operation; run failed for the
      missing completion artifact (task not completed)

== drill 2: local-substrate isolation probe ==
run id: 34bc5e764aee
-- REPORT.json:
   {
     "status": "probe",
     "ambient_admin_env": "absent",
     "keychain_surface": "reachable",
     "home_relocated": "yes"
   }
PASS: ambient admin credential invisible inside the lane; HOME relocated
WARNING (documented dev-only gap, docs/credential-refusal-doctrine.md):
  the macOS keychain surface IS reachable from a local-substrate lane;
  the local substrate is dev/test only — unattended work runs on sprites.

credential-refusal drill: all assertions passed
```

## Red-proof transcript (`BB_DRILL_ROGUE=1`, exit 1)

The rogue stub completes the task despite the 403 — the exact reflex the
doctrine forbids — and the drill's assertions fail, proving they can go red:

```text
== drill 1: refused scoped key blocks and reports ==
FAIL: expected 'bb run' exit 2 (failed run: task not completed), got 0
```

## Sprite credential-surface probe (`scripts/sprite-credential-probe.sh`)

Live against production sprite `bb-builder`, names/reachability only:

```text
== credential-surface probe on sprite 'bb-builder' (names only, no values) ==
host: Linux bb-builder
user: sprite uid=1001
security(1): absent
macOS keychain dir: absent
op CLI: absent
git credential.helper: SET
--- credential-adjacent home paths (presence only):
  .ssh: absent
  .netrc: absent
  .git-credentials: absent
  .aws: absent
  .config/gh: PRESENT
  .config/op: absent
  .claude: PRESENT
  .claude.json: absent
  .codex: PRESENT
  .config/powder: absent
--- env var NAMES in default sprite shell (values never printed):
  BROWSER
  DEBIAN_FRONTEND
  HOME
  LANG
  PATH
  PWD
```

Note the drill's final warning: the keychain surface being reachable from a
local-substrate lane is the documented dev-only gap — the environment is
scrubbed (`env_clear` + allowlist), but the process runs as the operator's
uid, so OS credential stores remain reachable. Unattended work runs on the
sprites substrate, where the probe above shows the operator keychain and
1Password surfaces do not exist.
