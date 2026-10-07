"""Short critical sections that preserve cancellation after bookkeeping."""
from contextlib import contextmanager
import signal
import threading


@contextmanager
def defer_interrupts():
    """Defer SIGINT until a short critical section has finished.

    Args:
        None.

    Yields:
        None: Protected execution region.

    Returns:
        Iterator[None]: Context manager for critical bookkeeping.

    Raises:
        KeyboardInterrupt: After the region if SIGINT was received.
        Exception: Any exception raised by the protected operation.

    Notes:
        Signal handlers can only be installed on the main Python thread.
        Non-main-thread callers retain their existing signal behavior.
    """
    if threading.current_thread() is not threading.main_thread():
        yield
        return
    previous = signal.getsignal(signal.SIGINT)
    interrupted = False
    def remember_interrupt(signum, frame):
        """Record cancellation without interrupting critical bookkeeping.

        Args:
            signum (int): Received signal number.
            frame (FrameType | None): Interrupted frame.
        Returns:
            None.
        Raises:
            None.
        """
        nonlocal interrupted
        interrupted = True
    signal.signal(signal.SIGINT, remember_interrupt)
    try:
        yield
    finally:
        signal.signal(signal.SIGINT, previous)
    if interrupted:
        raise KeyboardInterrupt
