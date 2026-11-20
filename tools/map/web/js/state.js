// Live state of one instance, polled from the map server. Nothing here is stored anywhere else:
// a reload starts empty (no history, no pins).

import * as api from "./api.js";

const HISTORY_S = 300;          // signal chart: last 5 minutes
export const COLORS = ["#2563eb", "#ea580c", "#16a34a", "#9333ea", "#db2777", "#0891b2"];

export const model = {
  inventory: null,              // agent: towers and devices
  world: null,                  // air: positions, travel, gains, radio clock
  subs: {},                     // provisioner: imsi -> subscriber (with live network state)
  components: {},               // agent: service -> container state
  unreachable: new Set(),       // agent: IMSIs the core marks unreachable (mobile reachable timer)
  history: {},                  // ue -> [{t, gains, serving}]
  pins: [],                     // handovers seen: {ue, from, to, x, y, at}
  errors: {},                   // source -> last error message
};

const prevNet = {};             // imsi -> {enb, mm}

export function cellColor(cellId) {
  const i = (model.world?.cells || []).findIndex(c => c.id === cellId);
  return COLORS[(i < 0 ? 0 : i) % COLORS.length];
}

export function deviceOf(ueId) {
  return model.inventory?.devices.find(d => d.id === ueId);
}

export function towerOf(cellId) {
  return model.inventory?.cells.find(c => c.id === cellId);
}

function cellByEnb(enbId) {
  return model.inventory?.cells.find(c => c.enb_id === enbId)?.id || null;
}

// What the network says about a UE: serving cell, registered/connected, and whether its
// container runs. noRadio: the air gives it no signal at all but the MME still lists it.
export function ueStatus(ue) {
  const dev = deviceOf(ue.id);
  const sub = dev ? model.subs[dev.imsi] : null;
  const net = sub?.network || null;
  const registered = net?.mm_state === "registered";
  const connected = registered && net?.cm_state === "connected";
  const comp = dev ? model.components[dev.service] : null;
  const down = comp ? comp.state !== "running" : false;
  const anyGain = Object.values(ue.gains || {}).some(g => g > 0);
  return {
    serving: registered ? cellByEnb(net.enb_id) : null,
    registered, connected, down, sub,
    unreachable: !!dev && model.unreachable.has(dev.imsi),
    noRadio: !anyGain && registered,
  };
}

export function towerStatus(cell) {
  const t = towerOf(cell.id);
  const comp = t ? model.components[t.service] : null;
  // s1: the tower's link to the core as the MME sees it; null when unknown (MME not reachable).
  return { down: comp ? comp.state !== "running" : false, tower: t, s1: comp?.s1 || null };
}

// A handover: the serving eNB changed between two polls while the UE stayed registered.
export function detectPins(prev, now, pos) {
  if (!prev || prev.mm !== "registered" || now.mm !== "registered") return null;
  if (prev.enb == null || now.enb == null || prev.enb === now.enb) return null;
  return { from: prev.enb, to: now.enb, x: pos.x, y: pos.y };
}

function recordHistory(now) {
  for (const ue of model.world.ues) {
    const h = (model.history[ue.id] ||= []);
    h.push({ t: now, gains: { ...ue.gains }, serving: ueStatus(ue).serving });
    while (h.length && now - h[0].t > HISTORY_S * 1000) h.shift();
  }
}

function checkHandovers(now) {
  if (!model.world || !model.inventory) return;
  for (const ue of model.world.ues) {
    const dev = deviceOf(ue.id);
    const net = dev && model.subs[dev.imsi]?.network;
    const cur = { enb: net?.enb_id ?? null, mm: net?.mm_state || "" };
    const pin = detectPins(prevNet[dev?.imsi], cur, ue);
    if (pin) model.pins.push({ ue: ue.id, from: cellByEnb(pin.from), to: cellByEnb(pin.to), x: pin.x, y: pin.y, at: now });
    if (dev) prevNet[dev.imsi] = cur;
  }
}

async function poll(source, fn, onChange) {
  try {
    await fn();
    delete model.errors[source];
  } catch (e) {
    model.errors[source] = String(e.message || e);
  }
  onChange();
}

export async function start(onChange) {
  await poll("agent", async () => { model.inventory = await api.get("agent", "/inventory"); }, onChange);
  const world = () => poll("air", async () => {
    model.world = await api.get("air", "/world");
    recordHistory(Date.now());
  }, onChange);
  const subs = () => poll("provisioner", async () => {
    const list = await api.get("provisioner", "/subscribers");
    model.subs = Object.fromEntries(list.map(s => [s.imsi, s]));
    checkHandovers(Date.now());
  }, onChange);
  const comps = () => poll("agent", async () => {
    const list = await api.get("agent", "/components");
    model.components = Object.fromEntries(list.map(c => [c.service, c]));
    model.unreachable = new Set((await api.get("agent", "/core/unreachable")).imsis);
  }, onChange);
  await Promise.all([world(), subs(), comps()]);
  // Each poll waits for the previous one to finish, so a slow API never gets a pile of requests.
  const every = (fn, ms) => { const loop = async () => { await fn(); setTimeout(loop, ms); }; setTimeout(loop, ms); };
  every(world, 1000);
  every(subs, 2000);
  every(comps, 5000);
}

export async function refreshWorld() {
  model.world = await api.get("air", "/world");
}
