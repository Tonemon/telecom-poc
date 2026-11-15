#!/usr/bin/env python3
"""GNU Radio "air" broker: one flowgraph for every srsRAN cell and UE.

Every UE hears every cell and every cell hears every UE, each through its own runtime
gain (see air.py). All endpoints run in one ZMQ REQ/REP lockstep: no sample is dropped
or inserted, so stream position stays time. GNU Radio is imported lazily in main() so
the pure modules stay testable without it.
"""
import os
import threading
import time

from air import load_config, plan_air
from api import make_server
from control import Controller
from world import World

CONFIG = os.environ.get("AIR_CONFIG", "/conf/air.json")


TICK_S = 0.2  # wall-clock period of the world tick


class Air:
    def __init__(self, cfg, gains):
        """gains: (ue, cell) -> starting gain for every link."""
        from gnuradio import gr, blocks, zeromq  # lazy: keeps the planner testable

        cx = gr.sizeof_gr_complex
        plan = plan_air(cfg)
        self._lock = threading.Lock()
        self.tb = gr.top_block()
        self.gain_blocks = {}

        def gain(ue, cell):
            g = blocks.multiply_const_cc(gains[(ue, cell)])
            self.gain_blocks[(ue, cell)] = g
            return g

        # Same socket parameters as the per-cell brokers had: 100 ms timeout, no tags, no HWM.
        cell_tx = {c.id: zeromq.req_source(cx, 1, c.dl, 100, False, -1) for c in cfg.cells}
        ue_tx = {u.id: zeromq.req_source(cx, 1, u.ul, 100, False, -1) for u in cfg.ues}

        # Downlink: each UE's rx = sum over cells of gain * cell tx.
        for u in cfg.ues:
            add = blocks.add_cc(1)
            for i, link in enumerate(plan.dl[u.id]):
                g = gain(link.ue, link.cell)
                self.tb.connect(cell_tx[link.cell], g, (add, i))
            self.tb.connect(add, zeromq.rep_sink(cx, 1, u.dl, 100, False, -1))
        first = plan.dl[cfg.ues[0].id][0]
        self._clock_block = self.gain_blocks[(first.ue, first.cell)]  # radio clock, see radio_samples()

        # Uplink: each cell's rx = sum over UEs of gain * UE tx. The UL gain blocks are
        # separate from the DL ones but share the same (ue, cell) value via set_gain().
        self.ul_gain_blocks = {}
        for c in cfg.cells:
            add = blocks.add_cc(1)
            for i, link in enumerate(plan.ul[c.id]):
                g = blocks.multiply_const_cc(gains[(link.ue, link.cell)])
                self.ul_gain_blocks[(link.ue, link.cell)] = g
                self.tb.connect(ue_tx[link.ue], g, (add, i))
            self.tb.connect(add, zeromq.rep_sink(cx, 1, c.ul, 100, False, -1))

    def set_gain(self, ue, cell, value):
        """Set the radio link between one UE and one cell, in both directions."""
        key = (ue, cell)
        if key not in self.gain_blocks:
            raise KeyError(f"unknown link {ue} <-> {cell}")
        with self._lock:
            self.gain_blocks[key].set_k(complex(value))
            self.ul_gain_blocks[key].set_k(complex(value))

    def radio_samples(self):
        """Samples that have passed through the air so far (on the first link). Divided by the
        sample rate this is radio time, which runs slower than wall time on a busy host."""
        return self._clock_block.nitems_written(0)


def log(msg):
    print(msg, flush=True)


def run_ticks(air, ctl, srate):
    """Advance the world by the radio time that passed, every TICK_S of wall time."""
    last_n, last_t = air.radio_samples(), time.monotonic()
    while True:
        time.sleep(TICK_S)
        n, t = air.radio_samples(), time.monotonic()
        try:
            ctl.tick((n - last_n) / srate, t - last_t)
        except Exception as e:  # never let a bad tick kill the clock
            log(f"air: tick failed: {e!r}")
        last_n, last_t = n, t


def main():
    with open(CONFIG) as f:
        cfg = load_config(f.read())
    holder = {}
    ctl = Controller(World(cfg), lambda ue, cell, g: holder["air"].set_gain(ue, cell, g), log=log)
    air = holder["air"] = Air(cfg, ctl.initial_gains())
    links = ", ".join(f"{u}<->{c}={g:.3g}" for (u, c), g in sorted(ctl.initial_gains().items()))
    log(f"air: {len(cfg.cells)} cells, {len(cfg.ues)} UEs; gains {links}")
    api = make_server(cfg.api_port, os.environ.get("AIR_TOKEN", "dev-air-token"), ctl, ctl.override_gain)
    threading.Thread(target=api.serve_forever, daemon=True).start()
    log(f"air: control API on port {cfg.api_port}")
    air.tb.start()
    threading.Thread(target=run_ticks, args=(air, ctl, cfg.srate), daemon=True).start()
    air.tb.wait()


if __name__ == "__main__":
    main()
