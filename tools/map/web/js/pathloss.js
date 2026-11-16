// Path loss: the same formula as the air (deploy/4g/broker/world.py, gain_for_distance).
// Amplitude 1.0 up to ref_km, then (ref/d)^(exponent/2), and 0 from edge_km on.

export function gain(dKm, p) {
  if (dKm >= p.edge_km) return 0;
  if (dKm <= p.ref_km) return 1;
  return Math.pow(p.ref_km / dKm, p.exponent / 2);
}

// Signal strength in dB relative to full signal (amplitude gain: 20 log10). -Infinity for 0.
export function db(g) {
  return g > 0 ? 20 * Math.log10(g) : -Infinity;
}

export function gainsAt(x, y, cells, p) {
  const out = {};
  for (const c of cells) out[c.id] = gain(Math.hypot(x - c.x, y - c.y), p);
  return out;
}
