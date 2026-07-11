/* Hallmark · pre-emit critique: P5 H5 E4 S5 R5 V5 */

const esc = (value) => String(value ?? "").replace(/[&<>"']/g, character => ({
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
  "'": "&#39;"
})[character]);

const workflowState = workflow => `
  <span class="state state-life">${esc(workflow.lifecycle)}</span>
  <span class="state state-trigger">${esc(workflow.trigger)}</span>
  <span class="state state-active">${workflow.active ? `${workflow.active} active run` : "No active run"}</span>`;

const topology = (corpus, prefix) => corpus.workflows[0].topology.map((node, index) => `
  <li class="${index === 2 ? "is-executing" : ""}">
    <span class="node-index">${String(index + 1).padStart(2, "0")}</span>
    <strong>${esc(node)}</strong>
    <small>${index === 0 ? "Trigger received" : index === 1 ? "Succeeded · domain blocked" : index === 2 ? "Executing · selected overlay" : "Awaiting verification"}</small>
  </li>`).join("");

const agentRows = corpus => corpus.agents.map(agent => `
  <article class="agent-row">
    <div><strong>${esc(agent.name)}</strong><span class="availability">${esc(agent.availability)}</span></div>
    <div><span>${esc(agent.model)}</span><small>${esc(agent.harness)}</small></div>
    <div><span>${esc(agent.ceiling)}</span><small>${esc(agent.workflows)}</small></div>
    <div class="agent-load"><b>${agent.active ? "Executing" : "Available"}</b><span>${agent.active ? `${agent.active} live` : "No active run"}</span></div>
  </article>`).join("");

const runRows = corpus => corpus.recentRuns.map(run => `
  <article class="run-row">
    <div><strong>${esc(run.workflow)}</strong><small>${esc(run.ref)}</small></div>
    <div><span>Execution</span><b>${esc(run.execution)}</b></div>
    <div><span>Domain result</span><b>${esc(run.domain)}</b></div>
    <div><span>Verification</span><b>${esc(run.verification)}</b></div>
    <div><span>Cost</span><b>${esc(run.cost === "Unavailable" ? "Unknown · unavailable" : run.cost)}</b></div>
  </article>`).join("");

const spendRows = corpus => corpus.spend.truth.map(item => `
  <article class="truth-row">
    <strong>${esc(item.label)}</strong>
    <span>${esc(item.value)}</span>
    <small>${esc(item.state)}</small>
  </article>`).join("");

const primaryNav = (prefix, vertical = false) => `
  <nav class="${prefix}-nav${vertical ? " is-vertical" : ""}" aria-label="Primary">
    <button type="button" data-view="workflows" aria-current="true">Workflows</button>
    <button type="button" data-view="agents">Agents</button>
    <button type="button" data-view="runs">Runs</button>
    <button type="button" data-view="spend">Spend</button>
    <button type="button" data-view="create">Create workflow</button>
  </nav>`;

const agentsPanel = (corpus, title) => `
  <section data-panel="agents" hidden class="panel agents-panel">
    <header class="panel-head"><div><h1>${esc(title)}</h1><p>Roster definitions remain distinct from live workflow assignment.</p></div><span class="fixture">${esc(corpus.notice)}</span></header>
    <div class="agent-key"><span>In use · assigned to configured workflows</span><span>Available · defined, not commissioned</span></div>
    <div class="agent-list">${agentRows(corpus)}</div>
  </section>`;

const runsPanel = (corpus, title) => `
  <section data-panel="runs" hidden class="panel runs-panel">
    <header class="panel-head"><div><h1>${esc(title)}</h1><p>Lifecycle, domain result, verification, and cost are separate columns.</p></div><span class="fixture">${esc(corpus.notice)}</span></header>
    <div class="live-run">
      <div><span>Live instance</span><strong>${esc(corpus.run.id)}</strong></div>
      <div><span>Lifecycle</span><strong>${esc(corpus.run.state)}</strong></div>
      <div><span>Current step</span><strong>${esc(corpus.run.step)}</strong></div>
      <div><span>Elapsed</span><strong>${esc(corpus.run.elapsed)}</strong></div>
      <div><span>Cost truth</span><strong>Unknown · ${esc(corpus.run.cost)}</strong></div>
    </div>
    <div class="run-list">${runRows(corpus)}</div>
  </section>`;

const spendPanel = (corpus, title) => `
  <section data-panel="spend" hidden class="panel spend-panel">
    <header class="panel-head"><div><h1>${esc(title)}</h1><p>Admission limits and accounting coverage are related, never conflated.</p></div><span class="fixture">${esc(corpus.notice)}</span></header>
    <div class="spend-layout">
      <div class="allowances"><h2>Control scopes</h2>${corpus.spend.scopes.map((scope, index) => `<div><span>${esc(scope)}</span><strong>${index === 2 ? "Configured" : index === 0 ? "Enforced" : "Inherited"}</strong></div>`).join("")}</div>
      <div class="truth-stack"><h2>Cost truth</h2>${spendRows(corpus)}</div>
    </div>
  </section>`;

