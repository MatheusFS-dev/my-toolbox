"""Crash and automatic restart accounting."""


class RestartPolicy:
    """Track crash budget while keeping automatic restarts budget-neutral."""

    def __init__(self, max_crash_retries, base_delay, multiplier, max_delay):
        """Initialize restart limits and delay calculation."""
        self.max_crash_retries = max_crash_retries
        self.base_delay = base_delay
        self.multiplier = multiplier
        self.max_delay = max_delay
        self.crash_count = 0
        self.scheduled_count = 0
        self.memory_count = 0

    def record_scheduled_restart(self):
        """Record a scheduled restart without consuming crash budget."""
        self.scheduled_count += 1
        return True

    def record_memory_restart(self):
        """Record a memory-limit restart without consuming crash budget."""
        self.memory_count += 1
        return True

    def record_crash(self):
        """Consume one crash retry and return whether and when to restart."""
        if self.crash_count >= self.max_crash_retries:
            return False, 0
        delay = min(self.max_delay, self.base_delay * (self.multiplier ** self.crash_count))
        self.crash_count += 1
        return True, delay
