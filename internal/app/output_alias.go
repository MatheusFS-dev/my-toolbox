package app

import (
	"io"
	"time"

	"github.com/MatheusFS-dev/my-toolbox/internal/output"
)

type toolColorMode = output.ColorMode

const (
	colorModeNever = output.ColorModeNever
	colorModeBasic = output.ColorModeBasic
	colorMode256   = output.ColorMode256
)

type toolOutput = output.ToolOutput

func newToolOutput(destination io.Writer, tool string, now func() time.Time, color toolColorMode) *toolOutput {
	return output.New(destination, tool, now, color)
}
func detectToolColor(destination io.Writer) toolColorMode { return output.DetectColor(destination) }