const createPanel = (corpus, title) => `
  <section data-panel="create" hidden class="panel create-panel">
    <header class="panel-head"><div><h1>${esc(title)}</h1><p>Begin with the outcome; resolve trigger, agent, authority, and limits after review.</p></div><span class="fixture">${esc(corpus.notice)}</span></header>
    <div class="create-layout">
      <ol class="create-steps">${corpus.creation.steps.map((step, index) => `<li class="${index === 0 ? "is-current" : ""}"><span>${index + 1}</span>${esc(step)}</li>`).join("")}</ol>
      <form class="goal-review" onsubmit="return false">
        <label for="${esc(title).replace(/\W/g, "-")}-goal">Workflow goal</label>
        <textarea id="${esc(title).replace(/\W/g, "-")}-goal" rows="4">${esc(corpus.creation.rawGoal)}</textarea>
        <div class="enhanced"><span>Enhanced goal · review before accepting</span><p>${esc(corpus.creation.enhancedGoal)}</p></div>
        <div class="activation-test"><div><span>Activation test</span><strong>${esc(corpus.creation.test)}</strong></div><button type="button" disabled aria-disabled="true">Activate after test</button></div>
      </form>
    </div>
  </section>`;

const foundation = scope => `
  html,body{overflow-x:clip}
  ${scope}{--font-display:"Avenir Next",Avenir,"Trebuchet MS",sans-serif;--font-body:"IBM Plex Sans","Segoe UI",sans-serif;--font-mono:var(--font-body);--space-2xs:.25rem;--space-xs:.5rem;--space-sm:.75rem;--space-md:1rem;--space-lg:1.5rem;--space-xl:2.5rem;--space-2xl:4rem;--radius-sm:.375rem;--radius-md:.75rem;--rule:1px;--dur-micro:120ms;--dur-short:220ms;--ease-out:cubic-bezier(.16,1,.3,1);--ease-in:cubic-bezier(.7,0,.84,0);--ease-in-out:cubic-bezier(.65,0,.35,1);font-family:var(--font-body);font-size:16px;line-height:1.5;color:var(--ink);background:var(--paper);min-height:100dvh;overflow-x:clip}
  ${scope},${scope} *{box-sizing:border-box}
  ${scope} button,${scope} textarea{font:inherit;color:inherit}
  ${scope} button{min-height:44px;white-space:nowrap;cursor:pointer;transition:transform var(--dur-micro) var(--ease-out),background-color var(--dur-short) var(--ease-out),color var(--dur-short) var(--ease-out)}
  ${scope} button:active{transform:translateY(1px)}
  ${scope} button:disabled{opacity:.5;cursor:not-allowed}
  ${scope} button:focus-visible{outline:3px solid var(--focus);outline-offset:2px}
  ${scope} textarea:focus-visible{outline:2px solid var(--focus);outline-offset:1px}
  ${scope} h1,${scope} h2,${scope} h3{font-family:var(--font-display);font-style:normal;letter-spacing:-.03em;overflow-wrap:anywhere;min-width:0}
  ${scope} h1{font-size:clamp(1.75rem,3vw,3rem);line-height:1.05;margin:0}
  ${scope} h2{font-size:1.25rem;line-height:1.15;margin:0}
  ${scope} p{max-width:65ch}
  ${scope} [hidden]{display:none!important}
  ${scope} .fixture{font-family:var(--font-mono);font-size:.7rem;line-height:1.35;color:var(--muted);max-width:34ch}
  ${scope} .panel{min-width:0}
  ${scope} .panel-head{display:grid;gap:var(--space-md);padding:var(--space-lg);border-bottom:var(--rule) solid var(--rule-color)}
  ${scope} .panel-head p{margin:var(--space-xs) 0 0;color:var(--muted)}
  ${scope} .panel-head .fixture{align-self:end}
  ${scope} .state{display:block;font-size:.75rem;line-height:1.35}
  ${scope} .state-life{font-weight:800;color:var(--ink)}
  ${scope} .state-trigger{color:var(--signal)}
  ${scope} .state-active{color:var(--muted)}
  ${scope} .agent-key{display:flex;flex-wrap:wrap;gap:var(--space-sm) var(--space-xl);padding:var(--space-md) var(--space-lg);border-bottom:var(--rule) solid var(--rule-color);font-size:.78rem;color:var(--muted)}
  ${scope} .agent-list,${scope} .run-list{display:grid}
  ${scope} .agent-row,${scope} .run-row{display:grid;gap:var(--space-md);padding:var(--space-md) var(--space-lg);border-bottom:var(--rule) solid var(--rule-color);align-items:center}
  ${scope} .agent-row>div,${scope} .run-row>div{display:grid;gap:var(--space-2xs);min-width:0}
  ${scope} .agent-row small,${scope} .run-row small,${scope} .run-row span{color:var(--muted)}
  ${scope} .availability{display:inline-block;margin-inline-start:var(--space-xs);font-size:.68rem;text-transform:uppercase;letter-spacing:.08em;color:var(--signal)}
  ${scope} .agent-load span{color:var(--muted);font-size:.75rem}
  ${scope} .live-run{display:grid;gap:var(--rule);background:var(--rule-color);border-bottom:var(--rule) solid var(--rule-color)}
  ${scope} .live-run>div{display:grid;gap:var(--space-2xs);padding:var(--space-md);background:var(--surface)}
  ${scope} .live-run span{font-size:.7rem;color:var(--muted);text-transform:uppercase;letter-spacing:.08em}
  ${scope} .spend-layout,${scope} .create-layout{display:grid;gap:var(--space-xl);padding:var(--space-lg)}
  ${scope} .allowances,${scope} .truth-stack{display:grid;align-content:start;border-top:var(--rule) solid var(--ink)}
  ${scope} .allowances h2,${scope} .truth-stack h2{padding:var(--space-md) 0}
  ${scope} .allowances>div,${scope} .truth-row{display:grid;gap:var(--space-xs);padding:var(--space-md) 0;border-top:var(--rule) solid var(--rule-color)}
  ${scope} .truth-row small{color:var(--muted)}
  ${scope} .create-steps{list-style:none;margin:0;padding:0;display:flex;flex-wrap:wrap;gap:var(--space-xs)}
  ${scope} .create-steps li{display:flex;align-items:center;gap:var(--space-xs);padding:var(--space-xs) var(--space-sm);border:var(--rule) solid var(--rule-color);font-size:.78rem}
  ${scope} .create-steps li span{font-family:var(--font-mono);color:var(--muted)}
  ${scope} .create-steps .is-current{border-color:var(--signal);color:var(--signal)}
  ${scope} .goal-review{display:grid;gap:var(--space-sm);max-width:78rem}
  ${scope} .goal-review>label{font-weight:800}
  ${scope} textarea{width:100%;min-height:7rem;resize:vertical;padding:var(--space-md);border:var(--rule) solid var(--rule-color);background:var(--surface);outline:2px solid transparent;outline-offset:1px}
  ${scope} textarea:active{background:var(--surface-2)}
  ${scope} textarea:disabled{opacity:.55;cursor:not-allowed;color:var(--muted)}
  ${scope} .enhanced{padding:var(--space-lg);background:var(--surface-2);border-top:3px solid var(--signal)}
  ${scope} .enhanced span{font-family:var(--font-mono);font-size:.72rem;color:var(--signal)}
  ${scope} .enhanced p{margin:var(--space-sm) 0 0;max-width:82ch}
  ${scope} .activation-test{display:flex;flex-wrap:wrap;justify-content:space-between;align-items:center;gap:var(--space-md);padding-top:var(--space-md);border-top:var(--rule) solid var(--rule-color)}
  ${scope} .activation-test div{display:grid;gap:var(--space-2xs)}
  ${scope} .activation-test span{font-size:.72rem;color:var(--muted);text-transform:uppercase;letter-spacing:.08em}
  ${scope} .activation-test button{border:var(--rule) solid var(--rule-color);background:var(--surface-2);padding-inline:var(--space-md)}
  @media (min-width:40rem){${scope} .panel-head{grid-template-columns:minmax(0,1fr) auto}${scope} .live-run{grid-template-columns:repeat(5,minmax(0,1fr))}${scope} .agent-row{grid-template-columns:1.1fr 1fr 1.6fr .7fr}${scope} .run-row{grid-template-columns:1.2fr repeat(4,minmax(0,1fr))}${scope} .allowances>div{grid-template-columns:1fr auto}${scope} .truth-row{grid-template-columns:.7fr 1fr 1.2fr}}
  @media (min-width:60rem){${scope} .spend-layout{grid-template-columns:minmax(0,.8fr) minmax(0,1.4fr)}${scope} .create-layout{grid-template-columns:15rem minmax(0,1fr)}${scope} .create-steps{display:grid;align-content:start}}
  @media (hover:hover) and (pointer:fine){${scope} button:hover{transform:translateY(-1px)}${scope} textarea:hover{background:var(--surface-2)}}
  @media (prefers-reduced-motion:reduce){${scope} *,${scope} *::before,${scope} *::after{animation-duration:150ms!important;animation-iteration-count:1!important;transition-duration:150ms!important}}
  @media (pointer:coarse){${scope} button{min-height:48px}}
`;

