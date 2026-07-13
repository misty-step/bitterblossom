const icon = (name, className = "") =>
  `<svg class="${className}" viewBox="0 0 24 24" aria-hidden="true"><use href="#${name}"></use></svg>`;

const sprite = () => `<svg hidden aria-hidden="true">
  <symbol id="swatch-book" viewBox="0 0 24 24"><path d="m4 5 12-2 3 16-12 2L4 5Z" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="m7 4 3 16M16 7h3M16 11h3M17 15h2" fill="none" stroke="currentColor" stroke-width="1.5"/></symbol>
  <symbol id="circle-check" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" stroke-width="1.6"/><path d="m8 12 2.5 2.5L16 9" fill="none" stroke="currentColor" stroke-width="1.6"/></symbol>
  <symbol id="circle-x" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" stroke-width="1.6"/><path d="m9 9 6 6m0-6-6 6" fill="none" stroke="currentColor" stroke-width="1.6"/></symbol>
  <symbol id="triangle-alert" viewBox="0 0 24 24"><path d="m12 4 9 16H3L12 4Z" fill="none" stroke="currentColor" stroke-width="1.6"/><path d="M12 9v5m0 3h.01" fill="none" stroke="currentColor" stroke-width="1.6"/></symbol>
  <symbol id="circle-dot" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" stroke-width="1.6"/><circle cx="12" cy="12" r="2" fill="currentColor"/></symbol>
  <symbol id="arrow-right" viewBox="0 0 24 24"><path d="M4 12h15m-6-6 6 6-6 6" fill="none" stroke="currentColor" stroke-width="1.6"/></symbol>
  <symbol id="plus" viewBox="0 0 24 24"><path d="M12 5v14M5 12h14" fill="none" stroke="currentColor" stroke-width="1.6"/></symbol>
</svg>`;

const modeButton = () =>
  `<button class="ae-button ae-button-quiet ae-button-compact" type="button" data-theme-toggle aria-label="Switch light and dark mode">${icon("circle-dot")}<span>MODE</span></button>`;

const status = (value) => {
  const lower = String(value).toLowerCase();
  const cls = lower.includes("blocked") || lower.includes("unavailable") ? "ae-err" : lower.includes("draft") || lower.includes("no runs") || lower.includes("not run") || lower.includes("superseded") || lower.includes("executing") ? "ae-warn" : "ae-ok";
  const glyph = cls === "ae-err" ? "circle-x" : cls === "ae-warn" ? "triangle-alert" : "circle-check";
  return `<span class="ae-status">${icon(glyph, cls)}<span>${value}</span></span>`;
};

const sourceNode = (label, state = "") => `<div class="source-node ${state}"><span class="ae-mono ae-chrome">${state ? "RUNNING" : "STEP"}</span><strong>${label}</strong></div>`;

const topology = (workflow, run) => `<div class="ae-flow apple-flow" aria-label="${workflow.name} topology with selected run">
  ${workflow.topology.map((node, index) => `${sourceNode(node, node === run.step ? "running" : "")} ${index < workflow.topology.length - 1 ? '<span class="flow-wire" aria-hidden="true">' + icon("arrow-right") + '</span>' : ""}`).join("")}
</div>`;

const workflowList = (c, compact = false) => `<div class="ae-list-rows workflow-list" aria-label="Configured workflows">
  ${c.workflows.map((workflow) => `<button class="ae-list-row ${workflow.name === c.run.workflow ? "is-selected" : ""}" type="button" aria-pressed="${workflow.name === c.run.workflow}">
    <span class="ae-list-cell">${icon(workflow.active ? "circle-dot" : "circle-check", workflow.active ? "ae-ok" : "ae-warn")}</span>
    <span class="ae-list-cell workflow-name"><strong>${workflow.name}</strong><span class="ae-meta">${workflow.trigger}</span></span>
    <span class="ae-list-cell workflow-state">${status(workflow.lifecycle)}<span class="ae-meta">${workflow.active ? `${workflow.active} active` : "No active run"}</span></span>
  </button>`).join("")}
</div>`;

const agentGroup = (label, agents) => `<section class="agent-group"><h3 class="ae-chrome">${label}</h3>${agents.map((agent) => `<article class="agent-row">
    <span class="agent-mark ae-cat-${agent.active ? "0" : "1"}">${agent.name.slice(0, 1)}</span>
    <div><strong>${agent.name}</strong><span class="ae-meta">${agent.model} · ${agent.harness}</span><span class="ae-meta">${agent.ceiling}</span></div>
    <span class="ae-num">${agent.active}</span>
  </article>`).join("")}</section>`;

