const e = (value) => String(value ?? "").replace(/[&<>\"]/g, (character) => ({
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;"
})[character]);

const tone = (value) => {
  const key = String(value).toLowerCase();
  if (key.includes("active") || key.includes("listening") || key.includes("achieved") || key.includes("available")) return "good";
  if (key.includes("executing") || key.includes("in use")) return "live";
  if (key.includes("blocked") || key.includes("unavailable") || key.includes("unknown")) return "warn";
  if (key.includes("superseded") || key.includes("draft") || key.includes("quiet") || key === "—") return "quiet";
  return "plain";
};

const badge = (value, label = value) => `<span class="badge ${tone(value)}">${e(label)}</span>`;
const costBadge = (value) => badge(value, value === "Unavailable" ? "Unknown cost" : value);

const topology = (workflow, run, className = "topology") => `<ol class="${className}" aria-label="${e(workflow.name)} topology">
  ${workflow.topology.map((step, index) => {
    const state = step === run.step ? "current" : index < workflow.topology.indexOf(run.step) ? "complete" : "waiting";
    return `<li class="${state}"><span class="node-index">${String(index + 1).padStart(2, "0")}</span><strong>${e(step)}</strong><small>${state === "current" ? e(run.state) : state === "complete" ? "Succeeded" : "Queued"}</small></li>`;
  }).join("")}
</ol>`;

const evidenceRows = (corpus) => corpus.recentRuns.map((run) => `<tr>
  <th scope="row"><strong>${e(run.workflow)}</strong><small>${e(run.ref)}</small></th>
  <td>${badge(run.execution)}</td><td>${badge(run.domain)}</td><td>${badge(run.verification)}</td><td>${costBadge(run.cost)}</td>
</tr>`).join("");

const agentRows = (corpus) => corpus.agents.map((agent) => `<tr>
  <th scope="row"><strong>${e(agent.name)}</strong><small>${e(agent.model)} · ${e(agent.harness)}</small></th>
  <td>${badge(agent.availability)}</td><td>${e(agent.ceiling)}</td><td>${e(agent.workflows)}</td><td>${agent.active ? badge("Executing", `${agent.active} active`) : badge("quiet", "No active run")}</td>
</tr>`).join("");

const spendTruth = (corpus) => corpus.spend.truth.map((item) => `<article class="truth-item ${tone(item.label)}">
  <span>${e(item.label)}</span><strong>${e(item.value)}</strong><small>${e(item.state)}</small>
</article>`).join("");

const createFlow = (corpus, prefix) => `<form class="create-flow" onsubmit="return false">
  <ol class="step-index" aria-label="Create workflow steps">${corpus.creation.steps.map((step, index) => `<li class="${index === 0 ? "active" : ""}"><span>${String(index + 1).padStart(2, "0")}</span>${e(step)}</li>`).join("")}</ol>
  <div class="goal-editor">
    <label for="${prefix}-goal">Workflow goal</label>
    <textarea id="${prefix}-goal" rows="3">${e(corpus.creation.rawGoal)}</textarea>
    <details open>
      <summary>Enhanced goal review <span>Operator approval required</span></summary>
      <p>${e(corpus.creation.enhancedGoal)}</p>
      <label class="check"><input type="checkbox"> I reviewed the authority boundary and external effect.</label>
    </details>
  </div>
  <fieldset class="activation-test">
    <legend>Activation test</legend>
    <p>${e(corpus.creation.test)}</p>
    <label><input type="radio" name="${prefix}-fixture" checked> Use fixture PR</label>
    <label><input type="radio" name="${prefix}-fixture"> Choose another fixture</label>
    <button type="button" onclick="this.textContent='Test ready';this.nextElementSibling.hidden=false">Run activation test</button>
    <small hidden aria-live="polite">Fixture accepted. Activation remains gated on goal and authority review.</small>
  </fieldset>
</form>`;

