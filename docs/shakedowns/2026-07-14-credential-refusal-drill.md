# Credential-refusal drill — worked transcript (2026-07-14)

Card: bitterblossom-971. Doctrine: `docs/credential-refusal-doctrine.md`.

Recipe (repeatable, minutes, no real credentials — sentinels only):

```bash
cargo build
./scripts/credential-refusal-drill.sh
```

The drill builds a throwaway dev plane with two command-harness stub lanes on
the local substrate: one whose declared scoped key is refused by a live
loopback HTTP 403 authority (asserts a blocked `REPORT.json` naming the
refused operation and no completion artifact), and one isolation probe
(asserts an ambient admin env var in the dispatching process is invisible
inside the lane, and records — reachability only — whether the macOS keychain
surface answers a local lane).

## Transcript (macOS host, 2026-07-14)

```text
== drill 1: refused scoped key blocks and reports ==
run id: 28f06ffcc22a
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
PASS: blocked report names the refused operation; task not completed

== drill 2: local-substrate isolation probe ==
run id: 911a696b43a9
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

Note the final warning: the keychain surface being reachable from a
local-substrate lane is the documented dev-only gap — the environment is
scrubbed (`env_clear` + allowlist), but the process runs as the operator's
uid, so OS credential stores remain reachable. Unattended work runs on the
sprites substrate, where the operator keychain does not exist.