const hallOne = corpus => {
  const workflow = corpus.workflows[0];
  return `
  <style>
    /* Hallmark · route: custom (bespoke) · structure: persistent control ledger · idea: "configuration left, evidence right" */
    ${foundation(".hall-ledger")}
    .hall-ledger{--paper:oklch(96% .012 62);--surface:oklch(98% .009 62);--surface-2:oklch(92% .016 62);--ink:oklch(20% .014 48);--muted:oklch(43% .014 48);--rule-color:oklch(76% .018 62);--signal:oklch(48% .16 32);--focus:oklch(62% .2 32);display:grid;grid-template-rows:auto auto minmax(0,1fr)}
    @media (prefers-color-scheme:dark){.hall-ledger{--paper:oklch(15% .014 48);--surface:oklch(18% .016 48);--surface-2:oklch(22% .018 48);--ink:oklch(94% .01 62);--muted:oklch(71% .012 62);--rule-color:oklch(33% .018 48);--signal:oklch(72% .14 38);--focus:oklch(78% .18 38)}}
    html[data-theme="light"] .hall-ledger{--paper:oklch(96% .012 62);--surface:oklch(98% .009 62);--surface-2:oklch(92% .016 62);--ink:oklch(20% .014 48);--muted:oklch(43% .014 48);--rule-color:oklch(76% .018 62);--signal:oklch(48% .16 32);--focus:oklch(62% .2 32)}
    html[data-theme="dark"] .hall-ledger{--paper:oklch(15% .014 48);--surface:oklch(18% .016 48);--surface-2:oklch(22% .018 48);--ink:oklch(94% .01 62);--muted:oklch(71% .012 62);--rule-color:oklch(33% .018 48);--signal:oklch(72% .14 38);--focus:oklch(78% .18 38)}
    .hall-ledger .mast{display:grid;grid-template-columns:1fr auto;align-items:center;gap:var(--space-md);padding:var(--space-sm) var(--space-md);border-bottom:var(--rule) solid var(--ink);background:var(--surface)}
    .hall-ledger .brand{font-family:var(--font-display);font-size:1.15rem;font-weight:800;letter-spacing:-.04em}.hall-ledger .brand span{color:var(--signal)}
    .hall-ledger .mast-meta{display:flex;align-items:center;gap:var(--space-sm)}.hall-ledger .mast .fixture{display:none}.hall-ledger [data-theme-toggle]{border:var(--rule) solid var(--ink);background:transparent;padding-inline:var(--space-sm)}
    .hall-ledger-nav{display:flex;overflow-x:auto;border-bottom:var(--rule) solid var(--rule-color);background:var(--surface)}
    .hall-ledger-nav button{border:0;border-inline-end:var(--rule) solid var(--rule-color);background:transparent;padding-inline:var(--space-md)}
    .hall-ledger-nav button[aria-current]{background:var(--ink);color:var(--paper)}
    .hall-ledger-nav button:last-child{margin-inline-start:auto;border-inline-start:var(--rule) solid var(--rule-color);color:var(--signal);font-weight:800}
    .hall-ledger .workbench{display:grid;min-height:calc(100dvh - 106px)}
    .hall-ledger .roster{border-bottom:var(--rule) solid var(--rule-color);background:var(--surface)}
    .hall-ledger .roster-title,.hall-ledger .evidence-title{padding:var(--space-sm) var(--space-md);font-family:var(--font-mono);font-size:.7rem;text-transform:uppercase;letter-spacing:.1em;border-bottom:var(--rule) solid var(--rule-color);color:var(--muted)}
    .hall-ledger .workflow-entry{padding:var(--space-md);border-bottom:var(--rule) solid var(--rule-color)}.hall-ledger .workflow-entry.is-selected{background:var(--surface-2)}
    .hall-ledger .workflow-entry h2{margin-bottom:var(--space-sm)}.hall-ledger .workflow-entry .state{margin-top:var(--space-2xs)}
    .hall-ledger .detail{min-width:0}.hall-ledger .detail-head{display:flex;flex-wrap:wrap;justify-content:space-between;gap:var(--space-md);padding:var(--space-lg);border-bottom:var(--rule) solid var(--rule-color)}
    .hall-ledger .detail-head p{margin:var(--space-xs) 0 0;color:var(--muted)}.hall-ledger .run-stamp{display:grid;gap:var(--space-2xs);align-content:start;font-family:var(--font-mono);font-size:.75rem}.hall-ledger .run-stamp b{color:var(--signal)}
    .hall-ledger .graph{list-style:none;margin:0;padding:var(--space-xl) var(--space-lg);display:grid;gap:var(--space-md)}
    .hall-ledger .graph li{position:relative;display:grid;grid-template-columns:2.5rem minmax(0,1fr);gap:0 var(--space-sm);padding:var(--space-md);border:var(--rule) solid var(--rule-color);background:var(--surface)}
    .hall-ledger .graph li:not(:last-child)::after{content:"";position:absolute;left:2rem;bottom:calc(var(--space-md) * -1 - 1px);width:var(--rule);height:var(--space-md);background:var(--rule-color)}
    .hall-ledger .graph .node-index{grid-row:1/3;font-family:var(--font-mono);color:var(--muted)}.hall-ledger .graph small{color:var(--muted)}.hall-ledger .graph .is-executing{border-color:var(--signal)}
    .hall-ledger .evidence{background:var(--surface);border-top:var(--rule) solid var(--rule-color)}.hall-ledger .evidence-block{padding:var(--space-md);border-bottom:var(--rule) solid var(--rule-color);display:grid;gap:var(--space-2xs)}.hall-ledger .evidence-block span{font-size:.7rem;color:var(--muted);text-transform:uppercase;letter-spacing:.08em}.hall-ledger .evidence-block strong{font-size:.9rem}.hall-ledger .evidence-block.is-blocked strong{color:var(--signal)}
    @media (min-width:40rem){.hall-ledger .mast{grid-template-columns:14rem 1fr auto}.hall-ledger .mast .fixture{display:block}.hall-ledger .workbench{grid-template-columns:14rem minmax(0,1fr)}.hall-ledger .roster{border-bottom:0;border-inline-end:var(--rule) solid var(--rule-color)}}
    @media (min-width:75rem){.hall-ledger .workbench{grid-template-columns:16rem minmax(0,1fr) 18rem}.hall-ledger .evidence{border-top:0;border-inline-start:var(--rule) solid var(--rule-color)}}
  </style>
  <article class="hall-ledger">
    <header class="mast"><div class="brand">Bitter<span>blossom</span></div><span class="fixture">${esc(corpus.notice)}</span><div class="mast-meta"><span class="fixture">Operator plane</span><button type="button" data-theme-toggle aria-label="Toggle light and dark theme">Theme</button></div></header>
    ${primaryNav("hall-ledger")}
    <main>
      <section data-panel="workflows" class="panel workbench">
        <aside class="roster"><div class="roster-title">Configured workflows</div>${corpus.workflows.map((item, index) => `<article class="workflow-entry ${index === 0 ? "is-selected" : ""}"><h2>${esc(item.name)}</h2>${workflowState(item)}<span class="state">Budget · ${esc(item.budget)}</span></article>`).join("")}</aside>
        <section class="detail"><header class="detail-head"><div><h1>${esc(workflow.name)}</h1><p>Stable configured graph with one selected run overlay.</p></div><div class="run-stamp"><span>${esc(corpus.run.id)}</span><b>${esc(corpus.run.state)} · ${esc(corpus.run.step)}</b><span>${esc(corpus.run.elapsed)}</span></div></header><ol class="graph">${topology(corpus, "hall-ledger")}</ol></section>
        <aside class="evidence"><div class="evidence-title">Run evidence</div><div class="evidence-block"><span>Workflow</span><strong>${esc(workflow.lifecycle)}</strong></div><div class="evidence-block"><span>Trigger health</span><strong>Listening</strong></div><div class="evidence-block"><span>Run lifecycle</span><strong>${esc(corpus.run.state)}</strong></div><div class="evidence-block is-blocked"><span>Domain result</span><strong>Succeeded · Blocked</strong></div><div class="evidence-block"><span>Verification</span><strong>Achieved</strong></div><div class="evidence-block"><span>Cost</span><strong>Unknown · ${esc(corpus.run.cost)}</strong></div></aside>
      </section>
      ${agentsPanel(corpus, "Agent roster")}${runsPanel(corpus, "Live and historical runs")}${spendPanel(corpus, "Spend controls")}${createPanel(corpus, "Create from a goal")}
    </main>
  </article>`;
};

