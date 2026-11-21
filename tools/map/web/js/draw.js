// Drawing: terrain and coverage heatmap on canvases, everything live (grid, towers, UEs, signal
// lines, pins) as SVG on top.

import { EXTENT, rowLabels } from "./grid.js";
import { pinLabel } from "./logic.js";
import { db, gainsAt } from "./pathloss.js";
import { cellColor, model, towerStatus, ueStatus } from "./state.js";

export function drawTerrain(canvas, terrain, v) {
  const ctx = canvas.getContext("2d");
  ctx.clearRect(0, 0, canvas.width, canvas.height);
  const [x0, y0] = v.toScreen(EXTENT.xmin, EXTENT.ymax);
  ctx.imageSmoothingEnabled = true;
  ctx.drawImage(terrain, x0, y0, (EXTENT.xmax - EXTENT.xmin) * v.scale, (EXTENT.ymax - EXTENT.ymin) * v.scale);
}

function rgb(hex) {
  const n = parseInt(hex.slice(1), 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

const STEP = 3; // heatmap sample every STEP screen pixels

// Shade every point by its strongest cell (colour) and how strong it is (opacity); then draw the
// handover lines: where one cell beats another by the A3 margin.
export function drawHeat(canvas, v, cells, pathloss, marginDb) {
  const ctx = canvas.getContext("2d");
  ctx.clearRect(0, 0, canvas.width, canvas.height);
  if (!cells.length) return;
  const cols = Math.ceil(v.width / STEP), rows = Math.ceil(v.height / STEP);
  const img = ctx.createImageData(cols, rows);
  const dbs = cells.map(() => new Float32Array(cols * rows));
  const colors = cells.map(c => rgb(cellColor(c.id)));
  for (let r = 0; r < rows; r++) {
    for (let c = 0; c < cols; c++) {
      const [x, y] = v.fromScreen(c * STEP + STEP / 2, r * STEP + STEP / 2);
      const g = gainsAt(x, y, cells, pathloss);
      let best = -1, bestDb = -Infinity;
      cells.forEach((cell, i) => {
        const d = db(g[cell.id]);
        dbs[i][r * cols + c] = d;
        if (d > bestDb) { bestDb = d; best = i; }
      });
      if (best < 0 || bestDb === -Infinity) continue;
      const a = Math.max(0, Math.min(1, (bestDb + 30) / 30));
      const k = (r * cols + c) * 4;
      [img.data[k], img.data[k + 1], img.data[k + 2]] = colors[best];
      img.data[k + 3] = Math.round(40 + 90 * a);
    }
  }
  const tmp = document.createElement("canvas");
  tmp.width = cols; tmp.height = rows;
  tmp.getContext("2d").putImageData(img, 0, 0);
  ctx.imageSmoothingEnabled = true;
  ctx.drawImage(tmp, 0, 0, cols * STEP, rows * STEP);

  // Handover lines for every ordered pair (i -> j): db_j - db_i = margin, where both are heard.
  for (let i = 0; i < cells.length; i++) {
    for (let j = 0; j < cells.length; j++) {
      if (i === j) continue;
      ctx.fillStyle = cellColor(cells[j].id);
      const f = idx => dbs[j][idx] - dbs[i][idx] - marginDb;
      const heard = idx => dbs[i][idx] > -Infinity && dbs[j][idx] > -Infinity;
      for (let r = 0; r < rows - 1; r++) {
        for (let c = 0; c < cols - 1; c++) {
          const k = r * cols + c;
          if (!heard(k) || !heard(k + 1) || !heard(k + cols)) continue;
          const s = Math.sign(f(k));
          if (s !== Math.sign(f(k + 1)) || s !== Math.sign(f(k + cols))) {
            if ((r + c) % 2 === 0) ctx.fillRect(c * STEP, r * STEP, STEP, STEP);
          }
        }
      }
    }
  }
}

const esc = s => String(s).replace(/[&<>"']/g, ch => `&#${ch.charCodeAt(0)};`);

function gridSvg(v) {
  let out = "";
  for (let x = EXTENT.xmin; x <= EXTENT.xmax; x++) {
    const [px, top] = v.toScreen(x, EXTENT.ymax), [, bottom] = v.toScreen(x, EXTENT.ymin);
    out += `<line class="grid" x1="${px}" y1="${top}" x2="${px}" y2="${bottom}"/>`;
    if (x < EXTENT.xmax) {
      const [cx] = v.toScreen(x + 0.5, 0);
      out += `<text class="gridlabel" x="${cx}" y="${top + 11}" text-anchor="middle">${x - EXTENT.xmin + 1}</text>`;
    }
  }
  const rows = rowLabels();
  for (let y = EXTENT.ymin; y <= EXTENT.ymax; y++) {
    const [left, py] = v.toScreen(EXTENT.xmin, y), [right] = v.toScreen(EXTENT.xmax, y);
    out += `<line class="grid" x1="${left}" y1="${py}" x2="${right}" y2="${py}"/>`;
    if (y < EXTENT.ymax) {
      const [, cy] = v.toScreen(0, y + 0.5);
      out += `<text class="gridlabel" x="${left + 4}" y="${cy + 4}">${rows[EXTENT.ymax - y - 1]}</text>`;
    }
  }
  return out;
}

function towerSvg(v, cell, selected) {
  const [px, py] = v.toScreen(cell.x, cell.y);
  const st = towerStatus(cell);
  const color = st.down ? "#9ca3af" : cellColor(cell.id);
  return `<g class="tower${selected ? " selected" : ""}" data-cell="${esc(cell.id)}" transform="translate(${px},${py})">
    <circle r="16" class="halo"/>
    <path d="M0,-18 L-9,10 L9,10 Z" fill="${color}" stroke="#111827" stroke-width="1.5"/>
    <line x1="-5" y1="-2" x2="5" y2="-2" stroke="#fff" stroke-width="1.5"/>
    <text y="26" text-anchor="middle" class="label">${esc(cell.name || cell.id)}${st.down ? " · off" : st.radio === "not on the air" ? " · not on the air" : st.s1 && !st.s1.up ? " · S1 down" : ""}</text>
  </g>`;
}

function ueSvg(v, ue, selected) {
  const [px, py] = v.toScreen(ue.x, ue.y);
  const st = ueStatus(ue);
  let fill = "#9ca3af", stroke = "#374151", cls = "ue";
  if (!st.down && st.registered && st.serving) {
    const col = cellColor(st.serving);
    fill = st.connected ? col : "#ffffff";
    stroke = col;
  }
  if (selected) cls += " selected";
  const mark = st.down ? `<text y="-12" text-anchor="middle" class="badge">off</text>`
    : st.radio === "not on the air" ? `<text y="-12" text-anchor="middle" class="badge">not on air</text>`
    : st.noRadio ? `<text y="4" text-anchor="middle" class="warn">!</text>` : "";
  return `<g class="${cls}" data-ue="${esc(ue.id)}" transform="translate(${px},${py})">
    <circle r="13" class="halo"/>
    <circle r="7" fill="${fill}" stroke="${stroke}" stroke-width="3"/>
    ${mark}
    <text y="22" text-anchor="middle" class="label">${esc(ue.id)}</text>
  </g>`;
}

function signalLinesSvg(v, world) {
  let out = "";
  for (const ue of world.ues) {
    const st = ueStatus(ue);
    if (st.down) continue;
    const [ux, uy] = v.toScreen(ue.x, ue.y);
    for (const cell of world.cells) {
      if (towerStatus(cell).down) continue; // an off tower sends nothing
      const g = ue.gains[cell.id] || 0;
      if (g <= 0) continue;
      const [cx, cy] = v.toScreen(cell.x, cell.y);
      const op = Math.max(0.15, Math.min(1, (db(g) + 30) / 30));
      const dash = st.serving === cell.id ? "" : `stroke-dasharray="6 5"`;
      out += `<line class="signal" x1="${ux}" y1="${uy}" x2="${cx}" y2="${cy}" stroke="${cellColor(cell.id)}" stroke-opacity="${op.toFixed(2)}" ${dash}/>`;
    }
  }
  return out;
}

function pinsSvg(v) {
  return model.pins.map(p => {
    const [px, py] = v.toScreen(p.x, p.y);
    const text = pinLabel(p);
    return `<g class="pin" transform="translate(${px},${py})">
      <line x1="0" y1="0" x2="0" y2="-22" stroke="#111827" stroke-width="1.5"/>
      <path d="M0,-22 L16,-17 L0,-12 Z" fill="${cellColor(p.to)}" stroke="#111827"/>
      <text x="18" y="-14" class="pinlabel">${esc(text)}</text>
    </g>`;
  }).join("");
}

export function drawOverlay(svg, v, sel, extra = "") {
  const w = model.world;
  if (!w) { svg.innerHTML = ""; return; }
  svg.innerHTML = gridSvg(v) + signalLinesSvg(v, w) + pinsSvg(v) +
    w.cells.map(c => towerSvg(v, c, sel.kind === "cell" && sel.id === c.id)).join("") +
    w.ues.map(u => ueSvg(v, u, sel.kind === "ue" && sel.id === u.id)).join("") + extra;
}