const agentRoster = (c) => {
  const inUse = c.agents.filter((agent) => agent.availability === "In use");
  const available = c.agents.filter((agent) => agent.availability === "Available");
  return `<div class="agent-roster">${agentGroup("In use", inUse)}${agentGroup("Available", available)}</div>`;
};

const runMatrix = (c) => `<div class="run-matrix ae-plate">
  <div><span class="ae-meta">EXECUTION</span>${status(c.run.state)}</div>
  <div><span class="ae-meta">DOMAIN RESULT</span>${status(c.workflows[0].latestDomain)}</div>
  <div><span class="ae-meta">VERIFICATION</span>${status(c.workflows[0].verification)}</div>
  <div><span class="ae-meta">COST</span>${status(c.run.cost)}</div>
</div>`;

const childTrail = (c) => `<div class="ae-trail child-trail" aria-label="Selected run child evidence">
  ${c.run.children.map((child) => `<div class="ae-trail-entry"><span class="ae-mono ae-chrome">${child.state}</span><span><strong>${child.name}</strong><span class="ae-meta">${child.result}</span></span>${status(child.state)}</div>`).join("")}
</div>`;

const history = (c) => `<div class="ae-list-rows history-list" aria-label="Live and historical evidence">
  ${c.recentRuns.map((run) => `<div class="ae-list-row"><span class="ae-list-cell"><span class="ae-mono ae-chrome">${run.ref}</span></span><span class="ae-list-cell"><strong>${run.workflow}</strong><span class="ae-meta">${run.execution} execution · ${run.domain} domain</span></span><span class="ae-list-cell">${status(run.verification)}<span class="ae-meta">${run.cost} cost</span></span></div>`).join("")}
</div>`;

const spend = (c) => `<div class="spend-grid">${c.spend.scopes.map((scope, index) => `<div class="spend-row"><span>${scope}</span><span class="ae-meta">${index === 0 ? "enforced ceiling" : "configured"}</span></div>`).join("")}<div class="truth-table">${c.spend.truth.map((item) => `<div><strong>${item.label}</strong><span>${item.value}</span><span class="ae-meta">${item.state}</span></div>`).join("")}</div></div>`;

const creation = (c) => `<section class="creation ae-settings"><div class="section-heading"><span class="ae-chrome">CREATE WORKFLOW</span><span class="ae-tag">goal first</span></div>
  <div class="goal-pair"><div><span class="ae-meta">RAW GOAL</span><p>${c.creation.rawGoal}</p></div><div><span class="ae-meta">ENHANCED GOAL · REVIEW BEFORE ACTIVATION</span><p>${c.creation.enhancedGoal}</p></div></div>
  <div class="creation-steps">${c.creation.steps.map((step, index) => `<span class="step ${index === 0 ? "is-current" : ""}"><span class="ae-num">0${index + 1}</span>${step}</span>`).join("")}</div>
  <div class="fixture-line">${icon("triangle-alert", "ae-warn")}<span>${c.creation.test}</span><button class="ae-button ae-button-primary" type="button">Test fixture ${icon("arrow-right")}</button></div>
</section>`;

const directManipulation = `tabindex="0" role="button" aria-label="Move the selected run overlay along its source path" onpointerdown="this.setPointerCapture(event.pointerId);this.dataset.grab=event.clientX;this.dataset.start=parseFloat(getComputedStyle(this).getPropertyValue('--apple-drag-x'))||0;this.dataset.moved='true'" onpointermove="if(this.hasPointerCapture(event.pointerId)){var next=Number(this.dataset.start||0)+event.clientX-Number(this.dataset.grab||event.clientX);this.style.setProperty('--apple-drag-x',Math.max(-28,Math.min(28,next))+'px')}" onpointerup="this.releasePointerCapture(event.pointerId)" onpointercancel="this.releasePointerCapture(event.pointerId)"`;

