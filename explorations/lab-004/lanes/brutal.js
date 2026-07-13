const esc = (value) => String(value ?? "").replace(/[&<>\"']/g, (char) => ({
  "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
})[char]);

const icon = (kind = "neutral") => {
  const cls = kind === "ok" ? "ae-ok" : kind === "warn" ? "ae-warn" : kind === "err" ? "ae-err" : "";
  const path = kind === "ok"
    ? '<path d="m9 12 2 2 4-4"/><circle cx="12" cy="12" r="9"/>'
    : kind === "warn"
      ? '<path d="M12 9v4m0 4h.01"/><path d="M10.3 3.7 2.2 18a2 2 0 0 0 1.8 3h16a2 2 0 0 0 1.8-3L13.7 3.7a2 2 0 0 0-3.4 0Z"/>'
      : kind === "err"
        ? '<path d="m9 9 6 6m0-6-6 6"/><circle cx="12" cy="12" r="9"/>'
        : '<circle cx="12" cy="12" r="9"/><path d="M12 8v4l3 2"/>';
  return `<svg class="ae-icon ${cls}" viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.7">${path}</svg>`;
};

const mode = () => `<button class="ae-button ae-button-quiet ae-button-compact" type="button" data-theme-toggle aria-label="Switch light and dark mode">${icon()}<span>MODE</span></button>`;

const status = (label, kind = "neutral") => `<span class="ae-status">${icon(kind)}<span class="ae-status-label">${esc(label)}</span></span>`;

const destinations = (active) => ["Workflows", "Agents", "Runs", "Spend", "Create workflow"].map((item, index) =>
  `<a href="#${item.toLowerCase().replaceAll(" ", "-")}" ${item === active ? 'aria-current="page" class="ae-accent ae-strong"' : ""}><span class="ae-chrome">0${index + 1}</span> ${esc(item)}</a>`
).join("");

const workflowRows = (corpus, compact = false) => corpus.workflows.map((workflow, index) => `
  <article class="${compact ? "brut-register" : "brut-workflow"}">
    <div class="brut-index ae-chrome">WF / 0${index + 1}</div>
    <div><strong>${esc(workflow.name)}</strong><div class="ae-dim">${esc(workflow.trigger)}</div></div>
    <div>${status(workflow.lifecycle, workflow.lifecycle === "Active" ? "ok" : "neutral")}</div>
    <div>${workflow.active ? status(`${workflow.active} active run`, "warn") : status("No active run")}</div>
    <div class="ae-chrome">${esc(workflow.budget)}</div>
  </article>`).join("");

const topology = (corpus, className = "") => `<section class="ae-flow ${className}" aria-label="PR Review topology and selected run overlay">
  ${corpus.workflows[0].topology.map((step, index) => `
    <div class="ae-node ${step === corpus.run.step ? "brut-running" : ""}">
      <span class="ae-node-kicker">0${index + 1} / ${step === corpus.run.step ? "EXECUTING" : "CONFIGURED"}</span>
      <strong class="ae-node-label">${esc(step)}</strong>
      ${step === corpus.run.step ? `<span>${status(corpus.run.state, "warn")}</span>` : ""}
    </div>${index < corpus.workflows[0].topology.length - 1 ? '<div class="brut-wire" aria-hidden="true">&gt;&gt;&gt;</div>' : ""}`
  ).join("")}
</section>`;

