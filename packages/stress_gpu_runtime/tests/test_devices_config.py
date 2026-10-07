"""CPU-only discovery and interactive configuration tests."""
import unittest
from types import SimpleNamespace
from unittest.mock import Mock

from stress_gpu_runtime.devices import discover_devices, sample_device
from stress_gpu_runtime.config import prompt_configuration


class DiscoveryTests(unittest.TestCase):
    """Exercise UUID mapping without a CUDA runtime."""

    def test_visibility_and_reordered_uuid_mapping(self):
        """Map CUDA ordinals by UUID.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If telemetry uses an ordinal instead of UUID.
        """
        cuda = Mock()
        cuda.is_available.return_value = True
        cuda.device_count.return_value = 2
        cuda.get_device_properties.side_effect = [
            SimpleNamespace(uuid="GPU-b", name="Second", total_memory=2000),
            SimpleNamespace(uuid="GPU-a", name="First", total_memory=1000),
        ]
        cuda.mem_get_info.side_effect = [(1500, 2000), (500, 1000)]
        nvml = Mock()
        nvml.nvmlDeviceGetHandleByUUID.side_effect = lambda uuid: {"GPU-a": 9, "GPU-b": 3}[uuid]
        nvml.nvmlDeviceGetTemperatureThreshold.return_value = 90
        devices = discover_devices(SimpleNamespace(cuda=cuda), nvml)
        self.assertEqual([d["uuid"] for d in devices], ["GPU-b", "GPU-a"])
        self.assertEqual(devices[0]["ordinal"], 0)
        self.assertEqual(devices[0]["free_bytes"], 1500)
        nvml.nvmlDeviceGetHandleByUUID.assert_any_call("GPU-b")
        self.assertEqual(devices[0]["slowdown_celsius"], 90)

    def test_no_cuda_is_explicit_error(self):
        """Reject systems without CUDA.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If discovery silently returns no devices.
        """
        torch = SimpleNamespace(cuda=SimpleNamespace(is_available=lambda: False))
        with self.assertRaisesRegex(RuntimeError, "CUDA"):
            discover_devices(torch, Mock())

    def test_unsupported_metrics_are_nullable_but_gpu_loss_fails(self):
        """Distinguish unsupported metrics from a lost device.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If errors become misleading zero metrics.
        """
        class NVError(Exception):
            """Fake NVML error."""
        nvml = Mock()
        nvml.NVMLError = NVError
        nvml.NVMLError_NotSupported = NVError
        nvml.NVMLError_FunctionNotFound = NVError
        nvml.nvmlDeviceGetUtilizationRates.side_effect = NVError()
        nvml.nvmlDeviceGetTemperature.side_effect = NVError()
        nvml.nvmlDeviceGetMemoryInfo.return_value = SimpleNamespace(used=100, total=200)
        nvml.nvmlDeviceGetPowerUsage.return_value = 50000
        nvml.nvmlDeviceGetClockInfo.return_value = 1000
        nvml.nvmlDeviceGetCurrentClocksEventReasons.return_value = 0
        sample = sample_device(nvml, "GPU-a")
        self.assertIsNone(sample["utilization_percent"])
        self.assertIsNone(sample["temperature_celsius"])
        self.assertEqual(sample["power_watts"], 50)
        nvml.nvmlDeviceGetHandleByUUID.side_effect = RuntimeError("GPU lost")
        with self.assertRaisesRegex(RuntimeError, "lost"):
            sample_device(nvml, "GPU-a")


class ConfigurationTests(unittest.TestCase):
    """Validate interactive values and defaults."""

    def test_defaults_and_continuous_mode(self):
        """Choose every GPU and full continuous load by default.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If defaults differ from the specification.
        """
        devices = [{"ordinal": 0}, {"ordinal": 1}]
        result = prompt_configuration(devices, Mock(side_effect=["", "", "loop"]))
        self.assertEqual(result["devices"], devices)
        self.assertEqual(result["load_percent"], 100)
        self.assertIsNone(result["duration_seconds"])

    def test_invalid_inputs_are_retried(self):
        """Retry duplicates, bounds, non-finite values and invalid durations.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If invalid input is accepted.
        """
        result = prompt_configuration(
            [{"ordinal": 0}, {"ordinal": 1}],
            Mock(side_effect=["0,0", "2", "1", "nan", "inf", "0", "101", "25", "nan", "inf", "-1", "0", "1.5"]),
        )
        self.assertEqual(result["devices"], [{"ordinal": 1}])
        self.assertEqual(result["load_percent"], 25)
        self.assertEqual(result["duration_seconds"], 1.5)

class CanonicalUUIDTests(unittest.TestCase):
    """Pin the actual PyTorch UUID representation and NVML lookup contract."""

    def test_bare_object_uuid_and_mig_lookup(self):
        """Resolve bare CUDA UUIDs without substituting a MIG parent.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If canonical UUID identity is wrong.
        """
        class UUIDObject:
            """Represent PyTorch's _CUuuid object."""
            def __str__(self):
                """Return the bare UUID emitted by PyTorch.

                Args:
                    None.
                Returns:
                    str: Bare UUID.
                Raises:
                    None.
                """
                return "12345678-1234-1234-1234-123456789abc"
        class NotFound(Exception):
            """Represent a documented NVML lookup miss."""
        nvml = Mock()
        nvml.NVMLError_NotFound = NotFound
        cuda = Mock()
        cuda.is_available.return_value = True
        cuda.device_count.return_value = 1
        cuda.get_device_properties.return_value = SimpleNamespace(uuid=UUIDObject(), name="Board")
        cuda.mem_get_info.return_value = (1000, 2000)
        physical = "GPU-12345678-1234-1234-1234-123456789abc"
        nvml.nvmlDeviceGetHandleByUUID.side_effect = lambda uuid: 10 if uuid == physical else (_ for _ in ()).throw(NotFound())
        devices = discover_devices(SimpleNamespace(cuda=cuda), nvml)
        self.assertEqual(devices[0]["uuid"], physical)
        mig = "MIG-12345678-1234-1234-1234-123456789abc"
        nvml.nvmlDeviceGetHandleByUUID.side_effect = lambda uuid: 11 if uuid == mig else (_ for _ in ()).throw(NotFound())
        devices = discover_devices(SimpleNamespace(cuda=cuda), nvml)
        self.assertEqual(devices[0]["uuid"], mig)
        nvml.nvmlDeviceGetHandleByUUID.side_effect = RuntimeError("permission denied")
        with self.assertRaisesRegex(RuntimeError, "permission denied"):
            discover_devices(SimpleNamespace(cuda=cuda), nvml)
