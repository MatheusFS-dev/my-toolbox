//go:build windows

package main

import (
	"fmt"
	"golang.org/x/sys/windows"
)

// acquireStressGPULock holds an OS lock released even after forced process exit.
// Args: path (string): Stable private lock file.
// Returns: func(): Release callback. error: Ownership, I/O or competing lock failure.
// Raises: None.
func acquireStressGPULock(path string) (func(), error) {
	file, err := openStressGPULock(path)
	if err != nil {
		return nil, err
	}
	handle := windows.Handle(file.Fd())
	overlapped := &windows.Overlapped{}
	if err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, overlapped); err != nil {
		file.Close()
		return nil, fmt.Errorf("another stress-gpu installation is running: %w", err)
	}
	return func() { windows.UnlockFileEx(handle, 0, 1, 0, overlapped); file.Close() }, nil
}
