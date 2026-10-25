"""Fake CUDA and process tests for adaptive workloads and cleanup."""
import queue
import unittest
from types import SimpleNamespace
from unittest.mock import Mock, patch

from stress_gpu_runtime.workload import allocate_workload, duty_pause, exercise_workload
from stress_gpu_runtime.session import run_session, shutdown_workers

MIB = 1024 * 1024


class WorkloadTests(unittest.TestCase):
    """Exercise allocation policy with lightweight tensor fakes."""

    def fake_torch(self, failures=0, error=None):
        """Create an allocation-counting backend.

        Args:
            failures (int): Initial out-of-memory failures.
            error (Exception | None): Unexpected allocation failure.
        Returns:
            Mock: Backend with byte accounting.
        Raises:
            None.
        """
        torch = Mock()
        torch.cuda.OutOfMemoryError = MemoryError
        torch.cuda.mem_get_info.return_value = (1024 * MIB, 2048 * MIB)
        torch.bytes_allocated = 0
        torch.failure_count = failures
        def empty(shape, device=None, dtype=None):
            """Allocate a fake tensor.

            Args:
                shape (tuple): Tensor dimensions.
                device (object): Ignored device.
                dtype (object): Float or byte type.
            Returns:
                Mock: Tensor with size metadata.
            Raises:
                MemoryError: For requested simulated failures.
                RuntimeError: For unexpected CUDA errors.
            """
            if error:
                raise error
            if torch.failure_count:
                torch.failure_count -= 1
                raise MemoryError("OOM")
            count = shape[0] if len(shape) == 1 else shape[0] * shape[1] * 4
            torch.bytes_allocated += count
            tensor = Mock()
            tensor.numel.return_value = shape[0] if len(shape) == 1 else shape[0] * shape[1]
            tensor.element_size.return_value = 1 if len(shape) == 1 else 4
            return tensor
        torch.empty.side_effect = empty
        return torch

    def test_memory_targets_and_oom_backoff(self):
        """Scale VRAM and recover from an initial OOM.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If allocation exceeds budgets or recovery fails.
        """
        full = self.fake_torch()
        a, b, c, chunks = allocate_workload(full, 0, 100)
        self.assertGreater(full.bytes_allocated, 900 * MIB)
        self.assertLess(full.bytes_allocated, 1024 * MIB)
        partial = self.fake_torch()
        allocate_workload(partial, 0, 25)
        self.assertLessEqual(partial.bytes_allocated, 256 * MIB)
        recovered = self.fake_torch(failures=1)
        allocate_workload(recovered, 0, 100)
        self.assertGreater(recovered.bytes_allocated, 0)
        self.assertTrue(chunks)
        for tensor in (a, b, c, *chunks):
            self.assertTrue(tensor.fill_.called)

    def test_one_percent_load_remains_usable(self):
        """Keep runtime headroom separate from a small memory target.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If the advertised minimum load cannot run.
        """
        torch = self.fake_torch()
        allocate_workload(torch, 0, 1)
        self.assertLessEqual(torch.bytes_allocated, 1024 * MIB / 100)
        self.assertGreater(torch.bytes_allocated, 0)

    def test_exhausted_and_non_oom_errors(self):
        """Propagate device failures and bound OOM retries.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If failures are silently hidden.
        """
        with self.assertRaisesRegex(RuntimeError, "memory"):
            allocate_workload(self.fake_torch(failures=100), 0, 100)
        with self.assertRaisesRegex(RuntimeError, "device lost"):
            allocate_workload(self.fake_torch(error=RuntimeError("device lost")), 0, 100)

    def test_touch_and_duty_cycle(self):
        """Touch memory and pace work according to requested duty.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If compute or memory stress is skipped.
        """
        torch = self.fake_torch()
        workload = allocate_workload(torch, 0, 25)
        exercise_workload(torch, workload, 0)
        torch.matmul.assert_called()
        workload[3][0].bitwise_xor_.assert_called_with(1)
        self.assertAlmostEqual(duty_pause(0.1, 25), 0.3)
        self.assertEqual(duty_pause(0.1, 100), 0)


