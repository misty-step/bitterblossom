import { CORPUS } from './data.js';
import { SPECS as PRECISION } from './lanes/precision.js';
import { SPECS as EDITORIAL } from './lanes/editorial.js';
import { SPECS as MOBILE } from './lanes/mobile.js';
import { SPECS as SPATIAL } from './lanes/spatial.js';
import { SPECS as OPERATIONS } from './lanes/operations.js';
import { SPECS as RESTRAINT } from './lanes/restraint.js';

const SPECS = [
  ...PRECISION,
  ...EDITORIAL,
  ...MOBILE,
  ...SPATIAL,
  ...OPERATIONS,
  ...RESTRAINT,
];

const app = document.querySelector('#app');
const query = new URLSearchParams(location.search);
const requested = query.get('option');
const state = {
  option: Math.max(0, SPECS.findIndex((spec) => spec.id === requested)),
  view: 'workflows',
  selected: null,
  workflowStage: 'configuration',
  labOpen: false,
  theme: localStorage.getItem('bb-lab-005-theme') || 'light',
};

const esc = (value) =>
  String(value ?? '').replace(
    /[&<>\"]/g,
    (char) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '\"': '&quot;' })[char],
  );

const icon = (name, tone = '') => {
  const paths = {
    check: '<path d="m5 12 4 4L19 6"/>',
    progress: '<circle cx="12" cy="12" r="9"/><path d="M12 3a9 9 0 0 1 9 9"/>',
    warn: '<path d="M12 3 2.5 20h19L12 3Z"/><path d="M12 9v5m0 3h.01"/>',
    arrow: '<path d="m9 18 6-6-6-6"/>',
    back: '<path d="m15 18-6-6 6-6"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    flower: '<circle cx="12" cy="12" r="3"/><path d="M12 16.5A4.5 4.5 0 1 1 7.5 12 4.5 4.5 0 1 1 12 7.5a4.5 4.5 0 1 1 4.5 4.5 4.5 4.5 0 1 1-4.5 4.5"/><path d="M12 7.5V9M7.5 12H9M16.5 12H15M12 16.5V15M8 8l1.88 1.88M16 8l-1.88 1.88M8 16l1.88-1.88M16 16l-1.88-1.88"/>',
  };
  return `<svg class="ae-icon ${tone}" data-lucide="${esc(name)}" aria-hidden="true" viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">${paths[name]}</svg>`;
};

const status = (glyph, label, tone = '') =>
  `<span class="status">${icon(glyph, tone)}<span>${esc(label)}</span></span>`;

function nav(spec) {
  const items = [
    ['workflows', 'Workflows'],
    ['runs', 'Runs'],
    ['agents', 'Agents'],
    ['spend', 'Spend'],
  ];
  return `<nav class="primary-nav" aria-label="Primary">${items
    .map(
      ([id, label]) =>
        `<button type="button" data-view="${id}" ${state.view === id ? 'aria-current="page"' : ''}>${label}</button>`,
    )
    .join('')}</nav>`;
}

function contextNav() {
  const staged = Boolean(state.selected);
  const items = staged
    ? [
        ['configuration', 'Configuration'],
        ['run', 'Current run'],
        ['evidence', 'Evidence'],
      ]
    : [
        ['workflows', 'Workflows'],
        ['runs', 'Runs'],
        ['agents', 'Agents'],
        ['spend', 'Spend'],
      ];
  return `<nav class="context-nav" aria-label="Workflow context">${items
    .map(([id, label]) => `<button type="button" ${id === state.workflowStage || (!staged && id === state.view) ? 'aria-current="step"' : ''} ${staged ? `data-stage="${id}"` : `data-view="${id}"`}>${esc(label)}</button>`)
    .join('')}</nav>`;
}

function chrome(spec) {
  return `<header class="product-chrome">
    <button class="wordmark ae-logo" type="button" data-view="workflows" aria-label="Bitterblossom home"><span class="ae-app-mark">${icon('flower')}</span><span class="ae-name">Bitterblossom</span></button>
    ${spec.navigation === 'top' ? nav(spec) : ''}
    <div class="chrome-actions">
      <div class="design-switcher" aria-label="Design candidates">
        <button type="button" data-prev aria-label="Previous design">${icon('back')}</button>
        <button class="lab-trigger" type="button" data-lab aria-expanded="${state.labOpen}">Design ${state.option + 1} / ${SPECS.length}</button>
        <button type="button" data-next aria-label="Next design">${icon('arrow')}</button>
      </div>
      <span class="plane-label">production</span>
      <button class="ae-button ae-button-quiet" type="button" data-theme>${state.theme}</button>
    </div>
  </header>`;
}

