import json
import unittest

from air import Link, load_config, plan_air, static_gains


def config(**overrides):
    cfg = {
        "srate": 23040000,
        "api_port": 8090,
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
        "gain": {"ue1": {"A": 1.0}, "ue2": {"A": 1.0}, "ue3": {"B": 1.0}},
    }
    cfg.update(overrides)
    return json.dumps(cfg)


class TestLoadConfig(unittest.TestCase):
    def test_load_two_cells_three_ues(self):
        cfg = load_config(config())
        self.assertEqual([c.id for c in cfg.cells], ["A", "B"])
        self.assertEqual([u.id for u in cfg.ues], ["ue1", "ue2", "ue3"])
        self.assertEqual((cfg.cells[1].x, cfg.cells[1].y), (10.0, 0.0))
        self.assertEqual((cfg.ues[2].x, cfg.ues[2].y), (9.0, 0.0))
        self.assertEqual(cfg.cells[0].dl, "tcp://10.0.0.40:2000")
        self.assertEqual(cfg.ues[0].ul, "tcp://10.0.0.61:2001")
        self.assertEqual(cfg.srate, 23040000.0)
        self.assertEqual(cfg.api_port, 8090)

    def test_duplicate_ue_id_rejected(self):
        ues = json.loads(config())["ues"]
        ues[1]["id"] = "ue1"
        with self.assertRaises(ValueError):
            load_config(config(ues=ues))

    def test_duplicate_address_rejected(self):
        ues = json.loads(config())["ues"]
        ues[2]["dl"] = "tcp://*:2100"  # same bind port as cell A's uplink
        with self.assertRaises(ValueError):
            load_config(config(ues=ues))

    def test_empty_cells_or_ues_rejected(self):
        with self.assertRaises(ValueError):
            load_config(config(cells=[]))
        with self.assertRaises(ValueError):
            load_config(config(ues=[]))

    def test_gain_for_unknown_ue_or_cell_rejected(self):
        with self.assertRaises(ValueError):
            load_config(config(gain={"ue9": {"A": 1.0}}))
        with self.assertRaises(ValueError):
            load_config(config(gain={"ue1": {"Z": 1.0}}))


class TestPlanAir(unittest.TestCase):
    def test_plan_dl_has_one_link_per_cell(self):
        plan = plan_air(load_config(config()))
        for ue in ("ue1", "ue2", "ue3"):
            self.assertEqual(plan.dl[ue], [Link(ue, "A"), Link(ue, "B")])

    def test_plan_ul_has_one_link_per_ue(self):
        plan = plan_air(load_config(config()))
        for cell in ("A", "B"):
            self.assertEqual(plan.ul[cell],
                             [Link("ue1", cell), Link("ue2", cell), Link("ue3", cell)])


class TestStaticGains(unittest.TestCase):
    def test_every_pair_has_a_gain_missing_ones_are_zero(self):
        gains = static_gains(load_config(config()))
        self.assertEqual(gains[("ue1", "A")], 1.0)
        self.assertEqual(gains[("ue1", "B")], 0.0)
        self.assertEqual(gains[("ue3", "A")], 0.0)
        self.assertEqual(gains[("ue3", "B")], 1.0)
        self.assertEqual(len(gains), 6)


if __name__ == "__main__":
    unittest.main()
