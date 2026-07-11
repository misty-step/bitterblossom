const OPTIONS = [
  { id:"BASE-1", group:"Baseline", lane:"Current", label:"Shipped noir-ledger reconstruction", move:"Dense task-and-table cockpit; baseline only." },
  { id:"ANTH-2", group:"Candidates", lane:"Anthropic", label:"Topology route map", move:"Configured workflows become navigable routes whose stops are agents and outcomes." },
  { id:"TASTE-1", group:"Candidates", lane:"Taste", label:"Signal Ledger", move:"A dense master-detail ledger makes the workflow roster permanent navigation." },
  { id:"TASTE-2", group:"Candidates", lane:"Taste", label:"Topology Board", move:"A horizontal workflow board divides configuration from selected-instance evidence." },
  { id:"TASTE-3", group:"Candidates", lane:"Taste", label:"Operator Dossier", move:"A compact casefile index pairs with a vertical graph and folio-edge navigation." },
  { id:"MIN-1", group:"Candidates", lane:"Minimal", label:"The Operating Dossier", move:"A persistent roster becomes the book spine for one editorial operating dossier." },
  { id:"MIN-2", group:"Candidates", lane:"Minimal", label:"The Workflow Folio", move:"The roster opens the page, the graph is the stage, and navigation moves to a bottom dock." },
  { id:"MIN-3", group:"Candidates", lane:"Minimal", label:"The Continuous Ledger", move:"One uninterrupted ruled audit surface replaces card-based master-detail." },
  { id:"BRUT-1", group:"Candidates", lane:"Brutalist", label:"Redline control ledger", move:"A ledger rail, three-column inspection, and evidence register separate plural state." },
  { id:"BRUT-2", group:"Candidates", lane:"Brutalist", label:"Foldout operations manual", move:"The configured roster unfolds into topology and a telemetry specification column." },
  { id:"BRUT-3", group:"Candidates", lane:"Brutalist", label:"Orthographic command table", move:"A right-edge command strip frames the stable graph as a coordinate plane." },
  { id:"HALL-2", group:"Candidates", lane:"Hallmark", label:"Workflow Strips", move:"The roster itself expands into topology and run evidence, removing a separate detail page." },
  { id:"HALL-3", group:"Candidates", lane:"Hallmark", label:"Evidence Folio", move:"The configured graph becomes the page while run state appears as marginal annotation." },
  { id:"IMPEC-1", group:"Candidates", lane:"Impeccable", label:"Three-pane control room", move:"Roster, configured topology, and live evidence occupy simultaneous scan lanes." },
  { id:"IMPEC-2", group:"Candidates", lane:"Impeccable", label:"Workflow dossier", move:"A persistent horizontal workflow index opens one operational dossier." },
  { id:"IMPEC-3", group:"Candidates", lane:"Impeccable", label:"Graph-first evidence workbench", move:"The stable graph sits between roster selection and a live evidence drawer." }
];

const nav=document.querySelector("#options"), frame=document.querySelector("#frame"), viewport=document.querySelector("#viewport"), counter=document.querySelector("#counter"), width=document.querySelector("#width"), height=document.querySelector("#height"), preset=document.querySelector("#preset"), scaleOut=document.querySelector("#scale");
let selected=localStorage.getItem("bb-lab-003-option")||"BASE-1";

function buildNav(){let group="";nav.replaceChildren();OPTIONS.forEach(o=>{if(o.group!==group){group=o.group;const h=document.createElement("div");h.className="group";h.textContent=group;nav.append(h)}const b=document.createElement("button");b.className=`option ${o.id==="BASE-1"?"baseline":""}`;b.dataset.id=o.id;b.innerHTML=`<span class="badge">${o.id} · ${o.lane}</span><strong>${o.label}</strong><small>${o.move}</small>`;b.addEventListener("click",()=>select(o.id));nav.append(b)});}
function select(id){selected=OPTIONS.some(o=>o.id===id)?id:"BASE-1";localStorage.setItem("bb-lab-003-option",selected);frame.src=`frame.html?v=3#${selected}`;document.querySelectorAll(".option").forEach(b=>b.toggleAttribute("aria-current",b.dataset.id===selected));const i=OPTIONS.findIndex(o=>o.id===selected);counter.textContent=`${i+1} / ${OPTIONS.length}`;document.querySelector(`[data-id="${selected}"]`)?.scrollIntoView({block:"nearest"});}
function fit(){const stage=document.querySelector(".stage").getBoundingClientRect();const w=Number(width.value),h=Number(height.value);const s=Math.min((stage.width-32)/w,(stage.height-32)/h,1);viewport.style.width=`${w}px`;viewport.style.height=`${h}px`;viewport.style.transform=`scale(${s})`;scaleOut.value=`${Math.round(s*100)}%`;}
function applyPreset(){if(preset.value==="fit"){const stage=document.querySelector(".stage").getBoundingClientRect();width.value=Math.max(320,Math.floor(stage.width-32));height.value=Math.max(480,Math.floor(stage.height-32));}else if(preset.value!=="custom"){[width.value,height.value]=preset.value.split("x")}localStorage.setItem("bb-lab-003-viewport",JSON.stringify({preset:preset.value,w:width.value,h:height.value}));fit();}
preset.addEventListener("change",applyPreset);[width,height].forEach(el=>el.addEventListener("input",()=>{preset.value="custom";fit()}));addEventListener("resize",fit);addEventListener("keydown",e=>{if(!["ArrowDown","ArrowUp"].includes(e.key)||/INPUT|SELECT/.test(document.activeElement.tagName))return;e.preventDefault();const i=OPTIONS.findIndex(o=>o.id===selected),d=e.key==="ArrowDown"?1:-1;select(OPTIONS[(i+d+OPTIONS.length)%OPTIONS.length].id)});
const saved=JSON.parse(localStorage.getItem("bb-lab-003-viewport")||"null");if(saved){preset.value=saved.preset;width.value=saved.w;height.value=saved.h}buildNav();select(selected);requestAnimationFrame(applyPreset);