function shell(spec, content) {
  const side = spec.navigation === 'rail';
  const bottom = spec.navigation === 'bottom-contextual';
  return `<main class="product layout-${spec.layout} roster-${spec.roster} detail-${spec.detail} density-${spec.density} nav-${spec.navigation}">
    ${chrome(spec)}
    ${side ? `<aside class="side-nav">${nav(spec)}<button class="create-link" type="button" data-view="create">${icon('plus')} Create workflow</button></aside>` : ''}
    <section class="workspace">${content}</section>
    ${bottom ? `<footer class="bottom-nav">${spec.layout === 'command-strip' ? contextNav() : nav(spec)}</footer>` : ''}
    ${spec.navigation === 'top' ? `<footer class="mobile-nav">${nav(spec)}</footer>` : ''}
  </main>`;
}

function heading(kicker, title, copy, action = '') {
  return `<header class="view-heading"><div><span class="eyebrow">${esc(kicker)}</span><h1>${esc(title)}</h1>${copy ? `<p>${esc(copy)}</p>` : ''}</div>${action}</header>`;
}

function workflowRoster(spec) {
  const action = `<button class="ae-button primary-action" type="button" data-view="create">${icon('plus')} Create workflow</button>`;
  const rows = CORPUS.workflows
    .map((workflow, index) => {
      const current = index === 0;
      return `<button class="workflow-row" type="button" data-workflow="${esc(workflow.name)}">
        <span class="row-index">0${index + 1}</span>
        <span class="row-main"><strong>${esc(workflow.name)}</strong><small>${esc(workflow.trigger)}</small></span>
        <span class="row-route">${esc(workflow.topology.slice(1, -1).join(' → '))}</span>
        <span class="row-state">${current ? status('check', 'active', 'ae-ok') : status('check', 'draft')}</span>
        ${icon('arrow')}
      </button>`;
    })
    .join('');
  return `${heading('Plane · production', 'Workflows', 'Configured work first. Select one to inspect its current run.', action)}
    <section class="roster" aria-label="Configured workflows">
      <header><span>Configured</span><span>${CORPUS.workflows.length} workflows</span></header>
      ${rows}
    </section>`;
}

function topology(workflow, live = false) {
  return `<div class="ae-flow-viewport topology" tabindex="0" aria-label="Workflow route">
    <div class="ae-flow flow-line">${workflow.topology
      .map(
        (node, index) => `<div class="flow-node ${live && node === CORPUS.run.step ? 'is-current' : ''}">
          <span>0${index + 1}</span><strong>${esc(node)}</strong>
          ${live && node === CORPUS.run.step ? `<small>${status('progress', 'executing', 'ae-progress')}</small>` : ''}
        </div>${index < workflow.topology.length - 1 ? '<i aria-hidden="true"></i>' : ''}`,
      )
      .join('')}</div>
  </div>`;
}

function runOverlay() {
  return `<section class="run-overlay">
    <header><div><span class="eyebrow">Selected run</span><h2>${esc(CORPUS.run.id)}</h2></div>${status('progress', CORPUS.run.state, 'ae-progress')}</header>
    <dl class="facts">
      <div><dt>current step</dt><dd>${esc(CORPUS.run.step)}</dd></div>
      <div><dt>elapsed</dt><dd>${esc(CORPUS.run.elapsed)}</dd></div>
      <div><dt>domain result</dt><dd>pending</dd></div>
      <div><dt>cost</dt><dd>${esc(CORPUS.run.cost)}</dd></div>
    </dl>
    <button class="text-action" type="button" data-stage="evidence">Open run evidence ${icon('arrow')}</button>
  </section>`;
}

function stageRail() {
  return `<ol class="stage-rail"><li class="${state.workflowStage === 'configuration' ? 'is-current' : ''}"><button type="button" data-stage="configuration"><span>01</span> Configuration</button></li><li class="${state.workflowStage === 'run' ? 'is-current' : ''}"><button type="button" data-stage="run"><span>02</span> Current run</button></li><li class="${state.workflowStage === 'evidence' ? 'is-current' : ''}"><button type="button" data-stage="evidence"><span>03</span> Evidence</button></li></ol>`;
}

function compactRoster() {
  return `<aside class="compact-roster"><span class="eyebrow">Configured</span>${CORPUS.workflows.map((workflow, index) => `<button type="button" data-workflow="${esc(workflow.name)}" ${state.selected === workflow.name ? 'aria-current="true"' : ''}><span>0${index + 1}</span><strong>${esc(workflow.name)}</strong><small>${esc(workflow.lifecycle)}</small></button>`).join('')}</aside>`;
}

