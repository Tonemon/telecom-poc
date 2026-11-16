// Moving things: send a UE somewhere (click a destination, pick a time-lapse speed), and drag a
// tower (ghost preview, then a confirmation listing who gains or loses coverage).

import * as api from "./api.js";
import { actionBuilders } from "./cards.js";
import { EXTENT, label } from "./grid.js";
import { getView, page, redraw } from "./main.js";
import { cmdButton, remember } from "./commands.js";
import { cellColor, model, refreshWorld } from "./state.js";

const SPEED_MIN = 300, SPEED_MAX = 1500, SPEED_DEFAULT = 1000;
const esc = s => String(s).replace(/[&<>"']/g, ch => `&#${ch.charCodeAt(0)};`);
const bottom = () => document.getElementById("bottom");

let pick = null;   // {ue, mode: "travel"|"jump"} while choosing a destination; plus {x, y} once chosen
let drag = null;   // {cell, x, y, startX, startY, moved}

function realMinutes(km, speed) {
  const sp = model.world?.radio?.speed || 0.1;
  return (km / speed) * 3600 / sp / 60;
}

function inside(x, y) {
  return x >= EXTENT.xmin && x <= EXTENT.xmax && y >= EXTENT.ymin && y <= EXTENT.ymax;
}

export function showMessage(html) {
  bottom().innerHTML = `<div class="panel">${html}<button data-close>Close</button></div>`;
}

function travelPanel() {
  const ue = model.world.ues.find(u => u.id === pick.ue);
  const km = Math.hypot(pick.x - ue.x, pick.y - ue.y);
  bottom().innerHTML = `<div class="panel">
    <div><b>${esc(pick.ue)}</b> → (${pick.x.toFixed(1)}, ${pick.y.toFixed(1)}) km · ${label(pick.x, pick.y)} · ${km.toFixed(1)} km</div>
    <label>Speed <input id="speed" type="range" min="${SPEED_MIN}" max="${SPEED_MAX}" step="50" value="${SPEED_DEFAULT}">
      <output id="speedOut"></output></label>
    <div class="muted small">Time-lapse speed: the radio runs slower than real time, so the radio can only follow up to ${SPEED_MAX} km/h.</div>
    <button class="primary" data-send>Send</button> <button data-cancel>Cancel</button></div>`;
  const update = () => {
    const s = Number(document.getElementById("speed").value);
    document.getElementById("speedOut").textContent = `${s} km/h · ≈ ${realMinutes(km, s).toFixed(1)} min real`;
  };
  document.getElementById("speed").addEventListener("input", update);
  update();
}

function cancel() {
  pick = null; drag = null;
  bottom().innerHTML = "";
  redraw();
}

async function sendTravel() {
  const speed = Number(document.getElementById("speed").value);
  const ue = pick.ue;
  const r = await api.send("air", "POST", `/ues/${encodeURIComponent(ue)}/travel`, { x: +pick.x.toFixed(2), y: +pick.y.toFixed(2), speed_kmh: speed });
  pick = null; // the trip is sent: later map clicks are ordinary clicks again
  if (!r.ok) showMessage(`<span class="error">${esc(r.json?.error || r.status)}</span>`);
  else { remember(`travel:${ue}`, r.command); bottom().innerHTML = ""; }
  await refreshWorld();
  redraw();
}

async function towerDropped() {
  const d = drag;
  const r = await api.send("air", "PUT", `/cells/${encodeURIComponent(d.cell)}/position?dry_run=1`, { x: +d.x.toFixed(2), y: +d.y.toFixed(2) });
  if (!r.ok) { showMessage(`<span class="error">${esc(r.json?.error || r.status)}</span>`); drag = null; return; }
  const effects = r.json.ues.filter(u => u.effect !== "none")
    .map(u => `${esc(u.id)} will ${u.effect === "loses" ? "<b>lose</b>" : "gain"} coverage`);
  bottom().innerHTML = `<div class="panel">
    <div>Move <b>cell ${esc(d.cell)}</b> to (${d.x.toFixed(1)}, ${d.y.toFixed(1)}) km · ${label(d.x, d.y)}?</div>
    <div>${effects.length ? effects.join("<br>") : "No device gains or loses coverage."}</div>
    <div class="muted small">Towers are heavy: this changes every device's signal right away.</div>
    <button class="primary" data-confirm>Move tower</button> <button data-cancel>Cancel</button></div>`;
}

async function confirmTower() {
  const d = drag;
  const r = await api.send("air", "PUT", `/cells/${encodeURIComponent(d.cell)}/position`, { x: +d.x.toFixed(2), y: +d.y.toFixed(2) });
  drag = null;
  bottom().innerHTML = "";
  if (!r.ok) showMessage(`<span class="error">${esc(r.json?.error || r.status)}</span>`);
  else remember(`cell:${d.cell}`, r.command);
  await refreshWorld();
  redraw();
}

function overlay(v) {
  let out = "";
  if (pick && pick.x != null && model.world) {
    const ue = model.world.ues.find(u => u.id === pick.ue);
    const [ax, ay] = v.toScreen(ue.x, ue.y), [bx, by] = v.toScreen(pick.x, pick.y);
    out += `<line class="route" x1="${ax}" y1="${ay}" x2="${bx}" y2="${by}"/>
      <path class="route" d="M${bx - 6},${by - 6} L${bx + 6},${by + 6} M${bx - 6},${by + 6} L${bx + 6},${by - 6}"/>`;
  }
  if (drag && drag.moved) {
    const [px, py] = v.toScreen(drag.x, drag.y);
    const r = (model.world?.pathloss?.edge_km || 7) * v.scale;
    out += `<g class="ghost"><circle cx="${px}" cy="${py}" r="${r}" fill="${cellColor(drag.cell)}" fill-opacity="0.15"
        stroke="${cellColor(drag.cell)}" stroke-dasharray="6 4"/>
      <path transform="translate(${px},${py})" d="M0,-18 L-9,10 L9,10 Z" fill="${cellColor(drag.cell)}" stroke="#111827"/></g>`;
  }
  return out;
}

function worldAt(ev) {
  const r = document.getElementById("map").getBoundingClientRect();
  return getView().fromScreen(ev.clientX - r.left, ev.clientY - r.top);
}

// startPick lets the next map click choose where a UE goes: travel (then a speed) or jump.
export function startPick(ue, mode = "travel") {
  pick = { ue, mode };
  showMessage(`Click ${mode === "jump" ? "where to put" : "a destination for"} <b>${esc(ue)}</b> on the map (Esc cancels).`);
}

async function jump(ue, x, y) {
  const r = await api.send("air", "PUT", `/ues/${encodeURIComponent(ue)}/position`, { x: +x.toFixed(2), y: +y.toFixed(2) });
  pick = null;
  if (!r.ok) showMessage(`<span class="error">${esc(r.json?.error || r.status)}</span>`);
  else { remember(`jump:${ue}`, r.command); bottom().innerHTML = ""; }
  await refreshWorld();
  redraw();
}

export function init() {
  page.extras.push(overlay);
  actionBuilders.ue.push(id => `<div class="actions"><button data-send-ue="${esc(id)}">Send somewhere…</button>${cmdButton(`travel:${id}`)}</div>`);

  document.addEventListener("click", ev => {
    const t = ev.target;
    if (t.dataset.sendUe) startPick(t.dataset.sendUe, "travel");
    else if (t.dataset.send !== undefined) sendTravel();
    else if (t.dataset.confirm !== undefined) confirmTower();
    else if (t.dataset.cancel !== undefined || t.dataset.close !== undefined) cancel();
  });
  document.addEventListener("keydown", ev => { if (ev.key === "Escape") cancel(); });

  const svg = document.getElementById("overlay");
  svg.addEventListener("mousedown", ev => {
    const g = ev.target.closest("[data-cell]");
    if (!g || pick || drag?.confirming) return; // one tower move at a time
    const [x, y] = worldAt(ev);
    drag = { cell: g.dataset.cell, x, y, startX: ev.clientX, startY: ev.clientY, moved: false };
    ev.preventDefault();
  });
  window.addEventListener("mousemove", ev => {
    if (!drag || drag.confirming) return;
    if (Math.hypot(ev.clientX - drag.startX, ev.clientY - drag.startY) > 4) drag.moved = true;
    [drag.x, drag.y] = worldAt(ev);
    if (drag.moved) redraw();
  });
  window.addEventListener("mouseup", () => {
    if (!drag || drag.confirming) return;
    if (!drag.moved || !inside(drag.x, drag.y)) { drag = null; redraw(); return; }
    drag.confirming = true;
    towerDropped();
  });

  // A click on the map (not on a tower or UE) picks the destination.
  page.onMapClick.push(ev => {
    if (!pick || ev.target.closest("[data-ue],[data-cell]")) return false;
    const [x, y] = worldAt(ev);
    if (!inside(x, y)) return true;
    if (pick.mode === "jump") { jump(pick.ue, x, y); return true; }
    pick = { ue: pick.ue, mode: "travel", x, y };
    travelPanel();
    redraw();
    return true;
  });
}
