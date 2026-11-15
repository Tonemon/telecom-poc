import json
import math
import unittest

from air import load_config
from world import World, gain_for_distance

PATHLOSS = {"ref_km": 1.0, "exponent": 3.0, "edge_km": 7.0}


def config(**overrides):
    cfg = {
        "srate": 11520000,
        "cells": [
            {"id": "A", "name": "cell-a", "x": 0, "y": 0,
             "dl": "tcp://10.0.0.40:2000", "ul": "tcp://*:2100"},
            {"id": "B", "name": "cell-b", "x": 10, "y": 0,
             "dl": "tcp://10.0.0.50:2000", "ul": "tcp://*:2200"},
        ],
        "ues": [
            {"id": "ue1", "x": 1, "y": 0, "dl": "tcp://*:2111", "ul": "tcp://10.0.0.61:2001"},
            {"id": "ue2", "x": 2, "y": 0, "dl": "tcp://*:2112", "ul": "tcp://10.0.0.62:2001"},
            {"id": "ue3", "x": 9, "y": 0, "dl": "tcp://*:2113", "ul": "tcp://10.0.0.63:2001"},
        ],
        "pathloss": PATHLOSS,
    }
    cfg.update(overrides)
    return load_config(json.dumps(cfg))


class TestPathLoss(unittest.TestCase):
    def test_gain_is_one_near_cell(self):
        self.assertEqual(gain_for_distance(0.0, PATHLOSS), 1.0)
        self.assertEqual(gain_for_distance(1.0, PATHLOSS), 1.0)

    def test_gain_zero_beyond_edge(self):
        self.assertEqual(gain_for_distance(7.0, PATHLOSS), 0.0)
        self.assertEqual(gain_for_distance(50.0, PATHLOSS), 0.0)

    def test_gain_monotonic_decreasing(self):
        gains = [gain_for_distance(d / 10, PATHLOSS) for d in range(0, 80)]
        for a, b in zip(gains, gains[1:]):
            self.assertGreaterEqual(a, b)
        self.assertTrue(0 < gain_for_distance(3.0, PATHLOSS) < 1)

    def test_log_distance_slope(self):
        # power ~ d^-n: doubling the distance costs 10*n*log10(2) dB = 9.03 dB for n = 3
        g2, g4 = gain_for_distance(2.0, PATHLOSS), gain_for_distance(4.0, PATHLOSS)
        self.assertAlmostEqual(20 * math.log10(g2 / g4), 30 * math.log10(2), places=6)


class TestWorldGains(unittest.TestCase):
    def test_overlap_zone_exists(self):
        w = World(config())
        both = []
        for x10 in range(1, 100):
            w.place("ue2", x10 / 10, 0)
            g = w.gains()
            if g[("ue2", "A")] > 0 and g[("ue2", "B")] > 0:
                both.append(x10 / 10)
        self.assertTrue(both)
        self.assertLess(min(both), 5)
        self.assertGreater(max(both), 5)

    def test_far_away_no_coverage(self):
        w = World(config())
        w.place("ue3", 30, 0)
        g = w.gains()
        self.assertEqual((g[("ue3", "A")], g[("ue3", "B")]), (0.0, 0.0))

    def test_start_positions_hear_their_own_cell(self):
        g = World(config()).gains()
        self.assertGreater(g[("ue1", "A")], g[("ue1", "B")])
        self.assertGreater(g[("ue2", "A")], g[("ue2", "B")])
        self.assertGreater(g[("ue3", "B")], g[("ue3", "A")])

    def test_shielded_forces_zero_gains(self):
        w = World(config())
        w.set_shielded("ue1", True)
        g = w.gains()
        self.assertEqual((g[("ue1", "A")], g[("ue1", "B")]), (0.0, 0.0))
        w.set_shielded("ue1", False)
        self.assertEqual(w.gains()[("ue1", "A")], 1.0)

    def test_unknown_ue_raises_key_error(self):
        w = World(config())
        with self.assertRaises(KeyError):
            w.place("ue9", 1, 1)
        with self.assertRaises(KeyError):
            w.travel("ue9", 1, 1, 10)
        with self.assertRaises(KeyError):
            w.set_shielded("ue9", True)

    def test_non_finite_coordinates_rejected(self):
        w = World(config())
        for bad in (float("nan"), float("inf")):
            with self.assertRaises(ValueError):
                w.place("ue1", bad, 0)
            with self.assertRaises(ValueError):
                w.travel("ue1", 0, bad, 10)


