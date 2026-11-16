import math
import random
import unittest

from activity import (BOUNDS, IN_COVERAGE_P, PAUSE_RANGE_S, SPEED_RANGE, STAY_P, Automation,
                      plan)
from world import gain_for_distance

PATHLOSS = {"ref_km": 1.0, "exponent": 3.0, "edge_km": 7.0}
CELLS = [{"id": "A", "x": 0.0, "y": 0.0}, {"id": "B", "x": 10.0, "y": 0.0}]


def covered(x, y):
    return any(gain_for_distance(math.hypot(x - c["x"], y - c["y"]), PATHLOSS) > 0 for c in CELLS)


class PlanTest(unittest.TestCase):
    def plans(self, n=3000):
        rng = random.Random(1)
        return [plan("ue1", CELLS, PATHLOSS, rng) for _ in range(n)]

    def test_constants(self):
        self.assertAlmostEqual(STAY_P, 1 / 3)
        self.assertEqual(IN_COVERAGE_P, 0.9)
        self.assertEqual(SPEED_RANGE, (300.0, 1500.0))
        self.assertEqual(PAUSE_RANGE_S, (2.0, 10.0))
        self.assertEqual(BOUNDS, (-10.0, 20.0, -10.0, 10.0))

    def test_plan_stays_about_a_third(self):
        stays = sum(p["action"] == "stay" for p in self.plans())
        self.assertTrue(900 < stays < 1100, stays)

    def test_plan_destinations_in_bounds_and_mostly_covered(self):
        trips = [p for p in self.plans() if p["action"] == "travel"]
        xmin, xmax, ymin, ymax = BOUNDS
        for p in trips:
            self.assertTrue(xmin <= p["x"] <= xmax and ymin <= p["y"] <= ymax, p)
        share = sum(covered(p["x"], p["y"]) for p in trips) / len(trips)
        self.assertTrue(0.85 <= share <= 0.95, share)

    def test_plan_speed_in_range_rounded(self):
        for p in self.plans():
            if p["action"] == "travel":
                self.assertTrue(300 <= p["speed_kmh"] <= 1500, p)
                self.assertEqual(p["speed_kmh"] % 10, 0, p)

    def test_plan_names_the_ue(self):
        self.assertEqual(plan("ue7", CELLS, PATHLOSS, random.Random(2))["ue"], "ue7")


class AutomationTest(unittest.TestCase):
    def setUp(self):
        self.a = Automation(random.Random(1), ["ue1", "ue2", "ue3"])

    def test_states_start_manual_with_realtime_off(self):
        self.assertFalse(self.a.realtime)
        self.assertEqual([self.a.state(u) for u in ("ue1", "ue2", "ue3")], ["manual"] * 3)

    def test_realtime_on_makes_all_eligible_due(self):
        self.a.on_shielded("ue3", True)
        self.a.on_manual("ue2")
        self.assertEqual(self.a.set_realtime(True), ["ue1"])
        self.assertEqual(self.a.state("ue1"), "auto")
        self.assertEqual(self.a.state("ue2"), "travelling")
        self.assertEqual(self.a.state("ue3"), "shielded")

    def test_pause_then_due_again(self):
        self.a.set_realtime(True)
        self.a.planned("ue1", {"action": "travel"})
        self.a.on_arrived("ue1")
        self.assertEqual(self.a.due(1.0), [])
        self.assertEqual(self.a.due(10.0), ["ue1"])
        self.assertEqual(self.a.due(10.0), [])  # due once, until planned again

    def test_stay_pauses_then_decides_again(self):
        self.a.set_realtime(True)
        self.a.planned("ue1", {"action": "stay"})
        self.assertEqual(self.a.due(1.0), [])
        self.assertIn("ue1", self.a.due(10.0))

    def test_manual_rejoins_after_arrival(self):
        self.a.set_realtime(True)
        self.a.on_manual("ue2")
        self.assertEqual(self.a.state("ue2"), "travelling")
        self.assertNotIn("ue2", self.a.due(100.0))
        self.a.on_arrived("ue2")
        self.assertEqual(self.a.state("ue2"), "auto")
        self.assertEqual(self.a.due(1.0), [])
        self.assertIn("ue2", self.a.due(10.0))

    def test_realtime_off_returns_only_automated_travellers(self):
        self.a.set_realtime(True)
        self.a.planned("ue1", {"action": "travel"})
        self.a.on_manual("ue2")
        self.assertEqual(self.a.set_realtime(False), ["ue1"])
        self.assertEqual(self.a.state("ue1"), "manual")
        self.assertEqual(self.a.state("ue2"), "travelling")
        self.assertEqual(self.a.due(100.0), [])

    def test_shielded_ue_never_due_until_unshielded(self):
        self.a.set_realtime(True)
        self.a.on_shielded("ue3", True)
        self.a.planned("ue1", {"action": "travel"})
        self.a.planned("ue2", {"action": "travel"})
        self.assertNotIn("ue3", self.a.due(100.0))
        self.a.on_shielded("ue3", False)
        self.assertEqual(self.a.state("ue3"), "auto")
        self.assertIn("ue3", self.a.due(10.0))

    def test_eligible_for_random(self):
        self.a.on_shielded("ue3", True)
        self.a.on_manual("ue2")
        self.assertEqual(self.a.eligible(), ["ue1"])

    def test_unknown_ue_raises(self):
        with self.assertRaises(KeyError):
            self.a.on_manual("ue9")


if __name__ == "__main__":
    unittest.main()
