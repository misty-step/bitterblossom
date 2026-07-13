const AESTHETIC = 'https://cdn.jsdelivr.net/gh/misty-step/aesthetic@v0.25.0/aesthetic.css';
const RECIPES = 'https://cdn.jsdelivr.net/gh/misty-step/aesthetic@v0.25.0/recipes/recipes.js';

const esc = (value) => String(value ?? '').replace(/[&<>"']/g, (char) => ({
  '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
}[char]));

const pick = (corpus, paths, fallback) => {
  for (const path of paths) {
    const value = path.split('.').reduce((current, key) => current?.[key], corpus);
    if (value !== undefined && value !== null && value !== '') return value;
  }
  return fallback;
};

const icon = (name, tone = '') => {
  const paths = {
    check: '<path d="m5 12 4 4L19 6"/>',
    alert: '<path d="m10.3 2.9-8.1 14A2 2 0 0 0 4 20h16a2 2 0 0 0 1.7-3.1l-8.1-14a2 2 0 0 0-3.3 0Z"/><path d="M12 9v4M12 17h.01"/>',
    play: '<path d="m7 4 13 8-13 8V4Z"/>',
    pause: '<path d="M6 4h4v16H6zM14 4h4v16h-4z"/>',
    clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
    branch: '<circle cx="6" cy="5" r="2"/><circle cx="18" cy="19" r="2"/><path d="M6 7v4a4 4 0 0 0 4 4h6M18 17v-4a4 4 0 0 0-4-4H6"/>',
    chevron: '<path d="m9 18 6-6-6-6"/>',
    swatch: '<path d="M4 4h16v16H4z"/><path d="M8 4v16M4 8h4M4 12h4M4 16h4"/>'
  };
  return `<svg class="ae-icon ${tone}" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="square" stroke-linejoin="miter">${paths[name] || paths.swatch}</svg>`;
};

const status = (label, kind = 'ok', glyph = 'check') => `<span class="ae-status"><span class="ae-icon ${kind === 'ok' ? 'ae-ok' : kind === 'warn' ? 'ae-warn' : 'ae-err'}">${icon(glyph)}</span><span class="ae-status-label">${esc(label)}</span></span>`;
const tag = (label) => `<span class="ae-tag">${esc(label)}</span>`;
const cell = (label, value, extra = '') => `<div class="ae-list-cell ${extra}"><span class="ae-list-label">${esc(label)}</span><span class="ae-list-value">${esc(value)}</span></div>`;

const model = (corpus) => ({
  workflow: esc(pick(corpus, ['workflow.name', 'workflows.0.name', 'name'], 'PR Review')),
  goal: esc(pick(corpus, ['workflow.goal', 'goal'], 'Review each eligible pull-request head and leave one evidence-backed review.')),
  trigger: esc(pick(corpus, ['workflow.trigger', 'trigger.label', 'triggers.0.label'], 'GitHub pull_request · reviewable head')),
  run: esc(pick(corpus, ['run.id', 'runs.0.id', 'selectedRun.id'], 'run/pr-review-1842')),
  agent: esc(pick(corpus, ['agent.name', 'agents.0.name'], 'Cerberus')),
  verifier: esc(pick(corpus, ['verifier.name', 'agents.1.name'], 'Fresh verifier')),
  repo: esc(pick(corpus, ['run.repo', 'repository', 'repo'], 'o/r')),
  cost: esc(pick(corpus, ['run.cost', 'selectedRun.cost'], 'reported · $0.42')),
  lastAccepted: esc(pick(corpus, ['trigger.lastAccepted', 'lastAccepted'], '12:04:18 CT')),
  corpusLabel: esc(pick(corpus, ['label', 'fixture.label'], 'shared fixture corpus'))
});

const modeScript = `<script>(function(){var r=document.documentElement;try{var m=localStorage.getItem('ae-mode');if(m==='dark'||m==='light'){r.classList.add(m);r.dataset.aeMode=m;}}catch(e){}document.addEventListener('click',function(e){var b=e.target.closest('[data-theme-toggle]');if(!b)return;var dark=r.classList.contains('dark')||(!r.classList.contains('light')&&window.matchMedia('(prefers-color-scheme: dark)').matches);var n=dark?'light':'dark';r.classList.toggle('dark',n==='dark');r.classList.toggle('light',n==='light');r.dataset.aeMode=n;try{localStorage.setItem('ae-mode',n);}catch(e){}});})();</script>`;