const shell = (className, body, title) => `<style>
  .${className}{min-height:100dvh;display:grid;background:var(--ae-surface);color:var(--ae-ink);overflow:hidden}
  .${className} .ae-app-mark{display:inline-grid;place-items:center;width:1.5rem;height:1.5rem}
  .${className} .ae-app-mark svg{width:1.35rem;height:1.35rem}
  .${className} .brand{display:flex;align-items:center;gap:.55rem;font-weight:800}
  .${className} .brand-name{letter-spacing:.02em}
  .${className} .rail{border-right:1px solid var(--ae-line);padding:1rem .75rem;display:flex;flex-direction:column;gap:1.5rem;min-width:0}
  .${className} .rail nav{display:grid;gap:.15rem}
  .${className} .rail nav button,.${className} .top-nav button{border:0;border-bottom:1px solid var(--ae-line);background:transparent;color:var(--ae-ink-muted);padding:.6rem .25rem;text-align:left;cursor:pointer}
  .${className} .rail nav button[aria-current],.${className} .top-nav button[aria-current]{color:var(--ae-ink);font-weight:800}
  .${className} .rail .mode{margin-top:auto}
  .${className} .topbar{display:flex;align-items:center;gap:1rem;border-bottom:1px solid var(--ae-line);padding:.7rem 1rem;min-width:0}
  .${className} .topbar .top-nav{display:flex;justify-content:center;gap:.3rem;flex:1;min-width:0}
  .${className} .topbar .top-nav button{border-bottom:0;padding:.35rem .45rem;white-space:nowrap}
  .${className} .topbar>.ae-button{flex:none}
  .${className} main{min-width:0;overflow:auto;padding:1.25rem}
  .${className} .topline,.${className} .section-heading{display:flex;align-items:flex-end;justify-content:space-between;gap:1rem}
  .${className} .topline{border-bottom:1px solid var(--ae-line);padding-bottom:.8rem;margin-bottom:.8rem}
  .${className} h1,.${className} h2,.${className} h3,.${className} p{margin:0}
  .${className} h1,.${className} h2,.${className} h3{font-weight:800}
  .${className} .section-heading{border-bottom:1px solid var(--ae-line);padding-bottom:.45rem;margin-bottom:.55rem}
  .${className} .ae-stat-badges{margin-bottom:1rem}
  .${className} .workflow-list{border-top:1px solid var(--ae-line)}
  .${className} .ae-list-row{border-bottom:1px solid var(--ae-line);display:grid;grid-template-columns:auto minmax(0,1fr) auto;gap:.65rem;align-items:start;padding:.65rem 0;background:transparent;color:inherit;text-align:left;cursor:pointer;width:100%}
  .${className} button.ae-list-row{border-inline:0;border-top:0}
  .${className} .ae-list-row.is-selected{border-left:2px solid var(--ae-accent);padding-left:.5rem}
  .${className} .ae-list-cell{display:grid;gap:.2rem;min-width:0}
  .${className} .ae-list-cell svg{width:1rem;height:1rem}
  .${className} .workflow-state{justify-items:end;text-align:right}
  .${className} .ae-status{white-space:nowrap}
  .${className} .ae-status svg{width:1rem;height:1rem;vertical-align:-.15rem;margin-right:.25rem}
  .${className} .apple-flow{display:flex;align-items:stretch;gap:.45rem;min-width:38rem;padding:1.1rem 0}
  .${className} .source-node{border:1px solid var(--ae-line);padding:.55rem;min-width:7rem;display:grid;gap:.35rem;align-content:center;background:var(--ae-surface)}
  .${className} .source-node.running{border-color:var(--ae-accent)}
  .${className} .flow-wire{display:grid;place-items:center;color:var(--ae-ink-faint)}
  .${className} .flow-wire svg{width:1.1rem;height:1.1rem}
  .${className} .run-overlay{border:1px solid var(--ae-accent);background:var(--ae-wash);padding:.7rem;display:grid;gap:.5rem;transform:translateX(var(--apple-drag-x,0px));touch-action:none;cursor:grab;will-change:transform}
  .${className} .run-overlay:active{cursor:grabbing}
  .${className} .run-overlay .overlay-head{display:flex;justify-content:space-between;gap:.75rem;align-items:start}
  .${className} .run-overlay .overlay-meta{display:grid;gap:.25rem}
  .${className} .run-overlay .grab-note{color:var(--ae-ink-muted);border-top:1px solid var(--ae-line);padding-top:.45rem}
  .${className} .run-matrix{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:.65rem}
  .${className} .run-matrix>div{display:grid;gap:.25rem;border-bottom:1px solid var(--ae-line);padding-bottom:.45rem}
  .${className} .child-trail{border-left:1px solid var(--ae-line);padding-left:.8rem}
  .${className} .ae-trail-entry{display:grid;grid-template-columns:auto minmax(0,1fr) auto;gap:.6rem;align-items:start;padding:.55rem 0;border-bottom:1px solid var(--ae-line)}
  .${className} .ae-trail-entry>span:nth-child(2){display:grid;gap:.2rem}
  .${className} .agent-roster{display:grid;gap:.8rem}
  .${className} .agent-group{border-top:1px solid var(--ae-line);padding-top:.5rem}
  .${className} .agent-group h3{color:var(--ae-ink-muted);margin-bottom:.25rem}
  .${className} .agent-row{display:grid;grid-template-columns:auto minmax(0,1fr) auto;gap:.6rem;align-items:start;border-bottom:1px solid var(--ae-line);padding:.55rem 0}
  .${className} .agent-row>div{display:grid;gap:.18rem}
  .${className} .agent-mark{display:grid;place-items:center;width:1.6rem;height:1.6rem;border:1px solid currentColor;font-family:var(--ae-font-mono);font-weight:800}
  .${className} .history-list{border-top:1px solid var(--ae-line)}
  .${className} .history-list .ae-list-row{cursor:default}
  .${className} .spend-grid{display:grid;gap:.45rem}
  .${className} .spend-row,.${className} .truth-table>div{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:.7rem;border-bottom:1px solid var(--ae-line);padding:.45rem 0}
  .${className} .truth-table{border-top:1px solid var(--ae-line);margin-top:.45rem}
  .${className} .truth-table>div{grid-template-columns:auto minmax(0,1fr) auto}
  .${className} .goal-pair{display:grid;grid-template-columns:1fr 1.5fr;gap:1rem}
  .${className} .goal-pair>div{border-bottom:1px solid var(--ae-line);padding:.6rem 0;display:grid;gap:.35rem}
  .${className} .creation-steps{display:flex;flex-wrap:wrap;gap:.8rem;border-bottom:1px solid var(--ae-line);padding:.6rem 0}
  .${className} .step{display:inline-flex;align-items:center;gap:.35rem;color:var(--ae-ink-muted)}
  .${className} .step.is-current{color:var(--ae-ink);font-weight:800}
  .${className} .fixture-line{display:flex;align-items:center;gap:.55rem;padding-top:.6rem}
  .${className} .fixture-line svg{width:1rem;height:1rem;flex:none}
  .${className} .fixture-line .ae-button{margin-left:auto}
  .${className} .ae-button{cursor:pointer}
  .${className} .workbench-grid{display:grid;grid-template-columns:minmax(12rem,.8fr) minmax(22rem,1.8fr) minmax(15rem,1fr);gap:1.25rem}
  .${className} .lower-grid,.${className} .after-grid,.${className} .spend-create-grid{display:grid;grid-template-columns:minmax(0,1.25fr) minmax(0,1fr);gap:1.25rem;margin-top:1.25rem}
  .${className} .evidence-heading{margin-top:1.1rem}
  .${className} .workflow-strip,.${className} .topology-aperture,.${className} .evidence-history,.${className} .configured-after{margin-bottom:1.25rem}
  .${className} .ae-wall{display:grid;grid-template-columns:repeat(auto-fit,minmax(13rem,1fr));gap:1rem}
  .${className} .workflow-card,.${className} .source-card{border:1px solid var(--ae-line);background:var(--ae-surface);color:inherit;text-align:left;padding:.8rem;display:grid;gap:.4rem;cursor:pointer}
  .${className} .workflow-card.is-selected{border-color:var(--ae-accent)}
  .${className} .workflow-card .ae-status{margin-top:.25rem}
  .${className} .topology-aperture{border-bottom:1px solid var(--ae-line);padding-bottom:1rem;position:relative}
  .${className} .overlay-on-flow{max-width:30rem;margin-top:-.25rem}
  .${className} .control-board{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:1rem;margin-bottom:1.25rem}
  .${className} .ae-column{min-width:0;border-top:1px solid var(--ae-line);padding-top:.55rem}
  .${className} .evidence-history{border-top:1px solid var(--ae-line);padding-top:.55rem}
  .${className} .run-first-grid{display:grid;grid-template-columns:minmax(14rem,.75fr) minmax(0,1.8fr);gap:1.25rem;margin-bottom:1.25rem}
  .${className} .context-rail{border-right:1px solid var(--ae-line);padding-right:1rem;display:grid;align-content:start;gap:1rem}
  .${className} .context-block{display:grid;gap:.3rem;border-bottom:1px solid var(--ae-line);padding-bottom:.8rem}
  .${className} .context-block .apple-flow{min-width:0;display:grid;gap:.35rem;padding:.2rem 0}
  .${className} .context-block .source-node{min-width:0}
  .${className} .context-block .flow-wire{transform:rotate(90deg);height:1rem}
  .${className} .spine-main{min-width:0}
  .${className} .run-spine{border-left:1px solid var(--ae-line);padding-left:1rem}
  .${className} .spine-head,.${className} .spine-foot{display:flex;align-items:center;gap:.7rem;border-bottom:1px solid var(--ae-line);padding:.6rem 0}
  .${className} .spine-foot{border-top:1px solid var(--ae-line);border-bottom:0}
  .${className} .spine-grab{max-width:30rem;margin:1rem auto 0}
  .${className} .source-card-head{display:flex;justify-content:space-between;gap:.6rem;align-items:start}
  .${className} .source-card{cursor:default}
  @media (max-width:900px){.${className}{display:block}.${className} .rail{border-right:0;border-bottom:1px solid var(--ae-line);padding:.7rem;display:grid;grid-template-columns:1fr auto;gap:.7rem;position:sticky;top:0;z-index:2;background:var(--ae-surface)}.${className} .rail nav{grid-column:1/-1;display:flex;overflow:auto;gap:.35rem}.${className} .rail nav button{white-space:nowrap;border-bottom:0;padding:.35rem .45rem}.${className} .rail .mode{margin:0}.${className} main{padding:.9rem}.${className} .workbench-grid,.${className} .run-first-grid{grid-template-columns:1fr}.${className} .context-rail{border-right:0;border-bottom:1px solid var(--ae-line);padding:0 0 1rem}.${className} .lower-grid,.${className} .after-grid,.${className} .spend-create-grid{grid-template-columns:1fr}.${className} .control-board{grid-template-columns:1fr}.${className} .topbar{padding:.7rem;flex-wrap:wrap}.${className} .topbar .top-nav{order:3;flex-basis:100%;justify-content:flex-start;overflow:auto}}
  @media (max-width:620px){.${className} main{padding:.75rem}.${className} .topline,.${className} .section-heading{align-items:flex-start;flex-direction:column;gap:.45rem}.${className} .workflow-state{justify-items:start;text-align:left}.${className} .run-matrix,.${className} .goal-pair{grid-template-columns:1fr}.${className} .fixture-line{align-items:flex-start;flex-wrap:wrap}.${className} .fixture-line .ae-button{margin-left:0}.${className} .apple-flow{min-width:0;display:grid;grid-template-columns:1fr;gap:.35rem}.${className} .flow-wire{transform:rotate(90deg);height:1rem}.${className} .source-node{min-width:0}.${className} .creation-steps{display:grid;grid-template-columns:repeat(2,minmax(0,1fr))}}
  @media (prefers-reduced-motion:reduce){.${className} .run-overlay{will-change:auto}}
</style>${sprite()}<div class="${className} ae-shell">${body}</div>`;

