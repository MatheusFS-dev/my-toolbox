"""Heartbeat timing and conservative memory-leak detection."""


class HeartbeatClock:
    """Track fixed positive heartbeat intervals in minutes."""

    def __init__(self, interval_minutes, started_at):
        """Create a clock whose first delivery follows one full interval."""
        if interval_minutes <= 0:
            raise ValueError("heartbeat interval must be positive")
        self.interval_seconds = interval_minutes * 60
        self.last_delivery = started_at

    def due(self, now):
        """Return whether the configured interval has elapsed."""
        return now - self.last_delivery >= self.interval_seconds

    def delivered(self, now):
        """Move the delivery baseline to the successful attempt time."""
        self.last_delivery = now


def possible_memory_leak(samples, settings):
    """Detect sustained process-tree RSS growth after the warm-up period."""
    if not settings.get("enabled", True) or len(samples) < 2:
        return False
    newest = samples[-1]
    if newest["elapsed_seconds"] < settings["warmup_seconds"]:
        return False
    window_start = newest["elapsed_seconds"] - settings["window_seconds"]
    candidates = [sample for sample in samples if sample["elapsed_seconds"] >= window_start]
    if len(candidates) < 2:
        return False
    oldest = candidates[0]
    elapsed_minutes = (newest["elapsed_seconds"] - oldest["elapsed_seconds"]) / 60
    if elapsed_minutes <= 0:
        return False
    growth = newest["ram_mib"] - oldest["ram_mib"]
    slope = growth / elapsed_minutes
    return growth >= settings["minimum_growth_mib"] and slope >= settings["minimum_slope_mib_per_minute"]
