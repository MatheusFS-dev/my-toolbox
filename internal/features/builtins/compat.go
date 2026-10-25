package builtins

import (
	"github.com/MatheusFS-dev/my-toolbox/internal/catalog"
	"github.com/MatheusFS-dev/my-toolbox/internal/execution"
	"github.com/MatheusFS-dev/my-toolbox/internal/output"
	"github.com/MatheusFS-dev/my-toolbox/internal/protocol"
	"io"
	"os/exec"
	"time"
)

type Command = catalog.Command
type Catalog = catalog.Catalog
type Question = protocol.Question

var ErrCancelled = protocol.ErrCancelled

func questionRequired(value bool) *bool { return protocol.QuestionRequired(value) }

type toolOutput = output.ToolOutput
type toolColorMode = output.ColorMode

const colorModeNever = output.ColorModeNever

func newToolOutput(destination io.Writer, tool string, now func() time.Time, color toolColorMode) *toolOutput {
	return output.New(destination, tool, now, color)
}
func detectToolColor(destination io.Writer) toolColorMode { return output.DetectColor(destination) }
func resolveUserCommand(name, platform string) (string, error) {
	return execution.ResolveUserCommand(name, platform)
}
func supportedPowerShellPath() (string, bool)      { return execution.SupportedPowerShellPath() }
func regularFile(path string) bool                 { return execution.RegularFile(path) }
func configureNonInteractive(command *exec.Cmd)    { execution.ConfigureNonInteractive(command) }
func LoadCatalogFile(path string) (Catalog, error) { return catalog.LoadCatalogFile(path) }

func stressGPUPython(platform string) ([]string, error) { return execution.StressGPUPython(platform) }
func supportsPythonVersion(path string, prefixArguments []string, condition string) bool {
	return execution.SupportsPythonVersion(path, prefixArguments, condition)
}

func ResolveRequirements(command Command, environment string) ([]catalog.Capability, error) {
	return catalog.ResolveRequirements(command, environment)
}
