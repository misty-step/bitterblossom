# Coverage Ratchet And Browser Execution Gate

Date: 2026-07-06
Backlog: 124 (application-floor gap: no coverage/browser-execution signal)

Two gates, both wired into `./scripts/verify.sh` so they run identically
locally and in CI (`.github/workflows/ci.yml`), neither a one-time arbitrary
threshold:

1. **Rust coverage ratchet** -- `cargo-llvm-cov` line-coverage floor.
2. **Dashboard browser execution gate** -- syntax validation, a headless
   load with zero console errors, and one behavioral click path for
   `src/operator.html`, at desktop and mobile widths.

## Rust Coverage Ratchet

### Running it locally

```bash
cargo install cargo-llvm-cov --locked   # one-time
rustup component add llvm-tools-preview # one-time
cargo llvm-cov --fail-under-lines 80.5 --lcov --output-path target/coverage/lcov.info
```

Or just run `./scripts/verify.sh`, which runs this as one of its steps. The
full per-file breakdown prints to the terminal; the `lcov.info` file under
`target/coverage/` (gitignored, not committed) is the coverage-proxy
artifact a CI job can upload if it wants a historical trend, e.g. via
`actions/upload-artifact`.

### The ratchet, not a fixed threshold

`COVERAGE_LINE_FLOOR` in `scripts/verify.sh` is a floor, not a target.
Baseline recorded 2026-07-06: **81.06% total line coverage** across `src/`.
The floor is set to 80.5% -- a little under the baseline so ordinary
measurement noise (platform differences, timing-sensitive tests) cannot
false-fail the gate.

- **The floor only ever rises**, and only as a deliberate act: when a change
  genuinely improves coverage, raise `COVERAGE_LINE_FLOOR` in the same
  commit, with a comment recording the new baseline and why. This is the
  exact same convention `SPINE_LOC_CAP` already uses in this script --
  matching the established pattern rather than inventing a new one.
- **The floor must never be lowered** to make a change pass. That is
  weakening a gate, the one thing this repo's quality doctrine forbids
  outright.
- A regression below the floor means real coverage was lost somewhere in
  `src/`. Find it (the per-file breakdown printed by the gate names exactly
  which file lost ground) and add the missing test, rather than treating
  the number as the problem.

### Waivers

A waiver is a rare, reviewed, visible exception -- never a silent bypass.

Set `BB_COVERAGE_WAIVER="<reason>"` when invoking `verify.sh` (or in the
CI workflow step, as a diff someone reviews) to let a coverage-floor miss
pass for that one run. The gate still runs and still tells you the actual
number; it just doesn't hard-fail. The reason is echoed loudly to
stdout/CI logs, and the workflow/PR diff that sets the env var is itself
the reviewable evidence -- there is no separate waiver file to keep in
sync, and no default anywhere sets this variable, so a bypass only ever
happens because someone deliberately configured it and that configuration
is visible in the same diff.

This does **not** cover an actual test failure -- `cargo test` (the
unconditional step before the coverage ratchet) always hard-fails on a
real test failure, waiver or not.

## Dashboard Browser Execution Gate

`scripts/dashboard-smoke.mjs` is invoked like any other external dev tool
(same convention as `scripts/capture-dashboard-screenshots.mjs` from
bitterblossom-119) -- this is a Rust-only crate; no `package.json` or
`node_modules` is committed.

### Running it locally

```bash
mkdir -p /tmp/bb-pw && cd /tmp/bb-pw && npm init -y && npm install playwright
npx playwright install chromium   # one-time browser download
cd /path/to/bitterblossom
cargo build
NODE_PATH=/tmp/bb-pw/node_modules node scripts/dashboard-smoke.mjs target/debug/bb
```

Or just run `./scripts/verify.sh` -- it runs this automatically if `node`
and `playwright` resolve, and skips with a clear message if they don't
(this gate is not required for local dev without Node; it *is* required in
CI, where `.github/workflows/ci.yml` always installs both).

### What it checks

1. **Syntax validation**: extracts `src/operator.html`'s single inline
   `<script>` block and runs `node --check` against it -- catches a broken
   script before it ever reaches a browser.
2. **Headless smoke load, zero console errors**: launches real headless
   Chromium, loads the dashboard, and asserts no `console.error` or
   uncaught page error occurs. Console assertions start *after* a valid
   token is submitted, not on the very first fresh load -- the dashboard's
   own `load()` always fires an unauthenticated fetch first and reacts to
   the expected 401 by showing the auth form; Chromium logs that expected,
   handled non-2xx resource load as a console error by design, which would
   make the gate fail on the dashboard's normal, correct behavior rather
   than a real problem.
3. **One behavioral click path**: fills and submits the auth token, then
   clicks into the Runs view (`[data-view-button="runs"]`) and asserts it
   becomes active -- the same auth-then-navigate path a real operator
   takes, not just a static render.
4. All of the above at both 1440px (desktop) and 390px (mobile), matching
   bitterblossom-119's required widths.

### Relationship to bitterblossom-119

bitterblossom-119 already proved, via manual Playwright capture
(`scripts/capture-dashboard-screenshots.mjs`), that all six required UX
states (auth-required, loading, error, populated, stale, empty) render
usably with no overlapping text, at both widths, via real rendered
screenshots. That work is not duplicated here. This gate adds what 119's
manual capture script does not: automated syntax validation, an explicit
zero-console-error assertion, and CI wiring so the check runs on every push
and PR rather than only when someone remembers to run the capture script by
hand.
