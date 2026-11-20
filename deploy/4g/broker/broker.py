#!/usr/bin/env python3
"""The "air": one shared radio medium for every srsRAN cell and UE.

Every UE hears every cell and every cell hears every UE, each through its own runtime gain
(see air.py). The air is the only clock; phones and towers can leave it and join it again while
it runs (engine.py, zmqair.py).
"""
import os
import threading
import time

from air import load_config
from api import make_server
from control import Controller
from world import World
from zmqair import ZmqAir

CONFIG = os.environ.get("AIR_CONFIG", "/conf/air.json")


TICK_S = 0.2  # wall-clock period of the world tick


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
    air = holder["air"] = ZmqAir(cfg, ctl.initial_gains(), on_presence=ctl.set_present)
    links = ", ".join(f"{u}<->{c}={g:.3g}" for (u, c), g in sorted(ctl.initial_gains().items()))
    log(f"air: {len(cfg.cells)} cells, {len(cfg.ues)} UEs; gains {links}")
    api = make_server(cfg.api_port, os.environ.get("AIR_TOKEN", "dev-air-token"), ctl, ctl.override_gain)
    threading.Thread(target=api.serve_forever, daemon=True).start()
    log(f"air: control API on port {cfg.api_port}")
    threading.Thread(target=run_ticks, args=(air, ctl, cfg.srate), daemon=True).start()
    air.run()


if __name__ == "__main__":
    main()