const brand = () => `<div class="brand"><span class="ae-app-mark">${icon("swatch-book")}</span><span class="brand-name">Bitterblossom</span></div>`;

const nav = (mode = "rail") => `<nav class="${mode === "top" ? "top-nav" : ""}" aria-label="Primary destinations"><button aria-current="page" type="button">Workflows</button><button type="button">Agents</button><button type="button">Runs</button><button type="button">Spend</button><button type="button">Create workflow</button></nav>`;

const optionOne = (c) => {
  const selected = c.workflows[0];
  const body = `<aside class="rail">${brand()}${nav()}${modeButton()}</aside><main>
    <header class="topline"><div><span class="ae-meta ae-mono">APPLE-1 · SOURCE-ANCHORED WORKBENCH</span><h1>Configured workflows</h1></div><span class="ae-meta">${c.notice}</span></header>
    <div class="ae-stat-badges"><div><strong class="ae-num">${c.workflows.length}</strong><span class="ae-meta">configured</span></div><div><strong class="ae-num">${c.run.state}</strong><span class="ae-meta">selected run</span></div><div><strong>${selected.latestDomain}</strong><span class="ae-meta">domain result</span></div><div><strong>${selected.cost}</strong><span class="ae-meta">cost truth</span></div></div>
    <section class="workbench-grid"><section class="roster-pane"><div class="section-heading"><h2>Workflow roster</h2><span class="ae-chrome">SOURCE</span></div>${workflowList(c)}</section>
      <section class="run-pane"><div class="section-heading"><div><h2>${selected.name}</h2><span class="ae-meta">${selected.trigger}</span></div><span class="ae-tag">selected live run</span></div>${topology(selected, c.run)}<div class="run-card-wrap"><article class="run-overlay" ${directManipulation}><div class="overlay-head"><div class="overlay-meta"><span class="ae-meta ae-mono">${c.run.id}</span><strong>${c.run.step} has the handoff</strong></div>${status(c.run.state)}</div><div class="ae-meta">${c.run.elapsed} · parent ${c.run.parent} · trigger ${c.run.trigger}</div><span class="grab-note ae-chrome">Drag from this source; the run overlay stays spatially attached.</span></article></div>${runMatrix(c)}</section>
      <aside class="evidence-pane"><div class="section-heading"><h2>Evidence now</h2><span class="ae-chrome">LIVE / HISTORY</span></div>${childTrail(c)}<div class="section-heading evidence-heading"><h2>History</h2></div>${history(c)}</aside></section>
    <section class="lower-grid"><section><div class="section-heading"><h2>Agent roster</h2><span class="ae-chrome">AUTHORITY VISIBLE</span></div>${agentRoster(c)}</section><section><div class="section-heading"><h2>Spend</h2><span class="ae-chrome">TRUTH BY SCOPE</span></div>${spend(c)}</section></section>${creation(c)}
  </main>`;
  return shell("apple-one", body, "Source-anchored workbench");
};