const commonStyle = `
  :root { color-scheme: light dark; }
  * { box-sizing: border-box; }
  html, body { min-height: 100%; }
  body { margin: 0; background: var(--ae-surface); color: var(--ae-ink); }
  button, a { cursor: pointer; }
  a { color: var(--ae-ink); }
  .impec-frame { min-height: 100dvh; display: flex; flex-direction: column; }
  .impec-top { min-height: 3.5rem; border-bottom: 1px solid var(--ae-line); display: flex; align-items: center; justify-content: space-between; gap: 1rem; padding: 0 1.25rem; }
  .impec-top .ae-logo { flex: 0 0 auto; }
  .impec-controls { display: flex; align-items: center; gap: .5rem; flex-wrap: wrap; }
  .impec-main { min-width: 0; flex: 1; padding: 1.25rem; }
  .impec-title { display: flex; align-items: end; justify-content: space-between; gap: 1rem; padding-bottom: 1rem; border-bottom: 1px solid var(--ae-line); }
  .impec-title h1 { margin: 0; font-weight: 800; letter-spacing: -.02em; text-wrap: balance; }
  .impec-title p { max-width: 65ch; margin: .35rem 0 0; }
  .impec-kicker, .impec-meta { color: var(--ae-ink-muted); font-family: var(--ae-font-mono); font-size: 13px; }
  .impec-kicker { margin: 0 0 .5rem; }
  .impec-section { min-width: 0; padding: 1rem 0; border-bottom: 1px solid var(--ae-line); }
  .impec-section > h2 { margin: 0 0 .75rem; font-weight: 550; }
  .impec-section > h2 span { color: var(--ae-ink-muted); font-family: var(--ae-font-mono); font-size: 13px; font-weight: 400; }
  .impec-rule { border-top: 1px solid var(--ae-line); }
  .impec-quiet { color: var(--ae-ink-muted); }
  .impec-mono { font-family: var(--ae-font-mono); }
  .impec-grid { display: grid; gap: 1rem; }
  .impec-panel { min-width: 0; }
  .impec-panel > header { display: flex; align-items: baseline; justify-content: space-between; gap: .75rem; padding-bottom: .5rem; border-bottom: 1px solid var(--ae-line); }
  .impec-panel h3 { margin: 0; font-weight: 550; }
  .impec-panel header .impec-meta { white-space: nowrap; }
  .impec-rows { display: grid; gap: 0; }
  .impec-row { min-width: 0; display: flex; align-items: baseline; justify-content: space-between; gap: .75rem; padding: .65rem 0; border-bottom: 1px solid var(--ae-line); }
  .impec-row > :first-child { min-width: 0; }
  .impec-row strong { font-weight: 550; }
  .impec-row .impec-meta { text-align: right; }
  .impec-flow { min-width: 0; overflow: auto; padding: 1rem 0; }
  .impec-flow .ae-flow { min-width: 31rem; }
  .impec-caption { margin: .5rem 0 0; color: var(--ae-ink-muted); font-size: 13px; }
  .impec-footer { padding: .75rem 1.25rem; color: var(--ae-ink-muted); font-family: var(--ae-font-mono); font-size: 13px; }
  .impec-state-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 0; border-top: 1px solid var(--ae-line); }
  .impec-state { min-width: 0; padding: .7rem .75rem .7rem 0; border-bottom: 1px solid var(--ae-line); }
  .impec-state:nth-child(3n+2), .impec-state:nth-child(3n+3) { padding-left: .75rem; border-left: 1px solid var(--ae-line); }
  .impec-state p { margin: .35rem 0 0; color: var(--ae-ink-muted); }
  .impec-actions { display: flex; flex-wrap: wrap; gap: .5rem; margin-top: .75rem; }
  .impec-create { display: grid; gap: 1rem; }
  .impec-create .ae-settings { border-top: 1px solid var(--ae-line); }
  .impec-create textarea { width: 100%; min-height: 5rem; resize: vertical; color: var(--ae-ink); background: var(--ae-surface); border: 0; border-bottom: 1px solid var(--ae-line); font: inherit; padding: .65rem 0; }
  .impec-create textarea:focus { outline: 1px solid var(--ae-accent); outline-offset: .2rem; }
  .impec-legend { display: flex; flex-wrap: wrap; gap: .75rem 1.25rem; margin-top: .75rem; color: var(--ae-ink-muted); font-size: 13px; }
  .impec-legend span { display: inline-flex; align-items: center; gap: .35rem; }
  @media (max-width: 760px) {
    .impec-top, .impec-main { padding-left: .75rem; padding-right: .75rem; }
    .impec-title { align-items: start; flex-direction: column; }
    .impec-state-grid { grid-template-columns: 1fr; }
    .impec-state:nth-child(3n+2), .impec-state:nth-child(3n+3) { padding-left: 0; border-left: 0; }
    .impec-state { padding-right: 0; }
    .impec-row { align-items: start; flex-direction: column; gap: .25rem; }
    .impec-row .impec-meta { text-align: left; }
  }
  @media (prefers-reduced-motion: reduce) { *, *::before, *::after { transition-duration: 0s !important; animation-duration: 0s !important; } }
`;

