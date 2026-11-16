// Signal chart for one UE: each cell's signal (dB) over the last minutes, the A3 band above the
// serving cell's line (what a neighbour must beat), and a marker at every handover.

import { pinLabel } from "./logic.js";
import { db } from "./pathloss.js";
import { cellColor, model } from "./state.js";

const esc = s => String(s).replace(/[&<>"']/g, ch => `&#${ch.charCodeAt(0)};`);
const W = 340, H = 150, PAD = { l: 34, r: 8, t: 8, b: 20 }, MIN_DB = -40, SPAN_S = 300;

export function signalChart(ueId, marginDb) {
  const hist = model.history[ueId] || [];
  if (hist.length < 2) return `<p class="muted">Collecting signal history…</p>`;
  const now = hist[hist.length - 1].t;
  const x = t => PAD.l + (1 - (now - t) / (SPAN_S * 1000)) * (W - PAD.l - PAD.r);
  const y = d => PAD.t + (Math.min(0, Math.max(MIN_DB, d)) / MIN_DB) * (H - PAD.t - PAD.b);
  const cells = model.world.cells.map(c => c.id);
  let out = `<svg class="chart" viewBox="0 0 ${W} ${H}" width="${W}" height="${H}">`;
  for (const d of [0, -10, -20, -30, -40]) {
    out += `<line x1="${PAD.l}" x2="${W - PAD.r}" y1="${y(d)}" y2="${y(d)}" class="axis"/>` +
      `<text x="${PAD.l - 4}" y="${y(d) + 3}" text-anchor="end" class="tick">${d}</text>`;
  }
  out += `<text x="${PAD.l}" y="${H - 4}" class="tick">−5 min</text><text x="${W - PAD.r}" y="${H - 4}" text-anchor="end" class="tick">now</text>`;
  // A3 band: from the serving cell's signal up to serving + margin.
  let band = "";
  for (const h of hist) {
    if (!h.serving || !(h.gains[h.serving] > 0)) continue;
    const d = db(h.gains[h.serving]);
    band += `<rect x="${x(h.t)}" y="${y(d + marginDb)}" width="${Math.max(1, (W - PAD.l - PAD.r) / SPAN_S)}" height="${Math.max(0, y(d) - y(d + marginDb))}" fill="${cellColor(h.serving)}" opacity="0.18"/>`;
  }
  out += band;
  for (const c of cells) {
    const pts = hist.filter(h => h.gains[c] > 0).map(h => `${x(h.t).toFixed(1)},${y(db(h.gains[c])).toFixed(1)}`);
    if (pts.length > 1) out += `<polyline points="${pts.join(" ")}" fill="none" stroke="${cellColor(c)}" stroke-width="2"/>`;
  }
  for (const p of model.pins.filter(p => p.ue === ueId && now - p.at < SPAN_S * 1000)) {
    out += `<line x1="${x(p.at)}" x2="${x(p.at)}" y1="${PAD.t}" y2="${H - PAD.b}" class="hoMark"/>` +
      `<text x="${x(p.at) + 3}" y="${PAD.t + 10}" class="tick">${esc(pinLabel({ from: p.from, to: p.to }))}</text>`;
  }
  return out + `</svg><div class="muted small">Signal per cell (dB vs full signal). Shaded: the ${marginDb} dB a neighbour must beat the serving cell by.</div>`;
}