const baseStyles = (root) => `
  ${root}{--bg:#f7f6f3;--paper:#fff;--ink:#20211f;--muted:#6f706c;--line:#deded9;--line-strong:#bdbdb6;--blue:#dcecf5;--blue-ink:#255d78;--green:#e5eee3;--green-ink:#315d35;--red:#f5e4e3;--red-ink:#8b3935;--yellow:#f5ecd2;--yellow-ink:#775b12;--lav:#ebe7f1;--sans:"SF Pro Display","Helvetica Neue",Helvetica,sans-serif;--serif:"Iowan Old Style","Palatino Linotype",Palatino,serif;--mono:"SF Mono",SFMono-Regular,Consolas,monospace;color:var(--ink);background:var(--bg);font:14px/1.45 var(--sans);height:100%;min-height:100vh;overflow:auto;color-scheme:light}
  html[data-theme="dark"] ${root}{--bg:#1d1e1c;--paper:#252624;--ink:#f1f0eb;--muted:#aaa9a2;--line:#3d3e3a;--line-strong:#5c5d57;--blue:#253944;--blue-ink:#b7ddee;--green:#29382a;--green-ink:#c3ddc0;--red:#452d2c;--red-ink:#efbfbb;--yellow:#403820;--yellow-ink:#ead899;--lav:#38333f;color-scheme:dark}
  @media (prefers-color-scheme:dark){html:not([data-theme="light"]) ${root}{--bg:#1d1e1c;--paper:#252624;--ink:#f1f0eb;--muted:#aaa9a2;--line:#3d3e3a;--line-strong:#5c5d57;--blue:#253944;--blue-ink:#b7ddee;--green:#29382a;--green-ink:#c3ddc0;--red:#452d2c;--red-ink:#efbfbb;--yellow:#403820;--yellow-ink:#ead899;--lav:#38333f;color-scheme:dark}}
  ${root} *{box-sizing:border-box}
  ${root} a{color:inherit}
  ${root} button,${root} input,${root} textarea{font:inherit}
  ${root} button,${root} a,${root} summary,${root} input,${root} textarea{outline-offset:3px}
  ${root} button:focus-visible,${root} a:focus-visible,${root} summary:focus-visible,${root} input:focus-visible,${root} textarea:focus-visible{outline:2px solid var(--blue-ink)}
  ${root} h1,${root} h2,${root} h3,${root} p{margin-top:0}
  ${root} h1,${root} h2{font-family:var(--serif);font-weight:500;letter-spacing:-.035em;line-height:1.04}
  ${root} h2{font-size:clamp(28px,3vw,46px)}
  ${root} small{color:var(--muted)}
  ${root} .eyebrow{font:600 10px/1.2 var(--mono);letter-spacing:.11em;text-transform:uppercase;color:var(--muted)}
  ${root} .badge{display:inline-flex;width:max-content;border-radius:999px;padding:4px 8px;background:var(--lav);font:600 10px/1 var(--mono);letter-spacing:.045em;text-transform:uppercase;color:var(--ink);white-space:nowrap}
  ${root} .badge.good{background:var(--green);color:var(--green-ink)}
  ${root} .badge.live{background:var(--blue);color:var(--blue-ink)}
  ${root} .badge.warn{background:var(--red);color:var(--red-ink)}
  ${root} .badge.quiet{background:transparent;border:1px solid var(--line);color:var(--muted)}
  ${root} table{border-collapse:collapse;width:100%;font-size:12px}
  ${root} th,${root} td{text-align:left;vertical-align:top;border-bottom:1px solid var(--line);padding:13px 10px}
  ${root} thead th{color:var(--muted);font:600 9px/1.2 var(--mono);letter-spacing:.08em;text-transform:uppercase}
  ${root} tbody th strong,${root} tbody th small{display:block}
  ${root} tbody th small{font-weight:400;margin-top:4px}
  ${root} .create-flow{display:grid;grid-template-columns:minmax(130px,.55fr) minmax(280px,1.8fr) minmax(220px,1fr);gap:0;border:1px solid var(--line);background:var(--paper)}
  ${root} .step-index{list-style:none;margin:0;padding:16px;border-right:1px solid var(--line)}
  ${root} .step-index li{display:grid;grid-template-columns:26px 1fr;gap:7px;align-items:center;padding:8px 4px;color:var(--muted);font-size:12px}
  ${root} .step-index li span{font:10px var(--mono)}
  ${root} .step-index li.active{color:var(--ink);font-weight:650}
  ${root} .goal-editor,${root} .activation-test{padding:22px}
  ${root} .goal-editor label,${root} legend{font:650 11px var(--mono);letter-spacing:.04em;text-transform:uppercase}
  ${root} textarea{display:block;width:100%;resize:vertical;margin:10px 0 16px;border:1px solid var(--line);border-radius:4px;padding:12px;background:var(--bg);color:var(--ink);line-height:1.5}
  ${root} details{border-top:1px solid var(--line);border-bottom:1px solid var(--line);padding:11px 0}
  ${root} summary{cursor:pointer;font-weight:650}
  ${root} summary span{float:right;color:var(--muted);font:10px var(--mono)}
  ${root} details p{margin:13px 0;color:var(--muted);font-family:var(--serif);font-size:16px;line-height:1.55}
  ${root} .check{display:flex;gap:8px;font:12px/1.4 var(--sans)!important;text-transform:none!important;letter-spacing:0!important}
  ${root} .activation-test{border:0;border-left:1px solid var(--line);margin:0}
  ${root} .activation-test label{display:block;margin:10px 0;font-size:12px}
  ${root} button{border:1px solid var(--ink);border-radius:5px;background:var(--ink);color:var(--paper);padding:9px 12px;cursor:pointer;transition:transform .15s ease,opacity .15s ease}
  ${root} button:active{transform:scale(.98)}
  ${root} [data-theme-toggle]{background:transparent;color:var(--ink);border-color:var(--line-strong)}
  ${root} section{scroll-margin-top:70px}
  @media (prefers-reduced-motion:reduce){${root} *{scroll-behavior:auto!important;transition:none!important}}
  @media (max-width:850px){${root} .create-flow{grid-template-columns:1fr}${root} .step-index{border-right:0;border-bottom:1px solid var(--line);display:flex;overflow:auto}${root} .step-index li{min-width:max-content}${root} .activation-test{border-left:0;border-top:1px solid var(--line)}}
`;

