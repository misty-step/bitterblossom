// bitterblossom-124: application-floor browser execution gate for the
// operator dashboard (src/operator.html, served by `bb serve`). Distinct
// from bitterblossom-119's rendered-screenshot proof (which pins the six
// UX states look right, at both widths, via manual capture): this is the
// automated, CI-wired check that the dashboard's inline script is at least
// syntactically valid, that a real headless browser can load it with zero
// console errors, and that one real behavioral click path (auth, then a
// view switch) actually works -- at desktop and mobile widths both.
//
// Not a repo dependency: this is a Rust-only crate (no package.json, no
// node_modules committed). Node and Playwright are invoked exactly like any
// other external dev tool, same convention as capture-dashboard-screenshots.mjs.
//
//   mkdir -p /tmp/bb-pw && cd /tmp/bb-pw && npm init -y && npm install playwright
//   npx playwright install --with-deps chromium   # one-time browser download
//
// Usage: node scripts/dashboard-smoke.mjs <bb-binary-path>
// Exits non-zero on a syntax error, a console error/pageerror at either
// width, or a missing expected element in the auth/view-switch click path.
import { chromium } from 'playwright';
import { mkdtemp, readFile, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn, spawnSync } from 'node:child_process';

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..');
const BB = process.argv[2] ?? join(ROOT, 'target/debug/bb');
const TOKEN = 'demo-token';
const WIDTHS = [
  { width: 1440, height: 900 },
  { width: 390, height: 844 },
];

let failures = 0;
function fail(msg) {
  console.error(`FAIL: ${msg}`);
  failures += 1;
}

// 1. Syntax validation: extract the dashboard's single inline <script>
// block and check it parses as valid JS, without executing it.
const html = await readFile(join(ROOT, 'src/operator.html'), 'utf8');
const scriptMatches = [...html.matchAll(/<script>([\s\S]*?)<\/script>/g)];
if (scriptMatches.length !== 1) {
  fail(`expected exactly one inline <script> block in src/operator.html, found ${scriptMatches.length}`);
} else {
  const scriptTmp = join(await mkdtemp(join(tmpdir(), 'bb-dashboard-smoke-')), 'inline.js');
  await writeFile(scriptTmp, scriptMatches[0][1]);
  const check = spawnSync(process.execPath, ['--check', scriptTmp], { encoding: 'utf8' });
  if (check.status !== 0) {
    fail(`inline <script> failed node --check:\n${check.stderr}`);
  } else {
    console.log('syntax: inline <script> parses cleanly');
  }
}

// 2. Seed a populated fixture plane and serve it (bitterblossom-119's
// seed-dashboard-fixture.sh recipe), on an ephemeral port.
const planeDir = await mkdtemp(join(tmpdir(), 'bb-dashboard-smoke-plane-'));
const seed = spawnSync('sh', [join(ROOT, 'scripts/seed-dashboard-fixture.sh'), planeDir], {
  env: { ...process.env, BB_BIN: BB },
  encoding: 'utf8',
});
if (seed.status !== 0) {
  console.error(seed.stdout, seed.stderr);
  throw new Error('seed-dashboard-fixture.sh failed');
}

const port = 18700 + Math.floor(Math.random() * 500);
const url = `http://127.0.0.1:${port}`;
const serve = spawn(BB, ['--config', planeDir, 'serve'], {
  env: { ...process.env, BB_API_TOKEN: TOKEN, BB_INGRESS_BIND: `127.0.0.1:${port}` },
  stdio: ['ignore', 'pipe', 'pipe'],
});
let serveExited = false;
serve.on('exit', () => {
  serveExited = true;
});

async function waitForServe() {
  const deadline = Date.now() + 10_000;
  while (Date.now() < deadline) {
    if (serveExited) throw new Error('bb serve exited before it started listening');
    try {
      const res = await fetch(`${url}/health`);
      if (res.ok) return;
    } catch {
      // not listening yet
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error(`bb serve did not become healthy at ${url} within 10s`);
}

try {
  await waitForServe();

  const browser = await chromium.launch();
  for (const dims of WIDTHS) {
    const context = await browser.newContext({ viewport: dims });
    const page = await context.newPage();

    // 3. Headless smoke load: fresh context, no token yet (auth-required).
    // The dashboard's own load() always fires an unauthenticated fetch
    // first and reacts to the expected 401 by showing the auth form --
    // Chromium logs that non-2xx resource load as a console error by
    // design, so console assertions start only after a valid token is
    // present (the path a real operator actually uses).
    await page.goto(url, { waitUntil: 'load' });
    const authVisible = await page.locator('#authForm').isVisible();
    if (!authVisible) fail(`${dims.width}px: auth form not visible on fresh load`);

    // Chromium delivers console events asynchronously: under CI load the
    // by-design pre-auth 401 (documented above) can flush AFTER the error
    // listener attaches, failing the run on an error we explicitly expect.
    // networkidle guarantees the unauthenticated fetch has fully settled
    // before assertions begin; post-auth 401s still fail the gate.
    await page.waitForLoadState('networkidle');

    const consoleErrors = [];
    page.on('console', (msg) => {
      if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', (err) => consoleErrors.push(String(err)));

    // 4. Behavioral click path: submit the token, then switch to the Runs
    // view -- the same auth-then-navigate path a real operator takes.
    await page.fill('#authToken', TOKEN);
    await page.click('#authForm button[type="submit"]');
    await page.waitForSelector('.shell:not(.is-hidden)', { timeout: 5000 }).catch(() => {});
    await page.waitForTimeout(300);
    await page.click('[data-view-button="runs"]');
    await page.waitForTimeout(300);
    const runsActive = await page.locator('[data-view-button="runs"].is-active').count();
    if (runsActive !== 1) fail(`${dims.width}px: clicking the Runs view button did not mark it active`);

    if (consoleErrors.length > 0) {
      fail(`${dims.width}px: ${consoleErrors.length} console error(s): ${consoleErrors.join(' | ')}`);
    } else {
      console.log(`${dims.width}px: headless load + auth + view-switch clean, zero console errors`);
    }
    await context.close();
  }
  await browser.close();
} finally {
  serve.kill();
  await rm(planeDir, { recursive: true, force: true });
}

if (failures > 0) {
  console.error(`dashboard-smoke: ${failures} failure(s)`);
  process.exit(1);
}
console.log('dashboard-smoke: ok');
