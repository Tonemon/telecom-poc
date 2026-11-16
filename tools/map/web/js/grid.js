// The world in km and its battleship grid: 1 km squares, rows A-T from the top (y = 10),
// columns 1-30 from the left (x = -10).

export const EXTENT = { xmin: -10, xmax: 20, ymin: -10, ymax: 10 };
const ROWS = "ABCDEFGHIJKLMNOPQRST";

export function label(x, y) {
  const row = Math.min(ROWS.length - 1, Math.max(0, Math.floor(EXTENT.ymax - y)));
  const col = Math.min(EXTENT.xmax - EXTENT.xmin, Math.max(1, Math.floor(x - EXTENT.xmin) + 1));
  return ROWS[row] + col;
}

export function rowLabels() {
  return ROWS.split("");
}

// A view maps km to screen pixels for a box of width x height, keeping km square.
export function view(width, height) {
  const w = EXTENT.xmax - EXTENT.xmin, h = EXTENT.ymax - EXTENT.ymin;
  const s = Math.min(width / w, height / h);
  const ox = (width - w * s) / 2, oy = (height - h * s) / 2;
  return {
    scale: s, ox, oy, width, height,
    toScreen: (x, y) => [ox + (x - EXTENT.xmin) * s, oy + (EXTENT.ymax - y) * s],
    fromScreen: (px, py) => [EXTENT.xmin + (px - ox) / s, EXTENT.ymax - (py - oy) / s],
  };
}
