// Power: switch a phone or a tower off and on, the way it happens in real life. The buttons only
// act on the device (through the agent: stop, kill, cut its network, start); whatever the core
// learns, it learns from the device over the radio, over S1, or by noticing on its own.

import * as api from "./api.js";
import { actionBuilders } from "./cards.js";
import { cmdButton, remember } from "./commands.js";
import { page, redraw } from "./main.js";
import { powerButtons } from "./logic.js";
import { deviceOf, model, towerOf } from "./state.js";

const esc = s => String(s).replace(/[&<>"']/g, ch => `&#${ch.charCodeAt(0)};`);

// What happened, in plain words: what the device sent, and how the network finds out.
const DONE = {
  switch_off: s => `${s} switched off: it sent a detach to the network as it shut down, so the core removed it at once.`,
  pull_battery: s => `Battery pulled: ${s} sent nothing. The tower notices the radio link is gone and releases it; the core keeps it registered until its timers run out: unreachable about 4.5 minutes after the tower lets go (T3412 + 4 min), detached about 4.5 minutes after that.`,
  switch_on: s => `${s} is starting: it searches for a cell and attaches from scratch (with a capture running, that is the textbook attach).`,
  shut_down: s => `${s} shut down: it closed its S1 link to the core cleanly. Its phones lose the signal and look for another cell.`,
  power_cut: s => `${s} lost power: it sent nothing at all. The core notices when its S1 heartbeats go unanswered (about half a minute); its phones lose the signal.`,
  power_on: s => `${s} is starting: it sets up S1 with the core again and phones in range find it.`,
};

const block = (kind, id) => `<h4>Power</h4><div id="power" data-kind="${kind}" data-id="${esc(id)}"></div><div id="power-result"></div>`;

function serviceOf(kind, id) {
  return kind === "cell" ? towerOf(id)?.service : deviceOf(id)?.service;
}

function render() {
  const el = document.getElementById("power");
  if (!el) return;
  const { kind, id } = el.dataset;
  const svc = serviceOf(kind, id);
  const comp = svc ? model.components[svc] : null;
  if (!comp) { el.innerHTML = `<span class="muted small">State unknown.</span>`; return; }
  const running = comp.state === "running";
  const key = `${kind}:${id}:${running}`;
  if (el.dataset.key === key) return; // unchanged: keep focus and the buttons as they are
  el.dataset.key = key;
  el.innerHTML = `<div class="actions">` + powerButtons(kind, running).map(b =>
    `<button data-power="${b.action}" data-svc="${esc(svc)}"${b.enabled ? "" : " disabled"}>${b.label}</button>${cmdButton(`power:${b.action}:${svc}`)}`
  ).join("") + `</div><div class="small muted">${running ? "On" : "Off"} (container ${esc(comp.state)}).</div>`;
}

async function press(btn) {
  const action = btn.dataset.power, svc = btn.dataset.svc;
  const out = document.getElementById("power-result");
  btn.disabled = true;
  try {
    const r = await api.send("agent", "POST", `/power/${encodeURIComponent(svc)}`, { action });
    if (r.ok) {
      remember(`power:${action}:${svc}`, r.json.command);
      if (out) out.innerHTML = `<div class="ok">${esc(DONE[action](svc))}</div>`;
      const comps = await api.get("agent", "/components").catch(() => null);
      if (comps) model.components = Object.fromEntries(comps.map(c => [c.service, c]));
    } else if (out) {
      out.innerHTML = `<div class="error">${esc(r.json?.error || `HTTP ${r.status}`)}</div>`;
    }
  } finally {
    const el = document.getElementById("power");
    if (el) delete el.dataset.key;
    redraw();
  }
}

export function init() {
  actionBuilders.cell.push(id => block("cell", id));
  actionBuilders.ue.push(id => block("ue", id));
  page.onRedraw.push(render);
  page.onSelect.push(render);
  document.addEventListener("click", ev => {
    const b = ev.target.closest("button[data-power]");
    if (b) press(b);
  });
}
