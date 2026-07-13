import{CORPUS}from'./data.js?v=5';
import{SPECS as baseline}from'./lanes/baseline.js?v=5';
import{SPECS as anthropic}from'./lanes/anthropic.js?v=5';
import{SPECS as minimal}from'./lanes/minimal.js?v=5';
import{SPECS as brutal}from'./lanes/brutal.js?v=5';
import{SPECS as apple}from'./lanes/apple.js?v=5';

const SPECS=Object.assign({},baseline,anthropic,minimal,brutal,apple),mount=document.querySelector('#mount');
let currentOption='BASE-1';
function optionFromHash(){const id=location.hash.slice(1);return SPECS[id]?id:currentOption}
function setMode(next){const root=document.documentElement;root.classList.remove('light','dark');root.classList.add(next);root.dataset.aeMode=next;root.style.colorScheme=next;try{localStorage.setItem('bb-lab-004-mode',next)}catch{}}
function toggleMode(){const root=document.documentElement,resolved=root.classList.contains('dark')||(!root.classList.contains('light')&&matchMedia('(prefers-color-scheme:dark)').matches)?'dark':'light';setMode(resolved==='dark'?'light':'dark')}
function render(){currentOption=optionFromHash();const spec=SPECS[currentOption]||SPECS['BASE-1'],template=document.createElement('template');template.innerHTML=spec.render(CORPUS);template.content.querySelectorAll('script,link[rel="stylesheet"]').forEach(node=>node.remove());mount.replaceChildren(template.content.cloneNode(true));document.title=`${currentOption} · ${spec.label}`}
addEventListener('hashchange',()=>{if(SPECS[location.hash.slice(1)])render()});
document.addEventListener('click',event=>{const toggle=event.target.closest('[data-theme-toggle]');if(toggle){event.preventDefault();toggleMode();return}const viewLink=event.target.closest('[data-view-link]');if(viewLink){event.preventDefault();const view=viewLink.dataset.viewLink;document.querySelectorAll('[data-view-link]').forEach(link=>link.classList.toggle('is-active',link===viewLink));document.querySelectorAll('[data-view]').forEach(panel=>panel.hidden=panel.dataset.view!==view);return}const anchor=event.target.closest('a[href^="#"]');if(anchor){event.preventDefault();const target=document.getElementById(anchor.getAttribute('href').slice(1));target?.scrollIntoView({behavior:matchMedia('(prefers-reduced-motion:reduce)').matches?'auto':'smooth',block:'start'})}});
render();
