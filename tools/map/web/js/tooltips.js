// "What's this?" tooltips: any element with data-term shows that term's glossary entry.

let glossary = {};
const box = document.createElement("div");
box.id = "tooltip";
box.hidden = true;

const esc = s => String(s).replace(/[&<>"']/g, ch => `&#${ch.charCodeAt(0)};`);

// term("PCI") is a label that explains itself on hover.
export function term(name, text = name) {
  return glossary[name] ? `<span class="term" data-term="${esc(name)}">${esc(text)}</span>` : esc(text);
}

export async function init() {
  document.body.appendChild(box);
  try { glossary = await (await fetch("glossary.json")).json(); } catch { glossary = {}; }
  document.addEventListener("mouseover", ev => {
    const el = ev.target.closest?.("[data-term]");
    if (!el) { box.hidden = true; return; }
    const g = glossary[el.dataset.term];
    if (!g) return;
    box.innerHTML = `<b>${esc(el.dataset.term)}</b> ${esc(g.text)}<div class="see">${esc(g.see)}</div>`;
    const r = el.getBoundingClientRect();
    box.hidden = false;
    box.style.left = `${Math.min(window.innerWidth - box.offsetWidth - 8, r.left)}px`;
    box.style.top = `${r.bottom + 6}px`;
  });
}
