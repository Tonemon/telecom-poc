import unittest

import numpy as np

from engine import Engine

K, AHEAD = 4, 2
A, B = ("cell", "A"), ("cell", "B")
UE1, UE2 = ("ue", "ue1"), ("ue", "ue2")


def ones(n, v=1.0):
    return np.full(n, v, np.complex64)


def ramp(n):
    return np.arange(n).astype(np.complex64)


class EngineTest(unittest.TestCase):
    def setUp(self):
        self.events = []
        gains = {("ue1", "A"): 0.5, ("ue1", "B"): 0.25, ("ue2", "A"): 1.0, ("ue2", "B"): 0.0}
        self.now = 0.0
        self.e = Engine(["A", "B"], ["ue1", "ue2"], gains, k=K, ahead=AHEAD,
                        on_presence=lambda ep, on: self.events.append((ep, on)),
                        gap_s=0.2, clock=lambda: self.now)

    def run_step(self, request=()):
        """One step; every endpoint in request asks for (and takes) its chunk."""
        self.assertTrue(self.e.ready())
        self.e.step()
        out = {}
        for ep in request:
            self.e.rx_request(ep)
            out[ep] = self.e.replies(ep)
        return out

    def test_join_sets_a0_to_air_time(self):
        self.run_step()
        self.run_step()
        self.e.join(UE1)
        self.e.tx_data(A, ones(100))  # A transmits but never joined: silence
        got = self.run_step([UE1])[UE1]
        self.assertEqual(len(got), 1)
        self.assertEqual(self.e.t, 3 * K)
        self.assertTrue(self.e.present(UE1))

    def test_tx_sample_j_lands_at_a0_plus_j(self):
        self.e.join(A)
        self.e.tx_data(A, ones(100))
        self.run_step([A])
        self.run_step([A])  # t = 2K
        self.e.join(UE1)
        self.e.tx_data(UE1, ramp(12))
        first = self.run_step([A])[A][0]
        second = self.run_step([A])[A][0]
        np.testing.assert_allclose(first, 0.5 * ramp(4))
        np.testing.assert_allclose(second, 0.5 * (ramp(4) + 4))

    def test_downlink_sums_cells_with_gains(self):
        self.e.join(A)
        self.e.join(B)
        self.e.tx_data(A, ones(8, 1.0))
        self.e.tx_data(B, ones(8, 2.0))
        self.e.join(UE1)
        got = self.run_step([UE1])[UE1][0]
        np.testing.assert_allclose(got, ones(K, 1.0))  # 0.5*1 + 0.25*2

    def test_absent_endpoint_is_silence(self):
        self.e.join(A)
        self.e.tx_data(UE2, ones(8, 3.0))  # ue2 never joined
        got = self.run_step([A])[A][0]
        np.testing.assert_allclose(got, np.zeros(K))

    def test_waits_for_endpoint_that_transmitted(self):
        self.e.join(UE1)
        self.e.tx_data(UE1, ones(K))
        self.run_step([UE1])
        self.assertFalse(self.e.ready())  # ue1 does not cover [K, 2K) yet
        self.e.tx_data(UE1, ones(K))
        self.assertTrue(self.e.ready())

    def test_partial_step_when_a_transmitter_is_short(self):
        # srsRAN may run only a few samples ahead (GNU Radio advanced by whatever was there):
        # the air steps as far as every waited-for endpoint reaches, never deadlocking on K.
        self.e.join(A)
        self.e.join(UE1)
        self.e.tx_data(UE1, ramp(K + 2))
        self.e.tx_data(A, ones(100))
        self.run_step([A])
        self.assertEqual(self.e.ready(), 2)
        got = self.run_step([A])[A][0]
        np.testing.assert_allclose(got, 0.5 * (ramp(2) + K))
        self.assertEqual(self.e.t, K + 2)
        self.assertEqual(self.e.ready(), 0)

    def test_burst_gap_stops_the_wait(self):
        # srsue ends a tx burst and sends nothing until its next transmission; the air waits
        # for a transmitter only while its stream keeps coming.
        self.e.join(A)
        self.e.join(UE1)
        self.e.tx_data(UE1, ones(K))
        self.run_step([A, UE1])
        self.now = 0.1
        self.assertEqual(self.e.ready(), 0)  # a short silence: still waiting
        self.now = 0.35
        self.assertEqual(self.e.ready(), K)  # burst over: the air goes on without ue1
        got = self.run_step([A, UE1])[A][0]
        np.testing.assert_allclose(got, np.zeros(K))
        # Next burst: srsRAN first fills the gap with zeros (too late, dropped), then sends.
        self.e.tx_data(UE1, np.concatenate([np.zeros(K, np.complex64), ones(2 * K, 2.0)]))
        self.assertEqual(self.e.ready(), K)  # waits for it again
        got = self.run_step([A])[A][0]
        np.testing.assert_allclose(got, ones(K, 1.0))  # 0.5 * 2.0, placed at a0 + j

    def test_burst_gap_does_not_drop_rx_of_a_synced_endpoint(self):
        self.e.join(UE1)
        self.e.tx_data(UE1, ones(K))
        self.run_step()
        self.now = 1.0
        for _ in range(AHEAD + 2):
            if self.e.ready():
                self.e.step()
        self.e.rx_request(UE1)
        self.e.rx_request(UE1)
        self.e.rx_request(UE1)
        got = self.e.replies(UE1)
        self.assertEqual(len(got), AHEAD)  # it stalled with a full queue: nothing was dropped
        self.assertEqual(self.e.t, AHEAD * K)

    def test_does_not_wait_for_endpoint_that_never_transmitted(self):
        self.e.join(UE1)
        for _ in range(5):
            self.run_step([UE1])
        self.assertEqual(self.e.t, 5 * K)

    def test_booting_endpoint_full_queue_drops_and_shifts_a0(self):
        self.e.join(A)
        self.e.join(UE1)  # a0 = 0, never requests, never transmits
        for _ in range(AHEAD + 2):
            self.run_step([A])
        self.e.rx_request(UE1)
        self.e.rx_request(UE1)
        self.e.rx_request(UE1)
        self.assertEqual(len(self.e.replies(UE1)), AHEAD)  # two chunks were dropped
        # Its sample 0 now belongs to air time 2K (two drops), so the next chunk A hears
        # (t = 4K) holds ue1's samples 2K..3K.
        self.e.tx_data(UE1, ramp(4 * K))
        got = self.run_step([A])[A][0]
        np.testing.assert_allclose(got, 0.5 * (ramp(K) + 2 * K))

    def test_transmitting_endpoint_full_queue_blocks(self):
        self.e.join(UE1)
        self.e.tx_data(UE1, ones(100))
        for _ in range(AHEAD):
            self.run_step()
        self.assertFalse(self.e.ready())
        self.e.rx_request(UE1)
        self.e.replies(UE1)
        self.assertTrue(self.e.ready())

    def test_tx_before_join_is_kept(self):
        self.e.tx_data(A, ramp(8))
        self.e.join(A)
        self.e.join(UE2)
        got = [self.run_step([UE2])[UE2][0] for _ in range(2)]
        np.testing.assert_allclose(got[0], ramp(4))
        np.testing.assert_allclose(got[1], ramp(4) + 4)

    def test_leave_resets_and_reports(self):
        self.e.join(UE1)
        self.e.tx_data(UE1, ones(100))
        self.e.rx_request(UE1)
        self.run_step()
        self.e.leave(UE1)
        self.assertFalse(self.e.present(UE1))
        self.assertEqual(self.events[-1], (UE1, False))
        self.assertEqual(self.e.replies(UE1), [])
        self.assertTrue(self.e.ready())  # its old tx no longer holds the air
        self.e.leave(UE1)  # no-op
        self.assertEqual(self.events.count((UE1, False)), 1)
        # A new process starts from its own sample 0 at the current air time.
        self.e.join(A)
        self.e.join(UE1)
        self.e.tx_data(UE1, ramp(8))
        got = self.run_step([A])[A][0]
        np.testing.assert_allclose(got, 0.5 * ramp(4))

    def test_reset_tx_forgets_the_old_stream_but_keeps_the_session(self):
        # The tx connection of a process dropped: what it sent is gone, the next process's tx
        # starts at its own sample 0, i.e. the session's a0.
        self.e.join(A)
        self.e.join(UE1)
        self.e.tx_data(UE1, ones(100, 9.0))
        self.run_step([A, UE1])
        self.e.reset_tx(UE1)
        self.assertTrue(self.e.present(UE1))
        self.assertEqual(self.e.ready(), K)  # no longer waits for the old stream
        self.e.tx_data(UE1, ramp(3 * K))      # the new process, from its sample 0 = a0 = 0
        got = self.run_step([A])[A][0]
        np.testing.assert_allclose(got, 0.5 * (ramp(K) + K))

    def test_leave_keep_tx_keeps_what_the_new_process_sent(self):
        self.e.join(A)
        self.e.tx_data(A, ramp(8))
        self.e.leave(A, keep_tx=True)
        self.assertFalse(self.e.present(A))
        self.e.join(A)
        self.e.join(UE2)
        got = self.run_step([UE2])[UE2][0]
        np.testing.assert_allclose(got, ramp(4))

    def test_leave_touches_only_that_endpoint(self):
        for ep in (UE1, UE2):
            self.e.join(ep)
            self.e.tx_data(ep, ones(100))
        self.run_step()
        self.e.rx_request(UE2)
        self.e.leave(UE1)
        self.assertTrue(self.e.present(UE2))
        self.assertEqual(len(self.e.replies(UE2)), 1)
        self.e.join(A)
        got = self.run_step([A])[A][0]
        np.testing.assert_allclose(got, ones(K, 1.0))  # ue2's buffered tx is intact (gain 1)

    def test_replies_one_per_pending_request(self):
        self.e.join(UE1)
        self.e.rx_request(UE1)
        self.e.rx_request(UE1)
        self.run_step()
        self.run_step()
        self.assertEqual(len(self.e.replies(UE1)), 2)
        self.assertEqual(self.e.replies(UE1), [])
        self.assertTrue(self.e.ready())

    def test_samples_before_t_are_discarded(self):
        self.e.join(A)
        self.e.join(UE1)
        for _ in range(3):
            self.run_step([A, UE1])
        self.e.tx_data(UE1, ramp(16))  # indices 0..15 = air time 0..15; t is 12
        got = self.run_step([A])[A][0]
        np.testing.assert_allclose(got, 0.5 * (ramp(4) + 12))

    def test_join_reports_presence(self):
        self.e.join(B)
        self.assertEqual(self.events, [(B, True)])

    def test_set_gain_applies_next_step(self):
        self.e.join(A)
        self.e.join(UE1)
        self.e.tx_data(A, ones(100))
        self.e.set_gain("ue1", "A", 0.1)
        got = self.run_step([UE1])[UE1][0]
        np.testing.assert_allclose(got, ones(K, 0.1), rtol=1e-6)


if __name__ == "__main__":
    unittest.main()
