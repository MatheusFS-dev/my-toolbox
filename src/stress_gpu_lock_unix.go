//go:build !windows

package main

import (
	"fmt"
	"syscall"
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
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("another stress-gpu installation is running: %w", err)
	}
	return func() { syscall.Flock(int(file.Fd()), syscall.LOCK_UN); file.Close() }, nil
}
