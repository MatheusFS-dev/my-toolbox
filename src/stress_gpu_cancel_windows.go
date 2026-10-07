//go:build windows

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// cancelStressGPUCommand terminates the dependency interpreter and its children.
// Args: command (*exec.Cmd): Started Windows dependency command.
// Returns: error: Process termination failure.
// Raises: None.
func cancelStressGPUCommand(command *exec.Cmd) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	taskkill := exec.CommandContext(ctx, filepath.Join(os.Getenv("SystemRoot"), "System32", "taskkill.exe"), "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F")
	configureNonInteractive(taskkill)
	if err := taskkill.Run(); err != nil {
		return command.Process.Kill()
	}
	return nil
}
