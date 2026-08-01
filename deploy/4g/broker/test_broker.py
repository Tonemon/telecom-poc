import unittest
from broker import plan_broker


class TestPlanBroker(unittest.TestCase):
    def test_two_ues(self):
        plan = plan_broker(
            enb_dl="tcp://10.0.0.40:2000",
            enb_ul="tcp://*:2100",
            ues=[("tcp://*:2111", "tcp://10.0.0.61:2001"),
                 ("tcp://*:2112", "tcp://10.0.0.62:2001")],
        )
        # Downlink: read once from eNB, fan out to each UE.
        self.assertEqual(plan.dl_source, "tcp://10.0.0.40:2000")
        self.assertEqual(plan.dl_sinks, ["tcp://*:2111", "tcp://*:2112"])
        # Uplink: read each UE, sum, emit once to eNB.
        self.assertEqual(plan.ul_sources, ["tcp://10.0.0.61:2001", "tcp://10.0.0.62:2001"])
        self.assertEqual(plan.ul_sink, "tcp://*:2100")

    def test_single_ue(self):
        plan = plan_broker("tcp://a:2000", "tcp://*:2100",
                           [("tcp://*:2111", "tcp://b:2001")])
        self.assertEqual(plan.dl_sinks, ["tcp://*:2111"])
        self.assertEqual(plan.ul_sources, ["tcp://b:2001"])


if __name__ == "__main__":
    unittest.main()
