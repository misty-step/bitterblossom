// Registry shell for LAB-001: viewport presets/custom sizing with scale-down,
// a sidebar of the seven OVR options, and an iframe onto frame.html#OPT-N.
// Uses window.el/markSVG from parts.js (loaded before this file in index.html
// — add that script tag if it's ever missing) with a tiny inline fallback so
// the shell still renders even if parts.js hasn't loaded on this page.
(function () {
  "use strict";

  const el =
    window.el ||
    function (tag, attrs, children) {
      const node = document.createElement(tag);
      Object.entries(attrs || {}).forEach(([k, v]) => {
        if (v == null) return;
        if (k === "class") node.className = v;
        else if (k.startsWith("on") && typeof v === "function") node.addEventListener(k.slice(2).toLowerCase(), v);
        else node.setAttribute(k, v);
      });
      (Array.isArray(children) ? children : [children]).forEach((c) => {
        if (c != null) node.appendChild(c instanceof Node ? c : document.createTextNode(String(c)));
      });
      return node;
    };

  const SECTIONS = [
    {
      id: "OVR",
      label: "OVR",
      options: [
        { id: "OPT-0", title: "Baseline (shipped)", note: "Faithful reproduction of the current overview. Reference only, unjudged." },
        { id: "OPT-1", title: "The queue", note: "Inbox metaphor — one triaged, deduped attention item at a time, j/k to step." },
        { id: "OPT-2", title: "The day ledger", note: "Accounting-book metaphor — closing ledger, one money line, reconciliation items." },
        { id: "OPT-3", title: "The roster", note: "Task families as cards in a fixed grid; parked/failing float to front." },
        { id: "OPT-4", title: "The plane", note: "Lifecycle columns: events in → triggers → runs → evidence. IA is the glossary." },
        { id: "OPT-5", title: "The console", note: "CLI-first skin over bb — command bar, saved queries, one output pane." },
        { id: "OPT-6", title: "The wire (inversion)", note: "Chronological narrated feed, newest first, grouped by cause, severity as rules." },
      ],
    },
  ];
  const ALL_OPTIONS = SECTIONS.flatMap((s) => s.options);

  const VIEWPORTS = [
    { id: "fit", label: "fit" },
    { id: "1440x900", label: "1440×900", w: 1440, h: 900 },
    { id: "1280x800", label: "1280×800", w: 1280, h: 800 },
    { id: "1024x768", label: "1024×768", w: 1024, h: 768 },
    { id: "768x1024", label: "768×1024", w: 768, h: 1024 },
    { id: "390x844", label: "390×844", w: 390, h: 844 },
  ];

  const LS_OPTION = "bb-lab-001-option";
  const LS_VIEWPORT = "bb-lab-001-viewport";
  const LS_CUSTOM = "bb-lab-001-custom";

  const state = {
    optionId: localStorage.getItem(LS_OPTION) || "OPT-0",
    viewportId: localStorage.getItem(LS_VIEWPORT) || "fit",
    custom: JSON.parse(localStorage.getItem(LS_CUSTOM) || "null") || { w: 1440, h: 900 },
    unbuilt: new Set(),
  };
  if (!ALL_OPTIONS.some((o) => o.id === state.optionId)) state.optionId = "OPT-0";

  const $ = (id) => document.getElementById(id);
  const frame = $("optionFrame");
  const stageScale = $("stageScale");
  const stageViewport = $("stageViewport");
  const readout = $("viewportReadout");

  $("brand").appendChild(el("span", {}, [window.markSVG ? window.markSVG(14) : ""]));
  $("brand").appendChild(document.createTextNode(" lab-001"));

  function selectOption(id, opts) {
    opts = opts || {};
    if (!ALL_OPTIONS.some((o) => o.id === id)) return;
    state.optionId = id;
    localStorage.setItem(LS_OPTION, id);
    frame.src = `frame.html#${id}`;
    renderRegistry();
  }

  function renderRegistry() {
    const list = $("registryList");
    list.innerHTML = "";
    for (const opt of ALL_OPTIONS) {
      const btn = el(
        "button",
        {
          class: [opt.id === state.optionId ? "is-selected" : "", state.unbuilt.has(opt.id) ? "is-unbuilt" : ""].filter(Boolean).join(" "),
          "aria-current": opt.id === state.optionId ? "true" : null,
          onclick: () => selectOption(opt.id),
        },
        [el("span", { class: "opt-title" }, [`${opt.id} — ${opt.title}`]), el("span", { class: "opt-note" }, [opt.note])]
      );
      list.appendChild(el("li", {}, [btn]));
    }
  }

  // Mark options whose file isn't present yet (other lanes build in parallel).
  // Best-effort: a HEAD request against options/opt-N.js. Silently no-ops if
  // the static server doesn't support HEAD or the check throws.
  function checkBuilt() {
    for (const opt of ALL_OPTIONS) {
      const n = opt.id.split("-")[1];
      fetch(`options/opt-${n}.js`, { method: "HEAD" })
        .then((res) => {
          if (!res.ok) {
            state.unbuilt.add(opt.id);
            renderRegistry();
          }
        })
        .catch(() => {});
    }
  }

  function renderPresets() {
    const group = $("presetGroup");
    group.innerHTML = "";
    for (const vp of VIEWPORTS) {
      group.appendChild(
        el(
          "button",
          {
            type: "button",
            class: vp.id === state.viewportId ? "is-active" : "",
            onclick: () => setViewport(vp.id),
          },
          [vp.label]
        )
      );
    }
  }

  function setViewport(id, custom) {
    state.viewportId = id;
    localStorage.setItem(LS_VIEWPORT, id);
    if (custom) {
      state.custom = custom;
      localStorage.setItem(LS_CUSTOM, JSON.stringify(custom));
    }
    renderPresets();
    applyViewport();
  }

  function applyViewport() {
    if (state.viewportId === "fit") {
      stageScale.classList.add("is-fit");
      stageScale.style.transform = "";
      stageScale.style.width = "";
      stageScale.style.height = "";
      readout.textContent = "fit";
      return;
    }
    stageScale.classList.remove("is-fit");
    const preset = VIEWPORTS.find((v) => v.id === state.viewportId);
    const w = state.viewportId === "custom" ? state.custom.w : preset.w;
    const h = state.viewportId === "custom" ? state.custom.h : preset.h;
    stageScale.style.width = `${w}px`;
    stageScale.style.height = `${h}px`;

    const availW = stageViewport.clientWidth - 4;
    const availH = stageViewport.clientHeight - 4;
    const scale = Math.min(1, availW / w, availH / h);
    stageScale.style.transform = `scale(${scale})`;
    readout.textContent = `${w}×${h} @ ${Math.round(scale * 100)}%`;
  }

  $("customForm").addEventListener("submit", (event) => {
    event.preventDefault();
    const w = parseInt($("customW").value, 10) || state.custom.w;
    const h = parseInt($("customH").value, 10) || state.custom.h;
    setViewport("custom", { w, h });
  });

  window.addEventListener("resize", () => {
    if (state.viewportId !== "fit") applyViewport();
  });

  document.addEventListener("keydown", (event) => {
    const tag = (event.target && event.target.tagName) || "";
    if (tag === "INPUT" || tag === "TEXTAREA") return;
    const idx = ALL_OPTIONS.findIndex((o) => o.id === state.optionId);
    if (event.key === "ArrowDown" || event.key === "j") {
      event.preventDefault();
      selectOption(ALL_OPTIONS[Math.min(ALL_OPTIONS.length - 1, idx + 1)].id);
    } else if (event.key === "ArrowUp" || event.key === "k") {
      event.preventDefault();
      selectOption(ALL_OPTIONS[Math.max(0, idx - 1)].id);
    }
  });

  renderRegistry();
  renderPresets();
  frame.src = `frame.html#${state.optionId}`;
  applyViewport();
  checkBuilt();
})();
