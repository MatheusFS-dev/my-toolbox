package app

import (
	"github.com/MatheusFS-dev/my-toolbox/internal/features/builtins"
	"io"
)

type ToolboxBuiltins = builtins.ToolboxBuiltins

func NewToolboxBuiltins(root, platform, version string, output io.Writer) *ToolboxBuiltins {
	return builtins.NewToolboxBuiltins(root, platform, version, output)
}
func runPlatformCleanup() (bool, error) { return builtins.RunPlatformCleanup() }
func runMonitorCLI(root, version string, arguments []string, input io.Reader, output, errorOutput io.Writer) int {
	return builtins.RunMonitorCLI(root, version, arguments, input, output, errorOutput)
}
func monitorInstalled() (bool, error)                  { return builtins.MonitorInstalled() }
func monitorUpdateNeeded(root string) (bool, error)    { return builtins.MonitorUpdateNeeded(root) }
func stressGPUInstalled(platform string) (bool, error) { return builtins.StressGPUInstalled(platform) }

func toolboxDataRoot(platform string) (string, error) { return builtins.ToolboxDataRoot(platform) }
func parseVersion(version string) ([3]int, error)     { return builtins.ParseVersion(version) }

const linuxToolboxWrapper = builtins.LinuxToolboxWrapper
const toolboxLinuxInstallerURL = builtins.ToolboxLinuxInstallerURL