const optionTwo = (c) => {
  const selected = c.workflows[0];
  const boardColumns = `<section class="ae-column"><div class="section-heading"><h2>Agents · in use</h2><span class="ae-chrome">2</span></div>${agentGroup("In use", c.agents.filter((agent) => agent.availability === "In use"))}</section><section class="ae-column"><div class="section-heading"><h2>Agents · available</h2><span class="ae-chrome">1</span></div>${agentGroup("Available", c.agents.filter((agent) => agent.availability === "Available"))}</section><section class="ae-column"><div class="section-heading"><h2>Selected evidence</h2><span class="ae-chrome">RUN</span></div>${runMatrix(c)}${childTrail(c)}</section>`;
  const body = `<header class="topbar"><div>${brand()}</div>${nav("top")}${modeButton()}</header><main>
    <header class="topline"><div><span class="ae-meta ae-mono">APPLE-2 · TOPOLOGY-FIRST BOARD</span><h1>Workflow control surface</h1></div><span class="ae-meta">${c.notice}</span></header>
    <section class="workflow-strip"><div class="section-heading"><h2>Configured workflow sources</h2><span class="ae-chrome">SELECT A SOURCE</span></div><div class="ae-wall">${c.workflows.map((workflow) => `<button class="workflow-card ${workflow.name === selected.name ? "is-selected" : ""}" type="button"><span class="ae-meta ae-mono">${workflow.lifecycle} · ${workflow.active ? `${workflow.active} active` : "No active run"}</span><strong>${workflow.name}</strong><span class="ae-meta">${workflow.trigger}</span>${status(workflow.latestExecution)}</button>`).join("")}</div></section>
    <section class="topology-aperture"><div class="section-heading"><div><h2>${selected.name} topology</h2><span class="ae-meta">A run is a moving trace through this fixed graph.</span></div><span class="ae-tag">live overlay</span></div>${topology(selected, c.run)}<article class="run-overlay overlay-on-flow" ${directManipulation}><div class="overlay-head"><span><span class="ae-meta ae-mono">${c.run.id}</span><strong>${c.run.step} · ${c.run.elapsed}</strong></span>${status(c.run.state)}</div><span class="ae-meta">${c.run.parent} commissioned the selected child path. Drag to keep the overlay with the source.</span></article></section>
    <section class="ae-board control-board">${boardColumns}</section>
    <section class="evidence-history"><div class="section-heading"><h2>Live and historical evidence</h2><span class="ae-chrome">NO COLLAPSED STATES</span></div>${history(c)}</section>
    <section class="spend-create-grid"><section><div class="section-heading"><h2>Spend controls</h2><span class="ae-chrome">SCOPE LADDER</span></div>${spend(c)}</section>${creation(c)}</section>
  </main>`;
  return shell("apple-two", body, "Topology-first board");
};