function renderMin1(corpus) {
  const selected = corpus.workflows[0];
  return `<div class="min1">
    <style>${baseStyles(".min1")}
      .min1{display:grid;grid-template-rows:54px 1fr}
      .min1 .mast{position:sticky;top:0;z-index:5;display:grid;grid-template-columns:230px 1fr auto;align-items:center;border-bottom:1px solid var(--line);background:var(--bg);padding:0 18px}
      .min1 .brand{font-family:var(--serif);font-size:19px;letter-spacing:-.03em}
      .min1 .primary{display:flex;gap:2px;justify-content:center}
      .min1 .primary a{padding:8px 11px;text-decoration:none;border-radius:4px;font-size:12px}.min1 .primary a:hover{background:var(--paper)}
      .min1 .mast-actions{display:flex;gap:8px}.min1 .mast-actions a{background:var(--ink);color:var(--paper);border-radius:5px;padding:9px 12px;text-decoration:none;font-weight:650;font-size:12px}
      .min1 .body{display:grid;grid-template-columns:minmax(260px,29vw) 1fr;min-height:0}
      .min1 .roster{border-right:1px solid var(--line);background:var(--paper);padding:28px 20px;min-width:0}
      .min1 .roster header{display:flex;align-items:end;justify-content:space-between;margin-bottom:18px}.min1 .roster h1{font-size:31px;margin:4px 0 0}.min1 .roster header small{font:10px var(--mono)}
      .min1 .workflow-card{display:block;border:1px solid var(--line);border-radius:8px;background:var(--paper);padding:16px;margin-bottom:10px;text-decoration:none;transition:transform .15s ease,background .15s ease}.min1 .workflow-card:hover{transform:translateY(-1px);background:var(--bg)}
      .min1 .workflow-card.selected{border-color:var(--line-strong)}.min1 .workflow-card .row{display:flex;justify-content:space-between;gap:8px}.min1 .workflow-card h3{font-family:var(--serif);font-size:19px;margin:10px 0 4px}.min1 .workflow-card p{font-size:12px;color:var(--muted);margin-bottom:14px}.min1 .workflow-card footer{display:flex;justify-content:space-between;border-top:1px solid var(--line);padding-top:10px;font:10px var(--mono);color:var(--muted)}
      .min1 .dossier{min-width:0;padding:36px clamp(22px,4vw,64px) 80px}.min1 .dossier>section{max-width:1180px;margin:0 auto 72px}
      .min1 .detail-head{display:grid;grid-template-columns:1.25fr .75fr;gap:32px;align-items:end;margin-bottom:28px}.min1 .detail-head h2{margin:5px 0 8px}.min1 .detail-head p{max-width:590px;color:var(--muted)}
      .min1 .facts{display:grid;grid-template-columns:repeat(2,1fr);border-top:1px solid var(--line)}.min1 .facts div{padding:10px 0;border-bottom:1px solid var(--line)}.min1 .facts span,.min1 .facts strong{display:block}.min1 .facts span{font:9px var(--mono);text-transform:uppercase;color:var(--muted);letter-spacing:.06em}.min1 .facts strong{margin-top:3px;font-size:12px}
      .min1 .topology{list-style:none;margin:0;padding:0;display:grid;grid-template-columns:repeat(4,1fr);border:1px solid var(--line);border-radius:8px;overflow:hidden;background:var(--paper)}.min1 .topology li{position:relative;padding:18px;min-height:102px;border-right:1px solid var(--line)}.min1 .topology li:last-child{border:0}.min1 .topology li.current{background:var(--blue)}.min1 .topology li.complete:after{content:"";position:absolute;left:18px;right:18px;bottom:12px;height:2px;background:var(--green-ink)}.min1 .topology strong,.min1 .topology small{display:block}.min1 .topology strong{margin-top:17px}.min1 .node-index{font:10px var(--mono);color:var(--muted)}
      .min1 .run-overlay{display:grid;grid-template-columns:1fr 2fr;margin-top:12px;border:1px solid var(--line);border-radius:8px;background:var(--paper)}.min1 .run-meta{padding:18px;border-right:1px solid var(--line)}.min1 .run-meta h3{font-family:var(--serif);font-size:20px;margin:7px 0}.min1 .run-meta dl{display:grid;grid-template-columns:1fr 1fr;gap:10px;margin:15px 0 0}.min1 dt{font:9px var(--mono);color:var(--muted);text-transform:uppercase}.min1 dd{margin:2px 0 0;font-size:12px}.min1 .children{padding:8px 18px}.min1 .child{display:grid;grid-template-columns:1.2fr auto 1fr;gap:14px;padding:11px 0;border-bottom:1px solid var(--line);font-size:12px}.min1 .child:last-child{border:0}
      .min1 .section-head{display:flex;justify-content:space-between;align-items:end;margin-bottom:15px}.min1 .section-head h2{font-size:32px;margin:3px 0 0}.min1 .table-shell{border:1px solid var(--line);border-radius:8px;background:var(--paper);overflow:auto}
      .min1 .spend-grid{display:grid;grid-template-columns:1.2fr 2fr;border:1px solid var(--line);border-radius:8px;background:var(--paper)}.min1 .scope-list{padding:20px;border-right:1px solid var(--line)}.min1 .scope-list h3{font-family:var(--serif);font-size:20px}.min1 .scope-list ol{padding:0;list-style:none}.min1 .scope-list li{border-bottom:1px solid var(--line);padding:10px 0;font-size:12px}.min1 .scope-list li:before{content:counter(list-item,decimal-leading-zero);font:9px var(--mono);color:var(--muted);margin-right:10px}.min1 .truth{display:grid;grid-template-columns:repeat(3,1fr)}.min1 .truth-item{padding:20px;border-right:1px solid var(--line)}.min1 .truth-item:last-child{border:0}.min1 .truth-item span,.min1 .truth-item strong,.min1 .truth-item small{display:block}.min1 .truth-item span{font:10px var(--mono);text-transform:uppercase}.min1 .truth-item strong{font-family:var(--serif);font-size:18px;margin:25px 0 6px}
      .min1 .fixture{position:fixed;right:16px;bottom:14px;z-index:4;background:var(--yellow);color:var(--yellow-ink);border:1px solid var(--line);border-radius:4px;padding:6px 9px;font:9px var(--mono)}
      @media(max-width:900px){.min1 .mast{grid-template-columns:1fr auto}.min1 .primary{display:none}.min1 .body{grid-template-columns:1fr}.min1 .roster{border-right:0;border-bottom:1px solid var(--line)}.min1 .detail-head,.min1 .run-overlay,.min1 .spend-grid{grid-template-columns:1fr}.min1 .run-meta,.min1 .scope-list{border-right:0;border-bottom:1px solid var(--line)}}
    </style>
    <header class="mast"><div class="brand">Bitterblossom</div><nav class="primary" aria-label="Primary">${[["Workflows","workflows"],["Agents","agents"],["Runs","runs"],["Spend","spend"]].map(([l,id])=>`<a href="#min1-${id}">${l}</a>`).join("")}</nav><div class="mast-actions"><button type="button" data-theme-toggle>Theme</button><a href="#min1-create">Create workflow</a></div></header>
    <div class="body">
      <aside class="roster" id="min1-workflows"><header><div><span class="eyebrow">Configured roster</span><h1>Workflows</h1></div><small>${corpus.workflows.length} total</small></header>
        ${corpus.workflows.map((workflow,index)=>`<a class="workflow-card ${index===0?"selected":""}" href="#min1-detail"><div class="row">${badge(workflow.lifecycle)}${workflow.active?badge("Executing",`${workflow.active} active`):badge("quiet","No active run")}</div><h3>${e(workflow.name)}</h3><p>${e(workflow.trigger)}</p><footer><span>${e(workflow.latestExecution)}</span><span>${e(workflow.latestDomain)}</span><span>${e(workflow.verification)}</span></footer></a>`).join("")}
      </aside>
      <main class="dossier">
        <section id="min1-detail"><div class="detail-head"><div><span class="eyebrow">Selected workflow · revision 07</span><h2>${e(selected.name)}</h2><p>${e(selected.trigger)}. The configured graph stays fixed; the selected run is shown as a separate evidence overlay.</p></div><div class="facts"><div><span>Lifecycle</span><strong>${e(selected.lifecycle)}</strong></div><div><span>Trigger health</span><strong>Listening</strong></div><div><span>Domain result</span><strong>${e(selected.latestDomain)}</strong></div><div><span>Verification</span><strong>${e(selected.verification)}</strong></div></div></div>
          ${topology(selected,corpus.run)}
          <div class="run-overlay"><div class="run-meta"><span class="eyebrow">Selected live overlay</span><h3>${e(corpus.run.id)}</h3>${badge(corpus.run.state)} ${badge("Listening","Trigger listening")}<dl><div><dt>Elapsed</dt><dd>${e(corpus.run.elapsed)}</dd></div><div><dt>Cost</dt><dd>${e(corpus.run.cost)}</dd></div><div><dt>Trigger</dt><dd>${e(corpus.run.trigger)}</dd></div><div><dt>Budget</dt><dd>${e(corpus.run.budget)}</dd></div></dl></div><div class="children">${corpus.run.children.map(child=>`<div class="child"><strong>${e(child.name)}</strong>${badge(child.state)}<span>${e(child.result)}</span></div>`).join("")}</div></div>
        </section>
        <section id="min1-agents"><div class="section-head"><div><span class="eyebrow">Roster definitions</span><h2>Agents</h2></div><span>${badge("In use")} ${badge("Available")}</span></div><div class="table-shell"><table><thead><tr><th>Agent</th><th>Availability</th><th>Authority ceiling</th><th>Workflows</th><th>Now</th></tr></thead><tbody>${agentRows(corpus)}</tbody></table></div></section>
        <section id="min1-runs"><div class="section-head"><div><span class="eyebrow">Live and historical evidence</span><h2>Runs</h2></div><small>Execution, result, and proof stay separate</small></div><div class="table-shell"><table><thead><tr><th>Workflow / reference</th><th>Execution</th><th>Domain result</th><th>Verification</th><th>Cost</th></tr></thead><tbody>${evidenceRows(corpus)}</tbody></table></div></section>
        <section id="min1-spend"><div class="section-head"><div><span class="eyebrow">Controls and coverage</span><h2>Spend</h2></div>${badge("Unavailable","Unknown cost is not $0")}</div><div class="spend-grid"><div class="scope-list"><h3>Policy scopes</h3><ol>${corpus.spend.scopes.map(scope=>`<li>${e(scope)}</li>`).join("")}</ol></div><div class="truth">${spendTruth(corpus)}</div></div></section>
        <section id="min1-create"><div class="section-head"><div><span class="eyebrow">Goal first · review before authority</span><h2>Create workflow</h2></div><small>Draft is saved before activation</small></div>${createFlow(corpus,"min1")}</section>
      </main>
    </div><div class="fixture">${e(corpus.notice)}</div>
  </div>`;
}

