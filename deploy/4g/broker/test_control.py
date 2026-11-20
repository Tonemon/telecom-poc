import random
import unittest

from control import Busy, Controller
from test_world import config
from world import World


def on_air_all(ctl):
    """Every cell and UE joined the air (a new controller starts with nobody on it)."""
    snap = ctl.snapshot()
    for cell in snap["cells"]:
        ctl.set_present(("cell", cell["id"]), True)
    for ue in snap["ues"]:
        ctl.set_present(("ue", ue["id"]), True)


class ControllerTest(unittest.TestCase):
    def setUp(self):
        self.applied = []
        self.c = Controller(World(config()), lambda ue, cell, g: self.applied.append((ue, cell, g)),
                            log=lambda *_: None, rng=random.Random(3))
        on_air_all(self.c)

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


    def ue(self, ue_id):
        return next(u for u in self.c.snapshot()["ues"] if u["id"] == ue_id)

    def test_random_activity_skips_shielded_and_manual(self):
        self.c.set_shielded("ue3", True)
        self.c.travel("ue2", 8, 0, 1000)
        plans = self.c.random_activity()
        self.assertEqual([p["ue"] for p in plans], ["ue1"])
        self.assertEqual(self.ue("ue2")["travel"]["x"], 8.0)  # manual trip untouched

    def test_random_activity_busy_while_realtime(self):
        self.c.set_realtime(True)
        with self.assertRaises(Busy):
            self.c.random_activity()

    def test_realtime_off_stops_automated_ues_where_they_are(self):
        self.c.set_realtime(True)
        self.c.tick(1.0, 10)
        moving = [u["id"] for u in self.c.snapshot()["ues"] if u["travel"]]
        self.assertTrue(moving)  # seed 3: at least one UE got a trip
        where = {u: (self.ue(u)["x"], self.ue(u)["y"]) for u in moving}
        stopped = self.c.set_realtime(False)
        self.assertEqual(sorted(stopped), sorted(moving))
        for u in moving:
            self.assertIsNone(self.ue(u)["travel"])
            self.assertEqual((self.ue(u)["x"], self.ue(u)["y"]), where[u])

    def test_realtime_off_keeps_manual_trip_going(self):
        self.c.set_realtime(True)
        self.c.travel("ue2", 8, 0, 1000)
        self.assertNotIn("ue2", self.c.set_realtime(False))
        self.assertEqual(self.ue("ue2")["travel"]["x"], 8.0)

    def test_tick_applies_new_plans_after_pause(self):
        self.c.set_shielded("ue2", True)
        self.c.set_shielded("ue3", True)
        self.c.set_realtime(True)
        for _ in range(50):          # 50 radio-s: ue1 decides several times (pauses are <= 10 s)
            self.c.tick(1.0, 10)
            if self.ue("ue1")["travel"]:
                break
        self.assertIsNotNone(self.ue("ue1")["travel"])

    def test_new_controller_has_nobody_on_air(self):
        c = Controller(World(config()), lambda *_: None, log=lambda *_: None)
        snap = c.snapshot()
        self.assertFalse(any(x["on_air"] for x in snap["cells"] + snap["ues"]))
        self.assertEqual({u["automation"] for u in snap["ues"]}, {"off"})
        self.assertEqual(c.random_activity(), [])

    def test_snapshot_has_on_air(self):
        self.c.set_present(("cell", "A"), False)
        snap = self.c.snapshot()
        self.assertFalse(next(x for x in snap["cells"] if x["id"] == "A")["on_air"])
        self.assertTrue(next(x for x in snap["cells"] if x["id"] == "B")["on_air"])
        self.assertTrue(self.ue("ue1")["on_air"])

    def test_going_off_air_stops_an_automated_trip(self):
        self.c.set_shielded("ue1", True)
        self.c.set_shielded("ue3", True)
        self.c.set_realtime(True)
        for _ in range(50):
            self.c.tick(1.0, 10)
            if self.ue("ue2")["travel"]:
                break
        self.assertIsNotNone(self.ue("ue2")["travel"])
        self.c.set_present(("ue", "ue2"), False)
        self.assertIsNone(self.ue("ue2")["travel"])
        self.assertEqual(self.ue("ue2")["automation"], "off")

    def test_random_activity_skips_off_ue(self):
        self.c.set_present(("ue", "ue2"), False)
        self.c.set_present(("ue", "ue3"), False)
        self.assertEqual([p["ue"] for p in self.c.random_activity()], ["ue1"])

    def test_manual_travel_of_an_off_ue_is_allowed(self):
        self.c.set_present(("ue", "ue2"), False)
        self.c.travel("ue2", 8, 0, 1000)
        self.assertEqual(self.ue("ue2")["travel"]["x"], 8.0)

    def test_snapshot_reports_automation_and_activity(self):
        self.c.set_shielded("ue3", True)
        self.c.travel("ue2", 8, 0, 1000)
        snap = self.c.snapshot()
        self.assertEqual(snap["activity"], {"realtime": False})
        self.assertEqual({u["id"]: u["automation"] for u in snap["ues"]},
                         {"ue1": "manual", "ue2": "travelling", "ue3": "shielded"})


if __name__ == "__main__":
    unittest.main()


class ControllerConcurrencyTest(unittest.TestCase):
    """The radio thread (tick) and API threads change the automation at the same time."""

    def test_realtime_off_during_a_tick_decision_stops_everyone(self):
        import threading
        import activity as act

        entered, release = threading.Event(), threading.Event()

        class BlockingWorld(World):
            def travel(self, ue, x, y, speed_kmh):
                if threading.current_thread().name == "tick":
                    entered.set()
                    release.wait(2)
                super().travel(ue, x, y, speed_kmh)

        orig = act.plan
        act.plan = lambda ue, cells, pathloss, rng: {"ue": ue, "action": "travel", "x": 5.0, "y": 3.0, "speed_kmh": 300.0}
        try:
            c = Controller(BlockingWorld(config()), lambda *_: None, log=lambda *_: None, rng=random.Random(1))
            on_air_all(c)
            c.set_shielded("ue2", True)
            c.set_shielded("ue3", True)
            c.set_realtime(True)
            c.stop("ue1")  # arrives at once: pauses, then due on the next tick
            tick = threading.Thread(target=c.tick, args=(20.0, 1.0), name="tick")
            tick.start()
            self.assertTrue(entered.wait(2))
            off = threading.Thread(target=c.set_realtime, args=(False,))
            off.start()
            off.join(0.3)
            release.set()
            tick.join(2)
            off.join(2)
        finally:
            act.plan = orig
        moving = [u["id"] for u in c.snapshot()["ues"] if u["travel"]]
        self.assertEqual(moving, [], "a UE kept travelling after realtime was switched off")
        self.assertFalse(c.snapshot()["activity"]["realtime"])