class SessionTests(unittest.TestCase):
    """Exercise shared readiness and bounded cleanup."""

    def config(self, **options):
        """Build a short test configuration.

        Args:
            **options (object): Configuration overrides.
        Returns:
            dict: Session values.
        Raises:
            None.
        """
        return {"devices": [{"ordinal": 0, "uuid": "GPU-a"}, {"ordinal": 1, "uuid": "GPU-b"}],
                "load_percent": 100, "duration_seconds": 0.02,
                "startup_timeout": 0.05, "sample_interval": 0.005, **options}

    def factory(self, ready=True, error=False, dead=False):
        """Construct workers emitting controlled startup messages.

        Args:
            ready (bool): Whether workers become ready.
            error (bool): Whether initialization fails.
            dead (bool): Whether workers unexpectedly disappear.
        Returns:
            tuple: Factory and created process list.
        Raises:
            None.
        """
        processes = []
        def create(**kwargs):
            """Build a fake process with a real message queue.

            Args:
                **kwargs (object): Process target and argument values.
            Returns:
                Mock: Fake worker.
            Raises:
                None.
            """
            device, load, start, stop, messages = kwargs["args"]
            process = Mock()
            process.is_alive.return_value = not dead
            process.exitcode = 9 if dead else None
            def launch():
                """Emit a startup event.

                Args:
                    None.
                Returns:
                    None.
                Raises:
                    None.
                """
                if ready:
                    messages.put({"kind": "ready", "uuid": device["uuid"], "allocated_bytes": 100})
                if error:
                    messages.put({"kind": "error", "uuid": device["uuid"], "error": "CUDA failed"})
            process.start.side_effect = launch
            processes.append(process)
            return process
        return create, processes

    def test_multiple_workers_ready_before_timed_run(self):
        """Start both selected workers and retain their samples.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If workers are not concurrent or duration is wrong.
        """
        factory, processes = self.factory()
        samples = []
        result = run_session(self.config(), lambda uuid: {"uuid": uuid}, samples.append, factory)
        self.assertEqual(result["stop_reason"], "duration completed")
        self.assertEqual(len(processes), 2)
        self.assertGreaterEqual(result["elapsed_seconds"], 0.02)
        self.assertEqual({s["uuid"] for s in samples}, {"GPU-a", "GPU-b"})
        self.assertEqual(result["allocations"], {"GPU-a": 100, "GPU-b": 100})

    def test_failure_timeout_and_disappearance_stop_every_worker(self):
        """Bound initialization and stop all peers after failure.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If an incomplete run looks successful.
        """
        for options in ({"ready": False}, {"error": True}, {"dead": True}):
            factory, processes = self.factory(**options)
            result = run_session(self.config(), lambda uuid: {"uuid": uuid}, Mock(), factory)
            self.assertTrue(result["errors"])
            self.assertNotEqual(result["stop_reason"], "duration completed")
            self.assertTrue(all(p.join.called for p in processes))

    def test_monitoring_failure_and_manual_stop(self):
        """Preserve error and manual stop semantics.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If monitoring errors become normal completion.
        """
        factory, _ = self.factory()
        result = run_session(self.config(), Mock(side_effect=RuntimeError("NVML lost")), Mock(), factory)
        self.assertIn("NVML lost", str(result["errors"]))
        factory, _ = self.factory()
        result = run_session(self.config(duration_seconds=None), Mock(side_effect=KeyboardInterrupt), Mock(), factory)
        self.assertEqual(result["stop_reason"], "stopped by user")
        self.assertFalse(result["errors"])

    def test_unresponsive_workers_are_terminated(self):
        """Escalate cleanup for hanging workers.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If a hung process is left running.
        """
        process = Mock()
        process.is_alive.return_value = True
        shutdown_workers([process], Mock(), grace_seconds=0)
        process.terminate.assert_called_once()
        process.kill.assert_called_once()

class InterruptionTests(unittest.TestCase):
    """Exercise cancellation during child registration and shutdown."""

    config = SessionTests.config
    factory = SessionTests.factory

    def test_interrupt_after_start_still_tracks_and_stops_child(self):
        """Defer a real SIGINT until process registration is complete.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If the started child escapes cleanup.
        """
        import signal
        factory, processes = self.factory()
        def interrupted_factory(**kwargs):
            """Wrap a process start with an interrupt after launch.

            Args:
                **kwargs (object): Process construction values.
            Returns:
                Mock: Instrumented process.
            Raises:
                None.
            """
            process = factory(**kwargs)
            launch = process.start.side_effect
            def start():
                """Launch the child and deliver SIGINT.

                Args:
                    None.
                Returns:
                    None.
                Raises:
                    KeyboardInterrupt: Unless supervision defers the signal.
                """
                launch()
                signal.raise_signal(signal.SIGINT)
            process.start.side_effect = start
            return process
        result = run_session(self.config(), Mock(), Mock(), interrupted_factory)
        self.assertEqual(result["stop_reason"], "stopped by user")
        self.assertTrue(processes[0].join.called)
        processes[0].terminate.assert_called_once()

    def test_interrupted_join_still_escalates_cleanup(self):
        """Retain the final result when cooperative joins are interrupted.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If interruption leaves the child alive.
        """
        process = Mock()
        process.is_alive.return_value = True
        process.join.side_effect = [KeyboardInterrupt(), None, None]
        shutdown_workers([process], Mock(), grace_seconds=0)
        process.terminate.assert_called_once()
        process.kill.assert_called_once()
