import json
import os
import unittest
from tempfile import TemporaryDirectory
from pathlib import Path

from monitor_runtime.config import ConfigError, default_config, load_config, save_json_atomic, validate_config, validate_credentials


class ConfigTests(unittest.TestCase):
    def test_default_config_has_approved_restart_and_sampling_values(self):
        config = default_config()
        self.assertEqual(config["restart"]["crash_retries"], 10)
        self.assertEqual(config["restart"]["base_delay_seconds"], 3)
        self.assertEqual(config["restart"]["backoff_multiplier"], 1.2)
        self.assertEqual(config["restart"]["max_delay_seconds"], 30)
        self.assertEqual(config["restart"]["rapid_crash_seconds"], 60)
        self.assertTrue(config["notifications"]["possible_code_error"])
        self.assertEqual(config["sampling_interval_seconds"], 1)
        self.assertEqual(config["leak_detection"]["warmup_seconds"], 300)

    def test_rapid_crash_threshold_must_be_positive_seconds(self):
        config = default_config()
        config["restart"]["rapid_crash_seconds"] = 0
        with self.assertRaisesRegex(ConfigError, "rapid-crash threshold"):
            validate_config(config)

    def test_load_config_rejects_future_schema(self):
        with TemporaryDirectory() as directory:
            path = Path(directory) / "config.json"
            path.write_text(json.dumps({"schema_version": 99}), encoding="utf-8")
            with self.assertRaisesRegex(ConfigError, "newer schema"):
                load_config(path)

    def test_memory_restart_requires_a_positive_limit(self):
        config = default_config()
        config["restart"]["memory_aware"] = True
        config["restart"]["memory_limit_gb"] = 0
        with self.assertRaisesRegex(ConfigError, "memory restart limit"):
            validate_config(config)

    def test_memory_restart_accepts_a_fractional_decimal_gb_limit(self):
        config = default_config()
        config["restart"]["memory_aware"] = True
        config["restart"]["memory_limit_gb"] = 1.5
        validated = validate_config(config)
        self.assertEqual(validated["restart"]["memory_limit_gb"], 1.5)

    def test_memory_and_time_restarts_are_mutually_exclusive(self):
        config = default_config()
        config["restart"]["memory_aware"] = True
        config["restart"]["memory_limit_gb"] = 0.5
        config["restart"]["scheduled_interval_minutes"] = 10
        with self.assertRaisesRegex(ConfigError, "cannot both"):
            validate_config(config)

    def test_leak_warmup_must_be_positive(self):
        config = default_config()
        config["leak_detection"]["warmup_seconds"] = 0
        with self.assertRaisesRegex(ConfigError, "warm-up"):
            validate_config(config)

    def test_schema_one_config_merges_memory_limit(self):
        config = validate_config({"schema_version": 1, "recipients": []})
        self.assertGreater(config["restart"]["memory_limit_gb"], 0)

    def test_schema_one_converts_legacy_mib_limit_to_decimal_gb(self):
        config = validate_config({
            "schema_version": 1,
            "recipients": [],
            "restart": {"memory_limit_mib": 1024},
        })
        self.assertEqual(config["restart"]["memory_limit_gb"], 1.073741824)
        self.assertNotIn("memory_limit_mib", config["restart"])

    def test_atomic_save_replaces_file_with_owner_only_mode(self):
        with TemporaryDirectory() as directory:
            path = Path(directory) / "config.json"
            save_json_atomic(path, {"schema_version": 1, "recipients": ["a@example.com"]})
            self.assertEqual(json.loads(path.read_text(encoding="utf-8"))["recipients"], ["a@example.com"])
            self.assertEqual(os.stat(path).st_mode & 0o777, 0o600)
            self.assertEqual(list(Path(directory).glob(".*.tmp")), [])

    def test_credentials_require_supported_encrypted_transport(self):
        with self.assertRaisesRegex(ConfigError, "security"):
            validate_credentials({"host": "smtp.example.com", "port": 25, "security": "plain", "sender": "a@example.com", "password": "secret"})
