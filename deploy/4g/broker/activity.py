"""Random and realtime activity: who moves where, and when they decide again.

Pure Python (no threads, no sockets): the controller asks plan() for a UE's next move and
keeps an Automation that tracks, per UE, whether it is under automation, on a manual trip,
shielded, or pausing before its next decision. Time only advances through due(radio_dt_s),
in radio seconds, like travel.
"""
import math

from world import gain_for_distance

STAY_P = 1 / 3                  # chance a UE stays where it is
IN_COVERAGE_P = 0.9             # chance a destination is somewhere a cell is heard
SPEED_RANGE = (300.0, 1500.0)   # km/h, radio time: what the handover can follow
PAUSE_RANGE_S = (2.0, 10.0)     # radio seconds between decisions (realtime activity)
BOUNDS = (-10.0, 20.0, -10.0, 10.0)  # xmin, xmax, ymin, ymax (km): the map
_TRIES = 1000


def _covered(x, y, cells, pathloss):
    return any(gain_for_distance(math.hypot(x - c["x"], y - c["y"]), pathloss) > 0 for c in cells)


def _point(rng, want_covered, cells, pathloss):
    xmin, xmax, ymin, ymax = BOUNDS
    for _ in range(_TRIES):
        x, y = rng.uniform(xmin, xmax), rng.uniform(ymin, ymax)
        if _covered(x, y, cells, pathloss) == want_covered:
            return x, y
    return rng.uniform(xmin, xmax), rng.uniform(ymin, ymax)


def plan(ue_id, cells, pathloss, rng):
    """One decision for one UE: {"ue", "action": "stay"} or a trip with x, y, speed_kmh."""
    if rng.random() < STAY_P:
        return {"ue": ue_id, "action": "stay"}
    x, y = _point(rng, rng.random() < IN_COVERAGE_P, cells, pathloss)
    speed = round(rng.uniform(*SPEED_RANGE) / 10) * 10
    return {"ue": ue_id, "action": "travel", "x": round(x, 2), "y": round(y, 2),
            "speed_kmh": float(min(max(speed, SPEED_RANGE[0]), SPEED_RANGE[1]))}


class Automation:
    """Per-UE automation state. state() uses the /world vocabulary:
    shielded, travelling (a manual trip), auto (under realtime activity), manual."""

    def __init__(self, rng, ues):
        self._rng = rng
        self.realtime = False
        self._ues = {u: {"shielded": False, "manual": False, "auto_trip": False, "pause": None}
                     for u in ues}
        self._order = list(ues)

    def _u(self, ue):
        if ue not in self._ues:
            raise KeyError(ue)
        return self._ues[ue]

    def state(self, ue):
        s = self._u(ue)
        if s["shielded"]:
            return "shielded"
        if s["manual"]:
            return "travelling"
        return "auto" if self.realtime else "manual"

    def eligible(self):
        """UEs a random decision may move: not shielded, not on a manual trip."""
        return [u for u in self._order if not self._ues[u]["shielded"] and not self._ues[u]["manual"]]

    def _pause(self, s):
        s["pause"] = self._rng.uniform(*PAUSE_RANGE_S) if self.realtime and not s["shielded"] else None

    def set_realtime(self, on):
        """On: the UEs due right away. Off: the UEs on an automated trip, to be stopped.
        Switching to the state it is already in changes nothing."""
        if bool(on) == self.realtime:
            return []
        self.realtime = bool(on)
        if on:
            due = []
            for u in self.eligible():
                s = self._ues[u]
                if not s["auto_trip"]:
                    s["pause"] = None
                    due.append(u)
            return due
        stopped = [u for u in self._order if self._ues[u]["auto_trip"]]
        for s in self._ues.values():
            s["auto_trip"], s["pause"] = False, None
        return stopped

    def planned(self, ue, decision):
        s = self._u(ue)
        if decision["action"] == "travel":
            s["auto_trip"], s["pause"] = True, None
        else:
            self._pause(s)

    def on_manual(self, ue):
        s = self._u(ue)
        s["manual"], s["auto_trip"], s["pause"] = True, False, None

    def on_arrived(self, ue):
        s = self._u(ue)
        s["manual"] = s["auto_trip"] = False
        self._pause(s)

    def on_shielded(self, ue, shielded):
        s = self._u(ue)
        s["shielded"] = bool(shielded)
        if shielded:
            s["pause"] = None
        elif not s["manual"] and not s["auto_trip"]:
            self._pause(s)

    def due(self, radio_dt_s):
        """Advance the pauses; the UEs whose pause ran out (each reported once)."""
        out = []
        for u in self._order:
            s = self._ues[u]
            if s["pause"] is None:
                continue
            s["pause"] -= radio_dt_s
            if s["pause"] <= 0:
                s["pause"] = None
                out.append(u)
        return out
