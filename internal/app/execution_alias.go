package app

import (
	"github.com/MatheusFS-dev/my-toolbox/internal/execution"
	"os/exec"
)

type BuiltinExecutor = execution.BuiltinExecutor
type ProcessExecutor = execution.ProcessExecutor

func configureNonInteractive(command *exec.Cmd) { execution.ConfigureNonInteractive(command) }
func supportedPowerShellPath() (string, bool)   { return execution.SupportedPowerShellPath() }
func regularFile(path string) bool              { return execution.RegularFile(path) }
func resolveUserCommand(name, platform string) (string, error) {
	return execution.ResolveUserCommand(name, platform)
}

func stressGPUPython(platform string) ([]string, error) { return execution.StressGPUPython(platform) }
