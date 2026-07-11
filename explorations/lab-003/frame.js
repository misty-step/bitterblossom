import { CORPUS } from "./data.js?v=3";
import { SPECS as baseline } from "./lanes/baseline.js?v=3";
import { SPECS as anthropic } from "./lanes/anthropic.js?v=3";
import { SPECS as taste } from "./lanes/taste.js?v=3";
import { SPECS as minimal } from "./lanes/minimal.js?v=3";
import { SPECS as brutal } from "./lanes/brutal.js?v=3";
import { SPECS as hallmark } from "./lanes/hallmark.js?v=3";
import { SPECS as impec } from "./lanes/impec.js?v=3";

const SPECS = Object.assign({}, baseline, anthropic, taste, minimal, brutal, hallmark, impec);
const mount = document.querySelector("#mount");

function currentId() {
  return location.hash.slice(1) || "BASE-1";
}

function render() {
  const id = currentId();
  const spec = SPECS[id] || SPECS["BASE-1"];
  mount.replaceChildren();
  const host = document.createElement("div");
  host.innerHTML = spec.render(CORPUS);
  mount.append(...host.childNodes);
  document.title = `${id} · ${spec.label}`;
  document.querySelectorAll("a[href='#']").forEach(link => link.addEventListener("click", event => event.preventDefault()));
  document.querySelectorAll("[data-theme-toggle]").forEach(button => button.addEventListener("click", () => {
    const root = document.documentElement;
    const resolved = root.dataset.theme || (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
    root.dataset.theme = resolved === "dark" ? "light" : "dark";
  }));
  document.querySelectorAll("[data-view]").forEach(button => button.addEventListener("click", () => {
    const view = button.dataset.view;
    document.querySelectorAll("[data-view]").forEach(item => item.toggleAttribute("aria-current", item === button));
    document.querySelectorAll("[data-panel]").forEach(panel => panel.hidden = panel.dataset.panel !== view);
  }));
}

addEventListener("hashchange", render);
render();
