// OPT-0 — baseline (shipped). Faithful static reproduction of src/operator.html's
// overview/runs/tasks/health surface, wired directly to window.BB_DATA instead
// of fetch()+bearer auth (there is no live API in this lab, only the inlined
// snapshot). Redundancies (budget ×3, DLQ count ×3) and the scrolling desk are
// reproduced deliberately — this is the round-1 reference, unjudged.
window.BB_OPTS = window.BB_OPTS || {};
window.BB_OPTS["OPT-0"] = {
  title: "Baseline (shipped)",
  notes: "Faithful reproduction of the current operator.html overview — rail, proof strip, stat cards, fleet, next action, recent runs. Reference only.",
  build(mount) {
    "use strict";
    mount.classList.add("bb-scrollable");

    const fmtMoney = (n) => (n == null ? "-" : `$${Number(n).toFixed(2)}`);
    const fmtSmallMoney = (n) => (n == null ? "-" : `$${Number(n).toFixed(4)}`);
    const fmtDuration = (ms) => {
      if (ms == null) return "-";
      const s = Math.round(ms / 1000);
      if (s < 60) return `${s}s`;
      const m = Math.floor(s / 60);
      const rest = s % 60;
      if (m < 60) return `${m}m ${rest}s`;
      return `${Math.floor(m / 60)}h ${m % 60}m`;
    };
    const short = (s, n = 12) => (!s ? "-" : s.length > n ? `${s.slice(0, n)}...` : s);
    const esc = (value) =>
      String(value ?? "-")
        .replaceAll("&", "&amp;")
        .replaceAll("<", "&lt;")
        .replaceAll(">", "&gt;")
        .replaceAll('"', "&quot;");
    const stateClass = (s) =>
      ["success", "failure", "awaiting_recovery", "blocked_budget", "retired", "pending", "running"].includes(s) ? s : "";

    const D = window.BB_DATA;

    mount.innerHTML = `
<style>
.opt0-shell{height:100%;display:grid;grid-template-columns:14rem minmax(0,1fr);overflow:hidden;font:14px/1.75 var(--font);color:var(--ink);}
.opt0-shell .rail{border-right:1px solid var(--line);background:var(--wash);padding:1.2rem 1.1rem;font:13px/1.7 var(--mono);color:var(--muted);overflow-y:auto;display:flex;flex-direction:column;}
.opt0-shell .brand{display:flex;align-items:center;gap:.4rem;color:var(--ink);font-weight:800;margin-bottom:1.2rem;}
.opt0-shell .rail button{display:block;width:100%;border:0;border-left:2px solid transparent;background:transparent;color:var(--muted);text-align:left;padding:.25rem 0 .25rem .55rem;margin:.05rem 0;font:inherit;cursor:pointer;}
.opt0-shell .rail button:hover,.opt0-shell .rail button.is-active{color:var(--ink);}
.opt0-shell .rail button.is-active{border-left-color:var(--accent);font-weight:550;}
.opt0-shell .rail-foot{margin-top:auto;padding-top:1.2rem;color:var(--faint);}
.opt0-shell .desk{min-width:0;overflow-y:auto;padding:1.4rem 1.8rem 2rem;}
.opt0-shell .topbar{border-bottom:1px solid var(--line);padding-bottom:.9rem;margin-bottom:.9rem;}
.opt0-shell .proof-strip{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));border:1px solid var(--frame);margin-bottom:.9rem;background:var(--line);gap:1px;font:12px/1.5 var(--mono);}
.opt0-shell .proof-strip span{min-width:0;background:var(--panel);padding:.46rem .6rem;}
.opt0-shell .proof-strip b,.opt0-shell .proof-strip em{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-style:normal;}
.opt0-shell .proof-strip b{color:var(--muted);font-weight:400;letter-spacing:.08em;text-transform:uppercase;}
.opt0-shell .proof-strip em{color:var(--ink);font-weight:800;font-variant-numeric:tabular-nums;}
.opt0-shell h1{font-weight:800;font-size:1.15em;}
.opt0-shell .plane-sentence{color:var(--muted);max-width:44rem;overflow-wrap:anywhere;}
.opt0-shell .chrome{font:12px/1.6 var(--mono);color:var(--muted);letter-spacing:.08em;text-transform:uppercase;}
.opt0-shell .view{display:none;}
.opt0-shell .view.is-active{display:block;}
.opt0-shell .summary{display:grid;grid-template-columns:repeat(5,minmax(0,1fr));gap:1px;background:var(--line);border:1px solid var(--frame);}
.opt0-shell .metric{min-width:0;min-height:5.6rem;padding:.75rem;background:var(--panel);}
.opt0-shell .metric strong,.opt0-shell .num{display:block;margin-top:.3rem;font:800 1em/1.5 var(--mono);font-variant-numeric:tabular-nums;}
.opt0-shell .metric p{color:var(--muted);margin-top:.2rem;}
.opt0-shell .meter{position:relative;height:4px;border:1px solid var(--line);background:var(--wash);margin-top:.6rem;}
.opt0-shell .meter span{position:absolute;inset:0 auto 0 0;width:0;background:var(--accent);}
.opt0-shell .meter span.ok{background:var(--ok);}
.opt0-shell .meter span.warn{background:var(--warn);}
.opt0-shell .meter span.err{background:var(--err);}
.opt0-shell .grid-2{display:grid;grid-template-columns:minmax(0,1.1fr) minmax(20rem,.85fr);gap:1rem;margin-top:1rem;}
.opt0-shell .plate{border:1px solid var(--frame);padding:.9rem;min-width:0;background:var(--panel);}
.opt0-shell .plate h2{margin:-0.9rem -0.9rem .7rem;padding:.44rem .65rem;border-bottom:1px solid var(--line);background:var(--wash);font:12px/1.5 var(--mono);font-weight:800;letter-spacing:.08em;text-transform:uppercase;}
.opt0-shell .table-wrap{overflow-x:auto;}
.opt0-shell table{width:100%;border-collapse:collapse;font:13px/1.6 var(--mono);}
.opt0-shell th{text-align:left;font-weight:400;color:var(--muted);background:var(--wash);letter-spacing:.06em;padding:.4rem .55rem;}
.opt0-shell td{border-top:1px solid var(--line);padding:.4rem .55rem;vertical-align:top;}
.opt0-shell tbody tr:first-child td{border-top:0;}
.opt0-shell .right{text-align:right;}
.opt0-shell .item{font-weight:550;color:var(--ink);}
.opt0-shell .muted{color:var(--muted);}
.opt0-shell .faint{color:var(--faint);}
.opt0-shell .ok{color:var(--ok);}
.opt0-shell .warn{color:var(--warn);}
.opt0-shell .err{color:var(--err);}
.opt0-shell .state{white-space:nowrap;font-weight:550;}
.opt0-shell .state::before{display:inline-block;margin-right:.35em;font-weight:800;}
.opt0-shell .state.success::before{content:"ok";color:var(--ok);}
.opt0-shell .state.failure::before,.opt0-shell .state.awaiting_recovery::before{content:"x";color:var(--err);}
.opt0-shell .state.blocked_budget::before,.opt0-shell .state.retired::before{content:"!";color:var(--warn);}
.opt0-shell .state.pending::before,.opt0-shell .state.running::before{content:">";color:var(--accent);}
.opt0-shell .empty{color:var(--muted);padding:.9rem 0;}
.opt0-shell .actions{display:grid;gap:.55rem;}
.opt0-shell .next-action{border-top:1px solid var(--line);padding-top:.55rem;}
.opt0-shell .next-action:first-child{border-top:0;padding-top:0;}
.opt0-shell .next-action code{display:block;margin-top:.22rem;padding-left:.55rem;border-left:1px solid var(--line);color:var(--muted);white-space:nowrap;overflow:hidden;text-overflow:ellipsis;}
.opt0-shell .health-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:1rem;margin-top:1rem;}
@media (max-width:980px){
.opt0-shell{grid-template-columns:1fr;overflow:visible;}
.opt0-shell .rail{border-right:0;border-bottom:1px solid var(--line);display:block;overflow:visible;padding:.7rem .9rem;}
.opt0-shell .brand{margin-bottom:.3rem;}
.opt0-shell .rail button{display:inline-block;width:auto;border-left:0;border-bottom:2px solid transparent;margin:0 .6rem .2rem 0;padding:.15rem .05rem;}
.opt0-shell .rail button.is-active{border-bottom-color:var(--accent);}
.opt0-shell .rail-foot{display:none;}
.opt0-shell .desk{overflow:visible;padding:.9rem;}
.opt0-shell .summary,.opt0-shell .grid-2,.opt0-shell .health-grid{grid-template-columns:1fr;}
.opt0-shell .proof-strip{grid-template-columns:1fr 1fr;}
}
</style>
<div class="opt0-shell">
  <nav class="rail" aria-label="Dashboard views">
    <div class="brand" id="opt0Brand"></div>
    <button class="is-active" data-view-button="overview">overview</button>
    <button data-view-button="runs">runs</button>
    <button data-view-button="tasks">tasks</button>
    <button data-view-button="health">health</button>
    <div class="rail-foot">
      <div>live (snapshot)</div>
      <div>${esc(short(D.status.generated_at, 19))}</div>
    </div>
  </nav>
  <main class="desk">
    <header class="topbar">
      <p class="chrome">event plane visibility</p>
      <h1 id="opt0PageTitle">overview</h1>
      <p id="opt0PlaneSentence" class="plane-sentence"></p>
    </header>
    <div class="proof-strip" aria-label="Plane proof strip">
      <span><b>ledger</b><em id="opt0Schema">-</em></span>
      <span><b>budget</b><em id="opt0Budget">-</em></span>
      <span><b>notify</b><em id="opt0Outbox">-</em></span>
      <span><b>freshness</b><em id="opt0Freshness">-</em></span>
    </div>
    <section class="view is-active" data-view="overview">
      <section class="summary" aria-label="Plane summary">
        <article class="metric"><span class="chrome">cost today</span><strong id="opt0CostToday">-</strong><div class="meter"><span id="opt0CostMeter"></span></div><p id="opt0CostNote"></p></article>
        <article class="metric"><span class="chrome">running</span><strong id="opt0Running">0</strong><p id="opt0Leases">0 leases</p></article>
        <article class="metric"><span class="chrome">pending</span><strong id="opt0Pending">0</strong><p id="opt0PendingAge">oldest -</p></article>
        <article class="metric"><span class="chrome">open dlq</span><strong id="opt0Dlq">0</strong><p>operator work</p></article>
        <article class="metric"><span class="chrome">triggers</span><strong id="opt0Triggers">0</strong><p id="opt0TriggerKinds">manual / cron / webhook</p></article>
      </section>
      <section class="grid-2">
        <article class="plate"><h2>Fleet</h2><div class="table-wrap"><table><thead><tr><th>host</th><th>task</th><th>run</th><th>phase</th><th class="right">cost</th></tr></thead><tbody id="opt0Fleet"></tbody></table></div></article>
        <article class="plate"><h2>Next Action</h2><div id="opt0Actions" class="actions"></div></article>
      </section>
      <article class="plate" style="margin-top:1rem"><h2>Recent Runs</h2><div class="table-wrap"><table><thead><tr><th>state</th><th>task</th><th>agent / model</th><th>duration</th><th class="right">cost</th><th class="right">tokens</th><th>evidence</th></tr></thead><tbody id="opt0RecentRuns"></tbody></table></div></article>
    </section>
    <section class="view" data-view="runs">
      <article class="plate"><h2>Runs</h2><div class="table-wrap"><table><thead><tr><th>created</th><th>state</th><th>task</th><th>trigger</th><th>agent</th><th>duration</th><th class="right">cost</th><th class="right">tokens</th><th>trace</th></tr></thead><tbody id="opt0RunRows"></tbody></table></div></article>
    </section>
    <section class="view" data-view="tasks">
      <article class="plate"><h2>Configured Tasks</h2><div class="table-wrap"><table><thead><tr><th>task</th><th>agent</th><th>substrate</th><th>triggers</th><th>budget</th><th>state</th></tr></thead><tbody id="opt0TaskRows"></tbody></table></div></article>
    </section>
    <section class="view" data-view="health">
      <section class="health-grid">
        <article class="plate"><h2>DLQ</h2><div class="table-wrap"><table><thead><tr><th>id</th><th>status</th><th>task</th><th>run</th><th>error</th></tr></thead><tbody id="opt0DlqRows"></tbody></table></div></article>
        <article class="plate"><h2>Leases</h2><div class="table-wrap"><table><thead><tr><th>host</th><th>run</th><th>acquired</th></tr></thead><tbody id="opt0LeaseRows"></tbody></table></div></article>
        <article class="plate"><h2>Ingress</h2><div class="table-wrap"><table><thead><tr><th>received</th><th>task</th><th>kind</th><th>duplicate</th></tr></thead><tbody id="opt0IngressRows"></tbody></table></div></article>
      </section>
    </section>
  </main>
</div>`;

    const $ = (id) => mount.querySelector(`#${id}`);
    $("opt0Brand").appendChild(window.markSVG(16));
    $("opt0Brand").appendChild(document.createTextNode(" bitterblossom"));

    // proof strip (redundancy #1 — budget/dlq also appear in the summary cards)
    const status = D.status || {};
    const summary = status.summary || {};
    const ledger = status.ledger || {};
    const notify = status.guards?.notify?.outbox || {};
    const freshness = status.freshness_contracts || [];
    const critical = freshness.filter((row) => row.notification_severity === "critical").length;
    const budget = summary.max_cost_per_day_usd;
    $("opt0Schema").textContent = ledger.schema_version == null ? "-" : `schema v${ledger.schema_version}`;
    $("opt0Budget").textContent = budget == null ? "no daily cap" : `${fmtMoney(summary.cost_today_usd || 0)} / ${fmtMoney(budget)}`;
    $("opt0Outbox").textContent = `${notify.pending || 0} pending / ${notify.failed || 0} failed`;
    $("opt0Freshness").textContent = `${critical} critical / ${freshness.length} total`;

    // plane sentence (redundancy #2 — DLQ/leases repeated again here)
    const running = D.runs.filter((r) => r.state === "running").length;
    const pending = D.runs.filter((r) => r.state === "pending").length;
    const openDlq = summary.open_dlq || 0;
    const activeLeases = summary.active_leases || D.leases.length;
    $("opt0PlaneSentence").textContent = [
      openDlq ? `${openDlq} open DLQ` : "DLQ clear",
      activeLeases ? `${activeLeases} lease${activeLeases === 1 ? "" : "s"}` : "no leases",
      pending ? `${pending} pending` : "queue clear",
      running ? `${running} running` : null,
      `${fmtMoney(summary.cost_today_usd || 0)} today`,
    ]
      .filter(Boolean)
      .join(" / ");

    // summary cards (redundancy #3 — budget/dlq a third time)
    const triggerKinds = { manual: 0, cron: 0, webhook: 0 };
    for (const task of D.tasks) {
      for (const trigger of task.trigger_details || []) {
        if (triggerKinds[trigger.kind] != null) triggerKinds[trigger.kind] += 1;
      }
    }
    const cost = summary.cost_today_usd || 0;
    const pct = budget ? Math.min(100, (cost / budget) * 100) : 0;
    const oldestPending = D.runs.filter((r) => r.state === "pending").at(-1);
    $("opt0CostToday").textContent = `${fmtMoney(cost)} / ${budget == null ? "-" : fmtMoney(budget)}`;
    $("opt0CostNote").textContent = budget == null ? "no daily cap" : `${pct.toFixed(0)}% of daily cap`;
    $("opt0CostMeter").style.width = `${pct}%`;
    $("opt0CostMeter").className = pct >= 90 ? "err" : pct >= 65 ? "warn" : "ok";
    $("opt0Running").textContent = running;
    $("opt0Leases").textContent = `${activeLeases} leases`;
    $("opt0Pending").textContent = pending;
    $("opt0PendingAge").textContent = oldestPending ? `oldest ${short(oldestPending.created_at, 19)}` : "oldest -";
    $("opt0Dlq").textContent = openDlq;
    $("opt0Triggers").textContent = Object.values(triggerKinds).reduce((a, b) => a + b, 0);
    $("opt0TriggerKinds").textContent = `${triggerKinds.manual} manual / ${triggerKinds.cron} cron / ${triggerKinds.webhook} webhook`;

    // fleet (empty — 0 leases in this snapshot; still costs ~half the grid-2 row)
    const runsById = new Map(D.runs.map((r) => [r.id, r]));
    const fleetRows = D.leases
      .map(
        (lease) => `<tr>
      <td class="item">${esc(lease.host)}</td>
      <td>${esc(runsById.get(lease.run_id)?.task)}</td>
      <td>${esc(short(lease.run_id))}</td>
      <td>${esc(runsById.get(lease.run_id)?.state || "leased")}</td>
      <td class="right">${fmtSmallMoney(runsById.get(lease.run_id)?.cost_usd)}</td>
    </tr>`
      )
      .join("");
    $("opt0Fleet").innerHTML = fleetRows || `<tr><td colspan="5" class="empty">no active host leases</td></tr>`;

    // next action — raw error text verbatim, incl. the duplicated canary-triage root cause
    const actions = status.tasks.flatMap((task) => (task.safe_next_actions || []).map((a) => ({ task: task.task, ...a })));
    const important = actions.filter((a) => a.kind !== "monitor").slice(0, 5);
    const chosen = important.length ? important : actions.slice(0, 3);
    $("opt0Actions").innerHTML =
      chosen
        .map(
          (a) => `<div class="next-action">
      <p><span class="${a.kind.includes("replay") || a.kind.includes("escalate") ? "warn" : "ok"}">${a.kind === "monitor" ? "ok" : "!"}</span> <span class="item">${esc(a.task)}</span> ${esc(a.reason)}</p>
      <code>${esc(a.command)}</code>
    </div>`
        )
        .join("") || `<p class="empty">no safe action surfaced</p>`;

    function runRow(run, compact) {
      const agent = `${run.agent_name || "-"}${run.agent_version ? `@v${run.agent_version}` : ""}`;
      // no /api/export snapshot in this lab, so per-run token totals are unavailable — shown as "-"
      if (compact) {
        return `<tr>
        <td><span class="state ${stateClass(run.state)}">${esc(run.state)}</span></td>
        <td class="item">${esc(run.task)}</td>
        <td>${esc(agent)}</td>
        <td>${fmtDuration(run.duration_ms)}</td>
        <td class="right">${fmtSmallMoney(run.cost_usd)}</td>
        <td class="right">-</td>
        <td>${esc(short(run.id))}</td>
      </tr>`;
      }
      return `<tr>
      <td>${esc(short(run.created_at, 19))}</td>
      <td><span class="state ${stateClass(run.state)}">${esc(run.state)}</span></td>
      <td class="item">${esc(run.task)}</td>
      <td>${esc(run.trigger_kind)}</td>
      <td>${esc(agent)}</td>
      <td>${fmtDuration(run.duration_ms)}</td>
      <td class="right">${fmtSmallMoney(run.cost_usd)}</td>
      <td class="right">-</td>
      <td>${esc(short(run.trace_id))}</td>
    </tr>`;
    }
    $("opt0RecentRuns").innerHTML =
      D.runs
        .slice(0, 10)
        .map((r) => runRow(r, true))
        .join("") || `<tr><td colspan="7" class="empty">no runs recorded</td></tr>`;
    $("opt0RunRows").innerHTML = D.runs.map((r) => runRow(r, false)).join("") || `<tr><td colspan="9" class="empty">no runs recorded</td></tr>`;

    function triggerLabel(trigger) {
      if (trigger.kind === "cron") return `cron ${esc(trigger.schedule)}`;
      if (trigger.kind === "webhook") return `webhook /hooks/${esc(trigger.route)}`;
      return esc(trigger.kind);
    }
    $("opt0TaskRows").innerHTML =
      D.tasks
        .map((task) => {
          const triggers = (task.trigger_details || []).map(triggerLabel).join("<br>");
          const b = [
            task.max_runs_per_day == null ? null : `${task.runs_today}/${task.max_runs_per_day} runs`,
            task.max_cost_per_run_usd == null ? null : `${fmtMoney(task.max_cost_per_run_usd)} per run`,
            task.timeout_minutes == null ? null : `${task.timeout_minutes}m timeout`,
          ]
            .filter(Boolean)
            .join("<br>") || "-";
          const parked = task.parked
            ? `<span class="warn">parked</span><br><span class="muted">${esc(task.parked)}</span>`
            : `<span class="ok">active</span>`;
          return `<tr>
        <td class="item">${esc(task.task)}</td>
        <td>${esc(task.agent)}<br><span class="muted">${esc(task.harness)} ${esc(task.model)}</span></td>
        <td>${esc(task.substrate)}</td>
        <td>${triggers || "-"}</td>
        <td>${b}</td>
        <td>${parked}</td>
      </tr>`;
        })
        .join("") || `<tr><td colspan="6" class="empty">no tasks configured</td></tr>`;

    $("opt0DlqRows").innerHTML =
      D.dlq
        .slice(0, 12)
        .map(
          (row) => `<tr>
      <td class="right">${row.id}</td><td>${esc(row.status)}</td><td class="item">${esc(row.task)}</td>
      <td>${esc(short(row.run_id))}</td><td>${esc(short(row.error, 80))}</td>
    </tr>`
        )
        .join("") || `<tr><td colspan="5" class="empty">no dead letters</td></tr>`;
    $("opt0LeaseRows").innerHTML =
      D.leases
        .map(
          (lease) => `<tr>
      <td class="item">${esc(lease.host)}</td><td>${esc(short(lease.run_id))}</td><td>${esc(short(lease.acquired_at, 19))}</td>
    </tr>`
        )
        .join("") || `<tr><td colspan="3" class="empty">no active leases</td></tr>`;
    $("opt0IngressRows").innerHTML =
      D.ingress
        .map(
          (event) => `<tr>
      <td>${esc(short(event.received_at, 19))}</td><td class="item">${esc(event.task)}</td>
      <td>${esc(event.trigger_kind)}</td><td>${event.duplicate ? "yes" : "no"}</td>
    </tr>`
        )
        .join("") || `<tr><td colspan="4" class="empty">no ingress events</td></tr>`;

    function switchView(view) {
      mount.querySelectorAll("[data-view]").forEach((el) => el.classList.toggle("is-active", el.dataset.view === view));
      mount.querySelectorAll("[data-view-button]").forEach((el) => el.classList.toggle("is-active", el.dataset.viewButton === view));
      $("opt0PageTitle").textContent = view;
    }
    mount.querySelectorAll("[data-view-button]").forEach((button) => {
      button.addEventListener("click", () => switchView(button.dataset.viewButton));
    });
  },
};
