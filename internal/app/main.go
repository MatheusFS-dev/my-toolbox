package app

import (
	"errors"
	"fmt"
	"github.com/MatheusFS-dev/my-toolbox/internal/features/ssh"
	"os"
	"path/filepath"
	"time"
)

// main loads the installed payload and executes one tb command.
func Run(version string) {
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
	if len(os.Args) > 1 && os.Args[1] == "__ssh-tools-status" {
		if len(os.Args) != 2 {
			os.Exit(1)
		}
		os.Exit(ssh.RunSSHStatusDisplay(root, os.Stdout, os.Stderr))
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
	ui := HuhUI{}
	app := App{
		Catalog:       catalog,
		Environment:   environment,
		Platform:      platform,
		UI:            ui,
		Executor:      ProcessExecutor{Root: root, Platform: platform, Environment: environment, Builtins: builtins, Input: os.Stdin, Output: os.Stdout, Error: os.Stderr},
		Output:        os.Stdout,
		Error:         os.Stderr,
		ArticleRoot:   filepath.Join(root, "packages", "search", "articles"),
		MacroRoot:     filepath.Join(root, "packages", "macros"),
		MacroWorkflow: DefaultMacroWorkflow{UI: ui, Output: os.Stdout},
		Version:       version,
		componentInstalled: func(name string) (bool, error) {
			switch name {
			case "uninstall-monitor":
				return monitorInstalled()
			case "uninstall-stress-gpu":
				return stressGPUInstalled(platform)
			default:
				return false, fmt.Errorf("unknown component %q", name)
			}
		},
	}
	if err := app.Execute(os.Args[1:]); err != nil {
		tool := "tb"
		if len(os.Args) > 1 {
			tool = os.Args[1]
		}
		messages := newToolOutput(os.Stderr, tool, time.Now, detectToolColor(os.Stderr))
		if errors.Is(err, ErrCancelled) {
			_ = messages.Info("Cancelled; nothing changed.")
		} else {
			_ = messages.Error(fmt.Sprintf("Operation did not complete and may have made partial changes. Review the cause and retry: %v", err))
		}
		os.Exit(1)
	}
}
