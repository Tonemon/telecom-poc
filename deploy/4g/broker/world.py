"""The world: where cells and UEs are, and how strongly each UE hears each cell.

Pure Python (no sockets), thread-safe. Positions are 2D, in kilometres; speeds in km/h.
Time only moves through tick(dt_s): the air feeds it *radio* seconds (counted in samples), so a
UE travels at its speed in the same time base as the LTE procedures it triggers, however slowly
the host runs the virtual radio.
"""
import dataclasses
import math
import threading

DEFAULT_PATHLOSS = {"ref_km": 1.0, "exponent": 3.0, "edge_km": 7.0}


def gain_for_distance(d_km, p):
    """Log-distance path loss as a linear amplitude in [0, 1].

    1.0 up to ref_km; then power falls as d^-exponent, i.e. amplitude (ref/d)^(exponent/2);
    0.0 at and beyond edge_km (the coverage edge: the virtual radio has no noise floor, so
    without a hard edge a cell would be decodable at any distance).
    """
    ref, n, edge = float(p["ref_km"]), float(p["exponent"]), float(p["edge_km"])
    if d_km >= edge:
        return 0.0
    if d_km <= ref:
        return 1.0
    return (ref / d_km) ** (n / 2.0)


def _finite(*values):
    for v in values:
        if isinstance(v, bool) or not isinstance(v, (int, float)) or not math.isfinite(v):
            raise ValueError(f"not a finite number: {v!r}")


class World:
    def __init__(self, cfg):
        self._lock = threading.Lock()
        self._cells = [dataclasses.replace(c) for c in cfg.cells]  # own copies: cells can move
        self._pathloss = {**DEFAULT_PATHLOSS, **(cfg.pathloss or {})}
        self._ues = {u.id: {"x": float(u.x), "y": float(u.y), "shielded": False, "travel": None}
                     for u in cfg.ues}
        self._order = [u.id for u in cfg.ues]

    def _ue(self, ue):
        if ue not in self._ues:
            raise KeyError(ue)
        return self._ues[ue]

    def place(self, ue, x, y):
        _finite(x, y)
        with self._lock:
            s = self._ue(ue)
            s["x"], s["y"], s["travel"] = float(x), float(y), None

    def travel(self, ue, x, y, speed_kmh):
        _finite(x, y, speed_kmh)
        if speed_kmh <= 0:
            raise ValueError("speed must be > 0 km/h")
        with self._lock:
            s = self._ue(ue)
            if (s["x"], s["y"]) == (float(x), float(y)):
                s["travel"] = None  # already there
            else:
                s["travel"] = {"x": float(x), "y": float(y), "speed_kmh": float(speed_kmh)}

    def place_cell(self, cell_id, x, y):
        _finite(x, y)
        with self._lock:
            i = self._cell_index(cell_id)
            self._cells[i] = dataclasses.replace(self._cells[i], x=float(x), y=float(y))

    def dry_run_cell(self, cell_id, x, y):
        """What moving a cell would do to every UE, without moving it."""
        _finite(x, y)
        with self._lock:
            i = self._cell_index(cell_id)
            moved = list(self._cells)
            moved[i] = dataclasses.replace(moved[i], x=float(x), y=float(y))
            out = []
            for ue in self._order:
                s = self._ues[ue]
                before = {c.id: self._gain(s, c) for c in self._cells}
                after = {c.id: self._gain(s, c) for c in moved}
                had, has = any(before.values()), any(after.values())
                effect = "gains" if has and not had else "loses" if had and not has else "none"
                out.append({"id": ue, "before": before, "after": after, "effect": effect})
            return out

    def stop(self, ue):
        with self._lock:
            self._ue(ue)["travel"] = None

    def _cell_index(self, cell_id):
        for i, c in enumerate(self._cells):
            if c.id == cell_id:
                return i
        raise KeyError(cell_id)

    def set_shielded(self, ue, shielded):
        with self._lock:
            self._ue(ue)["shielded"] = bool(shielded)

    def tick(self, dt_s):
        _finite(dt_s)
        if dt_s < 0:
            raise ValueError("dt must not be negative")
        with self._lock:
            for s in self._ues.values():
                t = s["travel"]
                if not t:
                    continue
                dx, dy = t["x"] - s["x"], t["y"] - s["y"]
                left = math.hypot(dx, dy)
                step = t["speed_kmh"] * dt_s / 3600.0
                if step >= left:
                    s["x"], s["y"], s["travel"] = t["x"], t["y"], None
                else:
                    s["x"] += dx / left * step
                    s["y"] += dy / left * step

    def _gain(self, s, cell):
        if s["shielded"]:
            return 0.0
        return gain_for_distance(math.hypot(s["x"] - cell.x, s["y"] - cell.y), self._pathloss)

    def gains(self):
        with self._lock:
            return {(ue, c.id): self._gain(self._ues[ue], c) for ue in self._order for c in self._cells}

    def snapshot(self):
        with self._lock:
            ues = []
            for ue in self._order:
                s = self._ues[ue]
                t = s["travel"]
                travel = None
                if t:
                    left = math.hypot(t["x"] - s["x"], t["y"] - s["y"])
                    travel = {"x": t["x"], "y": t["y"], "speed_kmh": t["speed_kmh"],
                              "eta_s": left / t["speed_kmh"] * 3600.0}
                ues.append({"id": ue, "x": s["x"], "y": s["y"], "shielded": s["shielded"],
                            "travel": travel,
                            "gains": {c.id: self._gain(s, c) for c in self._cells}})
            return {"cells": [{"id": c.id, "name": c.name, "x": c.x, "y": c.y} for c in self._cells],
                    "ues": ues, "pathloss": dict(self._pathloss)}
