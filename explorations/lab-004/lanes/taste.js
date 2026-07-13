const DEFAULT_CORPUS = {
  workflows: [{
    id: 'pr-review',
    name: 'PR Review',
    state: 'active',
    trigger: 'listening',
    event: 'pull_request.opened',
    goal: 'Review changed code and report the evidence before merge.',
    revision: 'rev.18',
    lastRun: '12 min ago',
  }],
  agents: [
    { name: 'Ariadne Quill', role: 'reviewer', state: 'in-use', model: 'openrouter / qwen3-coder' },
    { name: 'Morrow Finch', role: 'verifier', state: 'available', model: 'openrouter / gemini-flash' },
    { name: 'Sable Hart', role: 'arbiter', state: 'available', model: 'openrouter / deepseek-v3' },
  ],
  run: {
    id: 'run-0713-8d4a',
    state: 'succeeded',
    domain: 'blocked',
    verification: 'achieved',
    cost: '$0.084 / reported',
    started: '13:08:41',
    finished: '13:11:06',
  },
  spend: { today: '$1.42', limit: '$4.00', month: '$18.76', estimate: '$0.11' },
  history: [
    { time: '13:11', actor: 'Sable Hart', body: 'Verification achieved; domain result remains blocked.', mark: 'ok' },
    { time: '13:10', actor: 'Ariadne Quill', body: 'Found one blocking review fingerprint in changed files.', mark: 'warn' },
    { time: '13:08', actor: 'Bitterblossom', body: 'Accepted pull_request.opened and leased the workflow.', mark: 'ok' },
  ],
};