const shell = (variant, title, subtitle, body, rail = '') => `<!doctype html><html data-ae-theme="ultramarine"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>${esc(title)}</title><link rel="stylesheet" href="${AESTHETIC}"><style>${commonStyle}${variant}</style></head><body class="${esc(title)}"><div class="impec-frame"><header class="impec-top"><a class="ae-logo" href="#workflows" aria-label="Bitterblossom home"><span class="ae-app-mark">${icon('swatch')}</span><span class="ae-name">Bitterblossom</span></a><div class="impec-controls"><span class="impec-meta">${esc(subtitle)}</span><button class="ae-button ae-button-quiet ae-button-compact" type="button" data-theme-toggle data-ae-mode-toggle aria-label="Toggle light or dark mode">mode</button></div></header>${rail}<main class="impec-main">${body}</main><footer class="impec-footer">${esc(title)} · ${esc(subtitle)} · ${esc('Aesthetic v0.25.0')}</footer></div>${modeScript}<script src="${RECIPES}"></script></body></html>`;

const title = (d, kicker, heading, copy) => `<div class="impec-title"><div><p class="impec-kicker">${esc(kicker)}</p><h1>${esc(heading)}</h1><p class="impec-quiet">${esc(copy)}</p></div><div class="impec-meta">${d.corpusLabel}</div></div>`;

const roster = (d) => `<section class="impec-section" id="workflows"><h2>Configured workflows <span>stable definitions</span></h2><table class="ae-table"><thead><tr><th>Workflow</th><th>Trigger</th><th>Topology</th><th>Latest evidence</th></tr></thead><tbody><tr><td data-label="Workflow"><strong>${d.workflow}</strong><br>${tag('active')} ${status('configured', 'ok', 'check')}</td><td data-label="Trigger">${d.trigger}<br><span class="impec-meta">${status('listening', 'ok', 'clock')}</span></td><td data-label="Topology" class="impec-mono">trigger → ${d.agent}<br>→ verifier → done</td><td data-label="Latest evidence"><span class="ae-num ae-strong">${d.run}</span><br>${status('succeeded · blocked domain result', 'warn', 'alert')}</td></tr><tr><td data-label="Workflow"><strong>Canary incident resolution</strong><br>${tag('draft')}</td><td data-label="Trigger">service event<br>${status('delayed', 'warn', 'clock')}</td><td data-label="Topology" class="impec-mono">trigger → responder<br>→ verifier</td><td data-label="Latest evidence">${status('no active run', 'warn', 'clock')}</td></tr></tbody></table></section>`;

