// The page's pure logic: no DOM, so it is unit-tested with Node (tools/map/jstest, make test-map-js).

import { EXTENT } from "./grid.js";

export const MAP_VERSION = 1;

const num = v => typeof v === "number" && Number.isFinite(v);
const inside = (x, y) => x >= EXTENT.xmin && x <= EXTENT.xmax && y >= EXTENT.ymin && y <= EXTENT.ymax;

// validateMap checks a parsed map file against the running world before anything moves:
// version, known ids (each once), and numeric positions inside the map.
export function validateMap(file, world) {
  if (!file || typeof file !== "object") return { ok: false, errors: ["not a map file"] };
  const errors = [];
  if (file.version !== MAP_VERSION) {
    errors.push(`unsupported version ${JSON.stringify(file.version)} (this map reads version ${MAP_VERSION})`);
  }
  for (const [kind, list, known] of [["cell", file.cells, world.cells], ["device", file.ues, world.ues]]) {
    if (!Array.isArray(list)) { errors.push(`${kind}s missing`); continue; }
    const seen = new Set();
    for (const item of list) {
      const id = item?.id;
      if (!known.some(k => k.id === id)) { errors.push(`unknown ${kind} ${JSON.stringify(id)}`); continue; }
      if (seen.has(id)) { errors.push(`${kind} ${id} appears twice`); continue; }
      seen.add(id);
      if (!num(item.x) || !num(item.y)) errors.push(`${kind} ${id}: x and y must be numbers`);
      else if (!inside(item.x, item.y)) errors.push(`${kind} ${id}: (${item.x}, ${item.y}) is outside the map`);
    }
  }
  if (file.terrain && file.terrain.seed !== undefined && !num(file.terrain.seed)) errors.push("terrain seed must be a number");
  return { ok: errors.length === 0, errors };
}

// pinLabel is the text on a handover pin: "A→B", plus the pings lost when the strip was on.
export function pinLabel(p) {
  return `${p.from ?? "?"}→${p.to ?? "?"}${p.lost != null ? `, ${p.lost} lost` : ""}`;
}

const POWER = {
  ue: [["switch_off", "Switch off", true], ["pull_battery", "Pull battery", true], ["switch_on", "Switch on", false]],
  cell: [["shut_down", "Shut down", true], ["power_cut", "Power cut", true], ["power_on", "Power on", false]],
};

// powerButtons: a phone's or a tower's power buttons; the "off" ones only while it runs, the "on"
// one only while it does not.
export function powerButtons(kind, running) {
  return POWER[kind].map(([action, label, whenOn]) => ({ action, label, enabled: whenOn === running }));
}

// coreView: a phone as the core sees it, next to whether the phone itself is on. A phone that is
// off while the core still lists it is the "pull battery" lesson: the network doesn't know yet.
export function coreView({ on, mm, cm, unreachable }) {
  const core = mm === "registered" ? `registered, ${cm || "?"}` : "not registered";
  const text = on ? core : mm === "registered" ? `phone off · core still has it registered (${cm || "?"})` : "phone off · not registered";
  return unreachable && mm === "registered" ? `${text} · unreachable (mobile reachable timer expired)` : text;
}

// radioState: a device as the radio sees it. "not on the air": its container runs but the air
// has no session with it (it should rejoin by itself within seconds; if it stays like this, it is
// stuck and needs a restart).
export function radioState(running, onAir) {
  if (!running) return "off";
  return onAir === false ? "not on the air" : "on";
}

// chartCells: the cells the signal chart draws; a tower that is off sends nothing.
export function chartCells(cells, isOff) {
  return cells.filter(c => !isOff(c.id));
}
