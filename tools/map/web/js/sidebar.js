// Sidebar lists: towers and devices with their live status; the top bar's instance and radio speed.

import { gainsAt } from "./pathloss.js";
import { cellColor, deviceOf, model, towerStatus, ueStatus } from "./state.js";

const esc = s => String(s).replace(/[&<>"']/g, ch => `&#${ch.charCodeAt(0)};`);

export function ueStatusText(ue) {
  const st = ueStatus(ue);
  if (st.down) return "off";
  if (st.noRadio) return `no radio (MME still says cell ${st.serving || "?"})`;
  if (!st.registered) return "no service";
  return `cell ${st.serving || "?"} · ${st.connected ? "connected" : "idle"}`;
}

export function renderLists(el, sel) {
  const w = model.world;
  if (!w) { el.innerHTML = `<p class="muted">Waiting for the air…</p>`; return; }
  const towers = w.cells.map(c => {
    const st = towerStatus(c);
    const served = w.ues.filter(u => ueStatus(u).serving === c.id).length;
    return `<li class="row${sel.kind === "cell" && sel.id === c.id ? " selected" : ""}" data-cell="${esc(c.id)}">
      <span class="swatch" style="background:${st.down ? "#9ca3af" : cellColor(c.id)}"></span>
      <span class="name">${esc(c.name || c.id)}</span>
      <span class="status">${st.down ? "off" : st.s1 && !st.s1.up ? "S1 down" : `${served} UE${served === 1 ? "" : "s"}`}</span></li>`;
  }).join("");
  const devices = w.ues.map(u => {
    const st = ueStatus(u), dev = deviceOf(u.id);
    const col = st.serving && !st.down ? cellColor(st.serving) : "#9ca3af";
    const auto = u.automation && u.automation !== "manual" ? ` <span class="tag">${esc(u.automation)}</span>` : "";
    return `<li class="row${sel.kind === "ue" && sel.id === u.id ? " selected" : ""}" data-ue="${esc(u.id)}">
      <span class="swatch round" style="background:${col}"></span>
      <span class="name">${esc(u.id)} <span class="muted">${esc(dev?.model || "")}</span>${auto}</span>
      <span class="status">${esc(ueStatusText(u))}</span></li>`;
  }).join("");
  el.innerHTML = `<h2>Towers</h2><ul class="list">${towers}</ul><h2>Devices</h2><ul class="list">${devices}</ul>`;
}

export function renderTop(el) {
  const sp = model.world?.radio?.speed;
  const errors = Object.entries(model.errors).map(([k, v]) => `${k}: ${v}`).join(" · ");
  el.innerHTML = `<strong>${esc(model.inventory?.name || "telecom-poc")}</strong>
    <span class="muted">${esc(model.inventory?.operator?.name || "")} ${esc(model.inventory?.operator?.plmn || "")}</span>
    <span class="radio">${sp ? `radio runs at ${sp.toFixed(2)}× real time` : "radio clock starting…"}</span>
    ${errors ? `<span class="error">${esc(errors)}</span>` : ""}`;
}

// ?debug=1: the air's gains next to the page's own path-loss values, which must match.
export function renderDebug(el) {
  const w = model.world;
  if (!w) return;
  el.innerHTML = `<h2>Debug: air vs page path loss</h2><table class="debug">` + w.ues.map(u => {
    const mine = gainsAt(u.x, u.y, w.cells, w.pathloss);
    return w.cells.map(c => {
      const a = u.gains[c.id], b = mine[c.id];
      const ok = u.shielded || Math.abs(a - b) < 0.0005;
      return `<tr class="${ok ? "" : "bad"}"><td>${esc(u.id)}→${esc(c.id)}</td><td>${a.toFixed(3)}</td><td>${b.toFixed(3)}</td></tr>`;
    }).join("");
  }).join("") + `</table>`;
}
