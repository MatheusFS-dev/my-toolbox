//go:build windows

package execution

import (
	"os/exec"
	"syscall"
)

func ConfigureNonInteractive(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