function renderMin2(corpus) {
  const selected = corpus.workflows[0];
  return `<div class="min2">
    <style>${baseStyles(".min2")}
      .min2{padding-bottom:58px}.min2 .shell{max-width:1600px;margin:auto;padding:24px clamp(16px,3vw,46px) 80px}
      .min2 .cap{display:flex;align-items:center;justify-content:space-between;padding-bottom:18px;border-bottom:1px solid var(--line)}.min2 .wordmark{font-family:var(--serif);font-size:22px}.min2 .cap-right{display:flex;gap:8px;align-items:center}.min2 .notice{font:9px var(--mono);color:var(--muted)}
      .min2 .workflow-band{padding:38px 0 26px}.min2 .band-head{display:grid;grid-template-columns:1fr auto;align-items:end;margin-bottom:16px}.min2 .band-head h1{font-size:clamp(36px,5vw,66px);margin:4px 0}.min2 .band-head p{color:var(--muted);max-width:540px;margin:0}
      .min2 .workflow-row{display:grid;grid-template-columns:repeat(2,minmax(300px,1fr));gap:12px}.min2 .workflow{border:1px solid var(--line);border-radius:12px;background:var(--paper);padding:22px;display:grid;grid-template-columns:1.3fr .7fr;gap:20px;text-decoration:none}.min2 .workflow.selected{border-color:var(--ink)}.min2 .workflow h2{font-size:27px;margin:14px 0 6px}.min2 .workflow p{color:var(--muted);font-size:12px}.min2 .workflow-side{display:flex;flex-direction:column;justify-content:space-between;border-left:1px solid var(--line);padding-left:18px}.min2 .workflow-side dl{margin:0}.min2 .workflow-side dt{font:9px var(--mono);color:var(--muted);text-transform:uppercase}.min2 .workflow-side dd{margin:2px 0 12px;font-size:12px}
      .min2 .graph-stage{border:1px solid var(--line);border-radius:12px;background:var(--paper);overflow:hidden;margin-bottom:72px}.min2 .stage-cap{display:grid;grid-template-columns:1fr auto auto;gap:12px;align-items:center;padding:14px 18px;border-bottom:1px solid var(--line)}.min2 .stage-cap strong{font-family:var(--serif);font-size:18px}.min2 .stage-cap small{font:10px var(--mono)}
      .min2 .stage-body{display:grid;grid-template-columns:1fr 310px;min-height:330px}.min2 .diagram{padding:40px 32px;display:flex;align-items:center}.min2 .orbit{list-style:none;padding:0;margin:0;width:100%;display:flex;align-items:stretch}.min2 .orbit li{position:relative;flex:1;min-width:0;border-top:1px solid var(--line-strong);padding:23px 10px 0}.min2 .orbit li:before{content:"";position:absolute;top:-6px;left:10px;width:11px;height:11px;border-radius:50%;background:var(--paper);border:2px solid var(--line-strong)}.min2 .orbit li.current:before{background:var(--blue-ink);border-color:var(--blue-ink)}.min2 .orbit li.complete:before{background:var(--green-ink);border-color:var(--green-ink)}.min2 .orbit strong,.min2 .orbit small,.min2 .orbit .node-index{display:block}.min2 .orbit strong{font-family:var(--serif);font-size:17px}.min2 .orbit small{margin-top:4px}.min2 .node-index{font:9px var(--mono);color:var(--muted);margin-bottom:35px}
      .min2 .evidence-drawer{border-left:1px solid var(--line);background:var(--bg);padding:20px}.min2 .evidence-drawer h3{font-family:var(--serif);font-size:22px;margin:7px 0 4px}.min2 .evidence-drawer dl{display:grid;grid-template-columns:1fr 1fr;gap:12px 8px;border-top:1px solid var(--line);padding-top:15px}.min2 .evidence-drawer dt{font:9px var(--mono);color:var(--muted);text-transform:uppercase}.min2 .evidence-drawer dd{margin:2px 0;font-size:12px}.min2 .run-child{padding:9px 0;border-bottom:1px solid var(--line);font-size:11px}.min2 .run-child strong,.min2 .run-child span{display:block}.min2 .run-child span{color:var(--muted)}
      .min2 .folio{display:grid;grid-template-columns:220px 1fr;gap:30px;padding:50px 0;border-top:1px solid var(--line)}.min2 .folio-title{position:sticky;top:20px;align-self:start}.min2 .folio-title h2{font-size:35px;margin:5px 0}.min2 .folio-title p{color:var(--muted);font-size:12px}.min2 .folio-content{min-width:0}.min2 .table-shell{border-top:1px solid var(--line-strong);overflow:auto}
      .min2 .agent-cards{display:grid;grid-template-columns:repeat(3,1fr);gap:10px}.min2 .agent{border:1px solid var(--line);border-radius:8px;background:var(--paper);padding:18px}.min2 .agent h3{font-family:var(--serif);font-size:21px;margin:18px 0 3px}.min2 .agent dl{margin:18px 0 0}.min2 .agent dt{font:9px var(--mono);color:var(--muted);text-transform:uppercase}.min2 .agent dd{margin:2px 0 11px;font-size:11px}
      .min2 .spend-strip{display:grid;grid-template-columns:repeat(3,1fr);border:1px solid var(--line);border-radius:8px;overflow:hidden;background:var(--paper)}.min2 .truth-item{padding:24px;border-right:1px solid var(--line)}.min2 .truth-item:last-child{border:0}.min2 .truth-item span,.min2 .truth-item strong,.min2 .truth-item small{display:block}.min2 .truth-item span{font:9px var(--mono);text-transform:uppercase}.min2 .truth-item strong{font-family:var(--serif);font-size:20px;margin:24px 0 6px}.min2 .scopes{display:flex;flex-wrap:wrap;gap:6px;margin-top:12px}.min2 .scopes span{border:1px solid var(--line);border-radius:4px;padding:6px 8px;font:10px var(--mono)}
      .min2 .dock{position:fixed;z-index:8;left:18px;right:18px;bottom:12px;height:46px;display:grid;grid-template-columns:auto 1fr auto;align-items:center;border:1px solid var(--line-strong);border-radius:8px;background:var(--paper);padding:5px 7px}.min2 .dock-brand{font:650 11px var(--mono);padding:0 9px}.min2 .dock nav{display:flex;justify-content:center;gap:2px}.min2 .dock a{padding:7px 11px;text-decoration:none;border-radius:4px;font-size:11px}.min2 .dock a:hover{background:var(--bg)}.min2 .dock .create{background:var(--ink);color:var(--paper);font-weight:650}
      @media(max-width:900px){.min2 .workflow-row,.min2 .stage-body,.min2 .folio{grid-template-columns:1fr}.min2 .evidence-drawer{border-left:0;border-top:1px solid var(--line)}.min2 .folio-title{position:static}.min2 .agent-cards{grid-template-columns:1fr}.min2 .dock{left:6px;right:6px}.min2 .dock nav a{padding:7px 5px}.min2 .dock-brand{display:none}}
    </style>
    <div class="shell">
      <header class="cap"><div class="wordmark">Bitterblossom <span class="eyebrow">Operator plane</span></div><div class="cap-right"><span class="notice">${e(corpus.notice)}</span><button type="button" data-theme-toggle>Theme</button></div></header>
      <main>
        <section class="workflow-band" id="min2-workflows"><div class="band-head"><div><span class="eyebrow">Landing · configured system</span><h1>Workflow folio</h1></div><p>Choose a workflow, then inspect its immutable configured graph with one run overlaid. Operational states are labeled by kind, not compressed into health.</p></div><div class="workflow-row">${corpus.workflows.map((workflow,index)=>`<a class="workflow ${index===0?"selected":""}" href="#min2-graph"><div><div>${badge(workflow.lifecycle)} ${badge(workflow.trigger.includes("Listening")?"Listening":"quiet",workflow.trigger.includes("Listening")?"Trigger listening":"Trigger configured")}</div><h2>${e(workflow.name)}</h2><p>${e(workflow.trigger)}</p><small>${e(workflow.topology.join("  /  "))}</small></div><div class="workflow-side"><dl><dt>Live instance</dt><dd>${workflow.active?`${workflow.active} executing`:"No active run"}</dd><dt>Latest execution</dt><dd>${e(workflow.latestExecution)}</dd><dt>Domain result</dt><dd>${e(workflow.latestDomain)}</dd></dl><span>${badge(workflow.verification)}</span></div></a>`).join("")}</div></section>
        <section class="graph-stage" id="min2-graph"><header class="stage-cap"><strong>${e(selected.name)} · configured topology</strong><small>Graph revision 07</small>${badge("Listening","Trigger listening")}</header><div class="stage-body"><div class="diagram">${topology(selected,corpus.run,"orbit")}</div><aside class="evidence-drawer"><span class="eyebrow">Run overlay</span><h3>${e(corpus.run.id)}</h3><p>${badge(corpus.run.state)} ${costBadge(corpus.run.cost)}</p><dl><div><dt>Current step</dt><dd>${e(corpus.run.step)}</dd></div><div><dt>Elapsed</dt><dd>${e(corpus.run.elapsed)}</dd></div><div><dt>Parent</dt><dd>${e(corpus.run.parent)}</dd></div><div><dt>Budget</dt><dd>${e(corpus.run.budget)}</dd></div></dl>${corpus.run.children.map(child=>`<div class="run-child"><strong>${e(child.name)} · ${e(child.state)}</strong><span>${e(child.result)}</span></div>`).join("")}</aside></div></section>
        <section class="folio" id="min2-agents"><header class="folio-title"><span class="eyebrow">Definitions</span><h2>Agent roster</h2><p>Availability says whether an agent may be commissioned. Active count says whether it is working now.</p></header><div class="folio-content agent-cards">${corpus.agents.map(agent=>`<article class="agent"><div>${badge(agent.availability)} ${agent.active?badge("Executing",`${agent.active} active`):badge("quiet","No active run")}</div><h3>${e(agent.name)}</h3><small>${e(agent.model)} · ${e(agent.harness)}</small><dl><dt>Authority ceiling</dt><dd>${e(agent.ceiling)}</dd><dt>Configured workflows</dt><dd>${e(agent.workflows)}</dd></dl></article>`).join("")}</div></section>
        <section class="folio" id="min2-runs"><header class="folio-title"><span class="eyebrow">Evidence register</span><h2>Runs</h2><p>Lifecycle, domain outcome, verification, and spend keep independent columns.</p></header><div class="folio-content table-shell"><table><thead><tr><th>Workflow / reference</th><th>Execution</th><th>Domain result</th><th>Verification</th><th>Cost</th></tr></thead><tbody>${evidenceRows(corpus)}</tbody></table></div></section>
        <section class="folio" id="min2-spend"><header class="folio-title"><span class="eyebrow">Guardrails</span><h2>Spend</h2><p>Unknown coverage remains visible and never resolves to zero.</p><div class="scopes">${corpus.spend.scopes.map(scope=>`<span>${e(scope)}</span>`).join("")}</div></header><div class="folio-content spend-strip">${spendTruth(corpus)}</div></section>
        <section class="folio" id="min2-create"><header class="folio-title"><span class="eyebrow">Commission by intent</span><h2>Create workflow</h2><p>The goal is enhanced for review before trigger, authority, limits, and activation test are accepted.</p></header><div class="folio-content">${createFlow(corpus,"min2")}</div></section>
      </main>
    </div>
    <footer class="dock"><span class="dock-brand">BB / 03</span><nav aria-label="Primary"><a href="#min2-workflows">Workflows</a><a href="#min2-agents">Agents</a><a href="#min2-runs">Runs</a><a href="#min2-spend">Spend</a></nav><a class="create" href="#min2-create">Create workflow</a></footer>
  </div>`;
}

