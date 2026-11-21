// The card of the selected tower or device: everything the instance knows about it.
// #card-info and the chart refresh every poll; #card-actions only when the selection changes,
// so forms in it keep their input.

import { signalChart } from "./chart.js";
import { label } from "./grid.js";
import { db } from "./pathloss.js";
import { coreView } from "./logic.js";
import { cellColor, deviceOf, model, towerOf, towerStatus, ueStatus } from "./state.js";
import { term } from "./tooltips.js";

const esc = s => String(s ?? "–").replace(/[&<>"']/g, ch => `&#${ch.charCodeAt(0)};`);
const hex = n => (n == null ? "–" : "0x" + n.toString(16).toUpperCase());
const row = (k, v) => `<tr><th>${k}</th><td>${v}</td></tr>`;
const fmtDb = g => (g > 0 ? `${db(g).toFixed(1)} dB` : "not heard");

export const actionBuilders = { cell: [], ue: [] }; // fn(id) -> HTML, from the action modules

export function margin() {
  const a3 = model.inventory?.cells?.[0]?.a3;
  return a3 ? a3.offset_db + a3.hysteresis_db : 3;
}

function towerInfo(id) {
  const cell = model.world?.cells.find(c => c.id === id);
  if (!cell) return `<p class="muted">Unknown tower ${esc(id)}</p>`;
  const t = towerOf(id) || {}, st = towerStatus(cell), op = model.inventory?.operator || {};
  const served = model.world.ues.filter(u => ueStatus(u).serving === id).map(u => u.id);
  const neigh = (t.neighbours || []).map(n => {
    const other = model.inventory.cells.find(c => c.eci === n.eci);
    return `${other ? "cell " + esc(other.id) : "?"} (ECI ${hex(n.eci)}, PCI ${esc(n.pci)})`;
  }).join(", ");
  const comp = model.components[t.service];
  return `<h3><span class="swatch" style="background:${st.down ? "#9ca3af" : cellColor(id)}"></span> ${esc(cell.name || id)}
      <span class="muted">${esc(t.generation)} · LTE eNodeB${st.down ? " · <b>off</b>" : ""}</span></h3>
    <table class="kv">
    ${row("Container", `${esc(t.service)} · ${esc(comp?.state || "unknown")}${st.radio === "not on the air" ? ` · <b class="warnText">not on the air</b> (running, but the air has no session with it; if this lasts, restart it)` : ""}`)}
    ${row(term("S1", "Link to core"), !st.s1 ? "unknown (MME not reachable)"
      : st.s1.up ? `up · S1 Setup done · MME counts ${esc(st.s1.connected_ues)} connected UE${st.s1.connected_ues === 1 ? "" : "s"} · ${esc(st.s1.peer)}`
      : "<b class=\"warnText\">down: the MME has no S1 link with this eNB</b>")}
    ${row("Software", esc(t.software))}
    ${row("Operator", `${esc(op.name)} · ${term("PLMN")} ${esc(t.plmn)}`)}
    ${row("Frequency", `band ${esc(t.band)} · ${esc(t.dl_mhz)} MHz down · ${term("EARFCN")} ${esc(t.earfcn)}`)}
    ${row("Bandwidth", `${esc(t.bandwidth_mhz)} MHz (${esc(t.prb)} ${term("PRB")})`)}
    ${row(term("PCI"), esc(t.pci))}
    ${row("Cell ID", `${hex(t.cell_id)} · ${term("eNB ID")} ${hex(t.enb_id)} (${esc(t.enb_id)}) · ${term("ECI")} ${hex(t.eci)}`)}
    ${row(term("TAC"), esc(t.tac))}
    ${row("Neighbours", neigh || "none")}
    ${row("Handover", t.a3 ? `${term("A3")} offset ${esc(t.a3.offset_db)} dB + ${term("hysteresis")} ${esc(t.a3.hysteresis_db)} dB, ${term("time to trigger")} ${esc(t.a3.time_to_trigger_ms)} ms` : "–")}
    ${row("Serving now", served.length ? served.map(esc).join(", ") : "nobody")}
    ${row("Position", `(${cell.x.toFixed(1)}, ${cell.y.toFixed(1)}) km · ${label(cell.x, cell.y)}`)}
    ${row("Coverage", `${esc(model.world.pathloss.edge_km)} km radius (no signal beyond)`)}
    </table>`;
}

function ueInfo(id) {
  const ue = model.world?.ues.find(u => u.id === id);
  if (!ue) return `<p class="muted">Unknown device ${esc(id)}</p>`;
  const dev = deviceOf(id) || {}, st = ueStatus(ue), comp = model.components[dev.service] || {};
  const op = model.inventory?.operator || {};
  const sub = st.sub;
  // The phone itself (on or off) next to what the core thinks: they disagree after a pulled battery.
  const net = sub?.network || {};
  const unreachable = st.unreachable ? ` · <b class="warnText">unreachable</b> (mobile reachable timer expired)` : "";
  const network = st.down ? `<b>${esc(coreView({ on: false, mm: net.mm_state, cm: net.cm_state, unreachable: st.unreachable }))}</b>`
    : !st.registered ? "no service"
    : `cell ${esc(st.serving)} · ${term(st.connected ? "connected" : "idle")}${st.noRadio ? " · <b class=\"warnText\">radio gone, MME not yet aware</b>" : ""}${unreachable}`;
  const travel = ue.travel ? `→ (${ue.travel.x.toFixed(1)}, ${ue.travel.y.toFixed(1)}) at ${ue.travel.speed_kmh} km/h` : "";
  const auto = { manual: "manual", travelling: "travelling (manual trip)", auto: "under Realtime activity", shielded: "shielded (Faraday cage)" }[ue.automation] || ue.automation;
  return `<h3><span class="swatch round" style="background:${st.serving && !st.down ? cellColor(st.serving) : "#9ca3af"}"></span>
      ${esc(comp.hostname || id)} <span class="muted">${esc(dev.model)}</span></h3>
    <table class="kv">
    ${row("Device", `${esc(id)} · supports ${esc((dev.generations || []).join(", "))}${st.radio === "not on the air" ? ` · <b class="warnText">not on the air</b> (running, but the air has no session with it; if this lasts, restart it)` : ""}`)}
    ${row("SIM", `${term("IMSI")} ${esc(dev.imsi)}`)}
    ${row("Home network", `${esc(op.plmn === dev.home_plmn ? op.name : "?")} · ${term("PLMN")} ${esc(dev.home_plmn)} · ${term("APN")} ${esc(dev.apn)}`)}
    ${row("Subscriber", sub ? `${esc(sub.status)} · plan ${esc(sub.dl)}/${esc(sub.ul)} (${term("AMBR")})` : "not provisioned")}
    ${row("Network now", network)}
    ${row("IP", sub?.network?.ip ? `${esc(sub.network.ip)} <span class="muted">(from the SMF)</span>` : "–")}
    ${row(term("roaming", "Roaming"), "no")}
    ${row("Signal", model.world.cells.map(c => `${esc(c.id)}: ${fmtDb(ue.gains[c.id])}`).join(" · "))}
    ${row("Position", `(${ue.x.toFixed(1)}, ${ue.y.toFixed(1)}) km · ${label(ue.x, ue.y)} ${esc(travel)}`)}
    ${row("Moves", esc(auto))}
    </table>
    <div id="card-chart">${signalChart(id, margin())}</div>`;
}

export function renderInfo(sel) {
  const el = document.getElementById("card-info");
  if (!el) return;
  if (!sel.kind) { el.innerHTML = `<p class="muted">Click a tower or a device for its details.</p>`; return; }
  el.innerHTML = sel.kind === "cell" ? towerInfo(sel.id) : ueInfo(sel.id);
}

export function renderActions(sel) {
  const el = document.getElementById("card-actions");
  if (!el) return;
  el.innerHTML = sel.kind ? actionBuilders[sel.kind].map(fn => fn(sel.id)).join("") : "";
}