const hallTwo = corpus => {
  const pr = corpus.workflows[0];
  return `
  <style>
    /* Hallmark · route: custom (bespoke) · structure: expanding workflow strips · idea: "the roster is the detail navigation" */
    ${foundation(".hall-strips")}
    .hall-strips{--font-display:"Arial Narrow","Avenir Next Condensed","Gill Sans",sans-serif;--paper:oklch(97% .01 235);--surface:oklch(94% .016 235);--surface-2:oklch(89% .024 235);--ink:oklch(18% .018 235);--muted:oklch(42% .018 235);--rule-color:oklch(72% .03 235);--signal:oklch(48% .2 252);--focus:oklch(66% .22 252);display:grid;grid-template-columns:4.5rem minmax(0,1fr)}
    @media (prefers-color-scheme:dark){.hall-strips{--paper:oklch(13% .018 235);--surface:oklch(17% .02 235);--surface-2:oklch(22% .026 235);--ink:oklch(94% .012 235);--muted:oklch(70% .016 235);--rule-color:oklch(32% .03 235);--signal:oklch(72% .16 252);--focus:oklch(78% .2 252)}}
    html[data-theme="light"] .hall-strips{--paper:oklch(97% .01 235);--surface:oklch(94% .016 235);--surface-2:oklch(89% .024 235);--ink:oklch(18% .018 235);--muted:oklch(42% .018 235);--rule-color:oklch(72% .03 235);--signal:oklch(48% .2 252);--focus:oklch(66% .22 252)}
    html[data-theme="dark"] .hall-strips{--paper:oklch(13% .018 235);--surface:oklch(17% .02 235);--surface-2:oklch(22% .026 235);--ink:oklch(94% .012 235);--muted:oklch(70% .016 235);--rule-color:oklch(32% .03 235);--signal:oklch(72% .16 252);--focus:oklch(78% .2 252)}
    .hall-strips .side{position:sticky;top:0;height:100dvh;display:flex;flex-direction:column;background:var(--ink);color:var(--paper);border-inline-end:var(--rule) solid var(--rule-color)}
    .hall-strips .monogram{height:5rem;display:grid;place-items:center;font:800 1.4rem/1 var(--font-display);letter-spacing:-.08em;border-bottom:var(--rule) solid var(--rule-color)}
    .hall-strips-nav{display:flex;flex-direction:column}.hall-strips-nav button{width:100%;border:0;border-bottom:var(--rule) solid var(--rule-color);background:transparent;color:inherit;padding:var(--space-xs);font-family:var(--font-display);font-size:.68rem;font-weight:800;text-transform:uppercase;letter-spacing:.06em;white-space:normal;line-height:1.1}
    .hall-strips-nav button[aria-current]{background:var(--signal);color:var(--paper)}.hall-strips .side [data-theme-toggle]{margin-top:auto;border:0;border-top:var(--rule) solid var(--rule-color);background:transparent;color:inherit;font-size:.7rem}
    .hall-strips .stage{min-width:0;overflow:auto}.hall-strips .stage-head{display:flex;align-items:center;justify-content:space-between;gap:var(--space-md);min-height:5rem;padding:var(--space-sm) var(--space-lg);border-bottom:var(--rule) solid var(--ink)}.hall-strips .stage-head h1{font-size:1.35rem;text-transform:uppercase;letter-spacing:-.02em}.hall-strips .stage-head .fixture{text-align:end}
    .hall-strips .strip{border-bottom:var(--rule) solid var(--ink)}.hall-strips .strip-summary{display:grid;gap:var(--space-sm);padding:var(--space-lg);background:var(--surface)}.hall-strips .strip-summary h2{font-size:clamp(1.8rem,4vw,4.5rem);text-transform:uppercase;line-height:1}.hall-strips .strip-summary .strip-number{font-family:var(--font-mono);color:var(--signal)}
    .hall-strips .strip-meta{display:grid;gap:var(--space-sm);align-content:end}.hall-strips .strip-meta>div{display:grid;gap:var(--space-2xs)}.hall-strips .strip-meta span{font-size:.7rem;text-transform:uppercase;letter-spacing:.08em;color:var(--muted)}
    .hall-strips .expanded{display:grid;background:var(--paper)}.hall-strips .graph-field{padding:var(--space-xl) var(--space-lg);border-bottom:var(--rule) solid var(--ink)}.hall-strips .graph-field>p{margin:0 0 var(--space-lg);color:var(--muted)}
    .hall-strips .graph{list-style:none;margin:0;padding:0;display:grid;gap:var(--space-xs)}.hall-strips .graph li{display:grid;grid-template-columns:2.25rem minmax(0,1fr);gap:0 var(--space-sm);padding:var(--space-sm);border-top:var(--rule) solid var(--rule-color)}.hall-strips .graph .node-index{grid-row:1/3;font-family:var(--font-mono);color:var(--signal)}.hall-strips .graph small{color:var(--muted)}.hall-strips .graph .is-executing{background:var(--surface-2)}
    .hall-strips .overlay-ledger{display:grid;align-content:start;background:var(--ink);color:var(--paper)}.hall-strips .overlay-ledger>div{display:grid;grid-template-columns:1fr auto;gap:var(--space-sm);padding:var(--space-md);border-bottom:var(--rule) solid var(--rule-color)}.hall-strips .overlay-ledger span{color:var(--muted);font-size:.75rem}.hall-strips .overlay-ledger .blocked{color:var(--signal)}
    .hall-strips .draft-strip .strip-summary{background:var(--paper)}.hall-strips .draft-strip h2{font-size:clamp(1.5rem,3vw,2.5rem)}
    .hall-strips .panel-head{padding-top:var(--space-xl)}
    @media (min-width:40rem){.hall-strips{grid-template-columns:6.5rem minmax(0,1fr)}.hall-strips-nav button{font-size:.72rem;white-space:nowrap}.hall-strips .strip-summary{grid-template-columns:auto minmax(0,1.4fr) minmax(15rem,.8fr);align-items:end}.hall-strips .expanded{grid-template-columns:minmax(0,1.5fr) minmax(15rem,.7fr)}.hall-strips .graph-field{border-bottom:0;border-inline-end:var(--rule) solid var(--ink)}}
  </style>
  <article class="hall-strips">
    <aside class="side"><div class="monogram">bb</div>${primaryNav("hall-strips", true)}<button type="button" data-theme-toggle aria-label="Toggle light and dark theme">Theme</button></aside>
    <main class="stage"><header class="stage-head"><h1>Bitterblossom · workflows</h1><span class="fixture">${esc(corpus.notice)}</span></header>
      <section data-panel="workflows" class="panel">
        <article class="strip">
          <header class="strip-summary"><span class="strip-number">01</span><div><h2>${esc(pr.name)}</h2><span class="state state-trigger">${esc(pr.trigger)}</span></div><div class="strip-meta"><div><span>Configuration</span><strong>${esc(pr.lifecycle)}</strong></div><div><span>Live instances</span><strong>${pr.active}</strong></div></div></header>
          <div class="expanded"><div class="graph-field"><p>The roster row expands into the stable graph; the selected run is a state overlay, not a second object.</p><ol class="graph">${topology(corpus, "hall-strips")}</ol></div><aside class="overlay-ledger"><div><span>Run</span><strong>${esc(corpus.run.id)}</strong></div><div><span>Lifecycle</span><strong>${esc(corpus.run.state)}</strong></div><div><span>Domain result</span><strong class="blocked">Succeeded · Blocked</strong></div><div><span>Verification</span><strong>Achieved</strong></div><div><span>Cost</span><strong>Unknown</strong></div><div><span>Accounting</span><strong>${esc(corpus.run.cost)}</strong></div></aside></div>
        </article>
        <article class="strip draft-strip"><header class="strip-summary"><span class="strip-number">02</span><div><h2>${esc(corpus.workflows[1].name)}</h2><span class="state state-trigger">${esc(corpus.workflows[1].trigger)}</span></div><div class="strip-meta"><div><span>Configuration</span><strong>Draft</strong></div><div><span>Live instances</span><strong>No active run</strong></div></div></header></article>
        <article class="strip draft-strip"><header class="strip-summary"><span class="strip-number">History</span><div><h2>Prior overlay</h2><span class="state">${esc(corpus.recentRuns[1].ref)}</span></div><div class="strip-meta"><div><span>Lifecycle</span><strong>Superseded</strong></div><div><span>Verification</span><strong>Not required</strong></div></div></header></article>
      </section>
      ${agentsPanel(corpus, "Roster agents")}${runsPanel(corpus, "Run evidence")}${spendPanel(corpus, "Spend truth")}${createPanel(corpus, "Commission a workflow")}
    </main>
  </article>`;
};

