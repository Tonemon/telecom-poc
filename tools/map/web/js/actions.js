// Card actions: captures on towers and devices; on devices also jump, stop, shield, speedtest,
// live ping and the operator's actions (suspend, resume, change plan). Each has a terminal icon.

import * as api from "./api.js";
import { actionBuilders } from "./cards.js";
import { cmdButton, remember } from "./commands.js";
import { page, redraw } from "./main.js";
import { startPick } from "./move.js";
import { pings, setPing } from "./pingstrip.js";
import { deviceOf, model, refreshWorld, towerOf } from "./state.js";

const esc = s => String(s).replace(/[&<>"']/g, ch => `&#${ch.charCodeAt(0)};`);
const opt = list => list.map(v => `<option>${v}</option>`).join("");
const SUSPEND = ["MAINTENANCE", "NON_PAYMENT", "LOST_STOLEN", "FRAUD", "OTHER"];
const RESUME = ["CLEARED", "PAYMENT_RECEIVED", "RECOVERED", "OTHER"];
const PLAN = ["DOWNGRADE", "UPGRADE", "PROMOTION", "OTHER"];

function result(html, bad = false) {
  const el = document.getElementById("action-result");
  if (el) el.innerHTML = `<div class="${bad ? "error" : "ok"}">${html}</div>`;
}

const errText = r => esc(r.json?.error || `HTTP ${r.status}`);

function pcapBlock(service) {
  return `<h4>Packet capture</h4><div class="actions">
      <button data-act="pcap-start" data-svc="${esc(service)}">Start capture</button>
      <button data-act="pcap-stop" data-svc="${esc(service)}">Stop capture</button>${cmdButton(`pcap:${service}`)}</div>
    <div class="pcaps" data-pcaps="${esc(service)}"></div>`;
}

function towerActions(cellId) {
  const t = towerOf(cellId);
  return `<div class="actions"><span class="muted small">Drag the tower on the map to move it.</span>${cmdButton(`cell:${cellId}`)}</div>
    ${t ? pcapBlock(t.service) : ""}<div id="action-result"></div>`;
}

function ueActions(ue) {
  const dev = deviceOf(ue);
  const on = !!pings[ue]?.on;
  return `<div class="actions">
      <button data-act="jump" data-ue="${esc(ue)}">Jump to…</button>${cmdButton(`jump:${ue}`)}
      <button data-act="stop" data-ue="${esc(ue)}">Stop</button>${cmdButton(`stop:${ue}`)}
      <button data-act="shield" data-ue="${esc(ue)}" data-on="1">Shield</button>
      <button data-act="shield" data-ue="${esc(ue)}" data-on="0">Unshield</button>${cmdButton(`shield:${ue}`)}</div>
    <h4>Live ping</h4>
    <div class="actions"><label class="toggle"><input type="checkbox" data-act="ping" data-ue="${esc(ue)}"${on ? " checked" : ""}> ping 8.8.8.8 every 0.2 s</label>${cmdButton(`ping:${ue}`)}</div>
    <div id="ping-strip"></div>
    <h4>Speedtest</h4>
    <div class="actions"><button data-act="speedtest" data-ue="${esc(ue)}">Run speedtest</button>${cmdButton(`speedtest:${ue}`)}</div>
    ${dev ? pcapBlock(dev.service) : ""}
    <h4>Operator</h4>
    <div class="actions"><select id="suspendReason">${opt(SUSPEND)}</select><input id="opNote" placeholder="note (optional)" size="14">
      <button data-act="suspend" data-ue="${esc(ue)}">Suspend</button>${cmdButton(`suspend:${ue}`)}</div>
    <div class="actions"><select id="resumeReason">${opt(RESUME)}</select>
      <button data-act="resume" data-ue="${esc(ue)}">Resume</button>${cmdButton(`resume:${ue}`)}</div>
    <div class="actions"><input id="planDl" placeholder="down, e.g. 256K" size="9"><input id="planUl" placeholder="up, e.g. 128K" size="9">
      <select id="planReason">${opt(PLAN)}</select>
      <button data-act="plan" data-ue="${esc(ue)}">Change plan</button>${cmdButton(`plan:${ue}`)}</div>
    <div id="action-result"></div>`;
}

async function refreshPcaps() {
  const els = document.querySelectorAll("[data-pcaps]");
  if (!els.length) return;
  let list;
  try { list = await api.get("agent", "/pcaps"); } catch { return; }
  for (const el of els) {
    const svc = el.dataset.pcaps;
    const running = list.capturing.find(c => c.component === svc);
    const files = list.files.filter(f => f.name.startsWith(svc + "-")).slice(0, 5);
    el.innerHTML = (running ? `<div class="small"><b>capturing</b> to ${esc(running.file)}</div>` : "") +
      (files.length ? `<ul class="small files">${files.map(f =>
        `<li><a href="${api.href("agent", "/pcaps/" + encodeURIComponent(f.name))}" download>${esc(f.name)}</a> · ${(f.bytes / 1024).toFixed(1)} KB</li>`).join("")}</ul>`
        : `<div class="small muted">No captures yet.</div>`);
  }
}

async function run(el) {
  const act = el.dataset.act, ue = el.dataset.ue, svc = el.dataset.svc;
  const imsi = ue ? deviceOf(ue)?.imsi : null;
  el.disabled = true;
  try {
    let r;
    switch (act) {
      case "pcap-start":
        r = await api.send("agent", "POST", "/pcaps", { component: svc });
        if (r.ok) { remember(`pcap:${svc}`, r.json.command); result(`Capturing to ${esc(r.json.file)}.`); } else result(errText(r), true);
        await refreshPcaps();
        break;
      case "pcap-stop":
        r = await api.send("agent", "DELETE", `/pcaps/${encodeURIComponent(svc)}`);
        result(r.ok ? `Saved ${esc(r.json.file)}: download it below and open it in Wireshark.${r.json.note ? ` <span class="muted">(${esc(r.json.note)})</span>` : ""}` : errText(r), !r.ok);
        await refreshPcaps();
        break;
      case "jump":
        startPick(ue, "jump");
        break;
      case "stop":
        r = await api.send("air", "POST", `/ues/${encodeURIComponent(ue)}/stop`);
        if (r.ok) remember(`stop:${ue}`, r.command);
        result(r.ok ? `${esc(ue)} stopped where it is.` : errText(r), !r.ok);
        break;
      case "shield": {
        const on = el.dataset.on === "1";
        r = await api.send("air", "PUT", `/ues/${encodeURIComponent(ue)}/shielded`, { shielded: on });
        if (r.ok) remember(`shield:${ue}`, r.command);
        result(r.ok ? (on ? `${esc(ue)} is in a Faraday cage: no radio at all.` : `${esc(ue)} is out of the Faraday cage.`) : errText(r), !r.ok);
        break;
      }
      case "ping":
        r = await setPing(ue, el.checked);
        if (r.ok) remember(`ping:${ue}`, r.json.command); else { result(errText(r), true); el.checked = false; }
        break;
      case "speedtest":
        result("Running: 5 s down, then 5 s up…");
        r = await api.send("agent", "POST", `/ues/${encodeURIComponent(ue)}/speedtest`);
        if (r.ok) {
          remember(`speedtest:${ue}`, r.json.command);
          result(`Down <b>${r.json.down_mbps.toFixed(2)} Mbit/s</b>, up <b>${r.json.up_mbps.toFixed(2)} Mbit/s</b> to ${esc(r.json.server)}.
            <span class="muted">The virtual radio runs at about a tenth of real time, so this is far below what a 5 MHz cell can carry.</span>`);
        } else result(errText(r), true);
        break;
      case "suspend": case "resume": {
        const reason = document.getElementById(act + "Reason").value, note = document.getElementById("opNote")?.value || "";
        r = await api.send("provisioner", "POST", `/subscribers/${imsi}/${act}`, note && act === "suspend" ? { reason, note } : { reason });
        if (r.ok) remember(`${act}:${ue}`, r.command);
        result(r.ok ? (act === "suspend" ? `Suspended: the network detaches ${esc(ue)} now.` : `Resumed: ${esc(ue)} reattaches by itself within about a minute.`) : errText(r), !r.ok);
        break;
      }
      case "plan": {
        const dl = document.getElementById("planDl").value.trim(), ul = document.getElementById("planUl").value.trim();
        r = await api.send("provisioner", "PATCH", `/subscribers/${imsi}/plan`, { dl, ul, reason: document.getElementById("planReason").value });
        if (r.ok) remember(`plan:${ue}`, r.command);
        result(r.ok ? `Plan changed to ${esc(dl)}/${esc(ul)}. The device only feels it after its next attach (see scenario 2).` : errText(r), !r.ok);
        break;
      }
    }
  } finally {
    el.disabled = false;
    await refreshWorld().catch(() => {});
    redraw();
  }
}

export function init() {
  actionBuilders.cell.push(towerActions);
  actionBuilders.ue.push(ueActions);
  document.addEventListener("click", ev => {
    const el = ev.target.closest("button[data-act]");
    if (el) run(el);
  });
  document.addEventListener("change", ev => {
    if (ev.target.matches("input[data-act='ping']")) run(ev.target);
  });
  page.onSelect.push(() => refreshPcaps());
  const loop = async () => { await refreshPcaps(); setTimeout(loop, 5000); };
  setTimeout(loop, 5000);
}
