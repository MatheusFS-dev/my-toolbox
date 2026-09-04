//go:build windows

package main

import "fmt"

func (builtins *ToolboxBuiltins) installMonitor() error {
	return fmt.Errorf("Monitor supports Linux and WSL only")
}
