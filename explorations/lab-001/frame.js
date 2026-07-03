// Assembles SPECS from window.BB_OPTS (each options/opt-N.js registers one
// entry) and renders whichever ID is in location.hash into #mount.
// Unknown hashes are ignored (mount left as-is); a recognized OPT-N ID with
// no registered spec gets a quiet "not built yet" panel — this is what lets
// six option lanes build in parallel without frame.html breaking mid-round.
(function () {
  "use strict";

  const VALID_ID = /^OPT-[0-6]$/;

  function notBuiltPanel(id) {
    const panel = window.el("div", { class: "bb-not-built" });
    panel.appendChild(window.markSVG(28));
    panel.appendChild(window.el("p", { class: "bb-not-built-note" }, [`${id} — option not built yet.`]));
    return panel;
  }

  function render() {
    const mount = document.getElementById("mount");
    const id = location.hash.replace(/^#/, "");
    if (!VALID_ID.test(id)) return; // ignore unknown/empty hashes

    const SPECS = window.BB_OPTS || {};
    const spec = SPECS[id];
    mount.classList.remove("bb-scrollable");
    mount.innerHTML = "";

    if (!spec || typeof spec.build !== "function") {
      mount.appendChild(notBuiltPanel(id));
      return;
    }

    try {
      spec.build(mount);
    } catch (err) {
      console.error(`[lab-001] ${id} threw during build()`, err);
      mount.innerHTML = "";
      mount.appendChild(notBuiltPanel(id));
    }
  }

  window.addEventListener("hashchange", render);
  document.addEventListener("DOMContentLoaded", render);
  // In case DOMContentLoaded already fired by the time this executes.
  if (document.readyState !== "loading") render();
})();