const evidence = (corpus) => `<section id="runs" class="brut-section">
  <header class="brut-heading"><span class="ae-chrome">LIVE / HISTORY EVIDENCE</span><h2>${esc(corpus.run.id)}</h2></header>
  <div class="ae-stat-badges">
    <div class="ae-stat-badge"><span class="ae-stat-value">${esc(corpus.run.elapsed)}</span><span class="ae-stat-label">elapsed</span></div>
    <div class="ae-stat-badge"><span class="ae-stat-value">${esc(corpus.run.cost)}</span><span class="ae-stat-label">cost truth</span></div>
    <div class="ae-stat-badge"><span class="ae-stat-value">${esc(corpus.run.budget)}</span><span class="ae-stat-label">budget</span></div>
  </div>
  <div class="ae-trail">
    ${corpus.recentRuns.map((run) => `<div class="ae-trail-item"><div class="ae-trail-head"><span class="ae-trail-time">${esc(run.ref)}</span><span class="ae-trail-who">${esc(run.workflow)}</span></div><div class="ae-trail-body">${status(run.execution, run.execution === "Succeeded" ? "ok" : run.execution === "Superseded" ? "neutral" : "warn")} · domain <strong>${esc(run.domain)}</strong> · verification <strong>${esc(run.verification)}</strong> · cost <strong>${esc(run.cost)}</strong></div></div>`).join("")}
  </div>
</section>`;

const agents = (corpus) => ["In use", "Available"].map((group) => `<section class="brut-agent-group"><header class="brut-heading"><h2 class="ae-chrome">AGENTS / ${esc(group.toUpperCase())}</h2><strong>${corpus.agents.filter((agent) => agent.availability === group).length}</strong></header><div class="ae-wall">${corpus.agents.filter((agent) => agent.availability === group).map((agent) => `<article class="ae-wall-card"><div class="ae-wall-head"><strong>${esc(agent.name)}</strong>${status(agent.active ? "Executing" : "Available", agent.active ? "warn" : "ok")}</div><div class="ae-wall-meta">${esc(agent.model)}<br>${esc(agent.harness)}<br>${esc(agent.ceiling)}<br>${esc(agent.workflows)}</div></article>`).join("")}</div></section>`).join("");

const spend = (corpus) => `<section id="spend" class="brut-section"><header class="brut-heading"><span class="ae-chrome">SPEND / TRUTH LAYERS</span><h2>Controls before totals</h2></header><div class="brut-spend-grid"><div class="ae-settings">${corpus.spend.scopes.map((scope, index) => `<div class="ae-setting"><span>0${index + 1} ${esc(scope)}</span><span class="ae-setting-val">CONFIGURED</span></div>`).join("")}</div><div class="ae-list-rows">${corpus.spend.truth.map((truth) => `<div class="ae-list-row"><div class="ae-list-cell">${status(truth.label, truth.label === "Reported" ? "ok" : truth.label === "Estimated" ? "warn" : "err")}</div><div class="ae-list-cell"><strong>${esc(truth.value)}</strong><span class="ae-dim">${esc(truth.state)}</span></div></div>`).join("")}</div></div></section>`;

const creation = (corpus, lead = false) => `<section id="create-workflow" class="brut-section brut-create ${lead ? "brut-create-lead" : ""}"><header class="brut-heading"><span class="ae-chrome">GOAL-FIRST CREATION / 01—07</span><h2>Create workflow</h2></header><div class="brut-create-grid"><div><label class="ae-label" for="brut-goal-${lead ? "lead" : "std"}">Operator goal</label><textarea class="ae-input" id="brut-goal-${lead ? "lead" : "std"}" rows="4">${esc(corpus.creation.rawGoal)}</textarea><div class="brut-steps ae-chrome">${corpus.creation.steps.map((step, index) => `<span>${index + 1} ${esc(step)}</span>`).join("")}</div></div><div class="ae-panel"><span class="ae-label">Enhanced-goal review</span><p>${esc(corpus.creation.enhancedGoal)}</p><div class="ae-setting"><span>Fixture test</span><strong>${esc(corpus.creation.test)}</strong></div><button class="ae-button" type="button">Review fixture →</button></div></div></section>`;

