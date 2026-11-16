// Map files: download the current layout, upload one (or pick a preset) to restore it, and
// make new terrain. A file holds the terrain seed and the positions of towers and devices; not
// travel, shielding or automation.

import * as api from "./api.js";
import { page, redraw, setSeed } from "./main.js";
import { showMessage } from "./move.js";
import { model, refreshWorld } from "./state.js";

export const VERSION = 1;
const PRESETS = [["highway", "Highway"], ["tunnel", "Tunnel"], ["crowded-cell", "Crowded cell"]];
const esc = s => String(s).replace(/[&<>"']/g, ch => `&#${ch.charCodeAt(0)};`);

export function snapshot(world, seed) {
  return {
    version: VERSION,
    terrain: { seed },
    cells: world.cells.map(c => ({ id: c.id, x: c.x, y: c.y })),
    ues: world.ues.map(u => ({ id: u.id, x: +u.x.toFixed(3), y: +u.y.toFixed(3) })),
  };
}

// validate checks a parsed map file against the running world before anything moves.
export function validate(file, world) {
  const errors = [];
  if (!file || typeof file !== "object") return { ok: false, errors: ["not a map file"] };
  if (file.version !== VERSION) errors.push(`unsupported version ${JSON.stringify(file.version)} (this map reads version ${VERSION})`);
  const num = v => typeof v === "number" && Number.isFinite(v);
  for (const [kind, list, known] of [["cell", file.cells, world.cells], ["device", file.ues, world.ues]]) {
    if (!Array.isArray(list)) { errors.push(`${kind}s missing`); continue; }
    for (const item of list) {
      if (!known.some(k => k.id === item?.id)) errors.push(`unknown ${kind} ${JSON.stringify(item?.id)}`);
      else if (!num(item.x) || !num(item.y)) errors.push(`${kind} ${item.id}: x and y must be numbers`);
    }
  }
  if (file.terrain && file.terrain.seed !== undefined && !num(file.terrain.seed)) errors.push("terrain seed must be a number");
  return { ok: errors.length === 0, errors };
}

function download() {
  const blob = new Blob([JSON.stringify(snapshot(model.world, page.seed), null, 2)], { type: "application/json" });
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = `telecom-poc-map-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, "-")}.json`;
  a.click();
  URL.revokeObjectURL(a.href);
}

let pending = null;

function propose(file, name) {
  const v = validate(file, model.world);
  if (!v.ok) {
    showMessage(`<span class="error">Cannot use ${esc(name)}: ${v.errors.map(esc).join("; ")}. Nothing was moved.</span>`);
    return;
  }
  pending = file;
  const lines = [...file.cells.map(c => `tower ${esc(c.id)} → (${c.x}, ${c.y})`), ...file.ues.map(u => `${esc(u.id)} → (${u.x}, ${u.y})`)];
  document.getElementById("bottom").innerHTML = `<div class="panel">
    <div>Apply <b>${esc(name)}</b>? Towers move and devices jump right away:</div>
    <div class="small">${lines.join(" · ")}</div>
    <button class="primary" data-apply-map>Apply</button> <button data-cancel>Cancel</button></div>`;
}

async function apply() {
  const file = pending;
  pending = null;
  document.getElementById("bottom").innerHTML = "";
  const failed = [];
  for (const c of file.cells) {
    const r = await api.send("air", "PUT", `/cells/${encodeURIComponent(c.id)}/position`, { x: c.x, y: c.y });
    if (!r.ok) failed.push(`tower ${c.id}: ${r.json?.error || r.status}`);
  }
  for (const u of file.ues) {
    const r = await api.send("air", "PUT", `/ues/${encodeURIComponent(u.id)}/position`, { x: u.x, y: u.y });
    if (!r.ok) failed.push(`${u.id}: ${r.json?.error || r.status}`);
  }
  if (file.terrain?.seed !== undefined) setSeed(file.terrain.seed);
  if (failed.length) showMessage(`<span class="error">${failed.map(esc).join("; ")}</span>`);
  await refreshWorld();
  redraw();
}

export function init() {
  document.getElementById("controls").insertAdjacentHTML("afterbegin", `
    <select id="preset"><option value="">Presets…</option>${PRESETS.map(([v, n]) => `<option value="${v}">${n}</option>`).join("")}</select>
    <button id="mapDownload" title="Save terrain and positions as a file">Download map</button>
    <label class="filebtn" title="Restore terrain and positions from a file"><input type="file" id="mapUpload" accept=".json,application/json" hidden>Upload map</label>
    <button id="newTerrain" title="Generate different (fictional) terrain">New terrain</button>`);
  document.getElementById("mapDownload").addEventListener("click", download);
  document.getElementById("newTerrain").addEventListener("click", () => { setSeed(Math.floor(Math.random() * 1e9)); redraw(); });
  document.getElementById("mapUpload").addEventListener("change", async ev => {
    const f = ev.target.files[0];
    ev.target.value = "";
    if (!f) return;
    let file;
    try { file = JSON.parse(await f.text()); } catch { showMessage(`<span class="error">${esc(f.name)} is not JSON.</span>`); return; }
    propose(file, f.name);
  });
  document.getElementById("preset").addEventListener("change", async ev => {
    const name = ev.target.value;
    ev.target.value = "";
    if (!name) return;
    const file = await (await fetch(`presets/${name}.json`)).json();
    propose(file, PRESETS.find(p => p[0] === name)[1] + " preset");
  });
  document.addEventListener("click", ev => { if (ev.target.dataset.applyMap !== undefined) apply(); });
}
