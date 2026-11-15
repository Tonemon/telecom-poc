#!/usr/bin/env python3
"""GNU Radio "air" broker: one flowgraph for every srsRAN cell and UE.

Every UE hears every cell and every cell hears every UE, each through its own runtime
gain (see air.py). All endpoints run in one ZMQ REQ/REP lockstep: no sample is dropped
or inserted, so stream position stays time. GNU Radio is imported lazily in main() so
the pure modules stay testable without it.
"""
import os

from air import load_config, plan_air, static_gains

CONFIG = os.environ.get("AIR_CONFIG", "/conf/air.json")


class Air:
    def __init__(self, cfg):
        from gnuradio import gr, blocks, zeromq  # lazy: keeps the planner testable

        cx = gr.sizeof_gr_complex
        plan = plan_air(cfg)
        gains = static_gains(cfg)
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
        self.gain_blocks[key].set_k(complex(value))
        self.ul_gain_blocks[key].set_k(complex(value))


def main():
    with open(CONFIG) as f:
        cfg = load_config(f.read())
    air = Air(cfg)
    links = ", ".join(f"{u}<->{c}={g:g}" for (u, c), g in sorted(static_gains(cfg).items()))
    print(f"air: {len(cfg.cells)} cells, {len(cfg.ues)} UEs; gains {links}", flush=True)
    air.tb.start()
    air.tb.wait()


if __name__ == "__main__":
    main()
