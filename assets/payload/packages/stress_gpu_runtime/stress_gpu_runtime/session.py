"""Concurrent worker supervision with a shared start and bounded shutdown."""
import multiprocessing
import queue
import time

from .workload import run_worker
from .interrupts import defer_interrupts


def shutdown_workers(workers, stop_event, grace_seconds=5.0):
    """Stop all workers and escalate when cooperative shutdown fails.

    Args:
        workers (list): Started multiprocessing processes.
        stop_event (multiprocessing.Event): Shared stop event.
        grace_seconds (float): Total cooperative shutdown deadline.

    Returns:
        None.

    Raises:
        RuntimeError: If operating-system process cleanup fails.
    """
    stop_event.set()
    deadline = time.monotonic() + grace_seconds
    for worker in workers:
        try:
            worker.join(max(0.0, deadline - time.monotonic()))
        except KeyboardInterrupt:
            pass
    for worker in workers:
        if worker.is_alive():
            worker.terminate()
    deadline = time.monotonic() + 2.0
    for worker in workers:
        try:
            worker.join(max(0.0, deadline - time.monotonic()))
        except KeyboardInterrupt:
            pass
    for worker in workers:
        if worker.is_alive():
            worker.kill()
            try:
                worker.join(1.0)
            except KeyboardInterrupt:
                pass


def run_session(config, sampler, on_sample, process_factory=None):
    """Run selected GPUs concurrently and collect telemetry until stopped.

    Args:
        config (dict): Devices, percentage, duration and optional test timeouts.
        sampler (Callable[[str], dict]): UUID-based telemetry callback.
        on_sample (Callable[[dict], None]): Streaming observation callback.
        process_factory (Callable | None): Injectable process constructor.

    Returns:
        dict: Stop reason, elapsed seconds, errors and worker allocations.

    Raises:
        None. Initialization, telemetry, and worker errors appear in the result.

    Examples:
        result = run_session(config, sampler, on_sample)

    Notes:
        All selected workers must finish allocating before the timed interval
        begins. A lost worker or monitor stops peers rather than silently
        continuing a partial test. Optional metric values may remain None.
    """
    context = multiprocessing.get_context("spawn")
    start_event, stop_event = context.Event(), context.Event()
    messages = context.Queue(maxsize=256)
    factory = process_factory or context.Process
    workers = []
    allocations, last_progress = {}, {}
    errors = []
    started = None
    reason = "initialization failed"
    interval = config.get("sample_interval", 1.0)
    try:
        for device in config["devices"]:
            worker = factory(target=run_worker, args=(device, config["load_percent"], start_event, stop_event, messages))
            with defer_interrupts():
                worker.start()
                workers.append(worker)
        startup_deadline = time.monotonic() + config.get("startup_timeout", 300.0)
        while len(allocations) < len(workers):
            if time.monotonic() >= startup_deadline:
                raise RuntimeError("GPU worker initialization timed out.")
            try:
                message = messages.get(timeout=min(0.05, max(0.001, startup_deadline - time.monotonic())))
                if message["kind"] == "error":
                    raise RuntimeError(f'{message["uuid"]}: {message["error"]}')
                if message["kind"] == "ready":
                    allocations[message["uuid"]] = message["allocated_bytes"]
            except queue.Empty:
                pass
            if any(not worker.is_alive() for worker in workers):
                raise RuntimeError("GPU worker exited during initialization.")
        started = time.monotonic()
        last_progress = {d["uuid"]: started for d in config["devices"]}
        start_event.set()
        deadline = None if config["duration_seconds"] is None else started + config["duration_seconds"]
        reason = "duration completed"
        while True:
            while True:
                try:
                    message = messages.get_nowait()
                except queue.Empty:
                    break
                if message["kind"] == "error":
                    raise RuntimeError(f'{message["uuid"]}: {message["error"]}')
                last_progress[message["uuid"]] = time.monotonic()
            if any(not worker.is_alive() for worker in workers):
                raise RuntimeError("GPU worker exited unexpectedly.")
            if any(time.monotonic() - timestamp > config.get("worker_timeout", 120.0) for timestamp in last_progress.values()):
                raise RuntimeError("GPU worker stopped responding.")
            for device in config["devices"]:
                sample = sampler(device["uuid"])
                sample["elapsed_seconds"] = time.monotonic() - started
                sample["allocated_bytes"] = allocations[device["uuid"]]
                on_sample(sample)
            if deadline is not None and time.monotonic() >= deadline:
                break
            wait = interval if deadline is None else min(interval, max(0, deadline - time.monotonic()))
            stop_event.wait(wait)
    except KeyboardInterrupt:
        reason = "stopped by user"
    except Exception as error:
        errors.append(str(error))
        reason = "error, test incomplete"
    finally:
        elapsed = time.monotonic() - started if started is not None else 0.0
        try:
            with defer_interrupts():
                shutdown_workers(workers, stop_event)
        except KeyboardInterrupt:
            reason = "stopped by user" if not errors else reason
        except Exception as error:
            errors.append(f"Worker cleanup failed: {error}")
            reason = "error, test incomplete"
        messages.close()
        # Do not block cleanup waiting for an unread queue feeder.
        messages.cancel_join_thread()
    return {"stop_reason": reason, "elapsed_seconds": elapsed, "errors": errors, "allocations": allocations}
