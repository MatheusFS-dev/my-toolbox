"""Adaptive compute and memory workloads for one CUDA GPU."""
import math
import queue
import signal
import time

MIB = 1024 * 1024


def allocate_workload(torch_module, device, load_percent):
    """Allocate warmed compute matrices and actively used byte buffers.

    Args:
        torch_module (module): CUDA-enabled PyTorch or a test backend.
        device (int): CUDA-visible device ordinal.
        load_percent (float): Compute and free-VRAM fraction from 1 to 100.

    Returns:
        tuple: Input matrices, output matrix, and additional byte tensors.

    Raises:
        RuntimeError: If memory is insufficient or CUDA operations fail.
        ValueError: If the requested percentage is invalid.

    Examples:
        workload = allocate_workload(torch, 0, 100)

    Notes:
        The budget uses free VRAM after CUDA context initialization. Reserve
        workspace headroom, then consume the rest in progressively smaller
        chunks. Warm matmul before filling VRAM to include its workspace.
    """
    if not math.isfinite(load_percent) or not 1 <= load_percent <= 100:
        raise ValueError("Relative usage must be between 1 and 100.")
    free, _ = torch_module.cuda.mem_get_info(device)
    reserve = max(16 * MIB, min(256 * MIB, int(free * 0.02)))
    budget = min(int(free * load_percent / 100), free - reserve)
    minimum = 3 * 256 * 256 * 4
    if budget < minimum:
        raise RuntimeError("Insufficient free GPU memory for the requested load.")
    cuda_device = f"cuda:{device}"
    matrices = None
    for _ in range(12):
        dimension = min(16384, int(math.sqrt(budget * 0.35 / 12)))
        dimension = max(256, dimension // 256 * 256)
        pending = []
        try:
            for _ in range(3):
                tensor = torch_module.empty((dimension, dimension), device=cuda_device, dtype=torch_module.float32)
                pending.append(tensor)
                tensor.fill_(0.01)
            torch_module.matmul(pending[0], pending[1], out=pending[2])
            torch_module.cuda.synchronize(device)
            matrices = tuple(pending)
            break
        except torch_module.cuda.OutOfMemoryError:
            # Drop all references before asking the allocator to release caches.
            pending.clear()
            tensor = None
            torch_module.cuda.empty_cache()
            budget //= 2
            if budget < minimum:
                break
    if matrices is None:
        raise RuntimeError("Unable to allocate a usable workload after GPU memory retries.")
    used = sum(t.numel() * t.element_size() for t in matrices)
    chunks = []
    remaining = max(0, budget - used)
    chunk_size = min(128 * MIB, remaining)
    while remaining >= MIB and chunk_size >= MIB:
        size = min(chunk_size, remaining)
        try:
            tensor = torch_module.empty((size,), device=cuda_device, dtype=torch_module.uint8)
            tensor.fill_(0)
            chunks.append(tensor)
            remaining -= size
        except torch_module.cuda.OutOfMemoryError:
            tensor = None
            torch_module.cuda.empty_cache()
            chunk_size //= 2
    torch_module.cuda.synchronize(device)
    return (*matrices, chunks)


def exercise_workload(torch_module, workload, iteration):
    """Issue compute and touch the next allocated VRAM chunk.

    Args:
        torch_module (module): CUDA-enabled PyTorch.
        workload (tuple): Matrices and byte buffers from allocate_workload.
        iteration (int): Iteration count used to rotate across buffers.

    Returns:
        None.

    Raises:
        RuntimeError: If compute or memory access fails.
    """
    a, b, c, chunks = workload
    torch_module.matmul(a, b, out=c)
    if chunks:
        chunks[iteration % len(chunks)].bitwise_xor_(1)


def duty_pause(work_seconds, load_percent):
    """Calculate idle time needed for a requested active duty cycle.

    Args:
        work_seconds (float): Measured synchronized GPU work duration.
        load_percent (float): Requested active-time percentage.

    Returns:
        float: Idle seconds after this batch.

    Raises:
        ValueError: If the percentage is outside 1 to 100.

    Examples:
        duty_pause(0.1, 25) returns approximately 0.3 seconds.
    """
    if not math.isfinite(load_percent) or not 1 <= load_percent <= 100:
        raise ValueError("Relative usage must be between 1 and 100.")
    return max(0.0, work_seconds * (100 / load_percent - 1))


def run_worker(device, load_percent, start_event, stop_event, messages):
    """Stress one GPU until the shared stop event is set.

    Args:
        device (dict): Selected GPU with CUDA ordinal and UUID.
        load_percent (float): Relative load percentage.
        start_event (multiprocessing.Event): Shared workload start gate.
        stop_event (multiprocessing.Event): Cooperative shutdown signal.
        messages (multiprocessing.Queue): Readiness, progress and error channel.

    Returns:
        None.

    Raises:
        None. Worker errors are sent to the parent and end the worker.

    Notes:
        CUDA is imported inside a spawned child, avoiding forked CUDA contexts.
        Ctrl+C belongs to the supervisor, which stops every child together.
    """
    signal.signal(signal.SIGINT, signal.SIG_IGN)
    try:
        import torch
        torch.set_num_threads(1)
        ordinal = device["ordinal"]
        torch.cuda.set_device(ordinal)
        torch.backends.cuda.matmul.allow_tf32 = True
        workload = allocate_workload(torch, ordinal, load_percent)
        allocated = sum(t.numel() * t.element_size() for t in (*workload[:3], *workload[3]))
        messages.put({"kind": "ready", "uuid": device["uuid"], "allocated_bytes": allocated})
        while not start_event.wait(0.1):
            if stop_event.is_set():
                return
        iterations = 0
        last_progress = 0.0
        while not stop_event.is_set():
            begin = time.monotonic()
            # Synchronize each batch to bound queued GPU work and measure pacing.
            for _ in range(4):
                if stop_event.is_set():
                    break
                exercise_workload(torch, workload, iterations)
                iterations += 1
            torch.cuda.synchronize(ordinal)
            elapsed = time.monotonic() - begin
            if time.monotonic() - last_progress >= 0.5:
                try:
                    messages.put_nowait({"kind": "progress", "uuid": device["uuid"], "iterations": iterations})
                except queue.Full:
                    pass
                last_progress = time.monotonic()
            stop_event.wait(duty_pause(elapsed, load_percent))
    except Exception as error:
        messages.put({"kind": "error", "uuid": device["uuid"], "error": str(error)})
