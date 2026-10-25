package app

import (
	"github.com/MatheusFS-dev/my-toolbox/internal/ui"
	"io"
)

const maxPresentationWidth = ui.MaxPresentationWidth
const (
	ansiWhite     = ui.ANSIWhite
	ansiBlue      = ui.ANSIBlue
	ansiGreen     = ui.ANSIGreen
	ansiGray      = ui.ANSIGray
	ansiBoldWhite = ui.ANSIBoldWhite
	ansiBrightRed = ui.ANSIBrightRed
)

type commandGroup = ui.CommandGroup

func filteredCommands(catalog Catalog, environment string, includeDirect bool) []Command {
	return ui.FilteredCommands(catalog, environment, includeDirect)
}
func groupCommands(commands []Command) []commandGroup { return ui.GroupCommands(commands) }
func renderHelp(commands []Command, version string, terminalWidth int, styled bool) string {
	return ui.RenderHelp(commands, version, terminalWidth, styled)
}
func helpTerminalProperties(output io.Writer) (bool, int, error) {
	return ui.HelpTerminalProperties(output)
}
func wrapText(text string, width int) []string { return ui.WrapText(text, width) }
func presentationStyle(text, code string, enabled bool) string {
	return ui.PresentationStyle(text, code, enabled)
}