const commonCss = `
  .brut{height:100dvh;overflow:hidden;background:var(--ae-surface);color:var(--ae-ink);font-family:var(--ae-font);}
  .brut *{box-sizing:border-box}.brut a{color:inherit;text-decoration:none}.brut p,.brut h1,.brut h2{margin:0}.brut h1,.brut h2{font:inherit;font-weight:800}.brut strong{font-weight:800}.brut .ae-icon{flex:0 0 auto}
  .brut-stage{min-width:0;min-height:0;overflow:auto}.brut-section{border-top:1px solid var(--ae-line);padding:var(--ae-space-4,24px)}
  .brut-heading{display:flex;align-items:baseline;justify-content:space-between;gap:16px;margin-bottom:var(--ae-space-4,24px)}
  .brut-heading>strong,.brut-heading>h2{text-align:right}.brut-index{color:var(--ae-ink-faint)}.brut-running{outline:2px solid var(--ae-accent);outline-offset:-2px}
  .brut-wire{align-self:center;color:var(--ae-accent);font-family:var(--ae-font-mono)}
  .brut-spend-grid,.brut-create-grid{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:var(--ae-space-5,32px)}
  .brut-create textarea{width:100%;resize:vertical}.brut-create .ae-panel{padding:var(--ae-space-3,16px)}
  .brut-steps{display:flex;flex-wrap:wrap;gap:12px;margin-top:12px}.brut-agent-group{padding:var(--ae-space-4,24px);border-top:1px solid var(--ae-line)}
  @media(max-width:700px){.brut-spend-grid,.brut-create-grid{grid-template-columns:minmax(0,1fr)}.brut-create textarea{min-width:0;max-width:100%}.brut-heading{align-items:flex-start}.brut-heading>strong,.brut-heading>h2{max-width:58%}.brut-section,.brut-agent-group{padding:16px}}
`;

const renderLedger = (corpus) => `<div class="brut brut-ledger">
  <style>${commonCss}
    .brut-ledger{display:grid;grid-template-columns:168px minmax(0,1fr)}.brut-ledger .brut-rail{border-right:1px solid var(--ae-line);display:flex;flex-direction:column;min-height:0;padding:16px 12px}.brut-ledger .brut-rail nav{display:grid;gap:14px;margin-top:40px}.brut-ledger .brut-rail .ae-button{margin-top:auto}.brut-ledger .brut-mast{position:sticky;top:0;z-index:2;background:var(--ae-surface);border-bottom:1px solid var(--ae-line);padding:16px 24px;display:grid;grid-template-columns:minmax(0,1fr) auto;gap:20px}.brut-ledger .brut-mast-meta{display:flex;gap:24px;align-items:center}.brut-ledger .brut-workflow{display:grid;grid-template-columns:70px minmax(180px,2fr) minmax(100px,1fr) minmax(120px,1fr) minmax(140px,1fr);gap:12px;padding:16px 24px;border-bottom:1px solid var(--ae-line);align-items:center}.brut-ledger .brut-workflow:first-child{border-color:var(--ae-accent)}.brut-ledger .brut-detail-grid{display:grid;grid-template-columns:minmax(0,3fr) minmax(260px,1fr)}.brut-ledger .brut-detail-aside{border-left:1px solid var(--ae-line)}.brut-ledger .ae-flow{padding:24px;display:grid;grid-template-columns:1fr auto 1fr auto 1fr auto 1fr;align-items:stretch}.brut-ledger .ae-node{min-width:0}.brut-ledger .brut-notice{padding:8px 24px;border-bottom:1px solid var(--ae-line)}
    @media(max-width:800px){.brut-ledger{grid-template-columns:1fr;grid-template-rows:minmax(0,1fr) auto}.brut-ledger .brut-rail{grid-row:2;border-right:0;border-top:1px solid var(--ae-line);padding:8px;flex-direction:row;overflow:hidden}.brut-ledger .brut-rail>strong{display:none}.brut-ledger .brut-rail nav{display:flex;margin:0;gap:18px;white-space:nowrap;min-width:0;flex:1;overflow:auto}.brut-ledger .brut-rail .ae-button{margin:0 0 0 auto;flex:0 0 auto}.brut-ledger .brut-stage{grid-row:1}.brut-ledger .brut-workflow{grid-template-columns:42px 1fr}.brut-ledger .brut-workflow>*:nth-child(n+3){grid-column:2}.brut-ledger .brut-detail-grid{grid-template-columns:1fr}.brut-ledger .brut-detail-aside{border-left:0}.brut-ledger .ae-flow{grid-template-columns:1fr}.brut-ledger .brut-wire{padding:6px;transform:rotate(90deg);justify-self:center}.brut-ledger .brut-mast-meta{display:none}}
  </style>
  <aside class="brut-rail"><strong>BITTER<br>BLOSSOM®</strong><nav>${destinations("Workflows")}</nav>${mode()}</aside>
  <main class="brut-stage"><header class="brut-mast"><div><span class="ae-chrome">CONTROL LEDGER / CONFIGURED PLANE</span><h1>WHAT IS CONFIGURED</h1></div><div class="brut-mast-meta">${status("Plane active", "ok")}${status("Trigger listening", "ok")}</div></header><div class="brut-notice ae-chrome">${esc(corpus.notice)}</div>
    <section id="workflows" aria-label="Configured workflow roster">${workflowRows(corpus)}</section>
    <section class="brut-section"><header class="brut-heading"><span class="ae-chrome">PR REVIEW / STABLE CONFIGURATION</span><h2>Selected live overlay · ${esc(corpus.run.id)}</h2></header>${topology(corpus)}</section>
    <div class="brut-detail-grid"><div>${evidence(corpus)}<section id="agents">${agents(corpus)}</section></div><aside class="brut-detail-aside">${spend(corpus)}</aside></div>${creation(corpus)}
  </main>
</div>`;