class TestTravel(unittest.TestCase):
    def ue(self, w, ue_id):
        return next(u for u in w.snapshot()["ues"] if u["id"] == ue_id)

    def test_travel_moves_at_speed(self):
        w = World(config())
        w.travel("ue2", 8, 0, 36)  # 36 km/h = 0.01 km/s
        w.tick(10)
        self.assertAlmostEqual(self.ue(w, "ue2")["x"], 2.1, places=9)
        self.assertIsNotNone(self.ue(w, "ue2")["travel"])

    def test_travel_stops_at_destination(self):
        w = World(config())
        w.travel("ue2", 2.05, 0, 36)
        w.tick(10)  # would go 0.1 km, only 0.05 left
        ue = self.ue(w, "ue2")
        self.assertEqual((ue["x"], ue["y"]), (2.05, 0.0))
        self.assertIsNone(ue["travel"])

    def test_travel_diagonal(self):
        w = World(config())
        w.place("ue1", 0, 0)
        w.travel("ue1", 3, 4, 36)  # 5 km away
        w.tick(100)  # 1 km along the line
        ue = self.ue(w, "ue1")
        self.assertAlmostEqual(ue["x"], 0.6, places=9)
        self.assertAlmostEqual(ue["y"], 0.8, places=9)
        self.assertAlmostEqual(ue["travel"]["eta_s"], 400, places=6)

    def test_new_travel_replaces_from_current_position(self):
        w = World(config())
        w.travel("ue2", 8, 0, 36)
        w.tick(100)  # at x = 3
        w.travel("ue2", 3, 1, 36)  # turn north: no jump back to the start
        w.tick(50)
        ue = self.ue(w, "ue2")
        self.assertAlmostEqual(ue["x"], 3.0, places=9)
        self.assertAlmostEqual(ue["y"], 0.5, places=9)
        self.assertEqual(ue["travel"]["x"], 3)

    def test_place_cancels_travel(self):
        w = World(config())
        w.travel("ue2", 8, 0, 36)
        w.place("ue2", 5, 5)
        w.tick(100)
        ue = self.ue(w, "ue2")
        self.assertEqual((ue["x"], ue["y"], ue["travel"]), (5.0, 5.0, None))

    def test_travel_rejects_zero_negative_nan_speed(self):
        w = World(config())
        for bad in (0, -5, float("nan"), float("inf")):
            with self.assertRaises(ValueError):
                w.travel("ue2", 8, 0, bad)
        self.assertIsNone(self.ue(w, "ue2")["travel"])

    def test_travel_to_current_position_is_noop_arrived(self):
        w = World(config())
        w.travel("ue2", 2, 0, 36)
        ue = self.ue(w, "ue2")
        self.assertEqual((ue["x"], ue["y"], ue["travel"]), (2.0, 0.0, None))

    def test_tick_rejects_negative_dt(self):
        with self.assertRaises(ValueError):
            World(config()).tick(-1)

    def test_snapshot_shape(self):
        snap = World(config()).snapshot()
        self.assertEqual([c["id"] for c in snap["cells"]], ["A", "B"])
        ue = snap["ues"][0]
        self.assertEqual(set(ue), {"id", "x", "y", "shielded", "travel", "gains"})
        self.assertEqual(set(ue["gains"]), {"A", "B"})


if __name__ == "__main__":
    unittest.main()
