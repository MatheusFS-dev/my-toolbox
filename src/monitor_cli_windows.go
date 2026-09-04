//go:build windows

package main

import (
	"fmt"
	"io"
)

func runMonitorCLI(_ string, _ string, _ []string, _ io.Reader, _ io.Writer, errorOutput io.Writer) int {
	fmt.Fprintln(errorOutput, "Monitor supports Linux and WSL only")
	return 1
}