const hallThree = corpus => {
  const pr = corpus.workflows[0];
  return `
  <style>
    /* Hallmark · route: custom (bespoke) · structure: evidence folio · idea: "the graph is the page; runs are annotations" */
    ${foundation(".hall-folio")}
    .hall-folio{--font-display:"Iowan Old Style",Baskerville,"Times New Roman",serif;--font-body:"Avenir Next",Avenir,"Segoe UI",sans-serif;--paper:oklch(95% .018 118);--surface:oklch(98% .012 118);--surface-2:oklch(90% .025 118);--ink:oklch(20% .018 130);--muted:oklch(42% .02 130);--rule-color:oklch(74% .026 118);--signal:oklch(43% .14 145);--focus:oklch(58% .2 145);display:grid;grid-template-rows:auto minmax(0,1fr) auto}
    @media (prefers-color-scheme:dark){.hall-folio{--paper:oklch(14% .018 130);--surface:oklch(18% .02 130);--surface-2:oklch(23% .026 130);--ink:oklch(94% .014 118);--muted:oklch(70% .018 118);--rule-color:oklch(34% .028 130);--signal:oklch(72% .14 145);--focus:oklch(78% .18 145)}}
    html[data-theme="light"] .hall-folio{--paper:oklch(95% .018 118);--surface:oklch(98% .012 118);--surface-2:oklch(90% .025 118);--ink:oklch(20% .018 130);--muted:oklch(42% .02 130);--rule-color:oklch(74% .026 118);--signal:oklch(43% .14 145);--focus:oklch(58% .2 145)}
    html[data-theme="dark"] .hall-folio{--paper:oklch(14% .018 130);--surface:oklch(18% .02 130);--surface-2:oklch(23% .026 130);--ink:oklch(94% .014 118);--muted:oklch(70% .018 118);--rule-color:oklch(34% .028 130);--signal:oklch(72% .14 145);--focus:oklch(78% .18 145)}
    .hall-folio .folio-head{display:grid;grid-template-columns:1fr auto;gap:var(--space-md);align-items:center;padding:var(--space-sm) var(--space-lg);border-bottom:double 3px var(--ink);background:var(--surface)}.hall-folio .folio-head h1{font-size:1.4rem}.hall-folio .folio-head h1 span{font-family:var(--font-body);font-size:.7rem;font-weight:400;letter-spacing:.08em;text-transform:uppercase;color:var(--muted)}.hall-folio .head-tools{display:flex;gap:var(--space-sm);align-items:center}.hall-folio .head-tools .fixture{display:none}.hall-folio [data-theme-toggle]{border:var(--rule) solid var(--ink);background:transparent;padding-inline:var(--space-sm)}
    .hall-folio .folio-body{min-width:0}.hall-folio-nav{display:flex;overflow-x:auto;border-top:var(--rule) solid var(--ink);background:var(--surface)}.hall-folio-nav button{flex:1;border:0;border-inline-end:var(--rule) solid var(--rule-color);background:transparent;padding-inline:var(--space-sm)}.hall-folio-nav button[aria-current]{box-shadow:inset 0 3px var(--signal);color:var(--signal);font-weight:800}
    .hall-folio .spread{display:grid;min-height:calc(100dvh - 112px)}.hall-folio .index-page{padding:var(--space-lg);background:var(--surface);border-bottom:var(--rule) solid var(--rule-color)}.hall-folio .index-page>p{margin:0 0 var(--space-lg);color:var(--muted)}.hall-folio .folio-entry{padding:var(--space-md) 0;border-top:var(--rule) solid var(--rule-color)}.hall-folio .folio-entry h2{font-size:1.5rem}.hall-folio .folio-entry .state{margin-top:var(--space-2xs)}.hall-folio .folio-entry.is-open h2{color:var(--signal)}
    .hall-folio .graph-page{position:relative;min-width:0;padding:var(--space-xl) var(--space-lg);background:var(--paper)}.hall-folio .graph-page>header{display:flex;flex-wrap:wrap;justify-content:space-between;gap:var(--space-md);margin-bottom:var(--space-xl)}.hall-folio .graph-page header p{margin:var(--space-xs) 0 0;color:var(--muted)}.hall-folio .overlay-selector{display:grid;gap:var(--space-2xs);font-family:var(--font-mono);font-size:.72rem}.hall-folio .overlay-selector b{color:var(--signal)}
    .hall-folio .folio-graph{list-style:none;margin:0;padding:0;display:grid;gap:var(--space-lg)}.hall-folio .folio-graph li{position:relative;display:grid;grid-template-columns:3rem minmax(0,1fr);gap:0 var(--space-md);padding-bottom:var(--space-lg);border-bottom:var(--rule) solid var(--rule-color)}.hall-folio .folio-graph .node-index{grid-row:1/3;font:800 2rem/1 var(--font-display);color:var(--rule-color)}.hall-folio .folio-graph small{color:var(--muted)}.hall-folio .folio-graph .is-executing::after{content:"Selected run · executing";position:absolute;right:0;top:0;padding:var(--space-2xs) var(--space-xs);border:var(--rule) solid var(--signal);color:var(--signal);font:700 .65rem/1.4 var(--font-mono)}
    .hall-folio .margin-notes{display:grid;gap:var(--space-sm);margin-top:var(--space-xl);padding-top:var(--space-md);border-top:double 3px var(--ink)}.hall-folio .margin-note{display:grid;grid-template-columns:8rem minmax(0,1fr);gap:var(--space-md);font-size:.8rem}.hall-folio .margin-note span{color:var(--muted)}.hall-folio .margin-note.is-blocked strong{color:var(--signal)}
    .hall-folio .panel-head h1{font-family:var(--font-display)}
    @media (min-width:40rem){.hall-folio .head-tools .fixture{display:block}.hall-folio .spread{grid-template-columns:minmax(14rem,.7fr) minmax(0,1.8fr)}.hall-folio .index-page{border-bottom:0;border-inline-end:var(--rule) solid var(--rule-color)}}
    @media (min-width:72rem){.hall-folio .spread{grid-template-columns:20rem minmax(0,1fr)}.hall-folio .graph-page{padding-inline:clamp(2rem,6vw,7rem)}.hall-folio .folio-graph{max-width:62rem}.hall-folio .margin-notes{max-width:62rem;grid-template-columns:repeat(2,minmax(0,1fr))}.hall-folio .margin-note{grid-template-columns:7rem minmax(0,1fr)}}
  </style>
  <article class="hall-folio">
    <header class="folio-head"><h1>Bitterblossom <span>operator evidence folio</span></h1><div class="head-tools"><span class="fixture">${esc(corpus.notice)}</span><button type="button" data-theme-toggle aria-label="Toggle light and dark theme">Theme</button></div></header>
    <main class="folio-body">
      <section data-panel="workflows" class="panel spread">
        <aside class="index-page"><p>Configured workflow roster</p>${corpus.workflows.map((item,index)=>`<article class="folio-entry ${index===0?"is-open":""}"><h2>${esc(item.name)}</h2>${workflowState(item)}<span class="state">Cost · ${esc(item.cost)}</span></article>`).join("")}<article class="folio-entry"><h2>Prior run overlay</h2><span class="state state-life">Superseded</span><span class="state">${esc(corpus.recentRuns[1].ref)}</span></article></aside>
        <section class="graph-page"><header><div><h1>${esc(pr.name)}</h1><p>Configuration is permanent ink. The selected run appears only as annotation.</p></div><div class="overlay-selector"><span>Overlay · ${esc(corpus.run.id)}</span><b>${esc(corpus.run.state)} at ${esc(corpus.run.step)}</b><span>${esc(corpus.run.elapsed)}</span></div></header><ol class="folio-graph">${topology(corpus,"hall-folio")}</ol><div class="margin-notes"><div class="margin-note"><span>Trigger health</span><strong>Listening</strong></div><div class="margin-note"><span>Run lifecycle</span><strong>${esc(corpus.run.state)}</strong></div><div class="margin-note is-blocked"><span>Domain result</span><strong>Succeeded with blocked result</strong></div><div class="margin-note"><span>Verification</span><strong>Achieved</strong></div><div class="margin-note"><span>Cost truth</span><strong>Unknown · ${esc(corpus.run.cost)}</strong></div><div class="margin-note"><span>Budget</span><strong>${esc(corpus.run.budget)}</strong></div></div></section>
      </section>
      ${agentsPanel(corpus,"Roster register")}${runsPanel(corpus,"Instance record")}${spendPanel(corpus,"Authority and spend")}${createPanel(corpus,"Draft a workflow")}
    </main>
    ${primaryNav("hall-folio")}
  </article>`;
};

export const SPECS = {
  "HALL-1": {
    label: "Control Ledger",
    move: "Persistent workflow index, stable graph, and evidence rail form a three-part operator ledger.",
    philosophy: "Configuration, execution, domain outcome, verification, and accounting stay visually adjacent but semantically separate.",
    render: hallOne
  },
  "HALL-2": {
    label: "Workflow Strips",
    move: "The configured roster itself expands into topology and run evidence, eliminating the separate detail-page assumption.",
    philosophy: "A workflow is the primary product object; its row should hold the plane around it instead of handing authority to a dashboard shell.",
    render: hallTwo
  },
  "HALL-3": {
    label: "Evidence Folio",
    move: "The stable configured graph becomes the page while selectable run state appears as marginal annotation.",
    philosophy: "Treat immutable configuration as the durable record and transient execution as an evidentiary overlay, never a replacement diagram.",
    render: hallThree
  }
};
