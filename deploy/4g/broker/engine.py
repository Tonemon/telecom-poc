"""The air's engine: one clock for every cell and UE, and room for them to come and go.

Pure numpy, no sockets (zmqair.py does the ZMQ side), so it is unit-testable anywhere.

Every srsRAN endpoint blocks on the air: its tx is a REP the air asks for samples, its rx is a
REQ the air answers. So an endpoint's own sample index is exactly the number of samples it has
exchanged with the air since its process started, and nothing is ever dropped or inserted on a
stream it is reading: stream position stays time.

The air advances in chunks of k samples ("air time" t). When a new endpoint process asks for its
first rx chunk, its session starts: a0 = the air time of the chunk that becomes its sample 0.
Its tx sample j then belongs at air time a0 + j. An endpoint without a session, or whose tx does
not reach the current chunk, contributes silence. Each chunk is gained and summed exactly as in
air.py:

    DL into UE u   = sum over cells c of  g[u][c] * (cell c's transmit)
    UL into cell c = sum over UEs   u of  g[u][c] * (UE u's transmit)

Who the air waits for: an endpoint that has transmitted in this session (the others are blocked
on the air meanwhile, so no radio time passes for them). A booting endpoint that has not
transmitted yet never holds the air up: when its rx queue is full the oldest undelivered chunk
is dropped and its a0 moves on by k. Nothing it sent has been placed yet, so that is exact; to
the endpoint it is a gap in reception, which cell search tolerates.

Endpoints are keys ("cell", id) and ("ue", id): a cell and a UE may share an id.
"""
import threading

import numpy as np

K = 11520   # 1 ms at 11.52 MHz
AHEAD = 4   # chunks the air may queue for an endpoint that has not asked for them yet
C64 = np.complex64


class _Endpoint:
    def __init__(self):
        self.reset()

    def reset(self):
        self.a0 = None                   # air time of its sample 0; None: no session
        self.buf = np.zeros(0, C64)      # its tx samples not yet consumed
        self.buf_start = 0               # its own index of buf[0]
        self.transmitted = False         # any tx in this session (or before it started)
        self.queue = []                  # rx chunks ready to send, oldest first
        self.pending = 0                 # rx requests not answered yet

    def tx_end(self):
        return self.a0 + self.buf_start + len(self.buf)

    def chunk(self, t, k):
        """Its tx for air time [t, t+k); zeros where it has nothing."""
        out = np.zeros(k, C64)
        if self.a0 is None:
            return out
        start = t - self.a0 - self.buf_start  # index into buf of air time t
        lo, hi = max(0, start), min(len(self.buf), start + k)
        if hi > lo:
            out[lo - start:hi - start] = self.buf[lo:hi]
        return out

    def consume_until(self, t):
        """Forget tx samples for air time before t."""
        if self.a0 is None:
            return
        drop = min(len(self.buf), max(0, t - self.a0 - self.buf_start))
        if drop:
            self.buf = self.buf[drop:]
            self.buf_start += drop


class Engine:
    def __init__(self, cells, ues, gains, k=K, ahead=AHEAD, on_presence=lambda ep, on: None):
        self.cells, self.ues = list(cells), list(ues)
        self.k, self.ahead = k, ahead
        self.t = 0
        self._gain = {key: float(g) for key, g in gains.items()}
        self._gain_lock = threading.Lock()
        self._on_presence = on_presence
        self._eps = {("cell", c): _Endpoint() for c in self.cells}
        self._eps.update({("ue", u): _Endpoint() for u in self.ues})

    def set_gain(self, ue, cell, value):
        if ("ue", ue) not in self._eps or ("cell", cell) not in self._eps:
            raise KeyError(f"unknown link {ue} <-> {cell}")
        with self._gain_lock:
            self._gain[(ue, cell)] = float(value)

    # ---- endpoint events ----------------------------------------------------------------
    def tx_data(self, ep, samples):
        e = self._eps[ep]
        e.buf = np.concatenate([e.buf, np.asarray(samples, C64)])
        e.transmitted = True

    def join(self, ep):
        e = self._eps[ep]
        e.a0 = self.t
        e.queue, e.pending = [], 0
        self._on_presence(ep, True)

    def leave(self, ep):
        e = self._eps[ep]
        if e.a0 is None and not e.transmitted:
            return
        was_on = e.a0 is not None
        e.reset()
        if was_on:
            self._on_presence(ep, False)

    def rx_request(self, ep):
        self._eps[ep].pending += 1

    def replies(self, ep):
        e = self._eps[ep]
        n = min(e.pending, len(e.queue))
        out, e.queue = e.queue[:n], e.queue[n:]
        e.pending -= n
        return out

    def present(self, ep):
        return self._eps[ep].a0 is not None

    # ---- the clock ----------------------------------------------------------------------
    def ready(self):
        """May chunk [t, t+k) be produced now? Drops chunks for booting endpoints (see top)."""
        for e in self._eps.values():
            if e.a0 is None:
                continue
            if len(e.queue) >= self.ahead:
                if e.transmitted:
                    return False
                e.queue.pop(0)
                e.a0 += self.k
            if e.transmitted and e.tx_end() < self.t + self.k:
                return False
        return True

    def step(self):
        t, k = self.t, self.k
        with self._gain_lock:
            g = dict(self._gain)
        cell_tx = {c: self._eps[("cell", c)].chunk(t, k) for c in self.cells}
        ue_tx = {u: self._eps[("ue", u)].chunk(t, k) for u in self.ues}
        for u in self.ues:
            e = self._eps[("ue", u)]
            if e.a0 is not None:
                out = np.zeros(k, C64)
                for c in self.cells:
                    if g.get((u, c)):
                        out += C64(g[(u, c)]) * cell_tx[c]
                e.queue.append(out)
        for c in self.cells:
            e = self._eps[("cell", c)]
            if e.a0 is not None:
                out = np.zeros(k, C64)
                for u in self.ues:
                    if g.get((u, c)):
                        out += C64(g[(u, c)]) * ue_tx[u]
                e.queue.append(out)
        self.t = t + k
        for e in self._eps.values():
            e.consume_until(self.t)
