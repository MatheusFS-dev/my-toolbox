package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// BuiltinExecutor owns toolbox-provided installers and updates.
type BuiltinExecutor interface {
	SkipReason(name string) (string, error)
	Run(name string, arguments []string) error
}

const defaultArgumentsQuestionID = "use-default-arguments"

// ProcessExecutor invokes questionnaire adapters and interactive Python scripts.
type ProcessExecutor struct {
	Root     string
	Platform string
	Builtins BuiltinExecutor
	Input    io.Reader
	Output   io.Writer
	Error    io.Writer
}

// Questions discovers the next configuration question or builtin preflight skip.
//
// Args:
//   - command: Catalog command being configured.
//   - answers: Answers accumulated from earlier typed questions.
//   - arguments: Direct-command arguments forwarded to adapters and rejected by
//     interactive scripts.
//
// Returns:
//   - ProtocolResponse: Question, ready, or skipped state.
//   - error: Argument, entrypoint, interpreter, adapter, or protocol failure.
func (executor ProcessExecutor) Questions(command Command, answers map[string]any, arguments []string) (ProtocolResponse, error) {
	if command.Protocol == "builtin" {
		reason, err := executor.Builtins.SkipReason(command.Name)
		if err != nil {
			return ProtocolResponse{}, err
		}
		if reason != "" {
			return ProtocolResponse{Status: "skipped", Reason: reason}, nil
		}
		return ProtocolResponse{Status: "ready"}, nil
	}
	if command.Protocol == "interactive-python" {
		if len(arguments) > 0 {
			return ProtocolResponse{}, fmt.Errorf("command %s does not accept arguments", command.Name)
		}
		if len(command.DefaultArguments) == 0 {
			return ProtocolResponse{Status: "ready"}, nil
		}
		answer, exists := answers[defaultArgumentsQuestionID]
		if !exists {
			return ProtocolResponse{
				Status: "question",
				Question: &Question{
					ID:    defaultArgumentsQuestionID,
					Type:  "confirm",
					Title: fmt.Sprintf("Use all default values for %s?", command.Name),
				},
			}, nil
		}
		if _, valid := answer.(bool); !valid {
			return ProtocolResponse{}, fmt.Errorf("default-mode confirmation for %s must be boolean", command.Name)
		}
		return ProtocolResponse{Status: "ready"}, nil
	}
	return executor.invokeAdapter(command, "questions", answers, arguments)
}

// Run executes one fully configured builtin, adapter, or interactive command.
//
// Args:
//   - command: Catalog command to execute.
//   - answers: Complete validated questionnaire answers.
//   - arguments: Direct-command arguments.
//
// Returns:
//   - error: Argument, execution, or unexpected adapter-state failure.
func (executor ProcessExecutor) Run(command Command, answers map[string]any, arguments []string) error {
	if command.Protocol == "builtin" || command.Name == "update" {
		return executor.Builtins.Run(command.Name, arguments)
	}
	if command.Protocol == "interactive-python" {
		if len(arguments) > 0 {
			return fmt.Errorf("command %s does not accept arguments", command.Name)
		}
		scriptArguments := []string(nil)
		if len(command.DefaultArguments) > 0 {
			answer, exists := answers[defaultArgumentsQuestionID]
			if !exists {
				return fmt.Errorf("default-mode confirmation for %s is missing", command.Name)
			}
			confirmed, valid := answer.(bool)
			if !valid {
				return fmt.Errorf("default-mode confirmation for %s must be boolean", command.Name)
			}
			if confirmed {
				scriptArguments = command.DefaultArguments
			}
		}
		return executor.runInteractivePython(command, scriptArguments)
	}
	response, err := executor.invokeAdapter(command, "run", answers, arguments)
	if err != nil {
		return err
	}
	if response.Status != "ready" {
		return fmt.Errorf("adapter returned unexpected %q state during run", response.Status)
	}
	return nil
}

