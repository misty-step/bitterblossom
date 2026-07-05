# Operator dashboard rendered-screenshot proof (bitterblossom-119)

Twelve PNGs: the dashboard's six required UX states, each at desktop
(1440px) and mobile (390px) widths, captured with a real browser
(Playwright/Chromium) against a real `bb serve` process. Full-page
captures -- scroll past the first screen for `stale`/`populated`.

| State | Desktop | Mobile | What it proves |
|---|---|---|---|
| auth-required | `auth-required-1440.png` | `auth-required-390.png` | Fresh session, no token -- the auth panel, not a blank page. |
| loading | `loading-1440.png` | `loading-390.png` | Auth form submitted; `/api/*` responses held back so "Checking token..." is actually visible instead of racing past in a few ms. |
| error | `error-1440.png` | `error-390.png` | Token already valid but every `/api/*` request fails -- the generic "Failed to fetch" banner, fixed in this card (see below) to actually be visible. |
| empty/no-data | `empty-1440.png` | `empty-390.png` | A plane with zero tasks/runs/anything -- every card and table shows an explicit empty state, not a crash or a blank. |
| stale | `stale-1440.png` | `stale-390.png` | Runs view, with the `stale_executing` freshness badge (bitterblossom-118) on a run frozen 40 minutes without progress. |
| populated | `populated-1440.png` | `populated-390.png` | Full fixture: DLQ, lease, running + completed runs, budget, triggers, safe next action. |

## Bug fixed while producing this proof

Before this card, a stored-but-still-valid token combined with a **non-auth**
fetch failure (network down, 5xx, oversized response -- anything that isn't
a 401) left the auth overlay locked over the shell forever. The error banner
was written to the DOM and marked visible, but nobody could ever see it,
because the auth panel `showAuth()` unconditionally called at boot never got
dismissed except by `showDashboard()` on success. Fixed in `src/
operator.html`'s `load()` catch block by calling `showDashboard()` on the
generic-error path too, so the error banner in the shell is actually
reachable. See `error-1440.png`/`error-390.png` for the fixed behavior.

## How to reproduce

```sh
# 1. One-time Playwright setup (Node/Playwright are external dev tools here
#    -- this is a Rust-only crate, nothing added to Cargo.toml).
mkdir -p /tmp/bb-pw && cd /tmp/bb-pw
npm init -y && npm install playwright
npx playwright install chromium

# 2. Seed two fixture planes (from the bitterblossom repo root).
./scripts/seed-dashboard-fixture.sh /tmp/bb-dashboard-fixture
mkdir -p /tmp/bb-dashboard-empty
printf 'dev = true\n[ingress]\nbind = "127.0.0.1:0"\n' > /tmp/bb-dashboard-empty/plane.toml

# 3. Serve both (separate terminals, or background with &).
BB_INGRESS_BIND=127.0.0.1:18790 BB_API_TOKEN=demo-token \
  target/debug/bb --config /tmp/bb-dashboard-fixture serve
BB_INGRESS_BIND=127.0.0.1:18791 BB_API_TOKEN=demo-token \
  target/debug/bb --config /tmp/bb-dashboard-empty serve

# 4. Insert the stale run -- must happen AFTER bb serve is already up, or
#    boot-time recovery sweeps it into a dead letter instead (see the
#    seed-dashboard-stale-run.sh header for why).
./scripts/seed-dashboard-stale-run.sh /tmp/bb-dashboard-fixture

# 5. Capture.
node scripts/capture-dashboard-screenshots.mjs \
  docs/screenshots/operator-dashboard \
  http://127.0.0.1:18790 http://127.0.0.1:18791
```