const topology = (d) => `<section class="impec-panel" id="topology"><header><h3>PR Review topology</h3><span class="impec-meta">revision r17 · selected run</span></header><div class="impec-flow"><div class="ae-flow"><div class="ae-flow-node"><span class="ae-flow-index">01</span><strong>Trigger</strong><small>${d.trigger}</small></div><div class="ae-flow-wire"></div><div class="ae-flow-node is-active"><span class="ae-flow-index">02</span><strong>${d.agent}</strong><small>review + receipts</small></div><div class="ae-flow-wire"></div><div class="ae-flow-node"><span class="ae-flow-index">03</span><strong>${d.verifier}</strong><small>evidence-backed verdict</small></div><div class="ae-flow-wire"></div><div class="ae-flow-node"><span class="ae-flow-index">04</span><strong>Route</strong><small>blocked → request changes</small></div></div></div><div class="impec-caption">Stable graph in ink; the selected run overlays only the active stage and its evidence.</div><div class="impec-legend"><span>${icon('play', 'ae-ok')} executing</span><span>${icon('check', 'ae-ok')} verification achieved</span><span>${icon('alert', 'ae-warn')} domain result blocked</span></div></section>`;

const runOverlay = (d) => `<section class="impec-panel" id="run"><header><h3>Selected live-run overlay</h3><span class="impec-meta">${d.run}</span></header><div class="ae-list-rows"><div class="ae-list-row">${cell('Run', d.run, 'impec-mono')}${cell('Lifecycle', 'executing')}${cell('Cost', d.cost)}</div><div class="ae-list-row">${cell('Step', d.agent)}${cell('Domain result', 'blocked')}${cell('Verification', 'achieved')}</div><div class="ae-list-row">${cell('Authority', 'review + comment')}${cell('Last accepted', d.lastAccepted)}${cell('Head', 'pinned')}</div></div><div class="impec-actions"><button class="ae-button ae-button-quiet" type="button">open evidence</button><button class="ae-button" type="button">follow live run</button></div></section>`;

const agents = (d) => `<section class="impec-section" id="agents"><h2>Roster agents <span>pinned revisions</span></h2><div class="impec-grid" style="grid-template-columns:repeat(2,minmax(0,1fr))"><div class="ae-plate"><div class="ae-plate-cap">In use</div><div class="ae-plate-body"><div class="impec-row"><strong>${d.agent}</strong>${status('executing', 'ok', 'play')}</div><div class="impec-row"><span>${d.verifier}</span>${status('verification achieved', 'ok', 'check')}</div></div></div><div class="ae-plate"><div class="ae-plate-cap">Available</div><div class="ae-plate-body"><div class="impec-row"><span>QA specialist</span>${tag('catalogued')}</div><div class="impec-row"><span>Incident responder</span>${tag('catalogued')}</div></div></div></div></section>`;

const evidence = (d) => `<section class="impec-section" id="evidence"><h2>Evidence <span>live / history, never one status</span></h2><div class="impec-grid" style="grid-template-columns:repeat(2,minmax(0,1fr))"><div><div class="impec-meta">Live</div><div class="ae-trail"><div class="ae-trail-item is-active"><div class="ae-trail-head"><span class="ae-trail-time">12:04:24</span><span class="ae-trail-who">${d.agent}</span></div><div class="ae-trail-body">Execution is active; formal review waits on the final head check.</div></div><div class="ae-trail-item"><div class="ae-trail-head"><span class="ae-trail-time">12:04:18</span><span class="ae-trail-who">Trigger</span></div><div class="ae-trail-body">Accepted ${d.repo} pull-request head.</div></div></div></div><div><div class="impec-meta">History</div><div class="ae-trail"><div class="ae-trail-item"><div class="ae-trail-head"><span class="ae-trail-time">11:52:02</span><span class="ae-trail-who">${d.verifier}</span></div><div class="ae-trail-body">${status('verification achieved', 'ok', 'check')} Domain result remained blocked.</div></div><div class="ae-trail-item"><div class="ae-trail-head"><span class="ae-trail-time">11:47:39</span><span class="ae-trail-who">Run history</span></div><div class="ae-trail-body">Older head superseded; no post was emitted.</div></div></div></div></div></section>`;

