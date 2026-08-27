import os
import time

SELECTED_GPU = "0"
STRESS_SECONDS = 300
MATRIX_SIZE = 16384
WARMUP_ITERATIONS = 5
LOG_INTERVAL_SECONDS = 5
DTYPE_NAME = "float16"
USE_TF32 = True


os.environ["CUDA_DEVICE_ORDER"] = "PCI_BUS_ID"
os.environ["CUDA_VISIBLE_DEVICES"] = SELECTED_GPU


import torch


def configure_torch():
    """Configures PyTorch CUDA math settings for high GPU utilization.

    Args:
        None.

    Returns:
        None.

    Raises:
        None.

    Example:
        configure_torch()
    """
    torch.backends.cuda.matmul.allow_tf32 = USE_TF32
    torch.backends.cudnn.allow_tf32 = USE_TF32
    torch.set_float32_matmul_precision("high")


def get_dtype(dtype_name):
    """Converts a dtype name into a PyTorch dtype.

    Args:
        dtype_name (str): Name of the tensor dtype. Supported values are
            "float16", "bfloat16", and "float32".

    Returns:
        torch.dtype: PyTorch dtype object.

    Raises:
        ValueError: If the dtype name is unsupported.

    Example:
        dtype = get_dtype("float16")
    """
    if dtype_name == "float16":
        return torch.float16

    if dtype_name == "bfloat16":
        return torch.bfloat16

    if dtype_name == "float32":
        return torch.float32

    raise ValueError(f"Unsupported dtype: {dtype_name}")


def check_cuda_device():
    """Checks that exactly one CUDA-visible GPU is available to PyTorch.

    Args:
        None.

    Returns:
        torch.device: The selected CUDA device, mapped as cuda:0 by PyTorch.

    Raises:
        RuntimeError: If CUDA is unavailable or no visible GPU exists.

    Example:
        device = check_cuda_device()
    """
    if not torch.cuda.is_available():
        raise RuntimeError(
            "CUDA is not available. Check NVIDIA driver, PyTorch CUDA build, " "and CUDA_VISIBLE_DEVICES."
        )

    visible_count = torch.cuda.device_count()

    if visible_count < 1:
        raise RuntimeError("No CUDA GPU is visible to PyTorch.")

    device = torch.device("cuda:0")
    gpu_name = torch.cuda.get_device_name(device)

    print(f"CUDA_VISIBLE_DEVICES={SELECTED_GPU}", flush=True)
    print(f"PyTorch visible CUDA GPUs: {visible_count}", flush=True)
    print(f"Using PyTorch device cuda:0 mapped to physical GPU {SELECTED_GPU}", flush=True)
    print(f"GPU name: {gpu_name}", flush=True)

    return device


def create_stress_tensors(device, matrix_size, dtype):
    """Creates GPU tensors for compute-heavy matrix multiplication.

    Args:
        device (torch.device): CUDA device where tensors will be allocated.
        matrix_size (int): Square matrix dimension.
        dtype (torch.dtype): Tensor data type used for computation.

    Returns:
        tuple[torch.Tensor, torch.Tensor, torch.Tensor]: Two input matrices and
        one output matrix.

    Raises:
        RuntimeError: If the selected GPU does not have enough memory.

    Example:
        device = torch.device("cuda:0")
        a, b, c = create_stress_tensors(device, 16384, torch.float16)
    """
    try:
        a = torch.randn((matrix_size, matrix_size), device=device, dtype=dtype)
        b = torch.randn((matrix_size, matrix_size), device=device, dtype=dtype)
        c = torch.empty((matrix_size, matrix_size), device=device, dtype=dtype)
        torch.cuda.synchronize(device)
        return a, b, c
    except RuntimeError as exc:
        raise RuntimeError(
            f"Failed to allocate tensors on {device}. "
            f"Reduce MATRIX_SIZE from {matrix_size} to 12288 or 8192."
        ) from exc


def run_warmup(device, a, b, c):
    """Runs warm-up matrix multiplications before the stress loop.

    Args:
        device (torch.device): CUDA device being stressed.
        a (torch.Tensor): First input matrix.
        b (torch.Tensor): Second input matrix.
        c (torch.Tensor): Output matrix.

    Returns:
        None.

    Raises:
        RuntimeError: If CUDA execution fails.

    Example:
        run_warmup(device, a, b, c)
    """
    for _ in range(WARMUP_ITERATIONS):
        torch.matmul(a, b, out=c)

    torch.cuda.synchronize(device)


def stress_selected_gpu(device, matrix_size, stress_seconds, dtype):
    """Runs a sustained compute workload on the selected CUDA GPU.

    Args:
        device (torch.device): CUDA device to stress.
        matrix_size (int): Square matrix dimension.
        stress_seconds (int): Stress duration in seconds.
        dtype (torch.dtype): Tensor data type used for computation.

    Returns:
        None.

    Raises:
        RuntimeError: If tensor allocation or CUDA execution fails.

    Example:
        device = torch.device("cuda:0")
        stress_selected_gpu(device, 16384, 300, torch.float16)
    """
    a, b, c = create_stress_tensors(device, matrix_size, dtype)
    run_warmup(device, a, b, c)

    start_time = time.time()
    last_log_time = start_time
    iterations = 0

    while True:
        current_time = time.time()
        elapsed_time = current_time - start_time

        if elapsed_time >= stress_seconds:
            break

        torch.matmul(a, b, out=c)
        iterations += 1

        if current_time - last_log_time >= LOG_INTERVAL_SECONDS:
            torch.cuda.synchronize(device)
            elapsed_time = time.time() - start_time
            print(
                f"elapsed={elapsed_time:.1f}s " f"iterations={iterations}",
                flush=True,
            )
            last_log_time = time.time()

    torch.cuda.synchronize(device)
    total_time = time.time() - start_time

    print(
        f"Finished stress test. " f"iterations={iterations}, total_time={total_time:.1f}s",
        flush=True,
    )


def main():
    """Runs the single-GPU stress test.

    Args:
        None.

    Returns:
        None.

    Raises:
        RuntimeError: If no CUDA device is usable or the stress workload fails.
        ValueError: If the selected dtype name is unsupported.

    Example:
        main()
    """
    configure_torch()
    dtype = get_dtype(DTYPE_NAME)
    device = check_cuda_device()
    stress_selected_gpu(device, MATRIX_SIZE, STRESS_SECONDS, dtype)


if __name__ == "__main__":
    main()
