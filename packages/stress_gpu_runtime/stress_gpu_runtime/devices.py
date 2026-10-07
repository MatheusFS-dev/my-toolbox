"""CUDA discovery and read-only NVIDIA telemetry."""
from uuid import UUID


def optional_metric(nvml, function, *args):
    """Read a metric while distinguishing unsupported calls from device errors.

    Args:
        nvml (module): NVIDIA management bindings.
        function (str): NVML function name.
        *args (object): Function arguments.

    Returns:
        object | None: Metric value, or None when unsupported.

    Raises:
        NVMLError: For lost devices, driver failures, or authorization errors.
    """
    callback = getattr(nvml, function, None)
    if callback is None:
        return None
    unsupported = tuple(
        error for name in ("NVMLError_NotSupported", "NVMLError_FunctionNotFound")
        if isinstance(error := getattr(nvml, name, None), type)
    )
    try:
        return callback(*args)
    except unsupported:
        return None


def discover_devices(torch_module, nvml_module):
    """Find CUDA-visible NVIDIA devices and resolve telemetry by UUID.

    Args:
        torch_module (module): CUDA-enabled PyTorch, injectable for tests.
        nvml_module (module): Initialized NVIDIA management bindings.

    Returns:
        list[dict]: CUDA ordinals, UUIDs, resource capacities and thresholds.

    Raises:
        RuntimeError: If CUDA is unavailable or UUID identity cannot be resolved.
        NVMLError: If NVIDIA telemetry cannot resolve a selected device.

    Examples:
        devices = discover_devices(torch, pynvml)

    Notes:
        CUDA_VISIBLE_DEVICES is respected by PyTorch. MIG instances must have
        UUID telemetry support or discovery fails instead of monitoring a parent.
    """
    cuda = torch_module.cuda
    if not cuda.is_available() or cuda.device_count() == 0:
        raise RuntimeError("No NVIDIA CUDA GPU is available. Check the driver and CUDA-enabled PyTorch installation.")
    devices = []
    for ordinal in range(cuda.device_count()):
        properties = cuda.get_device_properties(ordinal)
        uuid = getattr(properties, "uuid", None)
        if not uuid:
            raise RuntimeError("CUDA device UUID is unavailable. Repair the application with tb install-stress-gpu.")
        uuid, handle = resolve_nvml_uuid(nvml_module, uuid)
        free, total = cuda.mem_get_info(ordinal)
        devices.append({
            "ordinal": ordinal, "uuid": uuid, "name": properties.name,
            "free_bytes": free, "total_bytes": total,
            "slowdown_celsius": optional_metric(nvml_module, "nvmlDeviceGetTemperatureThreshold", handle, 1),
        })
    return devices


def sample_device(nvml_module, uuid):
    """Sample a particular board without treating unavailable metrics as zero.

    Args:
        nvml_module (module): Initialized NVIDIA management bindings.
        uuid (str): CUDA device UUID.

    Returns:
        dict: Nullable utilization, memory, temperature, power and clock metrics.

    Raises:
        NVMLError: If the device is lost or monitoring fails unexpectedly.
    """
    handle = nvml_module.nvmlDeviceGetHandleByUUID(uuid)
    utilization = optional_metric(nvml_module, "nvmlDeviceGetUtilizationRates", handle)
    memory = optional_metric(nvml_module, "nvmlDeviceGetMemoryInfo", handle)
    power = optional_metric(nvml_module, "nvmlDeviceGetPowerUsage", handle)
    events = optional_metric(nvml_module, "nvmlDeviceGetCurrentClocksEventReasons", handle)
    if events is None:
        events = optional_metric(nvml_module, "nvmlDeviceGetCurrentClocksThrottleReasons", handle)
    return {
        "uuid": uuid,
        "utilization_percent": utilization.gpu if utilization is not None else None,
        "memory_used_bytes": memory.used if memory is not None else None,
        "memory_total_bytes": memory.total if memory is not None else None,
        "temperature_celsius": optional_metric(nvml_module, "nvmlDeviceGetTemperature", handle, 0),
        "power_watts": power / 1000 if power is not None else None,
        "graphics_clock_mhz": optional_metric(nvml_module, "nvmlDeviceGetClockInfo", handle, 0),
        "memory_clock_mhz": optional_metric(nvml_module, "nvmlDeviceGetClockInfo", handle, 2),
        "clock_event_reasons": events,
        # NVML defines these bits for software and hardware thermal slowdown.
        "thermal_throttling": bool(events & (0x20 | 0x40)) if events is not None else None,
    }


def resolve_nvml_uuid(nvml, cuda_uuid):
    """Resolve PyTorch's bare UUID to the canonical physical or MIG identity.

    Args:
        nvml (module): Initialized NVIDIA bindings.
        cuda_uuid (object): PyTorch _CUuuid, UUID text, or UUID bytes.

    Returns:
        tuple[str, object]: Canonical NVML identifier and matching device handle.

    Raises:
        RuntimeError: If no physical or MIG device matches the CUDA UUID.
        NVMLError: If authorization or device/driver access fails.

    Notes:
        A CUDA MIG UUID is tested against the MIG namespace after a documented
        physical-device lookup miss. A parent device is never substituted.
    """
    if isinstance(cuda_uuid, bytes):
        value = str(UUID(bytes=cuda_uuid)) if len(cuda_uuid) == 16 else cuda_uuid.decode()
    else:
        value = str(cuda_uuid)
    candidates = [value] if value.startswith(("GPU-", "MIG-")) else ["GPU-" + value, "MIG-" + value]
    misses = tuple(error for error in [getattr(nvml, "NVMLError_NotFound", None)] if isinstance(error, type))
    for candidate in candidates:
        try:
            return candidate, nvml.nvmlDeviceGetHandleByUUID(candidate)
        except misses:
            continue
    raise RuntimeError(f"No matching NVIDIA GPU/MIG telemetry device for CUDA UUID {value}. Check NVML and MIG UUID support.")