const spend = (d) => `<section class="impec-section" id="spend"><h2>Spend <span>coverage is explicit</span></h2><div class="ae-stat-badges"><div class="ae-stat-badge"><span class="ae-stat-value ae-num">$2.80</span><span class="ae-stat-label">reported today</span></div><div class="ae-stat-badge"><span class="ae-stat-value ae-num">$0.42</span><span class="ae-stat-label">${d.workflow} · reported</span></div><div class="ae-stat-badge"><span class="ae-stat-value ae-num">—</span><span class="ae-stat-label">one provider cost unavailable</span></div></div><div class="impec-grid" style="grid-template-columns:1.2fr .8fr;margin-top:1rem"><div class="impec-rows"><div class="impec-row"><span>Plane ceiling <span class="impec-meta">hard</span></span><span class="ae-num ae-strong">$10 / $25</span></div><div class="impec-row"><span>Workflow daily <span class="impec-meta">admission-only</span></span><span class="ae-num ae-strong">$2.80 / $8</span></div><div class="impec-row"><span>Run group <span class="impec-meta">advisory</span></span><span class="ae-num ae-strong">partial coverage</span></div></div><div><div class="ae-meter"><div class="ae-meter-fill ae-ok" style="width:35%"></div><div class="ae-meter-mark" style="left:35%"></div></div><p class="impec-caption">Unknown is not zero. Aggregates show reported, estimated, and unavailable separately.</p></div></div></section>`;

const create = (d) => `<section class="impec-section" id="create"><h2>Create workflow <span>goal first</span></h2><div class="impec-create"><label for="goal"><strong>What should this workflow accomplish?</strong></label><textarea id="goal" aria-label="Workflow goal">${d.goal}</textarea><div class="ae-settings"><div class="ae-settings-row"><span class="ae-settings-label">Enhanced goal review</span><span class="ae-settings-value">outcome · evidence · boundaries · prohibited actions</span></div><div class="ae-settings-row"><span class="ae-settings-label">Fixture test</span><span class="ae-settings-value">captured pull-request head · ready</span></div><div class="ae-settings-row"><span class="ae-settings-label">Activation</span><span class="ae-settings-value">draft until explicitly accepted</span></div></div><div class="impec-actions"><button class="ae-button" type="button">review enhanced goal</button><button class="ae-button ae-button-quiet" type="button">run fixture test</button></div></div></section>`;

const states = () => `<section class="impec-section" id="states"><h2>State register <span>separate facts</span></h2><div class="impec-state-grid"><div class="impec-state">${status('active', 'ok', 'check')}<p>Workflow lifecycle</p></div><div class="impec-state">${status('draft', 'warn', 'clock')}<p>Workflow lifecycle</p></div><div class="impec-state">${status('listening trigger', 'ok', 'clock')}<p>Trigger health</p></div><div class="impec-state">${status('executing', 'ok', 'play')}<p>Run lifecycle</p></div><div class="impec-state">${status('succeeded · blocked domain result', 'warn', 'alert')}<p>Run and domain result</p></div><div class="impec-state">${status('superseded', 'warn', 'clock')}<p>Run disposition</p></div><div class="impec-state">${status('verification achieved', 'ok', 'check')}<p>Verification</p></div><div class="impec-state">${status('cost unavailable', 'warn', 'alert')}<p>Spend truth</p></div><div class="impec-state">${status('no active run', 'warn', 'clock')}<p>Live view</p></div></div></section>`;

const nav = (items) => `<nav class="ae-rail" aria-label="Primary navigation"><a href="#workflows" class="ae-nav-item is-active">${icon('branch')}<span>Workflows</span></a>${items.map(([id, label]) => `<a href="#${id}" class="ae-nav-item">${icon(id === 'spend' ? 'swatch' : id === 'create' ? 'play' : 'clock')}<span>${label}</span></a>`).join('')}</nav>`;

