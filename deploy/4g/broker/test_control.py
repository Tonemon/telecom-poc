import unittest

from control import Controller
from test_world import config
from world import World


class ControllerTest(unittest.TestCase):
    def setUp(self):
        self.applied = []
        self.c = Controller(World(config()), lambda ue, cell, g: self.applied.append((ue, cell, g)),
                            log=lambda *_: None)

    def gains_of(self, ue):
        return next(u for u in self.c.snapshot()["ues"] if u["id"] == ue)["gains"]

    def test_initial_gains_follow_world(self):
        g = self.c.initial_gains()
        self.assertEqual(g[("ue1", "A")], 1.0)
        self.assertEqual(g[("ue3", "A")], 0.0)

    def test_tick_pushes_only_changed_gains(self):
        self.c.tick(0, 0)
        self.assertEqual(self.applied, [])  # nothing moved yet
        self.c.travel("ue2", 8, 0, 360)  # 0.1 km/s
        self.applied.clear()
        self.c.tick(10, 1)  # ue2 now at x = 3
        self.assertTrue(self.applied)
        self.assertEqual({ue for ue, _, _ in self.applied}, {"ue2"})

    def test_override_holds_until_next_move(self):
        self.c.override_gain("ue1", "B", 0.5)
        self.assertIn(("ue1", "B", 0.5), self.applied)
        self.c.tick(1, 1)
        self.assertEqual(self.gains_of("ue1")["B"], 0.5)  # tick does not undo the override
        self.c.place("ue1", 1, 0)
        self.assertEqual(self.gains_of("ue1")["B"], 0.0)  # back to the world's value
        self.assertIn(("ue1", "B", 0.0), self.applied)

    def test_shield_pushes_zero_and_back(self):
        self.c.set_shielded("ue3", True)
        self.assertIn(("ue3", "B", 0.0), self.applied)
        self.assertEqual(self.gains_of("ue3"), {"A": 0.0, "B": 0.0})
        self.c.set_shielded("ue3", False)
        self.assertGreater(self.gains_of("ue3")["B"], 0)

    def test_snapshot_flags_override(self):
        self.assertFalse(next(u for u in self.c.snapshot()["ues"] if u["id"] == "ue1")["override"])
        self.c.override_gain("ue1", "A", 0.2)
        self.assertTrue(next(u for u in self.c.snapshot()["ues"] if u["id"] == "ue1")["override"])

    def test_radio_clock(self):
        self.c.tick(0.5, 5)
        self.c.tick(0.5, 5)
        radio = self.c.snapshot()["radio"]
        self.assertAlmostEqual(radio["time_s"], 1.0)
        self.assertAlmostEqual(radio["speed"], 0.1)

    def test_unknown_ue_and_bad_values_raise(self):
        with self.assertRaises(KeyError):
            self.c.travel("ue9", 1, 1, 10)
        with self.assertRaises(ValueError):
            self.c.travel("ue2", 8, 0, 0)
        with self.assertRaises(KeyError):
            self.c.override_gain("ue1", "Z", 1.0)

    def test_place_cell_pushes_changed_gains_only(self):
        self.applied.clear()
        self.c.place_cell("B", 12, 0)  # ue3 now 3 km away; ue1/ue2 stay out of range
        self.assertTrue(self.applied)
        self.assertEqual({cell for _, cell, _ in self.applied}, {"B"})
        self.assertEqual({ue for ue, _, _ in self.applied}, {"ue3"})

    def test_place_cell_clears_overrides(self):
        self.c.override_gain("ue3", "B", 0.5)
        self.c.place_cell("B", 9, 0)
        self.assertEqual(self.gains_of("ue3")["B"], 1.0)  # world value, override gone

    def test_dry_run_changes_nothing(self):
        self.applied.clear()
        res = self.c.dry_run_cell("B", 30, 0)
        self.assertEqual(self.applied, [])
        self.assertEqual({r["id"]: r["effect"] for r in res}["ue3"], "loses")

    def test_stop_pushes_nothing_when_position_unchanged(self):
        self.c.travel("ue2", 8, 0, 1000)
        self.c.tick(3.6, 36)
        self.applied.clear()
        self.c.stop("ue2")
        self.assertEqual(self.applied, [])
        self.assertIsNone(next(u for u in self.c.snapshot()["ues"] if u["id"] == "ue2")["travel"])


if __name__ == "__main__":
    unittest.main()
