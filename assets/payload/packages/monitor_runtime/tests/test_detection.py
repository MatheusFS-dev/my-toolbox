import unittest

from monitor_runtime.detection import HeartbeatClock, possible_memory_leak


class DetectionTests(unittest.TestCase):
    def test_heartbeat_uses_positive_minutes_from_last_delivery(self):
        clock = HeartbeatClock(interval_minutes=1, started_at=10)
        self.assertFalse(clock.due(69.9))
        self.assertTrue(clock.due(70))
        clock.delivered(70)
        self.assertFalse(clock.due(129.9))

    def test_leak_requires_warmup_growth_and_slope_thresholds(self):
        settings = {"enabled": True, "warmup_seconds": 300, "window_seconds": 300, "minimum_growth_mib": 100, "minimum_slope_mib_per_minute": 5}
        samples = [{"elapsed_seconds": 300, "ram_mib": 100}, {"elapsed_seconds": 600, "ram_mib": 210}]
        self.assertTrue(possible_memory_leak(samples, settings))
        samples[-1]["ram_mib"] = 150
        self.assertFalse(possible_memory_leak(samples, settings))
