const DATA = {
  workflows: [
    {
      id: 'pr-review',
      name: 'PR Review',
      description: 'Review every ready pull-request head and publish one grounded formal review.',
      lifecycle: 'Active',
      trigger: 'GitHub · pull_request.synchronize',
      agents: ['Cerberus', 'Verifier'],
      budget: '$18 / run group',
      latest: 'Blocked · verification achieved',
      steps: [
        ['radio-tower', 'GitHub event', 'pull_request.synchronize'],
        ['bot', 'Cerberus', 'Formal review'],
        ['shield-check', 'Verifier', 'Read-only proof'],
        ['flag', 'Review posted', 'Terminal receipt'],
      ],
    },
    {
      id: 'canary-resolution',
      name: 'Canary Resolution',
      description: 'Triage an incident signal, verify the diagnosis, and record a bounded resolution path.',
      lifecycle: 'Draft',
      trigger: 'Canary · incident.opened',
      agents: ['Incident Hound', 'Verifier'],
      budget: '$12 / run group',
      latest: 'No runs',
      steps: [
        ['radio-tower', 'Canary event', 'incident.opened'],
        ['bot', 'Incident Hound', 'Triage'],
        ['shield-check', 'Verifier', 'Resolution proof'],
        ['flag', 'Post-mortem', 'Terminal record'],
      ],
    },
  ],
  runs: [
    { id: 'run-84e08b7', ref: 'head 84e08b7', workflow: 'PR Review', workflowId: 'pr-review', state: 'Executing', started: '13 minutes ago', result: 'Pending', verification: 'In progress', cost: '$3.18 reported', active: true },
    { id: 'run-65078ec', ref: 'head 65078ec', workflow: 'PR Review', workflowId: 'pr-review', state: 'Succeeded', started: 'Yesterday, 18:42', result: 'Blocked', verification: 'Achieved', cost: '$7.42 reported' },
    { id: 'run-18af11c', ref: 'head 18af11c', workflow: 'PR Review', workflowId: 'pr-review', state: 'Superseded', started: 'Jul 11, 09:14', result: '—', verification: 'Not required', cost: '$1.06 reported' },
  ],
  agents: [
    { id: 'cerberus', name: 'Cerberus', description: 'Inspects a change and submits one formal, evidence-grounded review.', model: 'gpt-5.5 · xhigh', harness: 'Roster', authority: 'Read code · submit formal review', workflows: ['PR Review'], revision: 'rev. 12' },
    { id: 'verifier', name: 'Verifier', description: 'Reproduces claims and records achieved, not achieved, or inconclusive evidence.', model: 'gpt-5.6-luna · high', harness: 'Roster', authority: 'Read-only · verification commands', workflows: ['PR Review', 'Canary Resolution'], revision: 'rev. 8' },
    { id: 'incident-hound', name: 'Incident Hound', description: 'Builds a bounded incident diagnosis from live service evidence.', model: 'gpt-5.6-luna · xhigh', harness: 'Roster', authority: 'Read telemetry · propose remediation', workflows: ['Canary Resolution'], revision: 'draft. 3' },
  ],
};

const app = document.querySelector('#app');
const root = document.documentElement;
const storedTheme = root.classList.contains('dark')
  ? 'dark'
  : root.classList.contains('light')
    ? 'light'
    : matchMedia('(prefers-color-scheme: dark)').matches
      ? 'dark'
      : 'light';
const state = {
  view: 'workflows', workflowId: null, runId: null, theme: storedTheme,
  navOpen: false, createStep: 0, createMode: 'new', enhanced: false, goalChoice: 'original', selectedAgents: new Set(['cerberus', 'verifier']),
  newAgent: false, newAgentName: 'Review Coordinator', newAgentResponsibility: 'Coordinate the review and commission bounded specialists when the change earns them.', trigger: 'github', goalOriginal: 'Review every ready pull-request head thoroughly and post one formal review.', idempotency: 'pr:{number}:{head_sha}',
  authority: { read: true, review: true, push: false, issues: false }, broker: 'Mint · github.review-bot',
  limits: { runGroup: '$18.00', runsDay: '60', duration: '30 minutes', output: '2 MB' },
  testedRevision: null, confirmed: false, activated: false,
  governors: { plane: '$500.00', runGroup: '$18.00', daily: '60' }, governorsSaved: false,
};

