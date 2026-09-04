package main

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"
)

const maxPresentationWidth = 72

const (
	ansiWhite     = "37"
	ansiGreen     = "32"
	ansiGray      = "90"
	ansiBoldWhite = "1;37"
	ansiBrightRed = "91"
)

type commandGroup struct {
	Category string
	Commands []Command
}

// filteredCommands returns catalog commands visible in one presentation.
//
// Args:
//   - catalog: Validated catalog in public command order.
//   - environment: Current environment name used for support filtering.
//   - includeDirect: When true, include both list and direct commands for help;
//     when false, include only list-visible commands for interactive selection.
//
// Returns:
//   - []Command: Supported commands in unchanged catalog order.
//
// Raises:
//   - None.
func filteredCommands(catalog Catalog, environment string, includeDirect bool) []Command {
	commands := make([]Command, 0, len(catalog.Commands))
	for _, command := range catalog.Commands {
		if !command.SupportsEnvironment(environment) {
			continue
		}
		if !includeDirect && command.Visibility != "list" {
			continue
		}
		command.presentationEnvironment = environment
		commands = append(commands, command)
	}
	return commands
}

// groupCommands groups already-filtered commands by first category appearance.
//
// Args:
//   - commands: Supported commands in catalog order. Callers must filter
//     visibility and environment support before grouping.
//
// Returns:
//   - []commandGroup: Non-empty groups preserving category and command order.
//
// Raises:
//   - None.
func groupCommands(commands []Command) []commandGroup {
	groups := []commandGroup{}
	indexes := map[string]int{}
	for _, command := range commands {
		index, exists := indexes[command.Category]
		if !exists {
			index = len(groups)
			indexes[command.Category] = index
			groups = append(groups, commandGroup{Category: command.Category})
		}
		groups[index].Commands = append(groups[index].Commands, command)
	}
	return groups
}

// renderHelp renders categorized help at the requested terminal width.
//
// Args:
//   - commands: Environment-filtered catalog commands, including direct-only
//     commands when they are supported.
//   - version: Current toolbox version displayed beside the title.
//   - terminalWidth: Live TTY width, or 80 for redirected output. Content is
//     capped at 72 columns and narrower positive widths are honored.
//   - styled: When true, add foreground and bold ANSI styling. When false,
//     produce the identical hierarchy and wrapping without escape sequences.
//
// Returns:
//   - string: Complete help text with a trailing newline.
//
// Raises:
//   - None.
//
// Example:
//
//	help := renderHelp(commands, "1.2.3", 80, false)
func renderHelp(commands []Command, version string, terminalWidth int, styled bool) string {
	width := min(terminalWidth, maxPresentationWidth)
	if width < 1 {
		width = 1
	}
	var output strings.Builder
	writeHeading := func(text string) {
		output.WriteString(presentationStyle(text, ansiBoldWhite, styled))
		output.WriteByte('\n')
	}
	writeWrapped := func(text string, indentation int, color string) {
		for _, line := range wrapText(text, max(1, width-indentation)) {
			output.WriteString(strings.Repeat(" ", min(indentation, width)))
			output.WriteString(presentationStyle(line, color, styled))
			output.WriteByte('\n')
		}
	}

	writeHeading("TOOLBOX " + version)
	writeWrapped("Portable terminal tools for supported Linux, WSL, and Windows environments.", 2, ansiWhite)
	output.WriteByte('\n')
	writeHeading("USAGE")
	usage := []struct {
		command     string
		description string
	}{
		{"tb list", "Select supported tools interactively and run them in catalog order."},
		{"tb <tool> [arguments...]", "Run one supported catalog tool directly, forwarding its arguments."},
		{"tb update", "Reinstall the toolbox when a newer release is available."},
		{"tb uninstall", "Confirm and remove the toolbox wrapper and installed versions."},
		{"tb version", "Print the installed toolbox version."},
		{"tb help", "Show this usage and the tools supported in the current environment."},
	}
	for _, entry := range usage {
		writeWrapped(entry.command, 2, ansiWhite)
		writeWrapped(entry.description, 4, ansiGray)
	}
	output.WriteByte('\n')
	writeHeading("AVAILABLE TOOLS")
	for groupIndex, group := range groupCommands(commands) {
		if groupIndex > 0 {
			output.WriteByte('\n')
		}
		writeWrapped(group.Category, 2, ansiBoldWhite)
		for _, command := range group.Commands {
			writeWrapped(command.Name, 4, ansiWhite)
			writeWrapped(command.Description, 6, ansiGray)
			if requirements := command.requirementText(); requirements != "" {
				writeWrapped(requirements, 6, ansiBrightRed)
			}
		}
	}
	return output.String()
}

