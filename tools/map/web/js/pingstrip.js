// Live ping strip: off by default (a constant ping keeps a device from ever going idle). When on,
// the agent pings from the device every 0.2 s; the strip shows the last minute, and handover pins
// get the number of pings lost around them.

import * as api from "./api.js";
import { page } from "./main.js";
import { model } from "./state.js";

const KEEP_MS = 60000, W = 340, H = 36;
export const pings = {}; // ue -> {on, next, results: [{n, seq, ok, rtt_ms, at(ms)}]}

export async function setPing(ue, on) {
  const r = await api.send("agent", "PUT", `/ues/${encodeURIComponent(ue)}/ping`, { on });
  if (!r.ok) return r;
  pings[ue] ||= { on: false, next: r.json.next, results: [] };
  pings[ue].on = on;
  if (on) pings[ue].next = r.json.next;
  return r;
}

async function poll() {
  for (const [ue, p] of Object.entries(pings)) {
    if (!p.on) continue;
    try {
      const st = await api.get("agent", `/ues/${encodeURIComponent(ue)}/ping?since=${p.next}`);
      p.on = st.on;
      p.next = st.next;
      for (const r of st.results) p.results.push({ ...r, at: Date.parse(r.at) });
      const cutoff = Date.now() - 5 * KEEP_MS;
      while (p.results.length && p.results[0].at < cutoff) p.results.shift();
    } catch { /* the next poll tries again */ }
  }
  countPinLosses();
}

// A pin gets its lost count once the strip has seen a few seconds after it; only if the ping
// was running before the handover.
function countPinLosses() {
  const now = Date.now();
  for (const pin of model.pins) {
    if (pin.lost != null || now - pin.at < 8000) continue;
    const res = pings[pin.ue]?.results || [];
    if (!res.length || res[0].at > pin.at - 20000) continue;
    pin.lost = res.filter(r => !r.ok && r.at >= pin.at - 20000 && r.at <= pin.at + 5000).length;
  }
}

export function stripSvg(ue) {
  const p = pings[ue];
  if (!p || (!p.on && !p.results.length)) return "";
  const now = Date.now();
  const recent = p.results.filter(r => now - r.at < KEEP_MS);
  const lost = recent.filter(r => !r.ok).length;
  const rtts = recent.filter(r => r.ok).map(r => r.rtt_ms);
  const avg = rtts.length ? rtts.reduce((a, b) => a + b, 0) / rtts.length : 0;
  const bars = recent.map(r => {
    const x = W - ((now - r.at) / KEEP_MS) * W;
    const h = r.ok ? Math.max(3, Math.min(H - 4, (r.rtt_ms / 600) * (H - 4))) : H - 4;
    return `<rect x="${x.toFixed(1)}" y="${H - h}" width="2" height="${h}" fill="${r.ok ? "#16a34a" : "#dc2626"}"/>`;
  }).join("");
  return `<svg class="strip" viewBox="0 0 ${W} ${H}" width="${W}" height="${H}">${bars}</svg>
    <div class="small muted">Last minute: ${recent.length} pings, <b>${lost} lost</b>${rtts.length ? `, avg ${avg.toFixed(0)} ms` : ""}${p.on ? "" : " · stopped"}</div>`;
}

export function init() {
  const loop = async () => { await poll(); setTimeout(loop, 1000); }; // never two polls at once
  setTimeout(loop, 1000);
  page.onRedraw.push(() => {
    const el = document.getElementById("ping-strip");
    if (el && page.sel.kind === "ue") el.innerHTML = stripSvg(page.sel.id);
  });
}
