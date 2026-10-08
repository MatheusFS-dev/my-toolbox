//go:build windows

package main

import "fmt"

func (builtins *ToolboxBuiltins) installMonitor() error {
	return fmt.Errorf("Monitor supports Linux and WSL only")
}

func (builtins *ToolboxBuiltins) uninstallMonitor() error {
	return fmt.Errorf("Monitor supports Linux and WSL only")
}

func monitorInstalled() (bool, error) {
	return false, nil
}

func monitorUpdateNeeded(string) (bool, error) {
	return false, nil
}
