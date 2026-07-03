// Shared, dependency-free DOM builders for LAB-001 options.
// Every option file (options/opt-N.js) may call these; keep additions here,
// not duplicated per-option.
(function () {
  "use strict";

  // el(tag, attrs, children) — minimal hyperscript-ish DOM builder.
  // attrs: className via `class`, event handlers via `onclick` etc, else
  // set as attributes. children: string | Node | array of either, nulls skipped.
  function el(tag, attrs, children) {
    const node = document.createElement(tag);
    for (const [key, value] of Object.entries(attrs || {})) {
      if (value == null || value === false) continue;
      if (key === "class") node.className = value;
      else if (key.startsWith("on") && typeof value === "function") {
        node.addEventListener(key.slice(2).toLowerCase(), value);
      } else {
        node.setAttribute(key, value);
      }
    }
    const list = Array.isArray(children) ? children : children == null ? [] : [children];
    for (const child of list) {
      if (child == null) continue;
      node.appendChild(child instanceof Node ? child : document.createTextNode(String(child)));
    }
    return node;
  }

  // captionBand(text) — monospace uppercase heading band for the top of a panel.
  function captionBand(text) {
    return el("div", { class: "bb-caption-band" }, [text]);
  }

  // proofStrip(cells) — bordered row of real plane facts.
  // cells: [{ label, value }]
  function proofStrip(cells) {
    const strip = el("div", { class: "bb-proof-strip" });
    for (const cell of cells) {
      strip.appendChild(
        el("span", {}, [
          el("b", {}, [cell.label]),
          el("em", { title: String(cell.value) }, [cell.value]),
        ])
      );
    }
    return strip;
  }

  // stat(label, value, context) — a single summary metric card.
  // context is optional muted subtext (string or Node, e.g. a meter element).
  function stat(label, value, context) {
    const card = el("article", { class: "bb-stat" }, [
      el("span", { class: "bb-chrome" }, [label]),
      el("strong", { class: "bb-stat-value" }, [value]),
    ]);
    if (context != null) card.appendChild(el("p", { class: "bb-stat-context" }, [context]));
    return card;
  }

  // dataTable({ cols, rows, page, pageSize, emptyText })
  // cols: [{ key, label, align: 'right'?, render(row) -> string|Node }]
  // Built-in pager: only rendered when more than one page exists.
  function dataTable(opts) {
    const cols = opts.cols || [];
    const rows = opts.rows || [];
    const pageSize = opts.pageSize || 10;
    const emptyText = opts.emptyText || "no data";
    let currentPage = opts.page || 0;
    const pageCount = Math.max(1, Math.ceil(rows.length / pageSize));

    const wrap = el("div", { class: "bb-table-wrap" });
    const headRow = el("tr");
    for (const col of cols) {
      headRow.appendChild(el("th", { class: col.align === "right" ? "right" : "" }, [col.label]));
    }
    const tbody = el("tbody");
    const table = el("table", { class: "bb-table" }, [el("thead", {}, [headRow]), tbody]);
    wrap.appendChild(table);

    const info = el("span", { class: "bb-pager-info" });
    const prevBtn = el("button", { class: "bb-quiet", type: "button" }, ["prev"]);
    const nextBtn = el("button", { class: "bb-quiet", type: "button" }, ["next"]);
    const pager = el("div", { class: "bb-pager" }, [prevBtn, info, nextBtn]);

    function renderPage() {
      tbody.innerHTML = "";
      const start = currentPage * pageSize;
      const slice = rows.slice(start, start + pageSize);
      if (!slice.length) {
        tbody.appendChild(
          el("tr", {}, [el("td", { class: "bb-empty", colspan: String(Math.max(1, cols.length)) }, [emptyText])])
        );
      } else {
        for (const row of slice) {
          const tr = el("tr");
          for (const col of cols) {
            const value = col.render ? col.render(row) : row[col.key] == null ? "-" : row[col.key];
            tr.appendChild(el("td", { class: col.align === "right" ? "right" : "" }, [value]));
          }
          tbody.appendChild(tr);
        }
      }
      info.textContent = `${currentPage + 1} / ${pageCount}`;
      prevBtn.disabled = currentPage === 0;
      nextBtn.disabled = currentPage >= pageCount - 1;
    }

    prevBtn.addEventListener("click", () => {
      if (currentPage > 0) {
        currentPage -= 1;
        renderPage();
      }
    });
    nextBtn.addEventListener("click", () => {
      if (currentPage < pageCount - 1) {
        currentPage += 1;
        renderPage();
      }
    });

    renderPage();
    if (pageCount > 1) wrap.appendChild(pager);
    return wrap;
  }

  // statusBar(cells) — thin one-line strip. cells: array of strings/Nodes,
  // rendered separated by a middle dot.
  function statusBar(cells) {
    const bar = el("div", { class: "bb-status-bar" });
    cells.forEach((cell, index) => {
      if (index > 0) bar.appendChild(el("span", { class: "bb-status-sep" }, ["·"]));
      bar.appendChild(el("span", {}, [cell]));
    });
    return bar;
  }

  // markSVG(size) — original 5-petal flower placeholder, currentColor.
  // Five ellipses rotated 72deg apart around a center dot; no external asset.
  function markSVG(size) {
    const ns = "http://www.w3.org/2000/svg";
    const svg = document.createElementNS(ns, "svg");
    svg.setAttribute("viewBox", "0 0 24 24");
    svg.setAttribute("width", String(size || 20));
    svg.setAttribute("height", String(size || 20));
    svg.setAttribute("fill", "none");
    svg.setAttribute("stroke", "currentColor");
    svg.setAttribute("stroke-width", "1.3");
    svg.setAttribute("aria-hidden", "true");
    for (let i = 0; i < 5; i += 1) {
      const petal = document.createElementNS(ns, "ellipse");
      petal.setAttribute("cx", "12");
      petal.setAttribute("cy", "6.6");
      petal.setAttribute("rx", "2.1");
      petal.setAttribute("ry", "4.6");
      petal.setAttribute("transform", `rotate(${i * 72} 12 12)`);
      svg.appendChild(petal);
    }
    const center = document.createElementNS(ns, "circle");
    center.setAttribute("cx", "12");
    center.setAttribute("cy", "12");
    center.setAttribute("r", "1.5");
    center.setAttribute("fill", "currentColor");
    center.setAttribute("stroke", "none");
    svg.appendChild(center);
    return svg;
  }

  const PARTS = { el, captionBand, proofStrip, stat, dataTable, statusBar, markSVG };
  // Namespaced (window.BB_PARTS.el(...)) and flat (window.el(...)) — option
  // files can use whichever reads better without an import step.
  window.BB_PARTS = PARTS;
  Object.assign(window, PARTS);
})();
