package app

import (
	"github.com/MatheusFS-dev/my-toolbox/internal/features/macros"
	"io"
)

type Macro = macros.Macro
type MacroArgument = macros.MacroArgument
type MacroAction = macros.MacroAction

const (
	MacroActionRun      = macros.MacroActionRun
	MacroActionDownload = macros.MacroActionDownload
)

type MacroSelection = macros.MacroSelection
type MacroWorkflow = macros.MacroWorkflow
type macroDriver = macros.MacroDriver

var macroDrivers = macros.MacroDrivers

func macroSupportsOS(macro Macro, goos string) bool { return macros.MacroSupportsOS(macro, goos) }
func loadMacros(root string, warnings io.Writer) ([]Macro, error) {
	return macros.LoadMacros(root, warnings)
}
func validateMacroArgument(argument MacroArgument, value string) error {
	return macros.ValidateMacroArgument(argument, value)
}

func loadMacro(root, subpackage, path string) (Macro, error) {
	return macros.LoadMacro(root, subpackage, path)
}

type DefaultMacroWorkflow = macros.DefaultMacroWorkflow

func probeAutoHotkeyV2(candidate string) error { return macros.ProbeAutoHotkeyV2(candidate) }
func findYdotool(home string) (string, error)  { return macros.FindYdotool(home) }
func probeYdotool(path string) error           { return macros.ProbeYdotool(path) }
func startYdotoolDaemon(daemon, socket, home string, needsSudo bool, output io.Writer) (string, error) {
	return macros.StartYdotoolDaemon(daemon, socket, home, needsSudo, output)
}