const esc = (v) => String(v ?? '').replace(/[&<>\"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '\"': '&quot;' })[c]);
const ICONS = {
  flower: 'flower-2',
  workflow: 'workflow',
  list: 'list',
  bot: 'bot',
  chart: 'chart-no-axes-column-increasing',
  plus: 'plus',
  menu: 'menu',
  close: 'x',
  arrow: 'chevron-right',
  back: 'chevron-left',
  check: 'check',
  'radio-tower': 'radio-tower',
  'shield-check': 'shield-check',
  flag: 'flag',
  chevron: 'chevron-right',
  spark: 'sparkles',
  sun: 'sun',
  moon: 'moon',
};
const icon = (name, cls = '') => `<i class="ae-icon ${cls}" data-lucide="${ICONS[name] || ICONS.workflow}" aria-hidden="true"></i>`;
const status = (label, kind = '') => `<span class="status">${icon(kind === 'ok' ? 'check' : kind === 'active' ? 'radio-tower' : 'workflow', kind ? `ae-${kind === 'active' ? 'progress' : kind}` : '')}<span>${esc(label)}</span></span>`;
const compact = () => window.matchMedia('(max-width: 720px)').matches;
const revisionFingerprint = () => JSON.stringify({ goal: state.goalChoice === 'enhanced' ? 'enhanced-v1' : state.goalOriginal, trigger: state.trigger, idempotency: state.idempotency, agents: [...state.selectedAgents], authority: state.authority, broker: state.broker, limits: state.limits });
const testIsCurrent = () => state.testedRevision === revisionFingerprint();
function invalidateProof() { state.testedRevision = null; state.confirmed = false; state.activated = false; }
function resetAuthoring(mode) { state.createMode=mode;state.createStep=0;state.enhanced=false;state.goalChoice='original';state.selectedAgents=new Set(['cerberus','verifier']);state.newAgent=false;state.newAgentName='Review Coordinator';state.newAgentResponsibility='Coordinate the review and commission bounded specialists when the change earns them.';state.trigger='github';state.goalOriginal='Review every ready pull-request head thoroughly and post one formal review.';state.idempotency='pr:{number}:{head_sha}';state.authority={read:true,review:true,push:false,issues:false};state.broker='Mint · github.review-bot';state.limits={runGroup:'$18.00',runsDay:'60',duration:'30 minutes',output:'2 MB'};invalidateProof(); }

function navigate(view, opts = {}) { state.view = view; state.workflowId = opts.workflowId ?? null; state.runId = opts.runId ?? null; state.navOpen = false; render(); }
function navItem(id, label, glyph) { const current = state.view === id || (id === 'workflows' && state.view === 'workflow') || (id === 'runs' && state.view === 'run'); return `<button type="button" data-nav="${id}" ${current ? 'aria-current="page"' : ''}>${icon(glyph)}<span>${label}</span></button>`; }
function sidebar() { return `<aside class="sidebar ${state.navOpen ? 'is-open' : ''}" ${compact()&&!state.navOpen?'inert aria-hidden="true"':''}><header><button class="ae-logo" type="button" data-nav="workflows"><span class="ae-app-mark">${icon('flower')}</span><span class="ae-name">Bitterblossom</span></button><button class="mobile-close" type="button" data-menu aria-label="Close navigation">${icon('close')}</button></header><nav aria-label="Primary">${navItem('workflows','Workflows','workflow')}${navItem('runs','Runs','list')}${navItem('agents','Agents','bot')}${navItem('spend','Spend','chart')}</nav><button class="create-nav" type="button" data-create>${icon('plus')}<span>Create workflow</span></button><footer><span>production plane</span><small>review fixture</small></footer></aside>`; }
function topbar() { const next = state.theme === 'light' ? 'dark' : 'light'; return `<header class="topbar"><button class="mobile-menu" type="button" data-menu aria-label="Open navigation">${icon('menu')}</button><span class="mobile-brand">${icon('flower')}</span><div class="top-context"><span>${state.view === 'workflow' ? 'Workflow detail' : state.view === 'run' ? 'Run detail' : state.view === 'create' ? 'Workflow authoring' : 'Operator control plane'}</span><small>fixture-backed interaction contract</small></div><button class="ae-mode" type="button" data-theme aria-label="Switch to ${next} mode" title="Switch to ${next} mode">${icon('sun','ae-sun')}${icon('moon','ae-moon')}</button></header>`; }
function heading(kicker, title, copy = '', actions = '') { return `<header class="page-heading"><div><span class="eyebrow">${esc(kicker)}</span><h1>${esc(title)}</h1>${copy ? `<p>${esc(copy)}</p>` : ''}</div>${actions ? `<div class="heading-actions">${actions}</div>` : ''}</header>`; }

function workflowsView() { return `${heading('Configured work','Workflows','Select a workflow to inspect its stable definition, execution path, and runs.','<button class="ae-button" type="button" data-create>'+icon('plus')+' Create workflow</button>')}<section class="workflow-list"><header><span>Workflow</span><span>Trigger / agents</span><span>Latest result</span><span>Allowance</span></header>${DATA.workflows.map(w=>`<button class="workflow-row" type="button" data-workflow="${w.id}"><span class="workflow-identity"><strong>${esc(w.name)}</strong><small>${esc(w.description)}</small></span><span><strong>${esc(w.trigger)}</strong><small>${esc(w.agents.join(' → '))}</small></span><span>${w.lifecycle==='Active'?status(w.lifecycle,'ok'):status(w.lifecycle)}<small>${esc(w.latest)}</small></span><span><strong>${esc(w.budget)}</strong><small>admission ceiling</small></span>${icon('chevron')}</button>`).join('')}</section>`; }
function topology(workflow) { return `<section class="section topology"><header><div><span class="eyebrow">Execution topology</span><h2>From trigger to durable result</h2></div><span>stable revision · rev. 18</span></header><ol class="topology-path">${workflow.steps.map(([glyph,name,desc],i)=>`<li><span class="topology-mark">${icon(glyph)}</span><strong>${esc(name)}</strong><small>${esc(desc)}</small>${i<workflow.steps.length-1?'<span class="route-label">then</span>':''}</li>`).join('')}</ol><p class="topology-note">Outcomes route explicitly: review clear → post approval; blocking findings → request changes; inconclusive evidence → comment without approval.</p></section>`; }
function workflowDetail() { const w=DATA.workflows.find(x=>x.id===state.workflowId)||DATA.workflows[0]; const runs=DATA.runs.filter(r=>r.workflowId===w.id); return `<button class="back" type="button" data-nav="workflows">${icon('back')} Workflows</button>${heading('Workflow',w.name,w.description,`<button class="ae-button ae-button-quiet" type="button" data-edit="${w.id}">Edit configuration</button><button class="ae-button" type="button" data-workflow-runs="${w.id}">View runs</button>`)}<section class="facts-section"><div><span>Lifecycle</span><strong>${w.lifecycle}</strong></div><div><span>Trigger</span><strong>${w.trigger}</strong></div><div><span>Agents</span><strong>${w.agents.join(' → ')}</strong></div><div><span>Run-group ceiling</span><strong>${w.budget}</strong></div></section>${topology(w)}<section class="section recent"><header><div><span class="eyebrow">Runs</span><h2>${runs.length ? 'Latest workflow runs' : 'No runs yet'}</h2></div>${runs.length?`<button class="text-button" type="button" data-workflow-runs="${w.id}">All runs ${icon('arrow')}</button>`:''}</header>${runs.slice(0,2).map(runRow).join('')||'<p class="empty-copy">Activate this workflow, then exercise its synthetic test to create the first run.</p>'}</section>`; }
function runRow(r) { return `<button class="run-row" type="button" data-run="${r.id}"><span><strong>${esc(r.ref)}</strong><small>${esc(r.workflow)} · ${esc(r.started)}</small></span><span>${status(r.state,r.active?'active':r.state==='Succeeded'?'ok':'')}</span><span><strong>${esc(r.result)}</strong><small>${esc(r.verification)}</small></span><span><strong>${esc(r.cost)}</strong><small>${r.active?'live receipt':'final receipt'}</small></span>${icon('chevron')}</button>`; }
function runsView() { const scoped=state.workflowId; const rows=DATA.runs.filter(r=>!scoped||r.workflowId===scoped).sort((a,b)=>Number(b.active)-Number(a.active)); const w=DATA.workflows.find(x=>x.id===scoped); return `${w?`<button class="back" type="button" data-workflow="${w.id}">${icon('back')} ${esc(w.name)}</button>`:''}${heading(w?'Workflow runs':'Execution history',w?`${w.name} runs`:'Runs',w?'Active first, then newest completed and superseded runs.':'Every accepted execution is inspectable; active runs sort first.')}<section class="run-list"><header><span>Run / workflow</span><span>Execution</span><span>Result / verification</span><span>Cost</span></header>${rows.map(runRow).join('')}</section>`; }
function runDetail() { const r=DATA.runs.find(x=>x.id===state.runId)||DATA.runs[0]; const executionKind=r.active?'active':r.state==='Succeeded'?'ok':''; const lifecycle=r.state==='Superseded'?`<ol class="attempt-path"><li class="is-done"><strong>Trigger accepted</strong><small>GitHub delivery 02JD…91 · deduplicated</small></li><li><strong>Run superseded</strong><small>A newer pull-request head was accepted before review execution.</small></li></ol>`:`<ol class="attempt-path"><li class="is-done"><strong>Trigger accepted</strong><small>GitHub delivery 02JD…91 · deduplicated</small></li><li class="is-done"><strong>Cerberus completed</strong><small>2 blocking findings · formal review prepared</small></li><li class="${r.active?'is-active':'is-done'}"><strong>Verifier ${r.active?'executing':'completed'}</strong><small>${r.active?'Reproducing claims':'Evidence achieved'}</small></li><li class="${r.active?'':'is-done'}"><strong>Publish result</strong><small>${r.active?'Waiting on verification':'Receipt recorded'}</small></li></ol>`; const evidence=r.state==='Superseded'?`<p class="empty-copy">No child agents ran and no review evidence was required. The provider receipt records admission work only.</p><div class="receipt"><span>Admission receipt</span><strong>${esc(r.cost)}</strong></div>`:`<div class="child-row"><strong>QA fixture</strong>${status(r.active?'Executing':'Succeeded',r.active?'active':'ok')}<small>${r.active?'Evidence in progress':'Evidence attached'}</small></div><div class="child-row"><strong>Maintainability critic</strong>${status('Succeeded','ok')}<small>2 blocking findings</small></div><div class="receipt"><span>Provider receipt</span><strong>${esc(r.cost)}</strong></div>`; return `<button class="back" type="button" data-workflow-runs="${r.workflowId}">${icon('back')} ${esc(r.workflow)} runs</button>${heading('Run',r.ref,`${r.workflow} · accepted ${r.started}`)}<section class="run-summary"><div>${status(r.state,executionKind)}<small>execution</small></div><div><strong>${esc(r.result)}</strong><small>domain result</small></div><div><strong>${esc(r.verification)}</strong><small>verification</small></div><div><strong>${esc(r.cost)}</strong><small>cost truth</small></div></section><div class="detail-columns"><section class="section"><header><div><span class="eyebrow">Lifecycle</span><h2>Attempt path</h2></div></header>${lifecycle}</section><section class="section"><header><div><span class="eyebrow">Children and evidence</span><h2>Run group</h2></div></header>${evidence}<div class="receipt"><span>Configuration revision</span><strong>rev. 18 · pinned</strong></div></section></div>`; }
function agentsView() { return `${heading('Reusable declarations','Agents','Select agents while authoring a workflow. Expand a declaration only when you need its implementation details.')}<section class="agent-list">${DATA.agents.map((a,i)=>`<details ${i===0?'open':''}><summary><span>${icon('bot')}<span><strong>${esc(a.name)}</strong><small>${esc(a.description)}</small></span></span>${icon('chevron')}</summary><div class="agent-detail"><dl><div><dt>Model</dt><dd>${esc(a.model)}</dd></div><div><dt>Harness</dt><dd>${esc(a.harness)}</dd></div><div><dt>Authority</dt><dd>${esc(a.authority)}</dd></div><div><dt>Revision</dt><dd>${esc(a.revision)}</dd></div><div><dt>Workflows</dt><dd>${esc(a.workflows.join(', ')||'Unassigned')}</dd></div></dl></div></details>`).join('')}</section>`; }
function spendView() { return `${heading('Reporting and governors','Spend','Receipts, estimates, ceilings, and coverage remain distinct.')}<section class="metric-grid"><article><span>Reported this month</span><strong>$184.62</strong><small>provider receipts</small></article><article><span>Plane ceiling</span><strong>${esc(state.governors.plane)}</strong><small>monthly governor</small></article><article><span>Forecast</span><strong>$276–$318</strong><small>estimated range</small></article><article><span>Coverage</span><strong>92%</strong><small>8% unavailable, never $0</small></article></section><div class="spend-columns"><section class="section"><header><div><span class="eyebrow">By workflow</span><h2>Month to date</h2></div></header><div class="bar-row"><span>PR Review</span><div><i style="width:72%"></i></div><strong>$132.18</strong></div><div class="bar-row"><span>Canary Resolution</span><div><i style="width:29%"></i></div><strong>$52.44</strong></div><div class="bar-row muted"><span>Unavailable</span><div><i style="width:8%"></i></div><strong>8%</strong></div></section><section class="section governors"><header><div><span class="eyebrow">Governors</span><h2>Control scopes</h2></div></header><label><span>Plane monthly ceiling</span><input class="ae-input" data-governor="plane" value="${esc(state.governors.plane)}" /></label><label><span>PR Review run-group ceiling</span><input class="ae-input" data-governor="runGroup" value="${esc(state.governors.runGroup)}" /></label><label><span>Daily run admission</span><input class="ae-input" data-governor="daily" value="${esc(state.governors.daily)}" /></label><button class="ae-button" type="button" data-save-governors>Save governors</button><span class="save-note" aria-live="polite">${state.governorsSaved?'All three values saved to the in-memory draft revision.':''}</span></section></div>`; }

const STEPS=['Goal','Trigger','Agents','Authority','Limits','Test','Activate'];
function stepper(){return `<ol class="stepper">${STEPS.map((s,i)=>`<li class="${i===state.createStep?'is-current':i<state.createStep?'is-done':''}"><button type="button" data-step="${i}"><span>${i<state.createStep?icon('check'):i+1}</span>${s}</button></li>`).join('')}</ol>`;}
function goalStep(){return `<div class="form-stack"><label><span>Original goal</span><textarea class="ae-input" data-goal>${esc(state.goalOriginal)}</textarea></label><div class="goal-tools"><button class="ae-button ae-button-quiet" type="button" data-enhance>${icon('spark')} Improve clarity with LLM</button><small>The original remains authoritative until you explicitly adopt a proposal.</small></div>${state.enhanced?`<section class="proposal"><header><span class="eyebrow">Optional enhanced goal</span>${status('proposal ready','ok')}</header><p>For each non-draft pull-request head, inspect the actual diff and repository evidence, commission bounded verification, then publish exactly one grounded formal review for the current head.</p><dl><div><dt>Assumption</dt><dd>Draft pull requests are excluded.</dd></div><div><dt>Acceptance</dt><dd>Exactly one review is published for the accepted head.</dd></div><div><dt>Boundary</dt><dd>Never push, merge, or mutate issues.</dd></div></dl><div class="choice-row"><button class="ae-button ${state.goalChoice==='original'?'is-selected':''}" type="button" data-goal-choice="original">Keep original</button><button class="ae-button ${state.goalChoice==='enhanced'?'is-selected':''}" type="button" data-goal-choice="enhanced">Use enhanced proposal</button></div></section>`:''}</div>`;}
function triggerStep(){return `<div class="form-stack"><p class="step-copy">Choose what accepts work into this workflow.</p><div class="choice-grid">${[['github','GitHub event','pull_request.synchronize'],['schedule','Schedule','Weekdays at 08:00'],['internal','Internal event','verification.requested']].map(([id,n,d])=>`<button type="button" data-trigger="${id}" class="choice ${state.trigger===id?'is-selected':''}">${icon(id==='schedule'?'list':'radio-tower')}<strong>${n}</strong><small>${d}</small></button>`).join('')}</div><label><span>Idempotency key</span><input class="ae-input" data-idempotency value="${esc(state.idempotency)}" /></label></div>`;}
function agentsStep(){return `<div class="form-stack"><p class="step-copy">Select reusable declarations in execution order.</p><div class="agent-picks">${DATA.agents.map(a=>`<label><input type="checkbox" value="${a.id}" data-agent ${state.selectedAgents.has(a.id)?'checked':''}/><span>${icon('bot')}<span><strong>${a.name}</strong><small>${a.description}</small></span></span></label>`).join('')}</div><button class="text-button" type="button" data-new-agent>${icon('plus')} No suitable declaration? Design a new agent</button>${state.newAgent?`<section class="inline-form"><label><span>Name</span><input class="ae-input" data-new-agent-name value="${esc(state.newAgentName)}" /></label><label><span>Natural-language responsibility</span><textarea class="ae-input" data-new-agent-responsibility>${esc(state.newAgentResponsibility)}</textarea></label><button class="ae-button" type="button" data-add-agent ${state.newAgentName.trim()&&state.newAgentResponsibility.trim()?'':'disabled'}>Add draft declaration</button></section>`:''}</div>`;}
function authorityStep(){return `<div class="form-stack"><p class="step-copy">Grant only the capabilities this workflow requires.</p><div class="authority-grid"><label><input type="checkbox" data-authority="read" ${state.authority.read?'checked':''}/> Read repository and pull-request diff</label><label><input type="checkbox" data-authority="review" ${state.authority.review?'checked':''}/> Submit one formal GitHub review</label><label><input type="checkbox" data-authority="push" ${state.authority.push?'checked':''}/> Push commits</label><label><input type="checkbox" data-authority="issues" ${state.authority.issues?'checked':''}/> Mutate issues</label></div><label><span>Credential broker</span><select class="ae-input" data-broker><option ${state.broker==='Mint · github.review-bot'?'selected':''}>Mint · github.review-bot</option><option ${state.broker==='None · read-only'?'selected':''}>None · read-only</option></select></label><p class="callout">Secrets are resolved by reference at execution time and never appear in the workflow revision.</p></div>`;}
function limitsStep(){return `<div class="form-stack limit-grid"><label><span>Run-group ceiling</span><input class="ae-input" data-limit="runGroup" value="${esc(state.limits.runGroup)}" /></label><label><span>Runs per day</span><input class="ae-input" data-limit="runsDay" value="${esc(state.limits.runsDay)}" /></label><label><span>Maximum duration</span><input class="ae-input" data-limit="duration" value="${esc(state.limits.duration)}" /></label><label><span>Maximum output</span><input class="ae-input" data-limit="output" value="${esc(state.limits.output)}" /></label><p class="callout">Admission limits are enforced before execution. Provider cost remains reported or estimated according to receipt coverage.</p></div>`;}
function testStep(){const passed=testIsCurrent();const agentNames=[...state.selectedAgents].map(id=>DATA.agents.find(a=>a.id===id)?.name).filter(Boolean);return `<div class="form-stack"><p class="step-copy">Exercise the complete workflow with a synthetic, non-publishing event before activation.</p><section class="test-card"><header><div><span class="eyebrow">Synthetic workflow test</span><h2>PR #1042 · fixture repository</h2></div>${passed?status('passed','ok'):status('not run')}</header><dl><div><dt>External effects</dt><dd>Disabled</dd></div><div><dt>Agents</dt><dd>${esc(agentNames.join(' → ')||'None selected')}</dd></div><div><dt>Expected terminal result</dt><dd>Review decision + evidence receipt</dd></div></dl>${passed?'<pre>accepted → selected agents succeeded → evidence achieved → publish suppressed</pre>':'<button class="ae-button" type="button" data-run-test>Run synthetic test</button>'}</section></div>`;}
function activateStep(){const passed=testIsCurrent();const authority=Object.entries(state.authority).filter(([,v])=>v).map(([k])=>({read:'Read repository',review:'Submit review',push:'Push commits',issues:'Mutate issues'})[k]);if(state.activated)return `<section class="activation-success">${icon('check','ae-ok')}<span class="eyebrow">Workflow active</span><h2>PR Review is listening</h2><p>The immutable revision is active. Future accepted events pin this exact configuration.</p><button class="ae-button" type="button" data-nav="workflows">Return to workflows</button></section>`;return `<div class="form-stack"><p class="step-copy">Review the immutable revision before activation.</p><section class="review-grid"><div><span>Goal</span><strong>${state.goalChoice==='enhanced'?'Enhanced proposal adopted':esc(state.goalOriginal)}</strong></div><div><span>Trigger</span><strong>${esc(state.trigger)} · ${esc(state.idempotency)}</strong></div><div><span>Agents</span><strong>${[...state.selectedAgents].map(id=>DATA.agents.find(a=>a.id===id)?.name).filter(Boolean).join(' → ')||'None selected'}</strong></div><div><span>Authority</span><strong>${esc(authority.join(' · ')||'No capabilities')}<br><small>${esc(state.broker)}</small></strong></div><div><span>Limits</span><strong>${esc(state.limits.runGroup)} · ${esc(state.limits.runsDay)}/day<br><small>${esc(state.limits.duration)} · ${esc(state.limits.output)}</small></strong></div><div><span>Preflight</span><strong>${passed?'Synthetic test passed for this revision':'Current-revision test required'}</strong></div></section><label class="confirm"><input type="checkbox" data-confirm ${state.confirmed?'checked':''}/> I confirm this revision, authority, and spend ceiling.</label><button class="ae-button" type="button" data-activate ${passed&&state.confirmed?'':'disabled'}>Activate workflow</button></div>`;}
function createView(){const bodies=[goalStep,triggerStep,agentsStep,authorityStep,limitsStep,testStep,activateStep];return `${heading(state.createMode==='edit'?'Edit workflow':'New workflow',state.createMode==='edit'?'Revise PR Review':'Create workflow',state.createMode==='edit'?'Activation creates a new immutable revision.':'Define the durable goal first; trigger, agents, authority, limits, proof, and activation follow.')}<div class="authoring">${stepper()}<section class="step-panel"><header><span class="eyebrow">Step ${state.createStep+1} of 7</span><h2>${STEPS[state.createStep]}</h2></header>${bodies[state.createStep]() }<footer><button class="ae-button ae-button-quiet" type="button" data-step-back ${state.createStep===0?'disabled':''}>Back</button><span>Draft revision · not active</span>${state.createStep<6?`<button class="ae-button" type="button" data-step-next ${state.createStep===5&&!testIsCurrent()?'disabled':''}>Continue ${icon('arrow')}</button>`:''}</footer></section></div>`;}

function content(){if(state.view==='workflows')return workflowsView();if(state.view==='workflow')return workflowDetail();if(state.view==='runs')return runsView();if(state.view==='run')return runDetail();if(state.view==='agents')return agentsView();if(state.view==='spend')return spendView();if(state.view==='create')return createView();return workflowsView();}
function applyTheme(theme) {
  state.theme = theme;
  root.classList.toggle('dark', theme === 'dark');
  root.classList.toggle('light', theme === 'light');
  root.style.colorScheme = theme;
  try { localStorage.setItem('ae-mode', theme); } catch (e) {}
  const toggle = app.querySelector('[data-theme]');
  if (toggle) {
    const next = theme === 'light' ? 'dark' : 'light';
    toggle.setAttribute('aria-label', `Switch to ${next} mode`);
    toggle.setAttribute('title', `Switch to ${next} mode`);
  }
}

let modeTransition = null;
let modeTimer = 0;
let modeRun = 0;
let modeTarget = null;
function toggleTheme() {
  const theme = (modeTarget ?? state.theme) === 'light' ? 'dark' : 'light';
  const id = ++modeRun;
  modeTarget = theme;
  if (modeTransition?.skipTransition) modeTransition.skipTransition();
  if (modeTimer) clearTimeout(modeTimer);
  modeTransition = null;
  modeTimer = 0;
  root.classList.remove('ae-vt-mode', 'ae-mode-easing');
  const flip = () => { if (id === modeRun) applyTheme(theme); };
  if (matchMedia('(prefers-reduced-motion: reduce)').matches) return flip();
  if (document.startViewTransition) {
    root.classList.add('ae-vt-mode');
    modeTransition = document.startViewTransition(flip);
    modeTransition.ready.catch(() => {});
    modeTransition.updateCallbackDone.catch(() => {});
    modeTransition.finished.catch(() => {}).finally(() => {
      if (id !== modeRun) return;
      root.classList.remove('ae-vt-mode');
      modeTransition = null;
      if (modeTimer) clearTimeout(modeTimer);
      modeTimer = 0;
    });
  } else {
    root.classList.add('ae-mode-easing');
    flip();
  }
  modeTimer = setTimeout(() => {
    if (id !== modeRun) return;
    root.classList.remove('ae-vt-mode', 'ae-mode-easing');
    modeTimer = 0;
  }, 180);
}

function render(){app.innerHTML=`<div class="app-shell">${sidebar()}${state.navOpen?'<button class="scrim" type="button" data-menu aria-label="Close navigation"></button>':''}<div class="app-main">${topbar()}<main class="workspace">${content()}</main></div></div>`;window.lucide.createIcons({attrs:{'stroke-width':1.7}});}

app.addEventListener('click',(e)=>{
  const b=e.target.closest('button');if(!b)return;
  if(b.matches('[data-nav]'))navigate(b.dataset.nav);
  else if(b.matches('[data-menu]')){state.navOpen=!state.navOpen;render();}
  else if(b.matches('[data-theme]'))toggleTheme();
  else if(b.matches('[data-workflow]'))navigate('workflow',{workflowId:b.dataset.workflow});
  else if(b.matches('[data-workflow-runs]')){state.view='runs';state.workflowId=b.dataset.workflowRuns;state.runId=null;state.navOpen=false;render();}
  else if(b.matches('[data-run]'))navigate('run',{runId:b.dataset.run});
  else if(b.matches('[data-create]')){state.view='create';state.navOpen=false;resetAuthoring('new');render();}
  else if(b.matches('[data-edit]')){state.view='create';state.navOpen=false;resetAuthoring('edit');render();}
  else if(b.matches('[data-step]')){state.createStep=Number(b.dataset.step);render();}
  else if(b.matches('[data-step-next]')){state.createStep=Math.min(6,state.createStep+1);render();}
  else if(b.matches('[data-step-back]')){state.createStep=Math.max(0,state.createStep-1);render();}
  else if(b.matches('[data-enhance]')){state.enhanced=true;render();}
  else if(b.matches('[data-goal-choice]')){if(state.goalChoice!==b.dataset.goalChoice){state.goalChoice=b.dataset.goalChoice;invalidateProof();}render();}
  else if(b.matches('[data-trigger]')){if(state.trigger!==b.dataset.trigger){state.trigger=b.dataset.trigger;invalidateProof();}render();}
  else if(b.matches('[data-new-agent]')){state.newAgent=true;render();}
  else if(b.matches('[data-add-agent]')){const name=state.newAgentName.trim(),description=state.newAgentResponsibility.trim();if(!name||!description)return;let id=name.toLowerCase().replace(/[^a-z0-9]+/g,'-').replace(/(^-|-$)/g,'')||'draft-agent';let agent=DATA.agents.find(a=>a.id===id);if(agent){agent.name=name;agent.description=description;}else{agent={id,name,description,model:'Unassigned',harness:'Roster',authority:'Draft · no authority granted',workflows:[],revision:'draft. 1'};DATA.agents.push(agent);}state.selectedAgents.add(id);state.newAgent=false;invalidateProof();render();}
  else if(b.matches('[data-run-test]')){state.testedRevision=revisionFingerprint();state.confirmed=false;render();}
  else if(b.matches('[data-activate]')&&testIsCurrent()&&state.confirmed){state.activated=true;render();}
  else if(b.matches('[data-save-governors]')){for(const input of app.querySelectorAll('[data-governor]'))state.governors[input.dataset.governor]=input.value;state.governorsSaved=true;render();}
});
app.addEventListener('input',(e)=>{
  if(e.target.matches('[data-goal]')){state.goalOriginal=e.target.value;invalidateProof();}
  else if(e.target.matches('[data-idempotency]')){state.idempotency=e.target.value;invalidateProof();}
  else if(e.target.matches('[data-limit]')){state.limits[e.target.dataset.limit]=e.target.value;invalidateProof();}
  else if(e.target.matches('[data-new-agent-name]')){state.newAgentName=e.target.value;invalidateProof();}
  else if(e.target.matches('[data-new-agent-responsibility]')){state.newAgentResponsibility=e.target.value;invalidateProof();}
});
app.addEventListener('change',(e)=>{
  if(e.target.matches('[data-agent]')){e.target.checked?state.selectedAgents.add(e.target.value):state.selectedAgents.delete(e.target.value);invalidateProof();}
  else if(e.target.matches('[data-authority]')){state.authority[e.target.dataset.authority]=e.target.checked;invalidateProof();}
  else if(e.target.matches('[data-broker]')){state.broker=e.target.value;invalidateProof();}
  else if(e.target.matches('[data-confirm]')){state.confirmed=e.target.checked;render();}
});
render();
