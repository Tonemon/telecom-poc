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

Who the air waits for: an endpoint whose tx burst is coming (the others are blocked on the air
meanwhile, so no radio time passes for them); see ready(). A booting endpoint that has not
transmitted yet never holds the air up: when its rx queue is full the oldest undelivered chunk
is dropped and its a0 moves on by k. Nothing it sent has been placed yet, so that is exact; to
the endpoint it is a gap in reception, which cell search tolerates.

Endpoints are keys ("cell", id) and ("ue", id): a cell and a UE may share an id.
"""
import threading
import time

import numpy as np

K = 11520   # 1 ms at 11.52 MHz
AHEAD = 4   # chunks the air may queue for an endpoint that has not asked for them yet
GAP_S = 0.2  # wall time without tx after which an endpoint's tx burst counts as ended
C64 = np.complex64


class _Endpoint:
    def __init__(self):
        self.reset()

    def reset(self):
        self.a0 = None                   # air time of its sample 0; None: no session
        self.buf = np.zeros(0, C64)      # its tx samples not yet consumed
        self.buf_start = 0               # its own index of buf[0]
        self.transmitted = False         # any tx in this session (or before it started)
        self.bursting = False            # tx is coming: the air waits for it
        self.last_tx = 0.0               # wall time of its last tx
        self.queue = []                  # rx chunks ready to send, oldest first
        self.pending = 0                 # rx requests not answered yet

    def queued(self):
        return sum(len(c) for c in self.queue)

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
    def __init__(self, cells, ues, gains, k=K, ahead=AHEAD, on_presence=lambda ep, on: None,
                 gap_s=GAP_S, clock=time.monotonic):
        self.cells, self.ues = list(cells), list(ues)
        self.k, self.ahead = k, ahead
        self.gap_s, self._clock = gap_s, clock
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
        e.transmitted = e.bursting = True
        e.last_tx = self._clock()

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

    def debug(self):
        """One line per endpoint: what the air knows about it (for AIR_DEBUG logging)."""
        out = []
        for ep, e in self._eps.items():
            end = e.tx_end() - self.t if e.a0 is not None else None
            out.append(f"{ep[1]}: a0={e.a0} tx_ahead={end} tx={e.transmitted} burst={e.bursting} "
                       f"queue={len(e.queue)} pending={e.pending}")
        return f"t={self.t} " + " | ".join(out)

    def _span(self):
        n = self.k
        for e in self._eps.values():
            if e.a0 is not None and e.bursting:
                n = min(n, e.tx_end() - self.t)
        return n

    def present(self, ep):
        return self._eps[ep].a0 is not None

    # ---- the clock ----------------------------------------------------------------------
    def ready(self):
        """How many samples the air may produce now (0: it must wait), at most k.

        The air waits for an endpoint only while its tx burst is coming. srsue ends a burst
        (e.g. around cell search and random access) and then sends nothing until its next
        transmission, when srsRAN fills the gap with zeros; those arrive too late and are
        dropped, which costs nothing. A burst counts as ended after gap_s of wall time without
        tx while the air needs it.

        The step is shorter than k when a bursting endpoint has transmitted only part of the
        next chunk: srsRAN may run just a few samples ahead of what it has received, and
        waiting for a whole chunk would deadlock (it waits for rx, the air for its tx).

        Booting endpoints (never transmitted) get the boot drop, see the top of the module.
        An endpoint that has transmitted and stops reading holds the air: dropping its rx would
        shift its timing."""
        n, now = self.k, self._clock()
        for e in self._eps.values():
            if e.a0 is None:
                continue
            while e.queued() >= self.ahead * self.k and not e.transmitted:
                e.a0 += len(e.queue.pop(0))
            if e.queued() >= self.ahead * self.k:
                return 0
            if e.bursting:
                have = e.tx_end() - self.t
                if have <= 0 and now - e.last_tx > self.gap_s:
                    e.bursting = False
                    continue
                n = min(n, have)
        return max(0, n)

    def step(self, n=None):
        """Produce air time [t, t+n) (n defaults to what ready() allows) into every rx queue."""
        k = self._span() if n is None else n
        if k <= 0:
            raise RuntimeError("step() while the air must wait")
        t = self.t
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