const optionThree = (c) => {
  const selected = c.workflows[0];
  const trail = `<div class="run-spine ae-trail"><div class="spine-head"><span class="ae-meta ae-mono">${c.run.id}</span>${status(c.run.state)}<span class="ae-meta">${c.run.elapsed}</span></div>${c.run.children.map((child, index) => `<div class="ae-trail-entry"><span class="ae-num">0${index + 1}</span><span><strong>${child.name}</strong><span class="ae-meta">${child.result}</span></span>${status(child.state)}</div>`).join("")}<div class="spine-foot">${status(selected.latestDomain)}<span class="ae-meta">${selected.verification} · ${selected.cost}</span></div></div>`;
  const body = `<header class="topbar"><div>${brand()}</div><div class="top-nav">${nav("top")}</div>${modeButton()}</header><main>
    <header class="topline"><div><span class="ae-meta ae-mono">APPLE-3 · RUN-FIRST EVIDENCE SPINE</span><h1>What is happening now</h1></div><span class="ae-meta">${c.notice}</span></header>
    <section class="run-first-grid"><aside class="context-rail"><div class="section-heading"><h2>Run context</h2><span class="ae-chrome">SOURCE AFTER</span></div><div class="context-block"><span class="ae-meta">SELECTED WORKFLOW</span><strong>${selected.name}</strong><span class="ae-meta">${selected.trigger}</span>${status(selected.lifecycle)}</div><div class="context-block">${topology(selected, c.run)}</div><div class="context-block"><span class="ae-meta">AUTHORITY</span><strong>${c.run.parent}</strong><span class="ae-meta">children inherit the declared ceiling</span></div></aside><section class="spine-main"><div class="section-heading"><div><h2>Live evidence spine</h2><span class="ae-meta">The run owns the first read; configuration remains one anchored context away.</span></div><span class="ae-tag">executing</span></div>${trail}<article class="run-overlay spine-grab" ${directManipulation}><div class="overlay-head"><span><span class="ae-meta ae-mono">CONTINUOUS TRACE</span><strong>Verifier follows the finger</strong></span>${icon("circle-dot", "ae-ok")}</div><span class="ae-meta">A direct grab preserves the selected run's place in the evidence sequence.</span></article></section></section>
    <section class="configured-after"><div class="section-heading"><h2>Configured sources</h2><span class="ae-chrome">THE OTHER DIRECTION</span></div><div class="ae-wall">${c.workflows.map((workflow) => `<article class="source-card"><div class="source-card-head"><strong>${workflow.name}</strong>${status(workflow.lifecycle)}</div><span class="ae-meta">${workflow.topology.join(" → ")}</span><span class="ae-meta">${workflow.trigger}</span><span class="ae-meta">${workflow.active ? `${workflow.active} active` : "No active run"} · ${workflow.cost}</span></article>`).join("")}</div></section>
    <section class="after-grid"><section><div class="section-heading"><h2>Agents by availability</h2><span class="ae-chrome">ROSTER</span></div>${agentRoster(c)}</section><section><div class="section-heading"><h2>Spend controls</h2><span class="ae-chrome">REPORT / ESTIMATE / GAP</span></div>${spend(c)}</section></section>${creation(c)}
  </main>`;
  return shell("apple-three", body, "Run-first evidence spine");
};

export const SPECS = {
  "APPLE-1": {
    label: "Source-anchored workbench",
    move: "A fixed workflow source anchors a directly movable live-run overlay beside evidence and authority.",
    philosophy: "Purpose and familiarity through a stable source-to-run map; direct manipulation keeps the selected run spatially continuous with its topology.",
    render: optionOne,
  },
  "APPLE-2": {
    label: "Topology-first board",
    move: "The topology is the stage: workflow sources sit above it and live, agent, and evidence lanes occupy a ruled board.",
    philosophy: "Agency and craft through a graph that exposes where a run is, what it can do, and what evidence it has without hiding state in a detail page.",
    render: optionTwo,
  },
  "APPLE-3": {
    label: "Run-first evidence spine",
    move: "Inverts configuration-first emphasis: the selected run and its evidence own the first read, with configuration anchored as context.",
    philosophy: "Simplicity by inversion: operators follow the active trace first, then inspect the declared source, authority, spend, and creation contract in place.",
    render: optionThree,
  },
};
