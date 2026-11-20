// Tests for the page's pure logic (no DOM): run with `make test-map-js` (Node in a container).
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { chartCells, coreView, pinLabel, powerButtons, radioState, validateMap } from "../web/js/logic.js";

const world = {
  cells: [{ id: "A", x: 0, y: 0 }, { id: "B", x: 10, y: 0 }],
  ues: [{ id: "ue1" }, { id: "ue2" }, { id: "ue3" }],
};
const preset = name => JSON.parse(readFileSync(new URL(`../web/presets/${name}.json`, import.meta.url)));
const errors = file => validateMap(file, world).errors.join("; ");

test("every shipped preset is valid", () => {
  for (const name of ["highway", "tunnel", "crowded-cell"]) {
    assert.deepEqual(validateMap(preset(name), world), { ok: true, errors: [] }, name);
  }
});

test("unknown ids and another version are refused", () => {
  const good = preset("highway");
  assert.match(errors({ ...good, cells: [{ id: "Z", x: 1, y: 2 }] }), /unknown cell "Z"/);
  assert.match(errors({ ...good, ues: [{ id: "ue9", x: 1, y: 2 }] }), /unknown device "ue9"/);
  assert.match(errors({ ...good, version: 2 }), /unsupported version 2/);
  assert.match(errors(null), /not a map file/);
});

test("coordinates must be numbers inside the map", () => {
  const good = preset("highway");
  assert.match(errors({ ...good, ues: [{ id: "ue1", x: "far", y: 0 }] }), /ue1: x and y must be numbers/);
  assert.match(errors({ ...good, cells: [{ id: "B", x: 25, y: 0 }] }), /cell B: \(25, 0\) is outside the map/);
  assert.match(errors({ ...good, ues: [{ id: "ue2", x: 0, y: -10.5 }] }), /device ue2: \(0, -10.5\) is outside the map/);
});

test("an id may appear only once", () => {
  const good = preset("highway");
  assert.match(errors({ ...good, ues: [...good.ues, { id: "ue1", x: 3, y: 0 }] }), /device ue1 appears twice/);
});

test("pin labels", () => {
  assert.equal(pinLabel({ from: "A", to: "B" }), "A→B");
  assert.equal(pinLabel({ from: "A", to: "B", lost: 1 }), "A→B, 1 lost");
  assert.equal(pinLabel({ from: "A", to: "B", lost: 0 }), "A→B, 0 lost");
  assert.equal(pinLabel({ from: null, to: "B" }), "?→B");
  assert.equal(pinLabel({ from: "A", to: undefined }), "A→?");
});

test("phone power buttons follow the container state", () => {
  assert.deepEqual(powerButtons("ue", true), [
    { action: "switch_off", label: "Switch off", enabled: true },
    { action: "pull_battery", label: "Pull battery", enabled: true },
    { action: "switch_on", label: "Switch on", enabled: false },
  ]);
  assert.deepEqual(powerButtons("ue", false).map(b => b.enabled), [false, false, true]);
});

test("tower power buttons follow the container state", () => {
  assert.deepEqual(powerButtons("cell", true).map(b => [b.action, b.label, b.enabled]), [
    ["shut_down", "Shut down", true], ["power_cut", "Power cut", true], ["power_on", "Power on", false],
  ]);
  assert.deepEqual(powerButtons("cell", false).map(b => b.enabled), [false, false, true]);
});

test("the core's view of a phone", () => {
  const reg = (cm, extra = {}) => ({ on: true, mm: "registered", cm, unreachable: false, ...extra });
  assert.equal(coreView(reg("connected")), "registered, connected");
  assert.equal(coreView(reg("idle")), "registered, idle");
  assert.equal(coreView({ on: true, mm: "", cm: "", unreachable: false }), "not registered");
  assert.equal(coreView(reg("idle", { on: false })), "phone off · core still has it registered (idle)");
  assert.equal(coreView(reg("idle", { on: false, unreachable: true })),
    "phone off · core still has it registered (idle) · unreachable (mobile reachable timer expired)");
  assert.equal(coreView({ on: false, mm: "", cm: "", unreachable: false }), "phone off · not registered");
});

test("a running radio that is not on the air is called out", () => {
  assert.equal(radioState(true, true), "on");
  assert.equal(radioState(false, false), "off");
  assert.equal(radioState(false, undefined), "off");
  assert.equal(radioState(true, false), "not on the air");
  assert.equal(radioState(true, undefined), "on"); // an air without on_air: assume it is
});

test("the signal chart leaves out towers that are off", () => {
  const cells = [{ id: "A" }, { id: "B" }];
  assert.deepEqual(chartCells(cells, id => id === "B").map(c => c.id), ["A"]);
  assert.deepEqual(chartCells(cells, () => false).map(c => c.id), ["A", "B"]);
});
