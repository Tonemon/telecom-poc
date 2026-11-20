"""The ZMQ side of the air: talks to every srsRAN endpoint and drives the engine (engine.py).

srsRAN keeps its stock sockets: an endpoint's tx is a REP the air asks for samples, its rx a REQ
the air answers. The air takes the robust sides of those pairs:

- a DEALER connected to each endpoint's tx REP, with a few requests outstanding (every reply is
  just the next part of the stream, so extra requests cost nothing; requests lost with a dead
  process are sent again after the disconnect), so a vanished peer never wedges a REQ/REP state machine and srsRAN
  never waits to be asked;
- a ROUTER bound for each endpoint's rx REQ, so a new process shows up as a new peer identity:
  that is when its session starts.

An endpoint has left only when its connection is gone: a socket monitor reports the disconnect,
and ZMQ heartbeats (answered by libzmq's own I/O thread, whatever the endpoint is doing) close a
connection whose peer vanished without closing it, e.g. a tower whose network was cut. Silence
is never taken as leaving: when the air waits for one endpoint, the others are quiet because
they are blocked on the air.
"""
import os
import threading
import time

import numpy as np
import zmq
from zmq.utils.monitor import recv_monitor_message

from engine import K, Engine

IN_FLIGHT = 4   # tx requests kept outstanding per endpoint, so srsRAN never waits to be asked


class _Tx:
    def __init__(self, ctx, addr, hb):
        self.sock = ctx.socket(zmq.DEALER)
        _common(self.sock, hb)
        self.sock.setsockopt(zmq.IMMEDIATE, 1)  # queue requests only to a connected peer
        self.sock.connect(addr)
        self.outstanding = 0  # requests lost with a peer that went away: reset on disconnect

    def request(self):
        while self.outstanding < IN_FLIGHT:
            try:
                self.sock.send_multipart([b"", b"\xff"], zmq.NOBLOCK)
            except zmq.Again:
                return  # nobody connected yet
            self.outstanding += 1

    def answered(self):
        self.outstanding = max(0, self.outstanding - 1)


class _Rx:
    def __init__(self, ctx, addr, hb):
        self.sock = ctx.socket(zmq.ROUTER)
        _common(self.sock, hb)
        self.sock.bind(addr)
        self.ident = None


def _common(sock, hb):
    ivl, timeout = hb
    sock.setsockopt(zmq.LINGER, 0)
    sock.setsockopt(zmq.HEARTBEAT_IVL, ivl)
    sock.setsockopt(zmq.HEARTBEAT_TIMEOUT, timeout)
    sock.setsockopt(zmq.HEARTBEAT_TTL, timeout)


class ZmqAir:
    def __init__(self, cfg, gains, on_presence=lambda ep, on: None,
                 heartbeat_ivl_ms=250, heartbeat_timeout_ms=1500, k=K):
        self.engine = Engine([c.id for c in cfg.cells], [u.id for u in cfg.ues], gains, k=k,
                             on_presence=on_presence)
        self._ctx = zmq.Context()
        hb = (heartbeat_ivl_ms, heartbeat_timeout_ms)
        self._tx, self._rx = {}, {}
        for c in cfg.cells:
            self._tx[("cell", c.id)] = _Tx(self._ctx, c.dl, hb)
            self._rx[("cell", c.id)] = _Rx(self._ctx, c.ul, hb)
        for u in cfg.ues:
            self._tx[("ue", u.id)] = _Tx(self._ctx, u.ul, hb)
            self._rx[("ue", u.id)] = _Rx(self._ctx, u.dl, hb)
        self._poller = zmq.Poller()
        self._data, self._mons = {}, {}
        for ep in self._tx:
            for s in (self._tx[ep].sock, self._rx[ep].sock):
                self._poller.register(s, zmq.POLLIN)
                self._data[s] = ep
                mon = s.get_monitor_socket(zmq.EVENT_DISCONNECTED)
                self._poller.register(mon, zmq.POLLIN)
                self._mons[mon] = ep
        self._stop = threading.Event()

    # ---- the interface broker.py and control.py use -------------------------------------
    def set_gain(self, ue, cell, value):
        self.engine.set_gain(ue, cell, value)

    def radio_samples(self):
        return self.engine.t

    def close(self):
        self._stop.set()

    # ---- the loop -----------------------------------------------------------------------
    def run(self):
        eng = self.engine
        debug = os.environ.get("AIR_DEBUG") == "1"
        last_dbg, steps = time.monotonic(), 0
        try:
            while not self._stop.is_set():
                if debug and time.monotonic() - last_dbg > 2:
                    print(f"air: debug {steps / (time.monotonic() - last_dbg):.0f} steps/s "
                          f"{eng.debug()}", flush=True)
                    last_dbg, steps = time.monotonic(), 0
                for tx in self._tx.values():
                    tx.request()
                self._poll(0 if eng.ready() else 5)
                if eng.ready():
                    eng.step()
                    steps += 1
                self._flush()
                if not any(eng.present(ep) for ep in self._rx):
                    time.sleep(0.001)
        finally:
            for s in list(self._data) + list(self._mons):
                s.close(linger=0)
            self._ctx.term()

    def _flush(self):
        for ep, rx in self._rx.items():
            if rx.ident is None:
                continue
            for chunk in self.engine.replies(ep):
                rx.sock.send_multipart([rx.ident, b"", chunk.tobytes()])

    def _poll(self, timeout_ms):
        for sock, _ in self._poller.poll(timeout_ms):
            if sock in self._mons:
                self._disconnected(sock)
            else:
                self._receive(sock)

    def _disconnected(self, mon):
        ep = self._mons[mon]
        while True:
            try:
                recv_monitor_message(mon, zmq.NOBLOCK)
            except zmq.Again:
                break
        self._leave(ep)

    def _leave(self, ep, drain=True):
        """Forget the endpoint, and (drain) whatever its dead process left queued on our sockets."""
        tx, rx = self._tx[ep], self._rx[ep]
        for s in (tx.sock, rx.sock) if drain else ():
            while True:
                try:
                    s.recv_multipart(zmq.NOBLOCK)
                except zmq.Again:
                    break
        tx.outstanding = 0
        rx.ident = None
        self.engine.leave(ep)

    def _receive(self, sock):
        ep = self._data[sock]
        tx, rx = self._tx[ep], self._rx[ep]
        while True:
            try:
                frames = sock.recv_multipart(zmq.NOBLOCK)
            except zmq.Again:
                return
            if sock is tx.sock:
                tx.answered()
                self.engine.tx_data(ep, np.frombuffer(frames[-1], np.complex64))
                continue
            ident = frames[0]
            if ident != rx.ident:
                if rx.ident is not None:
                    self._leave(ep, drain=False)  # a new process before the old one's disconnect
                rx.ident = ident
                self.engine.join(ep)
            self.engine.rx_request(ep)
