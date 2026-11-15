import json
import threading
import unittest
import urllib.error
import urllib.request

from api import make_server

TOKEN = "test-token"


class FakeWorld:
    def snapshot(self):
        return {
            "cells": [{"id": "A", "name": "cell-a", "x": 0.0, "y": 0.0},
                      {"id": "B", "name": "cell-b", "x": 10.0, "y": 0.0}],
            "ues": [{"id": "ue1", "x": 1.0, "y": 0.0, "gains": {"A": 1.0, "B": 0.0}},
                    {"id": "ue2", "x": 2.0, "y": 0.0, "gains": {"A": 1.0, "B": 0.0}}],
        }


class ApiTest(unittest.TestCase):
    def setUp(self):
        self.calls = []
        self.server = make_server(0, TOKEN, FakeWorld(),
                                  lambda ue, cell, value: self.calls.append((ue, cell, value)))
        self.port = self.server.server_address[1]
        threading.Thread(target=self.server.serve_forever, daemon=True).start()

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()

    def request(self, method, path, body=None, token=TOKEN, raw=None):
        data = raw if raw is not None else (json.dumps(body).encode() if body is not None else None)
        req = urllib.request.Request(f"http://127.0.0.1:{self.port}{path}", data=data, method=method)
        if token:
            req.add_header("Authorization", f"Bearer {token}")
        try:
            with urllib.request.urlopen(req) as r:
                return r.status, json.loads(r.read() or b"null")
        except urllib.error.HTTPError as e:
            return e.code, json.loads(e.read() or b"null")

    def test_healthz_no_auth(self):
        status, body = self.request("GET", "/healthz", token=None)
        self.assertEqual(status, 200)
        self.assertEqual(body, {"ok": True})

    def test_world_requires_token(self):
        self.assertEqual(self.request("GET", "/world", token=None)[0], 401)
        self.assertEqual(self.request("GET", "/world", token="wrong")[0], 401)

    def test_world_returns_snapshot(self):
        status, body = self.request("GET", "/world")
        self.assertEqual(status, 200)
        self.assertEqual([u["id"] for u in body["ues"]], ["ue1", "ue2"])

    def test_put_gains_calls_set_gain(self):
        status, _ = self.request("PUT", "/ues/ue2/gains", {"A": 0.1, "B": 1.0})
        self.assertEqual(status, 200)
        self.assertEqual(self.calls, [("ue2", "A", 0.1), ("ue2", "B", 1.0)])

    def test_put_gains_unknown_ue_404(self):
        self.assertEqual(self.request("PUT", "/ues/ue9/gains", {"A": 0.1})[0], 404)
        self.assertEqual(self.calls, [])

    def test_put_gains_unknown_cell_404(self):
        self.assertEqual(self.request("PUT", "/ues/ue2/gains", {"Z": 0.1})[0], 404)
        self.assertEqual(self.calls, [])

    def test_put_gains_negative_400(self):
        status, body = self.request("PUT", "/ues/ue2/gains", {"A": 0.5, "B": -1})
        self.assertEqual(status, 400)
        self.assertIn("error", body)
        self.assertEqual(self.calls, [])  # nothing applied when any value is bad

    def test_put_gains_rejects_non_numbers(self):
        for bad in ({"A": "1"}, {"A": True}, {"A": None}, [1, 2], "x"):
            self.assertEqual(self.request("PUT", "/ues/ue2/gains", bad)[0], 400, bad)
        self.assertEqual(self.request("PUT", "/ues/ue2/gains", raw=b"{not json")[0], 400)
        self.assertEqual(self.request("PUT", "/ues/ue2/gains", raw=b'{"A": NaN}')[0], 400)
        self.assertEqual(self.calls, [])

    def test_put_gains_requires_token(self):
        self.assertEqual(self.request("PUT", "/ues/ue2/gains", {"A": 0.1}, token=None)[0], 401)
        self.assertEqual(self.calls, [])

    def test_unknown_route_404(self):
        self.assertEqual(self.request("GET", "/nope")[0], 404)


if __name__ == "__main__":
    unittest.main()
