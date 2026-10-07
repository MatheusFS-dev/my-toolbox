//go:build !windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// cancelStressGPUCommand terminates the isolated dependency process group.
// Args: command (*exec.Cmd): Started dependency command with Setsid enabled.
// Returns: error: Kill failure, or os.ErrProcessDone when already exited.
// Raises: None.
func cancelStressGPUCommand(command *exec.Cmd) error {
	err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
