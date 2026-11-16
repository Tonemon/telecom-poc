"""Glue between the world (where UEs are), the air (the radio gains) and the API.

Pure Python, testable without GNU Radio. The air calls tick() with how much *radio* time passed
(samples / sample rate) and how much wall time; the controller moves the world on by the radio
time and pushes only the gains that changed to the air.

A manual gain override (PUT /ues/{id}/gains, for debugging and hand tests) holds until the next
move, position change or shielding change of that UE; then the world decides again.

Activity (activity.py): random_activity() gives every eligible UE one decision; realtime
activity keeps deciding after each arrival and pause. A manual travel takes a UE out of the
automation until it arrives; shielding until it is unshielded.
"""
import random
import threading

import activity


class Busy(Exception):
    """The request conflicts with what is running (e.g. random activity while realtime is on)."""


class Controller:
    def __init__(self, world, apply_gain, log=print, rng=None):
        self.world = world
        self._rng = rng or random.Random()
        self.automation = activity.Automation(self._rng, [u["id"] for u in world.snapshot()["ues"]])
        self._apply = apply_gain
        self._log = log
        self._lock = threading.Lock()
        self._overrides = {}  # ue -> {cell: gain}
        self._applied = dict(world.gains())
        self._radio_s = 0.0
        self._wall_s = 0.0

    # ---- what the air starts with -------------------------------------------------------
    def initial_gains(self):
        with self._lock:
            return dict(self._effective())

    # ---- commands (API) -----------------------------------------------------------------
    def place(self, ue, x, y):
        self.world.place(ue, x, y)
        self._log(f"air: {ue} placed at ({x:g}, {y:g})")
        self.automation.on_manual(ue)
        self.automation.on_arrived(ue)  # a jump arrives at once
        self._moved(ue)

    def travel(self, ue, x, y, speed_kmh):
        self.world.travel(ue, x, y, speed_kmh)
        self._log(f"air: {ue} travelling to ({x:g}, {y:g}) at {speed_kmh:g} km/h")
        self.automation.on_manual(ue)
        self._moved(ue)

    def set_shielded(self, ue, shielded):
        self.world.set_shielded(ue, shielded)
        self._log(f"air: {ue} {'shielded (no radio)' if shielded else 'unshielded'}")
        self.automation.on_shielded(ue, shielded)
        self._moved(ue)

    # ---- activity -----------------------------------------------------------------------
    def random_activity(self):
        """One decision for every UE that is not shielded or on a manual trip."""
        if self.automation.realtime:
            raise Busy("realtime activity is on")
        plans = self._decide(self.automation.eligible())
        self._log(f"air: random activity: {len(plans)} decisions")
        return plans

    def set_realtime(self, on):
        """Start or stop realtime activity. Off stops every automated UE where it is."""
        changed = self.automation.set_realtime(on)
        self._log(f"air: realtime activity {'on' if on else 'off'}")
        if on:
            self._decide(changed)
            return []
        for ue in changed:
            self.world.stop(ue)
        with self._lock:
            self._push()
        return changed

    def _decide(self, ues):
        snap = self.world.snapshot()
        plans = []
        for ue in ues:
            p = activity.plan(ue, snap["cells"], snap["pathloss"], self._rng)
            if p["action"] == "travel":
                self.world.travel(ue, p["x"], p["y"], p["speed_kmh"])
            self.automation.planned(ue, p)
            if p["action"] == "travel" and not self._travel_of(ue):
                self.automation.on_arrived(ue)  # destination was where it already is
            plans.append(p)
        with self._lock:
            self._push()
        return plans

    def _travel_of(self, ue):
        return next(u for u in self.world.snapshot()["ues"] if u["id"] == ue)["travel"]

    def stop(self, ue):
        self.world.stop(ue)
        self._log(f"air: {ue} stopped")
        self.automation.on_manual(ue)
        self.automation.on_arrived(ue)
        self._moved(ue)

    def place_cell(self, cell_id, x, y):
        self.world.place_cell(cell_id, x, y)
        self._log(f"air: cell {cell_id} moved to ({x:g}, {y:g})")
        with self._lock:
            self._overrides.clear()  # any UE's gain to this cell may have changed
            self._push()

    def dry_run_cell(self, cell_id, x, y):
        return self.world.dry_run_cell(cell_id, x, y)

    def override_gain(self, ue, cell, value):
        with self._lock:
            if (ue, cell) not in self._applied:
                raise KeyError(f"{ue}/{cell}")
            self._overrides.setdefault(ue, {})[cell] = float(value)
            self._log(f"air: {ue}<->{cell} gain overridden to {value:g}")
            self._push()

    def _moved(self, ue):
        with self._lock:
            self._overrides.pop(ue, None)
            self._push()

    # ---- time ---------------------------------------------------------------------------
    def tick(self, radio_dt_s, wall_dt_s):
        before = {u["id"]: u["travel"] for u in self.world.snapshot()["ues"]}
        self.world.tick(radio_dt_s)
        with self._lock:
            self._radio_s += radio_dt_s
            self._wall_s += wall_dt_s
            self._push()
        for u in self.world.snapshot()["ues"]:
            if before.get(u["id"]) and not u["travel"]:
                self._log(f"air: {u['id']} arrived at ({u['x']:g}, {u['y']:g})")
                self.automation.on_arrived(u["id"])
        due = self.automation.due(radio_dt_s)
        if due:
            self._decide(due)

    # ---- state --------------------------------------------------------------------------
    def snapshot(self):
        snap = self.world.snapshot()
        with self._lock:
            eff = self._effective()
            for u in snap["ues"]:
                u["gains"] = {c: eff[(u["id"], c)] for c in u["gains"]}
                u["override"] = u["id"] in self._overrides
                u["automation"] = self.automation.state(u["id"])
            snap["activity"] = {"realtime": self.automation.realtime}
            snap["radio"] = {
                "time_s": self._radio_s,
                "speed": (self._radio_s / self._wall_s) if self._wall_s > 0 else None,
            }
        return snap

    def _effective(self):
        gains = self.world.gains()
        for ue, cells in self._overrides.items():
            for cell, g in cells.items():
                gains[(ue, cell)] = g
        return gains

    def _push(self):
        for key, g in self._effective().items():
            if self._applied.get(key) != g:
                self._apply(key[0], key[1], g)
                self._applied[key] = g