// helpTerminalProperties detects whether help output is a TTY and its width.
//
// Args:
//   - output: Destination writer for help text.
//
// Returns:
//   - bool: True only when output exposes a terminal file descriptor.
//   - int: Live terminal width for a TTY, or the required 80-column basis for
//     redirected output.
//   - error: Terminal size lookup failure for detected TTY output.
//
// Raises:
//   - None. Terminal failures are returned as errors.
func helpTerminalProperties(output io.Writer) (bool, int, error) {
	file, hasDescriptor := output.(interface{ Fd() uintptr })
	if !hasDescriptor || !term.IsTerminal(file.Fd()) {
		return false, 80, nil
	}
	width, _, err := term.GetSize(file.Fd())
	if err != nil {
		return false, 0, fmt.Errorf("read terminal size: %w", err)
	}
	return true, width, nil
}

// wrapText wraps plain text without exceeding a visible column width.
//
// Args:
//   - text: Plain text to wrap. Consecutive whitespace is normalized because
//     catalog descriptions contain prose rather than preformatted content.
//   - width: Maximum visible columns per returned line; must be positive.
//
// Returns:
//   - []string: Wrapped lines. Words longer than width are split by rune.
//
// Raises:
//   - panic: If width is not positive.
func wrapText(text string, width int) []string {
	if width < 1 {
		panic("wrap width must be positive")
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	lines := []string{}
	current := ""
	for _, word := range words {
		for lipgloss.Width(word) > width {
			prefix, remainder := splitAtWidth(word, width)
			if current != "" {
				lines = append(lines, current)
				current = ""
			}
			lines = append(lines, prefix)
			word = remainder
		}
		if current == "" {
			current = word
			continue
		}
		if lipgloss.Width(current)+1+lipgloss.Width(word) <= width {
			current += " " + word
			continue
		}
		lines = append(lines, current)
		current = word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

// splitAtWidth divides one plain word at a visible column boundary.
//
// Args:
//   - text: Non-empty word whose visible width exceeds width.
//   - width: Positive maximum width of the returned prefix.
//
// Returns:
//   - string: Largest non-empty prefix fitting within width.
//   - string: Remaining suffix.
//
// Raises:
//   - None. Callers must satisfy the documented non-empty and width assumptions.
func splitAtWidth(text string, width int) (string, string) {
	end := 0
	for index := range text {
		if index == 0 {
			continue
		}
		if lipgloss.Width(text[:index]) > width {
			break
		}
		end = index
	}
	if end == 0 {
		_, size := utf8.DecodeRuneInString(text)
		end = size
	}
	return text[:end], text[end:]
}

// presentationStyle applies one ANSI SGR style when styling is enabled.
//
// Args:
//   - text: Text to style.
//   - code: ANSI SGR code without control-sequence delimiters.
//   - enabled: When true, wrap text with the style and reset sequences; when
//     false, return text byte-for-byte without styling.
//
// Returns:
//   - string: Styled or unchanged text.
//
// Raises:
//   - None.
func presentationStyle(text string, code string, enabled bool) string {
	if !enabled {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}