function configurationTask(workflow) {
  return `<section class="configuration-task"><section class="configuration">
    <header><span class="eyebrow">Stable configuration</span>${workflow.lifecycle === 'Active' ? status('check', 'active', 'ae-ok') : status('check', 'draft')}</header>
    <dl class="facts">
      <div><dt>trigger</dt><dd>${esc(workflow.trigger)}</dd></div>
      <div><dt>trigger health</dt><dd>${workflow.lifecycle === 'Active' ? status('check', 'listening', 'ae-ok') : 'not active'}</dd></div>
      <div><dt>allowance</dt><dd>${esc(workflow.budget)}</dd></div>
      <div><dt>verification policy</dt><dd>${workflow.lifecycle === 'Active' ? 'Required' : 'Not set'}</dd></div>
    </dl>
    <button class="text-action" type="button">Edit stable configuration ${icon('arrow')}</button>
  </section>${topology(workflow, false)}</section>`;
}

function runtimeTask(workflow) {
  if (workflow.name !== CORPUS.workflows[0].name) return `<section class="empty-task"><span class="eyebrow">Current run</span><h2>No active run</h2><p>This workflow has not been activated.</p><button class="text-action" type="button" data-stage="configuration">Return to configuration ${icon('arrow')}</button></section>`;
  return `<section class="runtime-task">${topology(workflow, true)}${runOverlay()}</section>`;
}

function evidenceTask(workflow) {
  return `<section class="evidence-task">${runsView(true, workflow.name)}</section>`;
}

function workflowDetail(spec, workflow) {
  const isActive = workflow.name === CORPUS.workflows[0].name;
  const back = `<button class="back-action" type="button" data-back>${icon('back')} All workflows</button>`;
  const stageAction = state.workflowStage === 'configuration' && isActive
    ? '<button class="ae-button primary-action" type="button" data-stage="run">View current run</button>'
    : state.workflowStage === 'run'
      ? '<button class="ae-button primary-action" type="button" data-stage="evidence">Open evidence</button>'
      : state.workflowStage === 'evidence'
        ? '<button class="ae-button" type="button" data-stage="configuration">Configuration</button>'
        : '';
  const task = state.workflowStage === 'run' ? runtimeTask(workflow) : state.workflowStage === 'evidence' ? evidenceTask(workflow) : configurationTask(workflow);
  const runtimeTitle = isActive ? CORPUS.run.id : `${workflow.name} · no active run`;
  const title = state.workflowStage === 'configuration' ? workflow.name : state.workflowStage === 'run' ? runtimeTitle : `${workflow.name} evidence`;
  const kicker = state.workflowStage === 'configuration' ? 'Workflow · configured truth' : state.workflowStage === 'run' ? 'Selected runtime' : 'Immutable record';
  const context = spec.layout === 'sequence' ? stageRail() : '';
  const persistentRoster = spec.layout === 'split-register' ? compactRoster() : '';
  return `${back}${heading(kicker, title, state.workflowStage === 'configuration' ? workflow.trigger : '', stageAction)}${context}<div class="task-layout">${persistentRoster}<div class="task-surface">${task}</div></div>`;
}

function runsView(embedded = false, workflowName = null) {
  const runs = workflowName ? CORPUS.recentRuns.filter((run) => run.workflow === workflowName) : CORPUS.recentRuns;
  return `${embedded ? '' : heading('Evidence', 'Runs', 'Execution, domain result, verification, and cost remain separate.')}
    <section class="ledger"><header><span>Workflow / ref</span><span>Execution</span><span>Domain</span><span>Verification</span><span>Cost</span></header>${runs
      .map((run) => `<article><span><strong>${esc(run.workflow)}</strong><small>${esc(run.ref)}</small></span><span>${run.execution === 'Succeeded' ? status('check', run.execution, 'ae-ok') : run.execution === 'Superseded' ? status('warn', run.execution, 'ae-warn') : esc(run.execution)}</span><span>${esc(run.domain)}</span><span>${esc(run.verification)}</span><span>${esc(run.cost)}</span></article>`)
      .join('')}</section>`;
}

function agentsView() {
  const group = (name) => `<section class="agent-group"><h2>${name}</h2>${CORPUS.agents.filter((agent) => agent.availability === name).map((agent) => `<article><header><strong>${esc(agent.name)}</strong>${agent.active ? status('progress', 'in use', 'ae-progress') : status('check', 'available')}</header><dl class="facts"><div><dt>model</dt><dd>${esc(agent.model)}</dd></div><div><dt>authority</dt><dd>${esc(agent.ceiling)}</dd></div><div><dt>workflows</dt><dd>${esc(agent.workflows)}</dd></div></dl></article>`).join('')}</section>`;
  return `${heading('Roster', 'Agents', 'Availability is not authority.')}<div class="agent-columns">${group('In use')}${group('Available')}</div>`;
}

