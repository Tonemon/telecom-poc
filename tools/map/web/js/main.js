// Wires the page together: picks the instance, starts polling, redraws on every change.

import * as api from "./api.js";
import { drawHeat, drawOverlay, drawTerrain } from "./draw.js";
import { label, view } from "./grid.js";
import { renderDebug, renderLists, renderTop } from "./sidebar.js";
import { model, start } from "./state.js";
import { generate } from "./terrain.js";

export const page = {
  sel: { kind: null, id: null },
  seed: Number(new URLSearchParams(location.search).get("seed")) || 20261116,
  debug: new URLSearchParams(location.search).has("debug"),
  overlayExtra: () => "",       // later modules add SVG (travel preview, ghost tower)
  onSelect: [],                 // later modules render cards
  onRedraw: [],
};

const $ = id => document.getElementById(id);
let v = null, terrain = null, heatKey = "", scheduled = false;

function margin() {
  const a3 = model.inventory?.cells?.[0]?.a3;
  return a3 ? a3.offset_db + a3.hysteresis_db : 3;
}

function resize() {
  const box = $("map").getBoundingClientRect();
  for (const id of ["terrain", "heat"]) {
    $(id).width = box.width; $(id).height = box.height;
  }
  $("overlay").setAttribute("width", box.width);
  $("overlay").setAttribute("height", box.height);
  v = view(box.width, box.height);
  heatKey = "";
  if (terrain) drawTerrain($("terrain"), terrain, v);
  redraw();
}

export function setSeed(seed) {
  page.seed = seed;
  terrain = generate(seed);
  if (v) drawTerrain($("terrain"), terrain, v);
}

export function getView() {
  return v;
}

function draw() {
  scheduled = false;
  if (!v) return;
  const w = model.world;
  if (w) {
    const key = JSON.stringify([w.cells, w.pathloss, margin(), v.width, v.height]);
    if (key !== heatKey) {
      drawHeat($("heat"), v, w.cells, w.pathloss, margin());
      heatKey = key;
    }
  }
  drawOverlay($("overlay"), v, page.sel, page.overlayExtra(v));
  renderLists($("lists"), page.sel);
  renderTop($("top"));
  if (page.debug) renderDebug($("debug"));
  page.onRedraw.forEach(fn => fn());
}

export function redraw() {
  if (!scheduled) { scheduled = true; requestAnimationFrame(draw); }
}

export function select(kind, id) {
  page.sel = kind ? { kind, id } : { kind: null, id: null };
  page.onSelect.forEach(fn => fn(page.sel));
  redraw();
}

function clickTarget(ev) {
  const g = ev.target.closest("[data-ue],[data-cell]");
  if (!g) return null;
  return g.dataset.ue ? { kind: "ue", id: g.dataset.ue } : { kind: "cell", id: g.dataset.cell };
}

async function main() {
  const list = await api.instances();
  api.use(list[0].name);
  setSeed(page.seed);
  // The map box changes size when the top bar or sidebar fill in, not only with the window.
  new ResizeObserver(resize).observe($("map"));
  resize();
  $("overlay").addEventListener("mousemove", ev => {
    if (!v) return;
    const r = $("map").getBoundingClientRect();
    const [x, y] = v.fromScreen(ev.clientX - r.left, ev.clientY - r.top);
    $("cursor").textContent = `${label(x, y)} · (${x.toFixed(1)}, ${y.toFixed(1)}) km`;
  });
  for (const el of [$("overlay"), $("lists")]) {
    el.addEventListener("click", ev => {
      const t = clickTarget(ev);
      if (t) select(t.kind, t.id);
    });
  }
  if (page.debug) $("debug").hidden = false;
  await start(redraw);
  document.body.dataset.ready = "1";
}

main();
