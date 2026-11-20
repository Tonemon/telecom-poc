"""Socket-level tests of the ZMQ air with fake srsRAN endpoints on 127.0.0.1.

A fake endpoint is what srsRAN's ZMQ RF driver is: a REP the air asks for tx samples, and a REQ
that asks the air for rx samples.
"""
import json
import socket
import threading
import time
import unittest

import numpy as np
import zmq

from air import load_config
from zmqair import ZmqAir

K = 64


def free_port():
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()
    return port


class FakeEndpoint:
    """tx: REP bound on tx_addr answering every request with K samples of `value`.
    rx: REQ connected to rx_addr, recording the first sample of every chunk it gets."""

    def __init__(self, ctx, tx_addr, rx_addr, value):
        self.value, self.got, self.running = value, [], True
        self.tx = ctx.socket(zmq.REP)
        self.tx.setsockopt(zmq.LINGER, 0)
        self.tx.bind(tx_addr)
        self.rx = ctx.socket(zmq.REQ)
        self.rx.setsockopt(zmq.LINGER, 0)
        self.rx.connect(rx_addr)
        self.threads = [threading.Thread(target=self._serve_tx, daemon=True),
                        threading.Thread(target=self._read_rx, daemon=True)]
        for th in self.threads:
            th.start()

    def _serve_tx(self):
        data = np.full(K, self.value, np.complex64).tobytes()
        while self.running:
            if self.tx.poll(20):
                self.tx.recv()
                self.tx.send(data)

    def _read_rx(self):
        while self.running:
            self.rx.send(b"\xff")
            while self.running and not self.rx.poll(20):
                pass
            if not self.running:
                return
            chunk = np.frombuffer(self.rx.recv(), np.complex64)
            self.got.append(complex(chunk[0]))

    def close(self):
        self.running = False
        for th in self.threads:
            th.join(1)
        self.tx.close()
        self.rx.close()


def wait_for(cond, timeout=5.0):
    end = time.monotonic() + timeout
    while time.monotonic() < end:
        if cond():
            return True
        time.sleep(0.02)
    return False


class ZmqAirTest(unittest.TestCase):
    def setUp(self):
        self.ctx = zmq.Context()
        p = {n: free_port() for n in ("a_tx", "a_rx", "ue1_tx", "ue1_rx", "ue2_tx", "ue2_rx")}
        self.addr = {n: f"tcp://127.0.0.1:{port}" for n, port in p.items()}
        cfg = load_config(json.dumps({
            "srate": 11520000, "api_port": 0,
            "cells": [{"id": "A", "name": "cell-a", "x": 0, "y": 0,
                       "dl": self.addr["a_tx"], "ul": self.addr["a_rx"]}],
            "ues": [{"id": "ue1", "x": 1, "y": 0, "dl": self.addr["ue1_rx"], "ul": self.addr["ue1_tx"]},
                    {"id": "ue2", "x": 2, "y": 0, "dl": self.addr["ue2_rx"], "ul": self.addr["ue2_tx"]}],
        }))
        self.events = []
        self.air = ZmqAir(cfg, {("ue1", "A"): 0.5, ("ue2", "A"): 0.25},
                          on_presence=lambda ep, on: self.events.append((ep, on)), k=K)
        self.thread = threading.Thread(target=self.air.run, daemon=True)
        self.thread.start()
        self.fakes = []

    def tearDown(self):
        for f in self.fakes:
            f.close()
        self.air.close()
        self.thread.join(2)
        self.ctx.destroy(linger=0)

    def fake(self, name, value):
        tx, rx = {"A": ("a_tx", "a_rx"), "ue1": ("ue1_tx", "ue1_rx"), "ue2": ("ue2_tx", "ue2_rx")}[name]
        f = FakeEndpoint(self.ctx, self.addr[tx], self.addr[rx], value)
        self.fakes.append(f)
        return f

    def test_downlink_reaches_a_ue(self):
        self.fake("A", 1.0)
        ue1 = self.fake("ue1", 0.0)
        self.assertTrue(wait_for(lambda: 0.5 in ue1.got[-5:]), ue1.got[-5:])

    def test_ue_leaves_on_disconnect_and_rejoins(self):
        self.fake("A", 1.0)
        ue1 = self.fake("ue1", 0.0)
        ue2 = self.fake("ue2", 0.0)
        self.assertTrue(wait_for(lambda: len(ue1.got) > 10 and len(ue2.got) > 10))
        ue1.close()
        self.fakes.remove(ue1)
        self.assertTrue(wait_for(lambda: (("ue", "ue1"), False) in self.events, 2.0), self.events)
        n = len(ue2.got)
        self.assertTrue(wait_for(lambda: len(ue2.got) > n + 10), "ue2 stopped while ue1 was away")
        ue1b = self.fake("ue1", 0.0)
        self.assertTrue(wait_for(lambda: self.events.count((("ue", "ue1"), True)) == 2), self.events)
        self.assertTrue(wait_for(lambda: 0.5 in ue1b.got[-5:]))
        n = len(ue2.got)
        self.assertTrue(wait_for(lambda: len(ue2.got) > n + 10), "ue2 stopped after ue1 came back")

    def test_tx_requests_are_pipelined(self):
        # srsRAN's tx blocks until asked; several requests in flight keep it from waiting on
        # the air after every subframe (a ROUTER stands in for the REP to see them queue up).
        peek = self.ctx.socket(zmq.ROUTER)
        peek.setsockopt(zmq.LINGER, 0)
        peek.bind(self.addr["ue1_tx"])
        seen = 0
        end = time.monotonic() + 2
        while time.monotonic() < end and seen < 4:
            if peek.poll(50):
                peek.recv_multipart()
                seen += 1
        peek.close()
        self.assertEqual(seen, 4)

    def test_radio_samples_advance(self):
        self.fake("A", 1.0)
        self.fake("ue1", 0.0)
        self.assertTrue(wait_for(lambda: self.air.radio_samples() > 10 * K))
        self.assertEqual(self.air.radio_samples() % K, 0)


if __name__ == "__main__":
    unittest.main()
