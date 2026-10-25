"""Check whether PyTorch can detect and use a CUDA GPU."""

import platform
import sys

import torch

MATRIX_SIZE = 2048


def main() -> int:
    """Print PyTorch GPU information and execute an operation on the GPU.

    Returns:
        Zero when PyTorch successfully performs a matrix multiplication on the
        GPU. One when CUDA is unavailable or GPU execution fails.
    """
    print("=" * 70)
    print("PyTorch GPU check")
    print("=" * 70)

    print(f"Python version:       {platform.python_version()}")
    print(f"PyTorch version:      {torch.__version__}")
    print(f"CUDA runtime version: {torch.version.cuda}")
    print(f"ROCm version:         {getattr(torch.version, 'hip', None)}")
    print(f"cuDNN version:        {torch.backends.cudnn.version()}")

    cuda_available = torch.cuda.is_available()

    print(f"CUDA available:       {cuda_available}")

    if not cuda_available:
        print("\nRESULT: FAIL")
        print("PyTorch cannot access a CUDA GPU.")
        return 1

    gpu_count = torch.cuda.device_count()
    print(f"CUDA GPUs detected:   {gpu_count}")

    for index in range(gpu_count):
        properties = torch.cuda.get_device_properties(index)
        total_memory_gib = properties.total_memory / (1024**3)

        print(f"\nGPU {index}")
        print(f"  Name:                {properties.name}")
        print(f"  Compute capability:  " f"{properties.major}.{properties.minor}")
        print(f"  Total memory:        {total_memory_gib:.2f} GiB")
        print(f"  Multiprocessors:     {properties.multi_processor_count}")

    device = torch.device("cuda:0")

    print(f"\nSelected device: {device}")
    print(f"Current device:  cuda:{torch.cuda.current_device()}")
    print(f"Device name:     {torch.cuda.get_device_name(device)}")
    print(f"\nRunning {MATRIX_SIZE} x {MATRIX_SIZE} matrix multiplication...")

    try:
        torch.cuda.reset_peak_memory_stats(device)

        with torch.inference_mode():
            matrix_a = torch.rand(
                (MATRIX_SIZE, MATRIX_SIZE),
                device=device,
                dtype=torch.float32,
            )
            matrix_b = torch.rand(
                (MATRIX_SIZE, MATRIX_SIZE),
                device=device,
                dtype=torch.float32,
            )

            result = matrix_a @ matrix_b

            # Wait until all queued CUDA operations have completed.
            torch.cuda.synchronize(device)

            checksum = result.sum().item()

        allocated_mib = torch.cuda.memory_allocated(device) / (1024**2)
        reserved_mib = torch.cuda.memory_reserved(device) / (1024**2)
        peak_mib = torch.cuda.max_memory_allocated(device) / (1024**2)

        print(f"Result device:       {result.device}")
        print(f"Result shape:        {tuple(result.shape)}")
        print(f"Result checksum:     {checksum:.6f}")
        print(f"Allocated memory:    {allocated_mib:.2f} MiB")
        print(f"Reserved memory:     {reserved_mib:.2f} MiB")
        print(f"Peak allocated:      {peak_mib:.2f} MiB")

        if result.device.type != "cuda":
            print("\nRESULT: FAIL")
            print("The matrix multiplication result is not stored on a CUDA GPU.")
            return 1

    except Exception as error:
        print("\nRESULT: FAIL")
        print(f"GPU execution failed: {type(error).__name__}: {error}")
        return 1

    print("\nRESULT: PASS")
    print("PyTorch detected the GPU and executed an operation on it.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
