// Fictional terrain from a seed: value noise for height and moisture, coloured like a terrain
// map (water, beach, grass, forest, hills), plus roads. Decoration only: it does not change the
// radio. Same seed, same map.

import { EXTENT } from "./grid.js";

function rng(seed) {
  let s = seed >>> 0 || 1;
  return () => {
    s ^= s << 13; s >>>= 0; s ^= s >>> 17; s ^= s << 5; s >>>= 0;
    return s / 4294967296;
  };
}

function noise(seed) {
  const r = rng(seed), size = 256, perm = new Uint8Array(512), vals = new Float32Array(size);
  for (let i = 0; i < size; i++) { perm[i] = i; vals[i] = r(); }
  for (let i = size - 1; i > 0; i--) { const j = Math.floor(r() * (i + 1)); [perm[i], perm[j]] = [perm[j], perm[i]]; }
  for (let i = 0; i < size; i++) perm[i + size] = perm[i];
  const v = (x, y) => vals[perm[(perm[x & 255] + y) & 511]];
  const smooth = t => t * t * (3 - 2 * t);
  const one = (x, y) => {
    const xi = Math.floor(x), yi = Math.floor(y), tx = smooth(x - xi), ty = smooth(y - yi);
    const a = v(xi, yi), b = v(xi + 1, yi), c = v(xi, yi + 1), d = v(xi + 1, yi + 1);
    return a + (b - a) * tx + (c - a) * ty + (a - b - c + d) * tx * ty;
  };
  return (x, y) => {
    let sum = 0, amp = 1, freq = 1, norm = 0;
    for (let o = 0; o < 5; o++) { sum += one(x * freq, y * freq) * amp; norm += amp; amp /= 2; freq *= 2; }
    return sum / norm;
  };
}

const PER_KM = 12; // terrain samples per km

// Returns an offscreen canvas covering EXTENT at PER_KM pixels per km.
export function generate(seed) {
  const w = (EXTENT.xmax - EXTENT.xmin) * PER_KM, h = (EXTENT.ymax - EXTENT.ymin) * PER_KM;
  const canvas = document.createElement("canvas");
  canvas.width = w; canvas.height = h;
  const ctx = canvas.getContext("2d"), img = ctx.createImageData(w, h);
  const height = noise(seed), moist = noise(seed * 7 + 3);
  for (let py = 0; py < h; py++) {
    for (let px = 0; px < w; px++) {
      const x = px / PER_KM / 9, y = py / PER_KM / 9;
      const e = height(x, y), m = moist(x + 40, y + 40);
      const shade = 0.85 + 0.3 * (height(x + 0.02, y + 0.02) - e) * 20;
      let c;
      if (e < 0.32) c = [120, 170, 214];                // water
      else if (e < 0.35) c = [226, 214, 170];           // beach
      else if (e < 0.6) c = m > 0.5 ? [150, 190, 120] : [184, 206, 140]; // grass / meadow
      else if (e < 0.72) c = [110, 160, 100];           // forest
      else c = [176, 160, 130];                          // hills
      const i = (py * w + px) * 4;
      img.data[i] = Math.min(255, c[0] * shade); img.data[i + 1] = Math.min(255, c[1] * shade);
      img.data[i + 2] = Math.min(255, c[2] * shade); img.data[i + 3] = 255;
    }
  }
  ctx.putImageData(img, 0, 0);
  roads(ctx, w, h, rng(seed + 11));
  return canvas;
}

function roads(ctx, w, h, r) {
  const road = (pts, width) => {
    ctx.lineCap = "round"; ctx.lineJoin = "round";
    for (const [col, extra] of [["#8a7f6a", 2], ["#f4efe1", 0]]) {
      ctx.strokeStyle = col; ctx.lineWidth = width + extra;
      ctx.beginPath(); pts.forEach(([x, y], i) => (i ? ctx.lineTo(x, y) : ctx.moveTo(x, y))); ctx.stroke();
    }
  };
  // The main road along y = 0, between the two towers: where the UEs start.
  road([[0, h / 2], [w, h / 2]], 4);
  for (let k = 0; k < 3; k++) {
    const pts = [];
    let x = r() * w, y = 0, dx = (r() - 0.5) * 3;
    while (y <= h) { pts.push([x, y]); y += h / 12; dx += (r() - 0.5) * 2; x += dx * PER_KM; }
    road(pts, 2);
  }
}