// runInteractivePython runs an installer with direct terminal stream access.
//
// Args:
//   - command: Interactive Python catalog command with an entrypoint for the
//     executor platform.
//   - arguments: Catalog-declared default-mode arguments, or an empty slice for
//     the script's normal interactive mode.
//
// Returns:
//   - error: Entrypoint, interpreter, terminal stream, or process failure.
//
// Raises:
//   - None.
func (executor ProcessExecutor) runInteractivePython(command Command, arguments []string) error {
	entrypoint, exists := command.Entrypoints[executor.Platform]
	if !exists || len(entrypoint) < 2 || entrypoint[0] != "python-script" {
		return fmt.Errorf("command %s has no interactive Python script for %s", command.Name, executor.Platform)
	}
	if executor.Input == nil || executor.Output == nil || executor.Error == nil {
		return fmt.Errorf("interactive command %s requires input, output, and error streams", command.Name)
	}
	interpreter, scriptIndex, err := selectPython(executor.Platform, entrypoint)
	if err != nil {
		return err
	}
	script := filepath.Join(executor.Root, filepath.FromSlash(entrypoint[scriptIndex]))
	processArguments := append([]string(nil), interpreter[1:]...)
	processArguments = append(processArguments, script)
	processArguments = append(processArguments, arguments...)
	process := exec.Command(interpreter[0], processArguments...)
	process.Stdin = executor.Input
	process.Stdout = executor.Output
	process.Stderr = executor.Error
	if err := process.Run(); err != nil {
		return fmt.Errorf("interactive script failed: %w", err)
	}
	return nil
}

func (executor ProcessExecutor) invokeAdapter(command Command, operation string, answers map[string]any, arguments []string) (ProtocolResponse, error) {
	entrypoint, exists := command.Entrypoints[executor.Platform]
	if !exists || len(entrypoint) < 2 || entrypoint[0] != "python-adapter" {
		return ProtocolResponse{}, fmt.Errorf("command %s has no Python adapter for %s", command.Name, executor.Platform)
	}
	interpreter, scriptIndex, err := selectPython(executor.Platform, entrypoint)
	if err != nil {
		return ProtocolResponse{}, err
	}
	script := filepath.Join(executor.Root, filepath.FromSlash(entrypoint[scriptIndex]))
	protocolArguments := arguments
	if protocolArguments == nil {
		protocolArguments = []string{}
	}
	request := ProtocolRequest{
		Operation: operation,
		Package:   PackageContext{Name: command.Package, Command: command.Name},
		Answers:   answers,
		Arguments: protocolArguments,
	}
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return ProtocolResponse{}, fmt.Errorf("encode adapter request: %w", err)
	}
	process := exec.Command(interpreter[0], append(interpreter[1:], script)...)
	configureNonInteractive(process)
	process.Stdin = bytes.NewReader(requestBytes)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	process.Stdout = &stdout
	process.Stderr = &stderr
	if err := process.Run(); err != nil {
		return ProtocolResponse{}, fmt.Errorf("adapter failed: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	response, err := DecodeProtocolResponse(&stdout)
	if err != nil {
		return ProtocolResponse{}, err
	}
	if operation == "run" && stderr.Len() > 0 && executor.Output != nil {
		_, _ = io.Copy(executor.Output, &stderr)
	}
	return response, nil
}

func selectPython(platform string, entrypoint []string) ([]string, int, error) {
	if strings.HasPrefix(platform, "linux-") {
		if path, err := exec.LookPath("python3"); err == nil {
			return []string{path}, 1, nil
		}
		if path, err := exec.LookPath("python2.7"); err == nil {
			if len(entrypoint) < 3 {
				return nil, 0, fmt.Errorf("command has no Python 2.7 fallback script")
			}
			return []string{path}, 2, nil
		}
		return nil, 0, fmt.Errorf("no supported interpreter found; install Python 3 or Python 2.7")
	}
	if platform == "windows-amd64" {
		if path, err := exec.LookPath("py"); err == nil {
			return []string{path, "-3"}, 1, nil
		}
		if path, err := exec.LookPath("python"); err == nil {
			return []string{path}, 1, nil
		}
		return nil, 0, fmt.Errorf("no supported interpreter found; install Python 3")
	}
	return nil, 0, fmt.Errorf("unsupported platform %q", platform)
}

// CurrentPlatform returns the catalog key for the running OS and architecture.
//
// Args: None.
//
// Returns:
//   - string: linux-amd64, linux-arm64, or windows-amd64.
//   - error: Unsupported OS or architecture.
func CurrentPlatform() (string, error) {
	key := runtime.GOOS + "-" + runtime.GOARCH
	for _, supported := range supportedPlatforms {
		if key == supported {
			return key, nil
		}
	}
	return "", fmt.Errorf("unsupported platform %s", key)
}

func executableRoot() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve toolbox executable: %w", err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve toolbox executable links: %w", err)
	}
	return filepath.Dir(path), nil
}
