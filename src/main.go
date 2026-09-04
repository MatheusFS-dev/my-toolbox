package main

import (
	"fmt"
	"os"
	"path/filepath"
)

var version = "development"

// main loads the installed payload and executes one tb command.
func main() {
	cleanupHandled, cleanupErr := runPlatformCleanup()
	if cleanupHandled {
		if cleanupErr != nil {
			fmt.Fprintln(os.Stderr, cleanupErr)
			os.Exit(1)
		}
		return
	}
	root, err := executableRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(os.Args) > 1 && os.Args[1] == "__monitor" {
		os.Exit(runMonitorCLI(root, version, os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	}
	catalog, err := LoadCatalogFile(filepath.Join(root, "commands.json"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	platform, err := CurrentPlatform()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	environment, err := CurrentEnvironment()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	builtins := NewToolboxBuiltins(root, platform, version, os.Stdout)
	app := App{
		Catalog:     catalog,
		Environment: environment,
		UI:          HuhUI{},
		Executor:    ProcessExecutor{Root: root, Platform: platform, Environment: environment, Builtins: builtins, Input: os.Stdin, Output: os.Stdout, Error: os.Stderr},
		Output:      os.Stdout,
		Version:     version,
	}
	if err := app.Execute(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