function spendView() {
  return `${heading('Controls', 'Spend', 'Unavailable never becomes zero.')}
    <div class="spend-layout"><section><h2>Control scopes</h2>${CORPUS.spend.scopes.map((scope, index) => `<div class="scope-row"><span>0${index + 1}</span><strong>${esc(scope)}</strong><button class="text-action" type="button">Inspect</button></div>`).join('')}</section><section><h2>Accounting truth</h2>${CORPUS.spend.truth.map((truth) => `<article class="truth-row"><header><strong>${esc(truth.label)}</strong>${truth.label === 'Unavailable' ? icon('warn', 'ae-warn') : icon('check', truth.label === 'Reported' ? 'ae-ok' : '')}</header><p>${esc(truth.value)}</p><small>${esc(truth.state)}</small></article>`).join('')}</section></div>`;
}

function createView() {
  return `${heading('New workflow · step 1 of 7', 'What should happen?', 'Start with the durable goal. Authority and activation come later.')}
    <section class="create-flow"><label><span class="eyebrow">Goal</span><textarea class="ae-input">${esc(CORPUS.creation.rawGoal)}</textarea></label><div class="enhanced"><span class="eyebrow">Enhanced goal · review required</span><p>${esc(CORPUS.creation.enhancedGoal)}</p></div><footer><span>${status('warn', CORPUS.creation.test, 'ae-warn')}</span><button class="ae-button primary-action" type="button">Accept goal and continue ${icon('arrow')}</button></footer></section>`;
}

function content(spec) {
  if (state.view === 'runs') return runsView();
  if (state.view === 'agents') return agentsView();
  if (state.view === 'spend') return spendView();
  if (state.view === 'create') return createView();
  if (state.selected) {
    const workflow = CORPUS.workflows.find((item) => item.name === state.selected);
    return workflowDetail(spec, workflow);
  }
  return workflowRoster(spec);
}

function labControls(spec) {
  return state.labOpen ? `<aside class="lab-panel"><header><strong>Candidate ${state.option + 1} of ${SPECS.length}</strong><button type="button" data-lab>Close</button></header><label>All candidates<select data-option>${SPECS.map((item, index) => `<option value="${index}" ${index === state.option ? 'selected' : ''}>${index + 1}. ${esc(item.id)} · ${esc(item.label)}</option>`).join('')}</select></label><dl><div><dt>philosophy</dt><dd>${esc(spec.philosophy)}</dd></div><div><dt>move</dt><dd>${esc(spec.move)}</dd></div><div><dt>phone</dt><dd>${esc(spec.mobile)}</dd></div></dl><footer><button type="button" data-prev>Previous</button><span>${state.option + 1} / ${SPECS.length}</span><button type="button" data-next>Next</button></footer></aside>` : '';
}

function render() {
  const spec = SPECS[state.option];
  document.documentElement.classList.toggle('dark', state.theme === 'dark');
  document.documentElement.classList.toggle('light', state.theme === 'light');
  app.innerHTML = `${shell(spec, content(spec))}${labControls(spec)}`;
  history.replaceState(null, '', `?option=${encodeURIComponent(spec.id)}`);
}

app.addEventListener('click', (event) => {
  const target = event.target.closest('button');
  if (!target) return;
  if (target.matches('[data-view]')) {
    state.view = target.dataset.view;
    state.selected = null;
  } else if (target.matches('[data-workflow]')) {
    state.selected = target.dataset.workflow;
    state.workflowStage = 'configuration';
  } else if (target.matches('[data-stage]')) {
    state.workflowStage = target.dataset.stage;
  } else if (target.matches('[data-back]')) {
    state.selected = null;
    state.workflowStage = 'configuration';
  } else if (target.matches('[data-theme]')) {
    state.theme = state.theme === 'light' ? 'dark' : 'light';
    localStorage.setItem('bb-lab-005-theme', state.theme);
  } else if (target.matches('[data-lab]')) {
    state.labOpen = !state.labOpen;
  } else if (target.matches('[data-prev]')) {
    state.option = (state.option - 1 + SPECS.length) % SPECS.length;
  } else if (target.matches('[data-next]')) {
    state.option = (state.option + 1) % SPECS.length;
  }
  render();
});

app.addEventListener('change', (event) => {
  if (event.target.matches('[data-option]')) {
    state.option = Number(event.target.value);
    render();
  }
});

addEventListener('keydown', (event) => {
  if (event.key === '[' || event.key === ']') {
    state.option = (state.option + (event.key === ']' ? 1 : -1) + SPECS.length) % SPECS.length;
    render();
  }
});

render();