const esc = (value) => String(value ?? '—')
  .replace(/&/g, '&amp;')
  .replace(/</g, '&lt;')
  .replace(/>/g, '&gt;')
  .replace(/"/g, '&quot;')
  .replace(/'/g, '&#39;');

const first = (value, fallback) => Array.isArray(value) && value.length ? value[0] : (value || fallback);

function normalize(input) {
  const source = input || {};
  const defaults = DEFAULT_CORPUS;
  return {
    workflows: source.workflows || (source.workflow ? [source.workflow] : defaults.workflows),
    agents: source.agents || defaults.agents,
    run: { ...defaults.run, ...(source.run || {}) },
    spend: { ...defaults.spend, ...(source.spend || {}) },
    history: source.history || defaults.history,
    creation: source.creation || {
      goal: defaults.workflows[0].goal,
      enhanced: 'Review pull requests, preserve evidence, and stop when a domain blocker is verified.',
      fixture: 'pull_request.opened → review → verify',
    },
  };
}

function icon(kind, className = '') {
  const paths = {
    check: '<path d="m5 12 4 4L19 6"/>',
    alert: '<path d="m10.3 2.6-8.4 14a2 2 0 0 0 1.7 3h16.8a2 2 0 0 0 1.7-3l-8.4-14a2 2 0 0 0-3.4 0Z"/><path d="M12 9v4M12 17h.01"/>',
    x: '<circle cx="12" cy="12" r="9"/><path d="m15 9-6 6M9 9l6 6"/>',
    play: '<path d="m9 6 10 6-10 6V6Z"/>',
    book: '<path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2Z"/>',
    chevron: '<path d="m9 18 6-6-6-6"/>',
  };
  return `<svg class="${esc(className)}" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="square" stroke-linejoin="miter">${paths[kind] || paths.chevron}</svg>`;
}

function status(label, kind = 'ok') {
  const glyph = kind === 'warn' ? 'alert' : kind === 'err' ? 'x' : 'check';
  const tone = kind === 'warn' ? 'ae-warn' : kind === 'err' ? 'ae-err' : 'ae-ok';
  return `<span class="ae-status">${icon(glyph, tone)}<span>${esc(label)}</span></span>`;
}

function shellStyle(variant) {
  return `<style>
    .taste-stage{min-height:100dvh;display:grid;overflow:hidden;}
    .taste-rail{display:flex;flex-direction:column;gap:1.25rem;padding:1rem;}
    .taste-main{min-width:0;overflow:auto;padding:1rem;}
    .taste-stack{display:grid;gap:1rem;}
    .taste-grid{display:grid;gap:1rem;}
    .taste-grid--rail{grid-template-columns:minmax(14rem,19rem) minmax(0,1fr);}
    .taste-grid--desk{grid-template-columns:minmax(0,1.5fr) minmax(14rem,.7fr);}
    .taste-grid--evidence{grid-template-columns:minmax(0,1fr) minmax(15rem,.42fr);}
    .taste-grid--three{grid-template-columns:repeat(3,minmax(0,1fr));}
    .taste-row{display:flex;align-items:center;justify-content:space-between;gap:1rem;min-width:0;}
    .taste-row--start{align-items:flex-start;}
    .taste-row--wrap{flex-wrap:wrap;}
    .taste-nav{display:grid;gap:.2rem;}
    .taste-list{display:grid;gap:0;list-style:none;margin:0;padding:0;}
    .taste-list li{min-width:0;}
    .taste-list a{display:flex;align-items:center;justify-content:space-between;gap:.75rem;}
    .taste-flow{min-height:10rem;}
    .taste-flow--wide{min-height:14rem;}
    .taste-flow--tall{min-height:24rem;}
    .taste-columns{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:0;}
    .taste-columns>*{min-width:0;}
    .taste-trail{display:grid;gap:1rem;}
    .taste-create{display:grid;gap:.75rem;}
    .taste-create textarea{width:100%;min-height:5rem;resize:vertical;}
    .taste-create input{width:100%;}
    .taste-figure{display:grid;gap:.4rem;}
    .taste-figure svg{width:100%;height:2.5rem;}
    .taste-figure--large svg{height:10rem;}
    .taste-sticky{position:sticky;top:0;align-self:start;}
    .taste-spread{display:grid;gap:0;}
    .taste-spread>*{min-width:0;}
    .taste-cursor{cursor:pointer;}
    .taste-screen{grid-column:1/-1;}
    @media (min-width:768px){
      .taste-stage--rail{grid-template-columns:13rem minmax(0,1fr);}
      .taste-stage--board{grid-template-rows:auto minmax(0,1fr);}
      .taste-stage--evidence{grid-template-columns:minmax(14rem,.4fr) minmax(0,1.6fr);}
      .taste-stage--evidence .taste-rail{grid-row:1/-1;}
      .taste-board-bar{grid-column:1/-1;}
      .taste-main--board{padding:1.5rem;}
      .taste-main--evidence{padding:1.5rem 2rem;}
    }
    @media (max-width:767px){
      .taste-stage{display:block;overflow:visible;}
      .taste-rail{padding:.75rem;}
      .taste-main{padding:.75rem;}
      .taste-grid--rail,.taste-grid--desk,.taste-grid--evidence,.taste-grid--three{grid-template-columns:1fr;}
      .taste-columns{grid-template-columns:1fr;}
      .taste-columns>*+*{margin-top:1rem;}
      .taste-sticky{position:static;}
      .taste-flow--tall{min-height:18rem;}
    }
  </style>`;
}

function modeScript() {
  return `<script>
    (() => {
      const root = document.documentElement;
      const saved = localStorage.getItem('ae-mode');
      if (saved === 'light' || saved === 'dark') { root.classList.add(saved); root.style.colorScheme = saved; }
      const button = document.querySelector('[data-theme-toggle]');
      if (!button) return;
      const sync = () => { button.textContent = root.classList.contains('dark') ? 'light mode' : 'dark mode'; };
      button.addEventListener('click', () => {
        const next = root.classList.contains('dark') ? 'light' : 'dark';
        root.classList.remove('light', 'dark'); root.classList.add(next); root.style.colorScheme = next;
        localStorage.setItem('ae-mode', next); sync();
      });
      sync();
    })();
  </script>`;
}

function frame(title, subtitle, body, variant) {
  return `<!doctype html><html data-ae-theme="ultramarine"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>${esc(title)}</title><link rel="stylesheet" href="https://cdn.jsdelivr.net/gh/misty-step/aesthetic@v0.25.0/aesthetic.css">${shellStyle(variant)}</head><body><div class="ae-shell taste-stage taste-stage--${variant}">${body}</div>${modeScript()}</body></html>`;
}

function rail(active = 'Workflows') {
  const links = ['Workflows', 'Agents', 'Runs', 'Spend', 'Create workflow'];
  return `<aside class="ae-rail taste-rail"><div class="ae-logo">${icon('book', 'ae-app-mark')}<span>Bitterblossom</span></div><nav class="taste-nav" aria-label="Primary">${links.map((link) => `<a class="${link === active ? 'ae-nav-item ae-active' : 'ae-nav-item'}" href="#${link.toLowerCase().replaceAll(' ', '-')}">${esc(link)}</a>`).join('')}</nav><div class="ae-settings"><div class="ae-setting-row"><span>plane</span><strong>authenticated</strong></div><div class="ae-setting-row"><span>revision</span><strong>rev.18</strong></div></div><button class="ae-btn taste-cursor" data-theme-toggle type="button" aria-label="Toggle light and dark mode">dark mode</button></aside>`;
}

function header(eyebrow, title, detail, actions = '') {
  return `<header class="ae-doc"><div class="taste-row taste-row--wrap"><div><p class="ae-label">${esc(eyebrow)}</p><h1 class="ae-strong">${esc(title)}</h1><p>${esc(detail)}</p></div><div class="taste-row">${actions}</div></div></header>`;
}

function statBand(data) {
  const values = [['workflows', data.workflows.length], ['live run', data.run.state], ['today', data.spend.today], ['agents', data.agents.length]];
  return `<div class="ae-stat-badges">${values.map(([label, value]) => `<div class="ae-stat-badge"><strong class="ae-num">${esc(value)}</strong><span>${esc(label)}</span></div>`).join('')}</div>`;
}

function workflowRoster(data, compact = false) {
  return `<section class="ae-plate"><div class="taste-row"><h2 class="ae-strong">Configured workflows</h2><span class="ae-tag">${data.workflows.length} total</span></div><ul class="ae-list-rows taste-list">${data.workflows.map((workflow) => `<li class="ae-list-row ${compact ? 'ae-active' : ''}"><div class="ae-list-cell"><strong>${esc(workflow.name)}</strong><span class="ae-meta">${esc(workflow.revision || 'immutable revision')} · ${esc(workflow.event || workflow.trigger || 'trigger')}</span></div><div class="ae-list-cell">${status(workflow.state || 'active', 'ok')}<span class="ae-meta">${esc(workflow.lastRun || 'no run')}</span></div></li>`).join('')}</ul><div class="ae-rule"></div><div class="taste-row"><span class="ae-label">trigger health</span>${status('listening', 'ok')}</div></section>`;
}

function topology(data, mode = 'default') {
  const run = data.run;
  return `<section class="ae-plate ${mode === 'wide' ? 'taste-flow--wide' : 'taste-flow'}"><div class="taste-row taste-row--wrap"><div><p class="ae-label">PR Review topology</p><h2 class="ae-strong">event → review → verify</h2></div><span class="ae-tag">selected ${esc(run.id)}</span></div><div class="ae-flow"><div class="ae-flow-node"><span class="ae-label">01 · trigger</span><strong>pull_request.opened</strong>${status('listening', 'ok')}</div><div class="ae-flow-node"><span class="ae-label">02 · agent</span><strong>Ariadne Quill</strong>${status('executing', 'ok')}</div><div class="ae-flow-node"><span class="ae-label">03 · gate</span><strong>verification</strong>${status(run.verification || 'achieved', 'ok')}</div></div><div class="ae-rule"></div><div class="taste-row taste-row--wrap"><span class="ae-meta">live-run overlay · ${esc(run.started)}—${esc(run.finished)}</span>${status(`execution ${run.state}`, 'ok')}${status(`domain ${run.domain}`, 'warn')}</div></section>`;
}

function runEvidence(data) {
  const run = data.run;
  return `<section class="ae-plate"><div class="taste-row"><h2 class="ae-strong">Selected run evidence</h2><span class="ae-tag">${esc(run.id)}</span></div><div class="ae-table"><div class="ae-table-row"><span>workflow state</span><strong>active</strong></div><div class="ae-table-row"><span>run lifecycle</span><strong>${esc(run.state)}</strong></div><div class="ae-table-row"><span>domain result</span><strong>${esc(run.domain)}</strong></div><div class="ae-table-row"><span>verification</span><strong>${esc(run.verification)}</strong></div><div class="ae-table-row"><span>cost truth</span><strong>${esc(run.cost)}</strong></div></div><div class="ae-rule"></div><div class="taste-row taste-row--wrap">${status('verification achieved', 'ok')}${status('domain result blocked', 'warn')}<span class="ae-meta">no active run after resolution</span></div></section>`;
}

function agentRoster(data, wall = false) {
  return `<section class="${wall ? 'ae-wall' : 'ae-plate'}"><div class="taste-row"><h2 class="ae-strong">Agents</h2><span class="ae-tag">roster pinned</span></div><div class="taste-grid taste-grid--three">${['in-use', 'available'].map((bucket) => `<div><p class="ae-label">${bucket === 'in-use' ? 'In use' : 'Available'}</p><ul class="ae-list-rows taste-list">${data.agents.filter((agent) => agent.state === bucket).map((agent) => `<li class="ae-list-row"><div class="ae-list-cell"><strong>${esc(agent.name)}</strong><span class="ae-meta">${esc(agent.role)} · ${esc(agent.model)}</span></div>${bucket === 'in-use' ? status('working', 'ok') : status('ready', 'ok')}</li>`).join('')}</ul></div>`).join('')}</div></section>`;
}

function spend(data) {
  return `<section class="ae-plate"><div class="taste-row"><h2 class="ae-strong">Spend controls</h2><span class="ae-tag">reported + estimated</span></div><div class="ae-table"><div class="ae-table-row"><span>today / limit</span><strong>${esc(data.spend.today)} / ${esc(data.spend.limit)}</strong></div><div class="ae-table-row"><span>month to date</span><strong>${esc(data.spend.month)}</strong></div><div class="ae-table-row"><span>next estimate</span><strong>${esc(data.spend.estimate)} / estimated</strong></div><div class="ae-table-row"><span>unavailable fields</span><strong>provider latency</strong></div></div><div class="ae-meter"><span style="width:35%"></span></div></section>`;
}

function history(data) {
  return `<section class="ae-plate"><div class="taste-row"><h2 class="ae-strong">Live / history</h2><span class="ae-tag">append-only</span></div><div class="ae-trail taste-trail">${data.history.map((entry) => `<article class="ae-trail-entry"><div class="ae-trail-mark">${icon(entry.mark === 'warn' ? 'alert' : 'check', entry.mark === 'warn' ? 'ae-warn' : 'ae-ok')}</div><div><div class="taste-row"><span class="ae-meta">${esc(entry.time)}</span><strong>${esc(entry.actor)}</strong></div><p>${esc(entry.body)}</p></div></article>`).join('')}</div></section>`;
}

function creation(data) {
  return `<section class="ae-plate taste-create" id="create-workflow"><div class="taste-row"><h2 class="ae-strong">Create workflow</h2><span class="ae-tag">goal first</span></div><label class="ae-label" for="goal">Goal</label><textarea id="goal">${esc(data.creation.goal)}</textarea><div class="ae-settings"><div class="ae-setting-row"><span>enhanced goal review</span><strong>${esc(data.creation.enhanced)}</strong></div><div class="ae-setting-row"><span>fixture test</span><strong>${esc(data.creation.fixture)}</strong></div></div><button class="ae-btn ae-primary taste-cursor" type="button">review enhanced goal</button></section>`;
}

function states() {
  return `<section class="ae-plate"><div class="taste-row"><h2 class="ae-strong">State register</h2><span class="ae-tag">not one traffic light</span></div><div class="taste-grid taste-grid--three"><div>${status('active', 'ok')}<span class="ae-meta">workflow</span></div><div>${status('draft', 'warn')}<span class="ae-meta">configuration</span></div><div>${status('listening trigger', 'ok')}<span class="ae-meta">ingress</span></div><div>${status('executing', 'ok')}<span class="ae-meta">lifecycle</span></div><div>${status('succeeded / blocked', 'warn')}<span class="ae-meta">domain</span></div><div>${status('superseded', 'warn')}<span class="ae-meta">revision</span></div><div>${status('unavailable cost', 'warn')}<span class="ae-meta">spend</span></div><div>${status('no active run', 'ok')}<span class="ae-meta">empty state</span></div><div>${status('verification achieved', 'ok')}<span class="ae-meta">proof</span></div></div></section>`;
}

function renderLedger(corpus) {
  const data = normalize(corpus);
  return frame('Bitterblossom · ledger rail', 'Ledger Rail', `<aside>${rail('Workflows')}</aside><main class="taste-main taste-stack">${header('workflow control plane / ledger rail', 'Configured work, at a glance', 'The rail is the operator’s index; the desk is the selected workflow and its evidence.', '<button class="ae-btn ae-primary taste-cursor" type="button">run workflow</button>')}${statBand(data)}<div class="taste-grid taste-grid--rail"><div class="taste-stack">${workflowRoster(data, true)}${agentRoster(data)}${spend(data)}</div><div class="taste-stack">${topology(data)}${runEvidence(data)}${history(data)}${states()}${creation(data)}</div></div></main>`, 'rail');
}

function renderBoard(corpus) {
  const data = normalize(corpus);
  return frame('Bitterblossom · flowboard', 'Flowboard', `<header class="ae-header taste-board-bar taste-rail"><div class="ae-logo">${icon('book', 'ae-app-mark')}<span>Bitterblossom / flowboard</span></div><div class="taste-row"><span class="ae-meta">authenticated · rev.18</span><button class="ae-btn taste-cursor" data-theme-toggle type="button">dark mode</button></div></header><main class="taste-main taste-main--board taste-stack">${header('topology first / selected run overlay', 'The work is a flow', 'The workflow is read as a route through trigger, agent, and proof; configuration follows the path.', '<a class="ae-link" href="#create-workflow">create workflow</a>')}${topology(data, 'wide')}<div class="taste-grid taste-grid--desk"><div class="taste-stack">${workflowRoster(data)}${agentRoster(data, true)}</div><div class="taste-stack">${runEvidence(data)}${spend(data)}${history(data)}</div></div><div class="taste-grid taste-grid--three">${states()}${creation(data)}</div></main>`, 'board');
}

function renderEvidence(corpus) {
  const data = normalize(corpus);
  return frame('Bitterblossom · evidence spine', 'Evidence Spine', `<aside>${rail('Runs')}</aside><main class="taste-main taste-main--evidence taste-stack"><div class="taste-row taste-row--wrap"><div><p class="ae-label">runs / durable evidence</p><h1 class="ae-strong">What happened, then why</h1><p>History is the primary surface; stable configuration remains beside it as context.</p></div><button class="ae-btn taste-cursor" data-theme-toggle type="button">dark mode</button></div>${statBand(data)}<div class="taste-grid taste-grid--evidence"><div class="taste-stack">${history(data)}${topology(data)}${runEvidence(data)}</div><aside class="taste-stack taste-sticky">${workflowRoster(data)}${agentRoster(data)}${spend(data)}${states()}${creation(data)}</aside></div></main>`, 'evidence');
}

const SPECS = {
  'TASTE-1': {
    label: 'Ledger Rail',
    move: 'Permanent rail index; selected workflow desk carries topology and evidence.',
    philosophy: 'A quiet operator instrument: stable destinations stay reachable while the active workflow earns the working surface.',
    render: renderLedger,
  },
  'TASTE-2': {
    label: 'Flowboard',
    move: 'Topology becomes the primary canvas; roster, evidence, and controls orbit the selected run.',
    philosophy: 'Read the system as a route, not a list: the live overlay is the organizing gesture and every state remains named.',
    render: renderBoard,
  },
  'TASTE-3': {
    label: 'Evidence Spine',
    move: 'Inverts configuration-first navigation: append-only history is the main desk, configuration is contextual.',
    philosophy: 'An operator trusts what can be traced. Start with the event trail, then expose the immutable setup that explains it.',
    render: renderEvidence,
  },
};

export { SPECS };
