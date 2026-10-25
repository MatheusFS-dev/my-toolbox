package output

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
)

type ColorMode int

const (
	ColorModeNever ColorMode = iota
	ColorModeBasic
	ColorMode256
)

type ToolOutput struct {
	destination io.Writer
	tool        string
	now         func() time.Time
	color       ColorMode
	started     bool
}

func New(destination io.Writer, tool string, now func() time.Time, color ColorMode) *ToolOutput {
	if now == nil {
		now = time.Now
	}
	return &ToolOutput{destination: destination, tool: tool, now: now, color: color}
}

func DetectColor(destination io.Writer) ColorMode {
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled {
		return ColorModeNever
	}
	file, ok := destination.(interface{ Fd() uintptr })
	if !ok || !term.IsTerminal(file.Fd()) || strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return ColorModeNever
	}
	if strings.Contains(strings.ToLower(os.Getenv("TERM")), "256color") || os.Getenv("COLORTERM") != "" {
		return ColorMode256
	}
	return ColorModeBasic
}

func (output *ToolOutput) Info(message string) error {
	return output.write("INFO", message, "")
}

func (output *ToolOutput) Prompt(message string) error {
	return output.write("PROMPT", message, "36")
}

func (output *ToolOutput) Menu(question string, options []string) error {
	if err := output.Prompt(question); err != nil {
		return err
	}
	for _, option := range options {
		line := "  " + option
		if output.color != ColorModeNever {
			line = "\x1b[36m" + line + "\x1b[0m"
		}
		if _, err := fmt.Fprintln(output.destination, line); err != nil {
			return err
		}
	}
	return nil
}

func (output *ToolOutput) Warn(message string) error {
	color := "33"
	if output.color == ColorMode256 {
		color = "38;5;208"
	}
	return output.write("WARN", message, color)
}

func (output *ToolOutput) Error(message string) error {
	return output.write("ERROR", message, "31")
}

func (output *ToolOutput) Success(message string) error {
	return output.write("SUCCESS", message, "32")
}

func (output *ToolOutput) External() io.Writer {
	return output.destination
}

func (output *ToolOutput) write(level, message, color string) error {
	if !output.started {
		if _, err := io.WriteString(output.destination, "\n"); err != nil {
			return err
		}
		output.started = true
	}
	message = strings.TrimSuffix(message, "\n")
	for _, line := range strings.Split(message, "\n") {
		prefix := fmt.Sprintf("[%s %s %s] ", output.now().Format("15:04:05"), output.tool, level)
		rendered := prefix + line
		if output.color != ColorModeNever && color != "" {
			rendered = "\x1b[" + color + "m" + rendered + "\x1b[0m"
		}
		if _, err := fmt.Fprintln(output.destination, rendered); err != nil {
			return err
		}
	}
	return nil
}
