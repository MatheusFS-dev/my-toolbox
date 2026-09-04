//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

func configureNonInteractive(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