function renderMin3(corpus) {
  const selected = corpus.workflows[0];
  return `<div class="min3">
    <style>${baseStyles(".min3")}
      .min3{display:grid;grid-template-columns:184px 1fr;background:var(--paper)}
      .min3 .rail{position:sticky;top:0;height:100vh;display:flex;flex-direction:column;border-right:1px solid var(--line);background:var(--bg);padding:18px 14px}.min3 .rail .brand{font-family:var(--serif);font-size:19px;padding:2px 8px 27px}.min3 .rail nav{display:grid;gap:3px}.min3 .rail a{padding:9px 8px;border-radius:4px;text-decoration:none;font-size:12px}.min3 .rail a:first-child,.min3 .rail a:hover{background:var(--paper)}.min3 .rail .rail-bottom{margin-top:auto;display:grid;gap:8px}.min3 .rail .create-link{background:var(--ink);color:var(--paper);font-weight:650}.min3 .rail .fixture-note{font:9px/1.4 var(--mono);color:var(--muted);padding:5px 8px}
      .min3 .ledger{min-width:0}.min3 .page-head{display:grid;grid-template-columns:1fr auto;align-items:end;padding:34px clamp(18px,3vw,44px) 24px;border-bottom:1px solid var(--line)}.min3 .page-head h1{font-size:clamp(38px,5vw,66px);margin:5px 0 0}.min3 .page-head p{max-width:460px;color:var(--muted);margin-bottom:5px}
      .min3 .ledger-section{display:grid;grid-template-columns:150px 1fr;border-bottom:1px solid var(--line);scroll-margin-top:0}.min3 .ledger-label{padding:22px 14px 22px clamp(18px,3vw,44px);border-right:1px solid var(--line);font:600 10px/1.3 var(--mono);text-transform:uppercase;letter-spacing:.07em;color:var(--muted)}.min3 .ledger-body{min-width:0;padding:22px clamp(18px,3vw,34px)}
      .min3 .workflow-ledger{border-top:1px solid var(--line)}.min3 .wf-row{display:grid;grid-template-columns:1.2fr .8fr 1fr .7fr .75fr;gap:12px;align-items:start;border-bottom:1px solid var(--line);padding:17px 0;text-decoration:none}.min3 .wf-row.header{font:9px var(--mono);text-transform:uppercase;color:var(--muted);padding:9px 0}.min3 .wf-row h2{font-family:var(--serif);font-size:23px;margin:0 0 4px}.min3 .wf-row p{font-size:11px;color:var(--muted);margin:0}.min3 .cell-label{display:block;font:9px var(--mono);color:var(--muted);text-transform:uppercase;margin-bottom:5px}
      .min3 .graph-ledger{display:grid;grid-template-columns:1fr 320px;border:1px solid var(--line);background:var(--paper)}.min3 .graph-main{padding:18px}.min3 .graph-head{display:flex;justify-content:space-between;align-items:start;border-bottom:1px solid var(--line);padding-bottom:16px}.min3 .graph-head h2{font-size:32px;margin:4px 0 0}.min3 .railgraph{list-style:none;padding:0;margin:28px 0 4px}.min3 .railgraph li{display:grid;grid-template-columns:42px 1fr auto;align-items:center;gap:12px;min-height:58px;border-top:1px solid var(--line);position:relative}.min3 .railgraph li:before{content:"";position:absolute;left:20px;top:0;bottom:0;border-left:1px solid var(--line-strong)}.min3 .railgraph .node-index{position:relative;z-index:1;background:var(--paper);border:1px solid var(--line-strong);border-radius:50%;width:29px;height:29px;display:grid;place-items:center}.min3 .railgraph .current .node-index{background:var(--blue);border-color:var(--blue-ink)}.min3 .railgraph strong{font-family:var(--serif);font-size:17px}.min3 .overlay{border-left:1px solid var(--line);background:var(--bg);padding:18px}.min3 .overlay h3{font-family:var(--serif);font-size:21px;margin:7px 0}.min3 .overlay dl{display:grid;grid-template-columns:1fr 1fr;gap:10px;border-top:1px solid var(--line);padding-top:13px}.min3 .overlay dt{font:9px var(--mono);text-transform:uppercase;color:var(--muted)}.min3 .overlay dd{margin:2px 0;font-size:11px}.min3 .overlay-child{border-top:1px solid var(--line);padding:9px 0;font-size:11px}.min3 .overlay-child strong,.min3 .overlay-child span{display:block}.min3 .overlay-child span{color:var(--muted);margin-top:2px}
      .min3 .table-shell{overflow:auto;border-top:1px solid var(--line-strong)}
      .min3 .spend-ledger{display:grid;grid-template-columns:220px 1fr}.min3 .scope-column{border-right:1px solid var(--line);padding-right:18px}.min3 .scope-column h3{font-family:var(--serif);font-size:20px}.min3 .scope-column div{font:10px var(--mono);padding:9px 0;border-top:1px solid var(--line)}.min3 .truth-column{display:grid;grid-template-columns:repeat(3,1fr)}.min3 .truth-item{padding:0 18px;border-right:1px solid var(--line)}.min3 .truth-item:last-child{border:0}.min3 .truth-item span,.min3 .truth-item strong,.min3 .truth-item small{display:block}.min3 .truth-item span{font:9px var(--mono);text-transform:uppercase}.min3 .truth-item strong{font-family:var(--serif);font-size:18px;margin:17px 0 4px}
      .min3 .create-flow{border:0}.min3 .create-wrap{border-top:1px solid var(--line-strong)}
      @media(max-width:900px){.min3{grid-template-columns:1fr}.min3 .rail{position:sticky;height:auto;z-index:5;border-right:0;border-bottom:1px solid var(--line);padding:8px;flex-direction:row;align-items:center;overflow:auto}.min3 .rail .brand{padding:5px 12px}.min3 .rail nav{display:flex}.min3 .rail .rail-bottom{margin:0 0 0 auto;display:flex}.min3 .fixture-note{display:none}.min3 .page-head,.min3 .ledger-section{grid-template-columns:1fr}.min3 .ledger-label{border-right:0;border-bottom:1px solid var(--line);padding:11px 18px}.min3 .graph-ledger,.min3 .spend-ledger{grid-template-columns:1fr}.min3 .overlay,.min3 .scope-column{border-left:0;border-right:0;border-top:1px solid var(--line);padding:18px 0}.min3 .wf-row{grid-template-columns:1fr 1fr}.min3 .wf-row.header{display:none}}
    </style>
    <aside class="rail"><div class="brand">Bitterblossom</div><nav aria-label="Primary"><a href="#min3-workflows">Workflows</a><a href="#min3-agents">Agents</a><a href="#min3-runs">Runs</a><a href="#min3-spend">Spend</a></nav><div class="rail-bottom"><a class="create-link" href="#min3-create">Create workflow</a><button type="button" data-theme-toggle>Theme</button><p class="fixture-note">${e(corpus.notice)}</p></div></aside>
    <main class="ledger"><header class="page-head"><div><span class="eyebrow">Configured operator system</span><h1>Workflow ledger</h1></div><p>The roster is the index. Each selected definition opens into its fixed graph, with live evidence attached alongside rather than replacing configuration.</p></header>
      <section class="ledger-section" id="min3-workflows"><div class="ledger-label">01 / Workflows</div><div class="ledger-body"><div class="workflow-ledger"><div class="wf-row header"><span>Definition</span><span>Lifecycle / trigger</span><span>Latest run truth</span><span>Verification</span><span>Spend</span></div>${corpus.workflows.map((workflow)=>`<a class="wf-row" href="#min3-topology"><div><h2>${e(workflow.name)}</h2><p>${e(workflow.trigger)}</p></div><div>${badge(workflow.lifecycle)}<span class="cell-label" style="margin-top:8px">${workflow.trigger.includes("Listening")?"Trigger listening":"Trigger configured"}</span></div><div><span class="cell-label">Instance</span><strong>${workflow.active?`${workflow.active} executing`:"No active run"}</strong><span class="cell-label" style="margin-top:8px">Execution ${e(workflow.latestExecution)} · Domain ${e(workflow.latestDomain)}</span></div><div>${badge(workflow.verification)}</div><div>${costBadge(workflow.cost)}</div></a>`).join("")}</div></div></section>
      <section class="ledger-section" id="min3-topology"><div class="ledger-label">02 / Selected graph</div><div class="ledger-body"><div class="graph-ledger"><div class="graph-main"><header class="graph-head"><div><span class="eyebrow">Stable configured topology · revision 07</span><h2>${e(selected.name)}</h2></div><div>${badge(selected.lifecycle)} ${badge("Listening","Trigger listening")}</div></header>${topology(selected,corpus.run,"railgraph")}</div><aside class="overlay"><span class="eyebrow">Selected live overlay</span><h3>${e(corpus.run.id)}</h3><p>${badge(corpus.run.state)} ${costBadge(corpus.run.cost)}</p><dl><div><dt>Step</dt><dd>${e(corpus.run.step)}</dd></div><div><dt>Elapsed</dt><dd>${e(corpus.run.elapsed)}</dd></div><div><dt>Trigger</dt><dd>${e(corpus.run.trigger)}</dd></div><div><dt>Budget</dt><dd>${e(corpus.run.budget)}</dd></div></dl>${corpus.run.children.map(child=>`<div class="overlay-child"><strong>${e(child.name)} · ${e(child.state)}</strong><span>${e(child.result)}</span></div>`).join("")}</aside></div></div></section>
      <section class="ledger-section" id="min3-agents"><div class="ledger-label">03 / Agents</div><div class="ledger-body"><div class="table-shell"><table><thead><tr><th>Agent</th><th>Availability</th><th>Authority ceiling</th><th>Workflows</th><th>Now</th></tr></thead><tbody>${agentRows(corpus)}</tbody></table></div></div></section>
      <section class="ledger-section" id="min3-runs"><div class="ledger-label">04 / Runs</div><div class="ledger-body"><div class="table-shell"><table><thead><tr><th>Workflow / reference</th><th>Execution</th><th>Domain result</th><th>Verification</th><th>Cost</th></tr></thead><tbody>${evidenceRows(corpus)}</tbody></table></div></div></section>
      <section class="ledger-section" id="min3-spend"><div class="ledger-label">05 / Spend</div><div class="ledger-body"><div class="spend-ledger"><div class="scope-column"><h3>Policy scopes</h3>${corpus.spend.scopes.map(scope=>`<div>${e(scope)}</div>`).join("")}</div><div class="truth-column">${spendTruth(corpus)}</div></div></div></section>
      <section class="ledger-section" id="min3-create"><div class="ledger-label">06 / Create</div><div class="ledger-body"><div class="create-wrap">${createFlow(corpus,"min3")}</div></div></section>
    </main>
  </div>`;
}

export const SPECS = {
  "MIN-1": {
    label: "The Operating Dossier",
    move: "A persistent workflow roster becomes the book spine while the selected definition, live overlay, evidence, spend, and creation flow read as one editorial dossier.",
    philosophy: "Warm monochrome, serif-led hierarchy, scarce pastel semantics, crisp bordered surfaces, and generous internal whitespace make a dense control plane feel calm without reducing operational truth.",
    render: renderMin1
  },
  "MIN-2": {
    label: "The Workflow Folio",
    move: "Inverts conventional top navigation: the configured workflow roster opens the page, the graph becomes a wide evidence stage, and primary navigation lives in a compact bottom dock.",
    philosophy: "Flat folio sheets, restrained geometry, typographic contrast, and desaturated state color treat the control plane as an inspectable publication rather than a dashboard.",
    render: renderMin2
  },
  "MIN-3": {
    label: "The Continuous Ledger",
    move: "Rejects card-based master-detail in favor of a single ruled ledger whose numbered sections keep configuration, runtime evidence, agents, spend, and creation in one uninterrupted audit surface.",
    philosophy: "A strict document grid, near-zero elevation, fine rules, compact metadata, and editorial display type maximize information density while preserving legibility and keyboard reach.",
    render: renderMin3
  }
};
