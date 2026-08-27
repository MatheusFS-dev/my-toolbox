"""Check whether TensorFlow and Keras can detect and use a GPU."""

import platform
import sys

import tensorflow as tf

BATCH_SIZE = 256
INPUT_SIZE = 1024


def main() -> int:
    """Print TensorFlow GPU information and execute a Keras inference on it.

    Returns:
        Zero when TensorFlow successfully executes a Keras model on the GPU.
        One when no GPU is available or GPU execution fails.
    """
    print("=" * 70)
    print("TensorFlow/Keras GPU check")
    print("=" * 70)

    print(f"Python version:     {platform.python_version()}")
    print(f"TensorFlow version: {tf.__version__}")
    print("Keras version:      " f"{getattr(tf.keras, '__version__', 'bundled with TensorFlow')}")
    print(f"Built with CUDA:    {tf.test.is_built_with_cuda()}")

    build_info = tf.sysconfig.get_build_info()

    for key in (
        "cuda_version",
        "cudnn_version",
        "is_cuda_build",
        "is_rocm_build",
        "is_tensorrt_build",
    ):
        if key in build_info:
            print(f"{key}: {build_info[key]}")

    physical_gpus = tf.config.list_physical_devices("GPU")

    print(f"\nPhysical GPUs detected: {len(physical_gpus)}")

    if not physical_gpus:
        print("\nRESULT: FAIL")
        print("TensorFlow did not detect any GPU.")
        return 1

    for index, gpu in enumerate(physical_gpus):
        print(f"\nGPU {index}")
        print(f"  TensorFlow name: {gpu.name}")

        details = tf.config.experimental.get_device_details(gpu)

        print(f"  Device name:     {details.get('device_name', 'Unavailable')}")
        print("  Compute capability: " f"{details.get('compute_capability', 'Unavailable')}")

        try:
            tf.config.experimental.set_memory_growth(gpu, True)
            print("  Memory growth:   Enabled")
        except RuntimeError as error:
            print(f"  Memory growth:   Could not be enabled: {error}")

    # Prevent TensorFlow from silently moving unsupported operations to the CPU.
    tf.config.set_soft_device_placement(False)

    logical_gpus = tf.config.list_logical_devices("GPU")
    print(f"\nLogical GPUs available: {len(logical_gpus)}")

    print("\nRunning a small Keras model on /GPU:0...")

    try:
        with tf.device("/GPU:0"):
            model = tf.keras.Sequential(
                [
                    tf.keras.layers.Input(shape=(INPUT_SIZE,)),
                    tf.keras.layers.Dense(512, activation="relu"),
                    tf.keras.layers.Dense(128),
                ]
            )

            inputs = tf.random.uniform(
                shape=(BATCH_SIZE, INPUT_SIZE),
                dtype=tf.float32,
            )

            outputs = model(inputs, training=False)

            # Converting to a Python value forces execution to complete.
            checksum = float(tf.reduce_sum(outputs).numpy())

        output_device = outputs.device
        used_gpu = "GPU" in output_device.upper()

        print(f"Output tensor device: {output_device}")
        print(f"Output shape:         {outputs.shape}")
        print(f"Output checksum:      {checksum:.6f}")

        try:
            memory_info = tf.config.experimental.get_memory_info("GPU:0")
            current_mib = memory_info["current"] / (1024**2)
            peak_mib = memory_info["peak"] / (1024**2)

            print(f"Current GPU memory:   {current_mib:.2f} MiB")
            print(f"Peak GPU memory:      {peak_mib:.2f} MiB")
        except (ValueError, KeyError) as error:
            print(f"GPU memory data:      Unavailable: {error}")

        if not used_gpu:
            print("\nRESULT: FAIL")
            print("The operation completed, but its output was not placed on a GPU.")
            return 1

    except Exception as error:
        print("\nRESULT: FAIL")
        print(f"GPU execution failed: {type(error).__name__}: {error}")
        return 1

    print("\nRESULT: PASS")
    print("TensorFlow detected the GPU and executed a Keras model on it.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
