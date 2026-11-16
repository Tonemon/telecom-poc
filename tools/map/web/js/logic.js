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