const renderCircuit = (corpus) => `<div class="brut brut-circuit">
  <style>${commonCss}
    .brut-circuit{display:grid;grid-template-rows:auto minmax(0,1fr) auto}.brut-circuit .brut-topbar{display:grid;grid-template-columns:auto minmax(0,1fr) auto;border-bottom:1px solid var(--ae-line);align-items:center}.brut-circuit .brut-topbar>*{padding:12px 16px}.brut-circuit .brut-topbar nav{display:flex;justify-content:center;gap:22px;border-left:1px solid var(--ae-line);border-right:1px solid var(--ae-line)}.brut-circuit .brut-stage{display:grid;grid-template-columns:minmax(0,2fr) minmax(300px,1fr);grid-template-rows:auto auto}.brut-circuit .brut-map{min-height:60vh;border-right:1px solid var(--ae-line);padding:24px;display:grid;grid-template-rows:auto minmax(240px,1fr) auto}.brut-circuit .brut-map .ae-flow{display:grid;grid-template-columns:1fr auto 1fr auto 1fr auto 1fr;align-items:center}.brut-circuit .brut-run-children{display:grid;grid-template-columns:repeat(3,1fr);border-top:1px solid var(--ae-line)}.brut-circuit .brut-run-children>*{padding:12px;border-right:1px solid var(--ae-line)}.brut-circuit .brut-inspector{min-width:0}.brut-circuit .brut-register{display:grid;grid-template-columns:42px 1fr;gap:10px;padding:14px;border-bottom:1px solid var(--ae-line)}.brut-circuit .brut-register>*:nth-child(n+3){grid-column:2}.brut-circuit .brut-evidence{grid-column:1/-1;display:grid;grid-template-columns:1fr 1fr}.brut-circuit .brut-evidence>*:nth-child(odd){border-right:1px solid var(--ae-line)}.brut-circuit .brut-bottom{border-top:1px solid var(--ae-line);display:flex;align-items:center;justify-content:space-between;padding:8px 16px}.brut-circuit .brut-coordinate{font-family:var(--ae-font-mono);color:var(--ae-accent)}
    @media(max-width:800px){.brut-circuit .brut-topbar{grid-template-columns:1fr auto}.brut-circuit .brut-topbar nav{grid-column:1/-1;grid-row:2;overflow:auto;justify-content:flex-start;border:0;border-top:1px solid var(--ae-line)}.brut-circuit .brut-stage{grid-template-columns:1fr}.brut-circuit .brut-map{border-right:0;min-height:0}.brut-circuit .brut-map .ae-flow{grid-template-columns:1fr}.brut-circuit .brut-wire{transform:rotate(90deg);justify-self:center;padding:6px}.brut-circuit .brut-run-children{grid-template-columns:1fr}.brut-circuit .brut-evidence{grid-template-columns:1fr}.brut-circuit .brut-evidence>*:nth-child(odd){border-right:0}.brut-circuit .brut-bottom{display:none}}
  </style>
  <header class="brut-topbar"><h1>BB / CIRCUIT REGISTER™</h1><nav>${destinations("Runs")}</nav>${mode()}</header>
  <main class="brut-stage"><section class="brut-map"><header class="brut-heading"><div><span class="ae-chrome">PR REVIEW / CONFIGURED TOPOLOGY IS THE NAVIGATION</span><h2>${esc(corpus.run.id)}</h2></div>${status(corpus.run.state, "warn")}</header>${topology(corpus)}<div class="brut-run-children">${corpus.run.children.map((child, index) => `<article><span class="ae-chrome">CHILD / 0${index + 1}</span><br><strong>${esc(child.name)}</strong><br>${status(child.state, child.state === "Succeeded" ? "ok" : "warn")}<p class="ae-dim">${esc(child.result)}</p></article>`).join("")}</div></section><aside class="brut-inspector"><header class="brut-heading brut-section"><span class="ae-chrome">WORKFLOW COORDINATES</span><h2>02 configured</h2></header>${workflowRows(corpus, true)}</aside><div class="brut-evidence">${evidence(corpus)}<section id="agents">${agents(corpus)}</section>${spend(corpus)}${creation(corpus)}</div></main>
  <footer class="brut-bottom ae-chrome"><span>${esc(corpus.notice)}</span><span class="brut-coordinate">RUN / ${esc(corpus.run.state.toUpperCase())} / STEP ${esc(corpus.run.step.toUpperCase())}</span></footer>
</div>`;

