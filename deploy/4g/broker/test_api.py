import json
import threading
import unittest
import urllib.error
import urllib.request

from api import make_server
from control import Controller
from test_world import config
from world import World

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


class WorldApiTest(ApiTest):
    """The mobility routes, against a real World behind a Controller."""

    def setUp(self):
        self.calls = []
        self.ctl = Controller(World(config()), lambda ue, cell, g: self.calls.append((ue, cell, g)),
                              log=lambda *_: None)
        self.server = make_server(0, TOKEN, self.ctl, self.ctl.override_gain)
        self.port = self.server.server_address[1]
        threading.Thread(target=self.server.serve_forever, daemon=True).start()

    def ue(self, ue_id):
        return next(u for u in self.request("GET", "/world")[1]["ues"] if u["id"] == ue_id)

    def test_put_gains_calls_set_gain(self):
        status, _ = self.request("PUT", "/ues/ue2/gains", {"A": 0.1, "B": 1.0})
        self.assertEqual(status, 200)
        self.assertEqual(self.ue("ue2")["gains"], {"A": 0.1, "B": 1.0})

    def test_travel_202(self):
        status, body = self.request("POST", "/ues/ue2/travel", {"x": 8, "y": 0, "speed_kmh": 100})
        self.assertEqual(status, 202)
        self.assertEqual(body["id"], "ue2")
        self.assertEqual(body["travel"]["x"], 8.0)
        self.assertEqual(self.ue("ue2")["travel"]["speed_kmh"], 100.0)

    def test_travel_bad_speed_400(self):
        for speed in (0, -10, "fast", None, True):
            status, body = self.request("POST", "/ues/ue2/travel", {"x": 8, "y": 0, "speed_kmh": speed})
            self.assertEqual(status, 400, speed)
            self.assertIn("error", body)
        self.assertIsNone(self.ue("ue2")["travel"])

    def test_travel_missing_field_400(self):
        self.assertEqual(self.request("POST", "/ues/ue2/travel", {"x": 8, "speed_kmh": 10})[0], 400)

    def test_travel_to_current_position_400(self):
        status, body = self.request("POST", "/ues/ue2/travel", {"x": 2, "y": 0, "speed_kmh": 10})
        self.assertEqual(status, 400)
        self.assertIn("already", body["error"])

    def test_travel_unknown_ue_404(self):
        self.assertEqual(self.request("POST", "/ues/ue9/travel", {"x": 8, "y": 0, "speed_kmh": 10})[0], 404)

    def test_position_updates_snapshot(self):
        status, body = self.request("PUT", "/ues/ue3/position", {"x": 30, "y": 5})
        self.assertEqual(status, 200)
        ue = self.ue("ue3")
        self.assertEqual((ue["x"], ue["y"]), (30.0, 5.0))
        self.assertEqual(ue["gains"], {"A": 0.0, "B": 0.0})  # far from both cells

    def test_shielded_toggle(self):
        self.assertEqual(self.request("PUT", "/ues/ue1/shielded", {"shielded": True})[0], 200)
        self.assertEqual(self.ue("ue1")["gains"], {"A": 0.0, "B": 0.0})
        self.assertTrue(self.ue("ue1")["shielded"])
        self.assertEqual(self.request("PUT", "/ues/ue1/shielded", {"shielded": False})[0], 200)
        self.assertEqual(self.ue("ue1")["gains"]["A"], 1.0)

    def test_shielded_must_be_bool_400(self):
        for bad in ({"shielded": "yes"}, {"shielded": 1}, {}):
            self.assertEqual(self.request("PUT", "/ues/ue1/shielded", bad)[0], 400, bad)

    def test_world_returns_snapshot(self):
        status, body = self.request("GET", "/world")
        self.assertEqual(status, 200)
        self.assertEqual([u["id"] for u in body["ues"]], ["ue1", "ue2", "ue3"])

    def test_world_has_radio_clock(self):
        self.assertIn("radio", self.request("GET", "/world")[1])

    def cell(self, cell_id):
        return next(c for c in self.request("GET", "/world")[1]["cells"] if c["id"] == cell_id)

    def test_put_cell_position(self):
        status, body = self.request("PUT", "/cells/B/position", {"x": 9, "y": 0})
        self.assertEqual(status, 200)
        self.assertEqual((body["id"], body["x"], body["y"]), ("B", 9.0, 0.0))
        self.assertEqual(body["command"], "curl -X PUT -H \"Authorization: Bearer $AIR_TOKEN\" "
                         "-d '{\"x\": 9, \"y\": 0}' http://127.0.0.1:8090/cells/B/position")
        self.assertEqual(self.cell("B")["x"], 9.0)
        self.assertEqual(self.ue("ue3")["gains"]["B"], 1.0)

    def test_put_cell_position_dry_run(self):
        status, body = self.request("PUT", "/cells/B/position?dry_run=1", {"x": 30, "y": 0})
        self.assertEqual(status, 200)
        self.assertEqual({u["id"]: u["effect"] for u in body["ues"]}["ue3"], "loses")
        self.assertEqual(self.cell("B")["x"], 10.0)

    def test_put_cell_position_errors(self):
        self.assertEqual(self.request("PUT", "/cells/Z/position", {"x": 1, "y": 0})[0], 404)
        self.assertEqual(self.request("PUT", "/cells/B/position", {"x": "far", "y": 0})[0], 400)
        self.assertEqual(self.request("PUT", "/cells/B/position", {"x": 1})[0], 400)
        self.assertEqual(self.request("POST", "/cells/B/position", {"x": 1, "y": 0})[0], 404)

    def test_post_stop(self):
        self.request("POST", "/ues/ue2/travel", {"x": 8, "y": 0, "speed_kmh": 1000})
        status, body = self.request("POST", "/ues/ue2/stop")
        self.assertEqual(status, 200)
        self.assertIsNone(body["travel"])
        self.assertEqual(body["command"], "curl -X POST -H \"Authorization: Bearer $AIR_TOKEN\" "
                         "http://127.0.0.1:8090/ues/ue2/stop")

    def test_action_responses_have_command(self):
        body = self.request("POST", "/ues/ue2/travel", {"x": 8, "y": 0, "speed_kmh": 1000})[1]
        self.assertEqual(body["command"], "make 4g-travel UE=ue2 TO=8,0 SPEED=1000")
        body = self.request("PUT", "/ues/ue2/position", {"x": 5.5, "y": 1})[1]
        self.assertEqual(body["command"], "make 4g-travel UE=ue2 AT=5.5,1")
        body = self.request("PUT", "/ues/ue2/shielded", {"shielded": True})[1]
        self.assertEqual(body["command"], "make 4g-travel UE=ue2 SHIELD=on")

    def test_world_has_pathloss(self):
        self.assertEqual(self.request("GET", "/world")[1]["pathloss"]["edge_km"], 7.0)


if __name__ == "__main__":
    unittest.main()
