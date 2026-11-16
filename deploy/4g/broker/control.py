"""Glue between the world (where UEs are), the air (the radio gains) and the API.

Pure Python, testable without GNU Radio. The air calls tick() with how much *radio* time passed
(samples / sample rate) and how much wall time; the controller moves the world on by the radio
time and pushes only the gains that changed to the air.

A manual gain override (PUT /ues/{id}/gains, for debugging and hand tests) holds until the next
move, position change or shielding change of that UE; then the world decides again.
"""
import threading


class Controller:
    def __init__(self, world, apply_gain, log=print):
        self.world = world
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
        self._moved(ue)

    def travel(self, ue, x, y, speed_kmh):
        self.world.travel(ue, x, y, speed_kmh)
        self._log(f"air: {ue} travelling to ({x:g}, {y:g}) at {speed_kmh:g} km/h")
        self._moved(ue)

    def set_shielded(self, ue, shielded):
        self.world.set_shielded(ue, shielded)
        self._log(f"air: {ue} {'shielded (no radio)' if shielded else 'unshielded'}")
        self._moved(ue)

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

    # ---- state --------------------------------------------------------------------------
    def snapshot(self):
        snap = self.world.snapshot()
        with self._lock:
            eff = self._effective()
            for u in snap["ues"]:
                u["gains"] = {c: eff[(u["id"], c)] for c in u["gains"]}
                u["override"] = u["id"] in self._overrides
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
