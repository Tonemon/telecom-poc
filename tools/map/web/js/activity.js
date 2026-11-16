// Random activity (one shot) and Realtime activity (keeps going until switched off). The air
// does the deciding; this only asks.

import * as api from "./api.js";
import { page, redraw } from "./main.js";
import { model, refreshWorld } from "./state.js";
import { showMessage } from "./move.js";

const esc = s => String(s).replace(/[&<>"']/g, ch => `&#${ch.charCodeAt(0)};`);

function render() {
  const on = !!model.world?.activity?.realtime;
  const btn = document.getElementById("randomBtn"), box = document.getElementById("realtime");
  if (!btn) return;
  btn.disabled = on;
  btn.title = on ? "Realtime activity is on" : "Every device either stays or gets a random trip";
  box.checked = on;
}

async function random() {
  const r = await api.send("air", "POST", "/activity/random");
  if (!r.ok) { showMessage(`<span class="error">${esc(r.json?.error || r.status)}</span>`); return; }
  page.lastCommand.random = r.command;
  const trips = r.json.plans.map(p => p.action === "stay" ? `${esc(p.ue)} stays` : `${esc(p.ue)} → (${p.x}, ${p.y}) at ${p.speed_kmh} km/h`);
  showMessage(`Random activity: ${trips.join(" · ")}`);
  await refreshWorld();
  redraw();
}

async function realtime(on) {
  const r = await api.send("air", "PUT", "/activity/realtime", { on });
  if (!r.ok) { showMessage(`<span class="error">${esc(r.json?.error || r.status)}</span>`); return; }
  page.lastCommand.realtime = r.command;
  if (!on && r.json.stopped.length) showMessage(`Realtime activity off: stopped ${r.json.stopped.map(esc).join(", ")} where they are.`);
  await refreshWorld();
  redraw();
}

export function init() {
  const controls = document.getElementById("controls");
  controls.insertAdjacentHTML("beforeend", `
    <button id="randomBtn">Random activity</button>
    <label class="toggle"><input type="checkbox" id="realtime"> Realtime activity</label>`);
  document.getElementById("randomBtn").addEventListener("click", random);
  document.getElementById("realtime").addEventListener("change", ev => realtime(ev.target.checked));
  page.onRedraw.push(render);
}
