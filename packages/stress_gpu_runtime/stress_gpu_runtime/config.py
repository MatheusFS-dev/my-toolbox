"""Interactive stress configuration."""
import math


def prompt_configuration(devices, input_fn=input):
    """Gather GPU selection, relative load and duration with input validation.

    Args:
        devices (list[dict]): Visible device descriptions with CUDA ordinals.
        input_fn (Callable[[str], str]): Prompt callback.

    Returns:
        dict: Selected devices, load_percent and optional duration_seconds.

    Raises:
        ValueError: If no GPUs were provided.
        EOFError: If interactive input ends.
        KeyboardInterrupt: If the user cancels configuration.

    Examples:
        prompt_configuration(devices) prompts for all three settings.
    """
    if not devices:
        raise ValueError("No GPUs are available.")
    while True:
        answer = input_fn("CUDA GPU numbers, comma-separated [all]: ").strip()
        try:
            numbers = [d["ordinal"] for d in devices] if answer.lower() in ("", "all") else [int(item.strip()) for item in answer.split(",")]
            by_ordinal = {d["ordinal"]: d for d in devices}
            if not numbers or len(numbers) != len(set(numbers)):
                raise ValueError("Choose each GPU only once.")
            selected = [by_ordinal[number] for number in numbers]
            break
        except (ValueError, KeyError):
            print("Enter distinct GPU numbers from the displayed list, or all.")
    while True:
        try:
            load = float(input_fn("Relative compute and available VRAM usage, 1-100% [100]: ").strip() or "100")
            if not math.isfinite(load) or not 1 <= load <= 100:
                raise ValueError("Load is outside the supported range.")
            break
        except ValueError:
            print("Enter a finite percentage from 1 to 100.")
    while True:
        answer = input_fn("Duration in seconds, or loop until Ctrl+C [loop]: ").strip().lower()
        if answer in ("", "loop"):
            duration = None
            break
        try:
            duration = float(answer)
            if not math.isfinite(duration) or duration <= 0:
                raise ValueError("Duration must be positive.")
            break
        except ValueError:
            print("Enter positive finite seconds or loop.")
    return {"devices": selected, "load_percent": load, "duration_seconds": duration}
