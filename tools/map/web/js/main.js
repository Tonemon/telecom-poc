// Wires the page together: picks the instance, starts polling, redraws on every change.

import * as actions from "./actions.js";
import * as activity from "./activity.js";
import * as commands from "./commands.js";
import * as mapfile from "./mapfile.js";
import * as pingstrip from "./pingstrip.js";
import * as power from "./power.js";
import * as api from "./api.js";
import { margin, renderActions, renderInfo } from "./cards.js";
import { drawHeat, drawOverlay, drawTerrain } from "./draw.js";
import { label, view } from "./grid.js";
import { renderDebug, renderLists, renderTop } from "./sidebar.js";
import { model, start, towerStatus } from "./state.js";
import { generate } from "./terrain.js";
import * as tooltips from "./tooltips.js";
import * as move from "./move.js";

export const page = {
  sel: { kind: null, id: null },
  seed: Number(new URLSearchParams(location.search).get("seed")) || 20261116,
  debug: new URLSearchParams(location.search).has("debug"),
  extras: [],                   // fn(view) -> SVG drawn on top (travel preview, ghost tower)
  onSelect: [],
  onRedraw: [],
  onMapClick: [],               // fn(event) -> true if it handled the click
  lastCommand: {},              // terminal command of the last action, per action
};

const $ = id => document.getElementById(id);
let v = null, terrain = null, heatKey = "", scheduled = false;

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
    const on = w.cells.filter(c => !towerStatus(c).down); // a tower that is off covers nothing
    const key = JSON.stringify([on, w.pathloss, margin(), v.width, v.height]);
    if (key !== heatKey) {
      drawHeat($("heat"), v, on, w.pathloss, margin());
      heatKey = key;
    }
  }
  drawOverlay($("overlay"), v, page.sel, page.extras.map(fn => fn(v)).join(""));
  renderInfo(page.sel);
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
  renderActions(page.sel);
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
  $("overlay").addEventListener("click", ev => {
    if (page.onMapClick.some(fn => fn(ev))) return;
    const t = clickTarget(ev);
    if (t) select(t.kind, t.id);
  });
  $("lists").addEventListener("click", ev => {
    const t = clickTarget(ev);
    if (t) select(t.kind, t.id);
  });
  await tooltips.init();
  move.init();
  commands.init();
  actions.init();
  power.init();
  pingstrip.init();
  mapfile.init();
  activity.init();
  select(null);
  if (page.debug) $("debug").hidden = false;
  await start(redraw);
  document.body.dataset.ready = "1";
}

main();
