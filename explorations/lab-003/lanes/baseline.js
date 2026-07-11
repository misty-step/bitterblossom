export const SPECS = {
  "BASE-1": {
    label: "Shipped noir-ledger reconstruction",
    philosophy: "Current baseline",
    move: "Task-and-table operator cockpit reconstructed with the proposed corpus; baseline only, not a candidate.",
    render(c) {
      const tasks = c.workflows.map(w => `<tr><td><strong>${w.name}</strong><small>${w.trigger}</small></td><td>${w.lifecycle}</td><td>${w.topology.slice(1, -1).join(" / ")}</td><td>${w.active}</td><td>${w.latestExecution}</td><td>${w.cost}</td></tr>`).join("");
      const runs = c.recentRuns.map(r => `<tr><td>${r.workflow}</td><td>${r.ref}</td><td>${r.execution}</td><td>${r.domain}</td><td>${r.verification}</td><td>${r.cost}</td></tr>`).join("");
      return `
        <style>
          :root { --b:#fcfcfc; --p:#fff; --w:#f1f1ef; --i:#151515; --m:#737373; --l:#d9d9d6; --a:#2643d0; --ok:#15714b; --er:#a84138; color-scheme:light; }
          :root[data-theme="dark"] { --b:#111; --p:#171717; --w:#222; --i:#f2f2ef; --m:#aaa; --l:#3b3b38; --a:#8c9eff; --ok:#56b88c; --er:#e47b71; color-scheme:dark; }
          @media (prefers-color-scheme:dark) { :root:not([data-theme]) { --b:#111; --p:#171717; --w:#222; --i:#f2f2ef; --m:#aaa; --l:#3b3b38; --a:#8c9eff; --ok:#56b88c; --er:#e47b71; color-scheme:dark; } }
          .base { height:100dvh; display:grid; grid-template-columns:190px 1fr; background:var(--b); color:var(--i); font:12px/1.35 Geist,Helvetica,Arial,sans-serif; }
          .base aside { border-right:1px solid var(--l); padding:14px; display:flex; flex-direction:column; gap:18px; background:var(--p); }
          .base .brand { font-weight:800; font-size:14px; letter-spacing:-.02em; }
          .base nav { display:grid; gap:3px; } .base nav button { border:0; background:transparent; color:var(--m); text-align:left; padding:7px 8px; cursor:pointer; }
          .base nav button[aria-current] { color:var(--i); background:var(--w); font-weight:700; }
          .base .mode { margin-top:auto; border:1px solid var(--l); color:var(--i); background:transparent; padding:7px; cursor:pointer; }
          .base main { min-width:0; overflow:auto; padding:18px 20px 36px; }
          .base .top { display:flex; justify-content:space-between; align-items:end; margin-bottom:16px; } .base h1 { margin:0; font-size:22px; letter-spacing:-.04em; } .base .fixture { color:var(--m); font-family:ui-monospace,monospace; }
          .base .proof { display:grid; grid-template-columns:repeat(4,1fr); border:1px solid var(--l); margin-bottom:14px; } .base .proof div { padding:10px; border-right:1px solid var(--l); } .base .proof div:last-child{border:0}.base .proof strong{display:block;font-size:16px}.base .proof span{color:var(--m)}
          .base section { border:1px solid var(--l); background:var(--p); margin-bottom:12px; } .base h2 { margin:0; padding:7px 10px; border-bottom:1px solid var(--l); background:var(--w); font:700 10px/1.2 ui-monospace,monospace; letter-spacing:.08em; text-transform:uppercase; }
          .base table { width:100%; border-collapse:collapse; font-family:ui-monospace,monospace; font-size:11px; } .base th,.base td{padding:8px 10px;border-bottom:1px solid var(--l);text-align:left;vertical-align:top}.base th{color:var(--m);font-weight:500}.base td small{display:block;color:var(--m);margin-top:3px}
          .base .blocked{color:var(--er)} .base .ok{color:var(--ok)}
          @media(max-width:700px){.base{display:block}.base aside{position:sticky;top:0;z-index:2;border:0;border-bottom:1px solid var(--l);padding:9px 10px;display:grid;grid-template-columns:1fr auto}.base nav{grid-column:1/-1;display:flex;overflow:auto}.base .mode{margin:0}.base main{padding:14px 10px}.base .proof{grid-template-columns:1fr 1fr}.base .proof div:nth-child(2){border-right:0}.base table{min-width:720px}.base section{overflow:auto}.base .top{align-items:start;gap:10px}.base h1{font-size:19px}}
        </style>
        <div class="base">
          <aside>
            <div class="brand">Bitterblossom</div>
            <nav aria-label="Primary">
              <button aria-current="page">Workflows</button><button>Agents</button><button>Runs</button><button>Spend</button>
            </nav>
            <button class="mode" data-theme-toggle>Light / dark</button>
          </aside>
          <main>
            <div class="top"><div><div class="fixture">BASELINE · CURRENT NOIR-LEDGER GRAMMAR</div><h1>Configured workflows</h1></div><span class="fixture">${c.notice}</span></div>
            <div class="proof"><div><strong>2</strong><span>configured</span></div><div><strong>1</strong><span>active instance</span></div><div><strong class="blocked">1 blocked</strong><span>latest domain result</span></div><div><strong>unknown</strong><span>cost coverage</span></div></div>
            <section><h2>Workflow configuration</h2><table><thead><tr><th>Workflow / trigger</th><th>State</th><th>Agents</th><th>Active</th><th>Latest execution</th><th>Cost</th></tr></thead><tbody>${tasks}</tbody></table></section>
            <section><h2>Run history</h2><table><thead><tr><th>Workflow</th><th>Reference</th><th>Execution</th><th>Domain</th><th>Verification</th><th>Cost</th></tr></thead><tbody>${runs}</tbody></table></section>
            <section><h2>Current limitation</h2><div style="padding:12px;max-width:75ch">The shipped grammar exposes complete configuration through dense expandable tables. It does not yet make the configured workflow topology, selected run path, or goal-first creation model visually primary.</div></section>
          </main>
        </div>`;
    }
  }
};
