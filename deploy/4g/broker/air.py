"""The shared "air": one radio medium for every cell and every UE.

Pure configuration and planning, no GNU Radio import, so it is unit-testable anywhere.
Each UE hears every cell and every cell hears every UE, each through its own gain:

    DL into UE u   = sum over cells c of  g[u][c] * (cell c's transmit)
    UL into cell c = sum over UEs   u of  g[u][c] * (UE u's transmit)

A gain of 0 means "out of range"; 1 means "full signal".
"""
import json
from dataclasses import dataclass, field


@dataclass
class Cell:
    id: str
    name: str
    x: float
    y: float
    dl: str  # the eNB's tx address; the air connects (REQ) to it
    ul: str  # the air binds (REP) here; the eNB's rx connects to it


@dataclass
class Ue:
    id: str
    x: float
    y: float
    dl: str  # the air binds (REP) here; the UE's rx connects to it
    ul: str  # the UE's tx address; the air connects (REQ) to it


@dataclass
class AirConfig:
    cells: list
    ues: list
    srate: float
    api_port: int
    pathloss: dict = field(default_factory=dict)
    gain: dict = field(default_factory=dict)  # {ue_id: {cell_id: float}}, static until the world runs


@dataclass(frozen=True)
class Link:
    ue: str
    cell: str


@dataclass
class AirPlan:
    dl: dict  # ue_id -> [Link] : one gained cell signal per cell, summed into that UE's rx
    ul: dict  # cell_id -> [Link]: one gained UE signal per UE, summed into that cell's rx


def _unique(values, what):
    seen = set()
    for v in values:
        if v in seen:
            raise ValueError(f"duplicate {what}: {v}")
        seen.add(v)


def load_config(text):
    raw = json.loads(text)
    cells = [Cell(str(c["id"]), str(c["name"]), float(c["x"]), float(c["y"]),
                  str(c["dl"]), str(c["ul"])) for c in raw.get("cells", [])]
    ues = [Ue(str(u["id"]), float(u["x"]), float(u["y"]), str(u["dl"]), str(u["ul"]))
           for u in raw.get("ues", [])]
    if not cells:
        raise ValueError("the air needs at least one cell")
    if not ues:
        raise ValueError("the air needs at least one UE")
    _unique([c.id for c in cells], "cell id")
    _unique([u.id for u in ues], "UE id")
    _unique([c.dl for c in cells] + [c.ul for c in cells] +
            [u.dl for u in ues] + [u.ul for u in ues], "address")

    gain = raw.get("gain", {})
    cell_ids = {c.id for c in cells}
    ue_ids = {u.id for u in ues}
    for ue_id, per_cell in gain.items():
        if ue_id not in ue_ids:
            raise ValueError(f"gain for unknown UE: {ue_id}")
        for cell_id in per_cell:
            if cell_id not in cell_ids:
                raise ValueError(f"gain for unknown cell: {ue_id} -> {cell_id}")

    return AirConfig(cells=cells, ues=ues, srate=float(raw["srate"]),
                     api_port=int(raw.get("api_port", 8090)),
                     pathloss=dict(raw.get("pathloss", {})), gain=gain)


def plan_air(cfg):
    return AirPlan(
        dl={u.id: [Link(u.id, c.id) for c in cfg.cells] for u in cfg.ues},
        ul={c.id: [Link(u.id, c.id) for u in cfg.ues] for c in cfg.cells},
    )


def static_gains(cfg):
    """Every (ue, cell) pair with its configured gain; pairs not listed are 0.0."""
    return {(u.id, c.id): float(cfg.gain.get(u.id, {}).get(c.id, 0.0))
            for u in cfg.ues for c in cfg.cells}