const renderProcedure = (corpus) => `<div class="brut brut-procedure">
  <style>${commonCss}
    .brut-procedure{display:grid;grid-template-columns:minmax(320px,42%) minmax(0,58%)}.brut-procedure .brut-build{border-right:1px solid var(--ae-line);display:grid;grid-template-rows:auto minmax(0,1fr);min-height:0}.brut-procedure .brut-build-head,.brut-procedure .brut-proof-head{padding:16px 20px;border-bottom:1px solid var(--ae-line);display:flex;justify-content:space-between;align-items:center}.brut-procedure .brut-build .brut-stage{padding:0}.brut-procedure .brut-create{border-top:0}.brut-procedure .brut-create-grid{grid-template-columns:1fr}.brut-procedure .brut-create-lead .ae-panel{border:1px solid var(--ae-accent)}.brut-procedure .brut-proof{display:grid;grid-template-rows:auto auto minmax(0,1fr);min-width:0;min-height:0}.brut-procedure .brut-proof-head nav{display:flex;gap:18px;overflow:auto;white-space:nowrap}.brut-procedure .brut-proof-register{display:grid;grid-template-columns:1fr 1fr;border-bottom:1px solid var(--ae-line)}.brut-procedure .brut-register{display:grid;grid-template-columns:40px 1fr;gap:10px;padding:12px;border-right:1px solid var(--ae-line)}.brut-procedure .brut-register>*:nth-child(n+3){grid-column:2}.brut-procedure .brut-proof .brut-stage>.brut-section:first-child{border-top:0}.brut-procedure .ae-flow{display:grid;grid-template-columns:1fr auto 1fr auto 1fr auto 1fr;align-items:center}.brut-procedure .brut-inversion{padding:8px 20px;border-bottom:1px solid var(--ae-line);color:var(--ae-accent)}
    @media(max-width:800px){.brut-procedure{grid-template-columns:minmax(0,1fr);grid-template-rows:minmax(0,1fr) auto}.brut-procedure .brut-build{border-right:0;min-width:0;width:100%}.brut-procedure .brut-build-head{min-width:0;gap:8px}.brut-procedure .brut-build-head>div{min-width:0;overflow-wrap:anywhere}.brut-procedure .brut-proof{display:contents}.brut-procedure .brut-proof-head{grid-row:2;position:sticky;bottom:0;background:var(--ae-surface);border-top:1px solid var(--ae-line);z-index:3;padding:10px}.brut-procedure .brut-proof-head>strong,.brut-procedure .brut-proof-head>.ae-button{display:none}.brut-procedure .brut-proof-register,.brut-procedure .brut-proof>.brut-stage,.brut-procedure .brut-inversion{grid-column:1;grid-row:1}.brut-procedure .brut-proof-register{display:none}.brut-procedure .brut-proof>.brut-stage{overflow:visible}.brut-procedure .brut-build>.brut-stage{overflow:auto;min-width:0}.brut-procedure .ae-flow{grid-template-columns:1fr}.brut-procedure .brut-wire{transform:rotate(90deg);justify-self:center;padding:6px}.brut-procedure .brut-inversion{position:sticky;top:0;background:var(--ae-surface);z-index:2}.brut-procedure .brut-build-head{padding:12px}}
  </style>
  <section class="brut-build"><header class="brut-build-head"><div><span class="ae-chrome">PRIMARY DESK / PROCEDURE 01</span><h1>COMMISSION THE GOAL</h1></div>${status("Draft", "neutral")}</header><main class="brut-stage">${creation(corpus, true)}</main></section>
  <section class="brut-proof"><header class="brut-proof-head"><strong>BITTERBLOSSOM®</strong><nav>${destinations("Create workflow")}</nav>${mode()}</header><div class="brut-inversion ae-chrome">INVERSION / CREATION IS THE HOME; CONFIGURATION IS THE PROOF LEDGER · ${esc(corpus.notice)}</div><div class="brut-proof-register">${workflowRows(corpus, true)}</div><main class="brut-stage"><section id="workflows" class="brut-section"><header class="brut-heading"><span class="ae-chrome">PR REVIEW / STABLE CONFIGURATION + SELECTED RUN</span><h2>${esc(corpus.run.id)}</h2></header>${topology(corpus)}</section>${evidence(corpus)}<section id="agents">${agents(corpus)}</section>${spend(corpus)}</main></section>
</div>`;

export const SPECS = {
  "BRUT-1": {
    label: "Control Ledger",
    move: "A fixed destination rail fronts a ruled workflow ledger; configuration stays above a split evidence-and-spend desk.",
    philosophy: "Swiss industrial print translated through Aesthetic law: uncompromising compartments, index marks, dense registers, and one ultramarine selection line in both light and dark.",
    render: renderLedger,
  },
  "BRUT-2": {
    label: "Circuit Register",
    move: "The workflow topology becomes the primary navigation surface, with roster coordinates at right and evidence quadrants below.",
    philosophy: "A technical schematic without terminal cosplay: orthogonal structure, exposed run branches, and status-bearing glyphs use only Aesthetic ink, line, and accent tokens.",
    render: renderCircuit,
  },
  "BRUT-3": {
    label: "Procedure Before Inventory",
    move: "Inverts the load-bearing assumption that inventory is home: goal-first creation owns the primary desk while configuration acts as its proof ledger.",
    philosophy: "An industrial procedure folio treats commissioning as the operational center and forces every proposed goal to face topology, authority, evidence, spend truth, and a fixture gate.",
    render: renderProcedure,
  },
};
