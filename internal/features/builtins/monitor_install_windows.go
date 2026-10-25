//go:build windows

package builtins

import "fmt"

func (builtins *ToolboxBuiltins) installMonitor() error {
	return fmt.Errorf("Monitor supports Linux and WSL only")
}

func (builtins *ToolboxBuiltins) uninstallMonitor() error {
	return fmt.Errorf("Monitor supports Linux and WSL only")
}

func MonitorInstalled() (bool, error) {
	return false, nil
}

func MonitorUpdateNeeded(string) (bool, error) {
	return false, nil
}
