import unittest

from monitor_runtime.restart import RestartPolicy


class RestartTests(unittest.TestCase):
    def test_crash_budget_counts_only_crashes_and_uses_capped_backoff(self):
        policy = RestartPolicy(max_crash_retries=2, base_delay=3, multiplier=1.2, max_delay=3.5)
        self.assertTrue(policy.record_scheduled_restart())
        self.assertEqual(policy.crash_count, 0)
        self.assertEqual(policy.record_crash(), (True, 3))
        self.assertEqual(policy.record_crash(), (True, 3.5))
        self.assertEqual(policy.record_crash(), (False, 0))

    def test_memory_restart_has_an_independent_budget_neutral_counter(self):
        policy = RestartPolicy(max_crash_retries=1, base_delay=0, multiplier=1, max_delay=0)
        self.assertTrue(policy.record_memory_restart())
        self.assertEqual(policy.memory_count, 1)
        self.assertEqual(policy.crash_count, 0)
        self.assertEqual(policy.scheduled_count, 0)
        self.assertEqual(policy.record_crash(), (True, 0))
