#!/usr/bin/env python3
"""GNU Radio ZMQ broker for one srsRAN cell: sum UE uplinks into the eNB,
fan the eNB downlink out to every UE. The pure planner is import-safe without
GNU Radio so it can be unit-tested; GNU Radio is imported lazily in main()."""
import os
from dataclasses import dataclass, field


@dataclass
class BrokerPlan:
    dl_source: str                                   # REQ source: connect to eNB tx
    dl_sinks: list = field(default_factory=list)     # REP sinks: bind, one per UE rx
    ul_sources: list = field(default_factory=list)   # REQ sources: connect to each UE tx
    ul_sink: str = ""                                # REP sink: bind, eNB rx connects here


def plan_broker(enb_dl, enb_ul, ues):
    return BrokerPlan(
        dl_source=enb_dl,
        dl_sinks=[dl for (dl, _ul) in ues],
        ul_sources=[ul for (_dl, ul) in ues],
        ul_sink=enb_ul,
    )


def parse_ues(spec):
    """'2111,10.0.0.61:2001 2112,10.0.0.62:2001' ->
       [('tcp://*:2111','tcp://10.0.0.61:2001'), ...]"""
    out = []
    for entry in spec.split():
        dl_port, ul = entry.split(",", 1)
        out.append((f"tcp://*:{dl_port}", f"tcp://{ul}"))
    return out


def main():
    from gnuradio import gr, blocks, zeromq  # lazy: keeps planner testable
    srate = float(os.environ["SRATE"])
    plan = plan_broker(os.environ["ENB_DL"], os.environ["ENB_UL"],
                       parse_ues(os.environ["UES"]))
    tb = gr.top_block()
    cx = gr.sizeof_gr_complex
    # Downlink: one REQ source from the eNB, fanned out to N REP sinks (UEs).
    dl_src = zeromq.req_source(cx, 1, plan.dl_source, 100, False, -1)
    for addr in plan.dl_sinks:
        tb.connect(dl_src, zeromq.rep_sink(cx, 1, addr, 100, False, -1))
    # Uplink: N REQ sources (UEs) summed, one REP sink to the eNB.
    adder = blocks.add_cc(1)
    for i, addr in enumerate(plan.ul_sources):
        tb.connect(zeromq.req_source(cx, 1, addr, 100, False, -1), (adder, i))
    tb.connect(adder, zeromq.rep_sink(cx, 1, plan.ul_sink, 100, False, -1))
    print(f"broker: DL {plan.dl_source} -> {plan.dl_sinks}; "
          f"UL {plan.ul_sources} -> {plan.ul_sink}", flush=True)
    tb.start()
    tb.wait()


if __name__ == "__main__":
    main()