const SPECS = {
  'IMPEC-1': {
    label: 'Run rail',
    move: 'Stable workflow roster on the desk; a right-hand run rail overlays one live instance without mutating the graph.',
    philosophy: 'Make configuration the first read, then let evidence arrive as a precise overlay. This is the familiar operator shell: rail for places, desk for topology, rail within the desk for the selected run.',
    render(corpus) {
      const d = model(corpus);
      const variant = `.impec-1 .impec-work { display:grid; grid-template-columns:minmax(0,1.55fr) minmax(16rem,.75fr); gap:1.25rem; } .impec-1 .impec-desk { min-width:0; } .impec-1 .impec-runrail { min-width:0; padding-left:1rem; border-left:1px solid var(--ae-line); } .impec-1 .impec-rail-note { padding:.75rem 0; border-bottom:1px solid var(--ae-line); } @media(max-width:760px){.impec-1 .impec-work{grid-template-columns:1fr}.impec-1 .impec-runrail{padding-left:0;border-left:0;border-top:1px solid var(--ae-line);padding-top:1rem}}`;
      const body = `${title(d, 'operator desk / configured first', 'The workflow desk', 'See what is configured before you inspect what ran. The selected PR Review run remains legible against its stable topology.')}<div class="impec-work"><div class="impec-desk">${roster(d)}<div class="impec-grid" style="grid-template-columns:minmax(0,1.2fr) minmax(0,.8fr);padding-top:1rem">${topology(d)}${agents(d)}</div>${evidence(d)}${spend(d)}${create(d)}${states()}</div><aside class="impec-runrail" aria-label="Selected run rail"><div class="impec-rail-note"><p class="impec-kicker">selected instance</p><h2>${d.run}</h2><p class="impec-quiet">${d.workflow}</p></div>${runOverlay(d)}<div class="impec-rail-note"><p class="impec-meta">Run fact</p><p>${status('executing', 'ok', 'play')} The agent is active; the workflow remains active.</p></div><div class="impec-rail-note"><p class="impec-meta">Domain fact</p><p>${status('blocked', 'warn', 'alert')} The result is blocked; verification is achieved.</p></div></aside></div>`;
      return shell('impec-1', 'Run rail', 'IMPEC-1 · run rail', `<div class="impec-1">${body}</div>`, nav([['agents', 'Agents'], ['evidence', 'Runs'], ['spend', 'Spend'], ['create', 'Create workflow']]));
    }
  },
  'IMPEC-2': {
    label: 'Workflow ledger',
    move: 'Replace the single dashboard landing with three ruled work columns: configured, in motion, and evidenced.',
    philosophy: 'Treat the workflow as a ledger entry that can be read across its lifecycle. The board gives topology and run state equal horizontal weight; the evidence spine below preserves the long-form trace.',
    render(corpus) {
      const d = model(corpus);
      const variant = `.impec-2 .impec-board { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); gap:0; border-top:1px solid var(--ae-line); border-bottom:1px solid var(--ae-line); } .impec-2 .impec-column { min-width:0; padding:1rem .9rem 1rem 0; } .impec-2 .impec-column + .impec-column { padding-left:.9rem; border-left:1px solid var(--ae-line); } .impec-2 .impec-column h2 { margin:0 0 .75rem; font-weight:550; } .impec-2 .impec-ledger { display:grid; grid-template-columns:minmax(0,1.1fr) minmax(0,.9fr); gap:1.25rem; margin-top:1rem; } .impec-2 .impec-ledger > * { min-width:0; } .impec-2 .impec-control-strip { display:flex; gap:.5rem; flex-wrap:wrap; padding:.75rem 0; border-bottom:1px solid var(--ae-line); } @media(max-width:760px){.impec-2 .impec-board,.impec-2 .impec-ledger{grid-template-columns:1fr}.impec-2 .impec-column{padding-right:0}.impec-2 .impec-column + .impec-column{padding-left:0;border-left:0;border-top:1px solid var(--ae-line)}}`;
      const board = `<section class="impec-section" id="workflows"><div class="impec-board"><div class="impec-column"><h2>Configured</h2><div class="impec-row"><strong>${d.workflow}</strong>${tag('active')}</div><p class="impec-caption">${d.trigger}</p><p class="impec-caption">trigger → ${d.agent} → verifier → done</p><div class="impec-actions"><button class="ae-button ae-button-quiet ae-button-compact" type="button">inspect graph</button></div></div><div class="impec-column"><h2>In motion</h2><div class="impec-row"><strong>${d.run}</strong>${status('executing', 'ok', 'play')}</div><p class="impec-caption">${d.agent} is active on ${d.repo}; one run admitted per head.</p><div class="impec-row"><span>Trigger</span>${status('listening', 'ok', 'clock')}</div></div><div class="impec-column"><h2>Evidenced</h2><div class="impec-row"><span>Latest result</span>${status('blocked', 'warn', 'alert')}</div><p class="impec-caption">${status('verification achieved', 'ok', 'check')} Cost ${d.cost}.</p><div class="impec-row"><span>Older head</span>${status('superseded', 'warn', 'clock')}</div></div></div></section>`;
      const body = `${title(d, 'workflow ledger / three readings', 'A ledger for motion', 'Read each workflow as configuration, live work, and evidence at once. No aggregate health score can hide the domain result.')}<div class="impec-control-strip"><button class="ae-button ae-button-quiet" type="button">Workflows</button><button class="ae-button ae-button-quiet" type="button">Agents</button><button class="ae-button ae-button-quiet" type="button">Runs</button><button class="ae-button ae-button-quiet" type="button">Spend</button><button class="ae-button" type="button">Create workflow</button></div>${board}<div class="impec-ledger"><div>${topology(d)}${evidence(d)}</div><div>${agents(d)}${spend(d)}${create(d)}</div></div>${states()}`;
      return shell('impec-2', 'Workflow ledger', 'IMPEC-2 · workflow ledger', `<div class="impec-2">${body}</div>`);
    }
  },
  'IMPEC-3': {
    label: 'Evidence atlas',
    move: 'Invert the landing assumption: live and historical evidence leads the viewport; configured workflow topology is the stable annotation beside it.',
    philosophy: 'When an operator arrives because something happened, the first question is “what needs attention?” Evidence becomes the atlas spine, while the workflow roster remains visible as context and the graph stays immutable beneath the selected run.',
    render(corpus) {
      const d = model(corpus);
      const variant = `.impec-3 .impec-atlas { display:grid; grid-template-columns:minmax(14rem,.55fr) minmax(0,1.45fr); gap:1.25rem; } .impec-3 .impec-spine { min-width:0; padding-right:1rem; border-right:1px solid var(--ae-line); } .impec-3 .impec-spine h2 { margin:0 0 .75rem; font-weight:550; } .impec-3 .impec-spine .ae-list-row { display:block; padding:.75rem 0; } .impec-3 .impec-spine .ae-list-row.is-selected { color:var(--ae-accent); } .impec-3 .impec-context { min-width:0; } .impec-3 .impec-context-head { display:flex; align-items:baseline; justify-content:space-between; gap:1rem; padding-bottom:.75rem; border-bottom:1px solid var(--ae-line); } .impec-3 .impec-context-head h2 { margin:0; font-weight:550; } .impec-3 .impec-matrix { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:1rem; } @media(max-width:760px){.impec-3 .impec-atlas{grid-template-columns:1fr}.impec-3 .impec-spine{padding-right:0;border-right:0;border-bottom:1px solid var(--ae-line);padding-bottom:1rem}.impec-3 .impec-matrix{grid-template-columns:1fr}}`;
      const evidenceSpine = `<aside class="impec-spine" aria-label="Evidence-first run index"><h2>What needs attention</h2><div class="ae-list-rows"><div class="ae-list-row is-selected"><strong>${d.run}</strong><br>${status('executing', 'ok', 'play')}<p class="impec-caption">${d.workflow}<br>${d.agent} · ${d.cost}</p></div><div class="ae-list-row"><strong>run/pr-review-1839</strong><br>${status('verification achieved', 'ok', 'check')}<p class="impec-caption">domain result blocked<br>reported · $0.38</p></div><div class="ae-list-row"><strong>run/pr-review-1834</strong><br>${status('superseded', 'warn', 'clock')}<p class="impec-caption">no post emitted<br>cost unavailable</p></div><div class="ae-list-row"><strong>No active run</strong><br>${status('idle', 'warn', 'clock')}<p class="impec-caption">Canary incident resolution<br>draft workflow</p></div></div><div class="impec-actions"><button class="ae-button" type="button">open selected evidence</button></div></aside>`;
      const body = `${title(d, 'evidence atlas / inverted landing', 'The evidence atlas', 'Start from the live and historical facts that brought you here. Configuration remains adjacent, stable, and inspectable.')}<div class="impec-atlas">${evidenceSpine}<div class="impec-context"><div class="impec-context-head"><h2>${d.workflow}</h2><span class="impec-meta">stable configuration · revision r17</span></div><div class="impec-matrix"><div>${topology(d)}</div><div>${runOverlay(d)}</div></div>${roster(d)}${agents(d)}${spend(d)}${create(d)}${states()}</div></div>`;
      return shell('impec-3', 'Evidence atlas', 'IMPEC-3 · evidence atlas', `<div class="impec-3">${body}</div>`);
    }
  }
};

export { SPECS };
