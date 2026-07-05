// bitterblossom-119: rendered-screenshot proof that the operator dashboard
// (src/operator.html, served by `bb serve`) actually loads and renders its
// six required UX states, at desktop and mobile widths, with no overlapping
// text.
//
// This is a plain Node/Playwright script, not a repo dependency -- this is
// a Rust-only crate (no package.json, no node_modules committed) and
// Playwright is invoked exactly like any other external dev tool:
//
//   mkdir -p /tmp/bb-pw && cd /tmp/bb-pw && npm init -y && npm install playwright
//   npx playwright install chromium   # one-time browser download
//
// Then seed and serve two fixture planes (see seed-dashboard-fixture.sh and
// seed-dashboard-stale-run.sh in this directory for the exact recipe,
// including why the stale run must be inserted *after* `bb serve` is
// already listening), and run:
//
//   node /path/to/capture-dashboard-screenshots.mjs \
//     docs/screenshots/operator-dashboard \
//     http://127.0.0.1:<populated-port> \
//     http://127.0.0.1:<empty-port>
//
// with BB_API_TOKEN=demo-token (or whatever TOKEN below is set to) on both
// `bb serve` processes.
import { chromium } from 'playwright';
import { mkdir } from 'node:fs/promises';

const OUT_DIR = process.argv[2];
const POPULATED_URL = process.argv[3]; // e.g. http://127.0.0.1:18790
const EMPTY_URL = process.argv[4]; // e.g. http://127.0.0.1:18791
const TOKEN = 'demo-token';

const WIDTHS = [
  { width: 1440, height: 900 },
  { width: 390, height: 844 },
];

await mkdir(OUT_DIR, { recursive: true });

const browser = await chromium.launch();

async function shoot(
  name,
  dims,
  url,
  { seedToken = false, delayApi = false, abortApi = false, afterLoad = null } = {}
) {
  const { width, height } = dims;
  const context = await browser.newContext({ viewport: { width, height } });
  if (seedToken) {
    await context.addInitScript((token) => {
      localStorage.setItem('bb-api-token', token);
    }, TOKEN);
  }
  const page = await context.newPage();
  if (delayApi) {
    await page.route('**/api/**', async (route) => {
      await new Promise((r) => setTimeout(r, 4000));
      await route.continue();
    });
  }
  if (abortApi) {
    await page.route('**/api/**', (route) => route.abort());
  }
  await page.goto(url, { waitUntil: 'load' });
  if (afterLoad) await afterLoad(page);
  await page.waitForTimeout(300);
  const path = `${OUT_DIR}/${name}-${width}.png`;
  await page.screenshot({ path, fullPage: true });
  console.log(`wrote ${path}`);
  await context.close();
}

for (const dims of WIDTHS) {
  // 1. auth-required: fresh context, no token, default landing state.
  await shoot('auth-required', dims, POPULATED_URL);

  // 2. loading: fresh context, submit the auth form with /api/* responses
  // held back so the transient "Checking token..." moment is captured
  // instead of racing past it in a few ms on a local server.
  await shoot('loading', dims, POPULATED_URL, {
    delayApi: true,
    afterLoad: async (page) => {
      await page.fill('#authToken', TOKEN);
      await page.click('#authForm button[type="submit"]');
      await page.waitForTimeout(200);
    },
  });

  // 3. error: token already valid, but every /api/* request is aborted so
  // the generic (non-auth) error banner renders over the (unlocked) shell.
  await shoot('error', dims, POPULATED_URL, { seedToken: true, abortApi: true });

  // 4. populated: token seeded, real server, rich fixture data, default
  // (overview) view.
  await shoot('populated', dims, POPULATED_URL, { seedToken: true });

  // 5. stale: same populated fixture, Runs view, where the freshness column
  // surfaces the seeded stale_executing run.
  await shoot('stale', dims, POPULATED_URL, {
    seedToken: true,
    afterLoad: async (page) => {
      await page.click('[data-view-button="runs"]');
    },
  });

  // 6. empty/no-data: token seeded, a plane with zero tasks/runs/anything.
  await shoot('empty', dims, EMPTY_URL, { seedToken: true });
}

await browser.close();
console.log('done');
