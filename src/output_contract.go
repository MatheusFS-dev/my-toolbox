package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
)

type toolColorMode int

const (
	colorModeNever toolColorMode = iota
	colorModeBasic
	colorMode256
)

type toolOutput struct {
	destination io.Writer
	tool        string
	now         func() time.Time
	color       toolColorMode
	started     bool
}

func newToolOutput(destination io.Writer, tool string, now func() time.Time, color toolColorMode) *toolOutput {
	if now == nil {
		now = time.Now
	}
	return &toolOutput{destination: destination, tool: tool, now: now, color: color}
}

func detectToolColor(destination io.Writer) toolColorMode {
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled {
		return colorModeNever
	}
	file, ok := destination.(interface{ Fd() uintptr })
	if !ok || !term.IsTerminal(file.Fd()) || strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return colorModeNever
	}
	if strings.Contains(strings.ToLower(os.Getenv("TERM")), "256color") || os.Getenv("COLORTERM") != "" {
		return colorMode256
	}
	return colorModeBasic
}

func (output *toolOutput) Info(message string) error {
	return output.write("INFO", message, "")
}

func (output *toolOutput) Prompt(message string) error {
	return output.write("PROMPT", message, "36")
}

func (output *toolOutput) Menu(question string, options []string) error {
	if err := output.Prompt(question); err != nil {
		return err
	}
	for _, option := range options {
		line := "  " + option
		if output.color != colorModeNever {
			line = "\x1b[36m" + line + "\x1b[0m"
		}
		if _, err := fmt.Fprintln(output.destination, line); err != nil {
			return err
		}
	}
	return nil
}

func (output *toolOutput) Warn(message string) error {
	color := "33"
	if output.color == colorMode256 {
		color = "38;5;208"
	}
	return output.write("WARN", message, color)
}

func (output *toolOutput) Error(message string) error {
	return output.write("ERROR", message, "31")
}

func (output *toolOutput) Success(message string) error {
	return output.write("SUCCESS", message, "32")
}

func (output *toolOutput) External() io.Writer {
	return output.destination
}

func (output *toolOutput) write(level, message, color string) error {
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
		if output.color != colorModeNever && color != "" {
			rendered = "\x1b[" + color + "m" + rendered + "\x1b[0m"
		}
		if _, err := fmt.Fprintln(output.destination, rendered); err != nil {
			return err
		}
	}
	return nil
}
