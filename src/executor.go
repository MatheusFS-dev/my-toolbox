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
	Root        string
	Platform    string
	Environment string
	Builtins    BuiltinExecutor
	Input       io.Reader
	Output      io.Writer
	Error       io.Writer
}

// Preflight validates every hard requirement without changing tool state.
func (executor ProcessExecutor) Preflight(command Command) error {
	capabilities, err := ResolveRequirements(command, executor.Environment)
	if err != nil {
		return err
	}
	failures := []Capability{}
	for _, capability := range capabilities {
		if !executor.supportsCapability(capability.ID) {
			failures = append(failures, capability)
		}
	}
	if len(failures) == 0 {
		return nil
	}
	var message strings.Builder
	message.WriteString("missing requirements:")
	for _, failure := range failures {
		fmt.Fprintf(&message, "\n- %s\n  %s", failure.Label, failure.Remediation)
	}
	return fmt.Errorf("%s", message.String())
}

func (executor ProcessExecutor) supportsCapability(id string) bool {
	switch id {
	case "python-workspace-linux":
		if path, err := exec.LookPath("python3"); err == nil && supportsPythonVersion(path, nil, "(3, 11) <= sys.version_info[:2]") {
			return true
		}
		if path, err := exec.LookPath("python2.7"); err == nil && supportsPythonVersion(path, nil, "sys.version_info[:2] == (2, 7)") {
			return supportsPythonVersion(path, nil, `getattr(__import__("toml"), "__version__", "") == "0.10.2"`)
		}
		return false
	case "python311":
		if executor.Environment == "windows" {
			if path, err := exec.LookPath("py"); err == nil && supportsPythonVersion(path, []string{"-3"}, "(3, 11) <= sys.version_info[:2]") {
				return true
			}
		}
		path, err := exec.LookPath("python3")
		if executor.Environment == "windows" {
			path, err = exec.LookPath("python")
		}
		return err == nil && supportsPythonVersion(path, nil, "(3, 11) <= sys.version_info[:2]")
	case "python3":
		path, err := exec.LookPath("python3")
		return err == nil && supportsPythonVersion(path, nil, "sys.version_info[:2] >= (3, 0)")
	case "codex-plugin-management":
		return executor.supportsPluginManagement("codex")
	case "claude-plugin-management":
		return executor.supportsPluginManagement("claude")
	case "antigravity-plugin-management":
		return executor.supportsPluginManagement("agy")
	case "debian-ubuntu":
		content, err := os.ReadFile("/etc/os-release")
		return err == nil && osReleaseIs(content, "debian", "ubuntu")
	case "wsl-ubuntu-supported":
		content, err := os.ReadFile("/etc/os-release")
		return err == nil && osReleaseVersionIs(content, "ubuntu", "22.04", "24.04")
	case "windows-build-supported":
		path, exists := supportedPowerShellPath()
		return exists && runCapabilityPath(path, "-NoProfile", "-NonInteractive", "-Command", "if ([Environment]::OSVersion.Version.Build -ge 17763) { exit 0 } else { exit 1 }")
	case "wsl":
		return runCapabilityCommand("wsl.exe", "-l", "-q")
	case "vscode-wsl":
		path, err := exec.LookPath("code")
		if err != nil {
			path, err = exec.LookPath("code.exe")
		}
		if err != nil {
			return false
		}
		output, err := exec.Command(path, "--list-extensions").Output()
		return err == nil && strings.Contains(strings.ToLower(string(output)), "ms-vscode-remote.remote-wsl")
	case "grub-files":
		return regularFile("/etc/default/grub") && regularFile("/boot/grub/grub.cfg")
	case "grub-utilities":
		_, err := exec.LookPath("update-grub")
		return err == nil
	case "powershell":
		_, exists := supportedPowerShellPath()
		return exists
	default:
		_, err := exec.LookPath(id)
		return err == nil
	}
}

func (executor ProcessExecutor) supportsPluginManagement(name string) bool {
	path, err := resolveUserCommand(name, executor.Platform)
	if err != nil {
		return false
	}
	command := exec.Command(path, "plugin", "list")
	configureNonInteractive(command)
	return command.Run() == nil
}

func runCapabilityCommand(name string, arguments ...string) bool {
	path, err := exec.LookPath(name)
	if err != nil {
		return false
	}
	return runCapabilityPath(path, arguments...)
}

func runCapabilityPath(path string, arguments ...string) bool {
	command := exec.Command(path, arguments...)
	configureNonInteractive(command)
	return command.Run() == nil
}

func supportedPowerShellPath() (string, bool) {
	for _, name := range []string{"powershell.exe", "powershell", "pwsh"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if runCapabilityPath(path, "-NoProfile", "-NonInteractive", "-Command", "if ($PSVersionTable.PSVersion -ge [Version]'5.1') { exit 0 } else { exit 1 }") {
			return path, true
		}
	}
	return "", false
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func osReleaseIs(content []byte, identifiers ...string) bool {
	return osReleaseVersionIs(content, "", identifiers...)
}

func osReleaseVersionIs(content []byte, identifier string, versions ...string) bool {
	values := map[string]string{}
	for _, line := range strings.Split(string(content), "\n") {
		key, value, found := strings.Cut(line, "=")
		if found {
			values[key] = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	if identifier == "" {
		for _, candidate := range versions {
			if values["ID"] == candidate {
				return true
			}
		}
		return false
	}
	if values["ID"] != identifier {
		return false
	}
	for _, version := range versions {
		if values["VERSION_ID"] == version {
			return true
		}
	}
	return false
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
	if command.Protocol == "interactive-script" {
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
	if command.Protocol == "interactive-script" {
		return executor.runInteractiveScript(command, arguments)
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

// runInteractiveScript runs a Bash or PowerShell script with terminal streams attached.
//
// Args:
//   - command: Interactive script command with a current-platform entrypoint. Bash
//     commands may request sudo elevation; PowerShell commands may not.
//   - arguments: Direct arguments forwarded unchanged after the script path.
//
// Returns:
//   - error: Entrypoint, stream, executable lookup, or process failure.
func (executor ProcessExecutor) runInteractiveScript(command Command, arguments []string) error {
	entrypoint, exists := command.Entrypoints[executor.Platform]
	if !exists || len(entrypoint) < 2 {
		return fmt.Errorf("command %s has no interactive script for %s", command.Name, executor.Platform)
	}
	if executor.Input == nil || executor.Output == nil || executor.Error == nil {
		return fmt.Errorf("interactive command %s requires input, output, and error streams", command.Name)
	}
	script := filepath.Join(executor.Root, filepath.FromSlash(entrypoint[1]))
	var process *exec.Cmd
	switch entrypoint[0] {
	case "bash-script":
		processArguments := []string{script}
		processArguments = append(processArguments, arguments...)
		if command.Elevation == "sudo" {
			processArguments = append([]string{"--", "bash"}, processArguments...)
			process = exec.Command("sudo", processArguments...)
		} else {
			process = exec.Command("bash", processArguments...)
		}
	case "powershell-script":
		powershell, exists := supportedPowerShellPath()
		if !exists {
			return fmt.Errorf("command %s requires Windows PowerShell 5.1 or PowerShell 7", command.Name)
		}
		processArguments := []string{"-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script}
		processArguments = append(processArguments, arguments...)
		process = exec.Command(powershell, processArguments...)
	default:
		return fmt.Errorf("command %s has unsupported script entrypoint type %q", command.Name, entrypoint[0])
	}
	process.Stdin = executor.Input
	process.Stdout = executor.Output
	process.Stderr = executor.Error
	if err := process.Run(); err != nil {
		return fmt.Errorf("interactive script failed: %w", err)
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
		if path, err := exec.LookPath("python3"); err == nil && supportsPythonVersion(path, nil, "(3, 11) <= sys.version_info[:2]") {
			return []string{path}, 1, nil
		}
		if path, err := exec.LookPath("python2.7"); err == nil && supportsPythonVersion(path, nil, "sys.version_info[:2] == (2, 7)") {
			if !supportsPythonVersion(path, nil, `getattr(__import__("toml"), "__version__", "") == "0.10.2"`) {
				return nil, 0, fmt.Errorf("Python 2.7 requires toml==0.10.2; run: python2.7 -m pip install --user toml==0.10.2")
			}
			if len(entrypoint) < 3 {
				return nil, 0, fmt.Errorf("command has no Python 2.7 fallback script")
			}
			return []string{path}, 2, nil
		}
		return nil, 0, fmt.Errorf("no supported interpreter found; install Python 3.11 or newer, or Python 2.7")
	}
	if platform == "windows-amd64" {
		if path, err := exec.LookPath("py"); err == nil && supportsPythonVersion(path, []string{"-3"}, "(3, 11) <= sys.version_info[:2]") {
			return []string{path, "-3"}, 1, nil
		}
		if path, err := exec.LookPath("python"); err == nil && supportsPythonVersion(path, nil, "(3, 11) <= sys.version_info[:2]") {
			return []string{path}, 1, nil
		}
		return nil, 0, fmt.Errorf("no supported interpreter found; install Python 3.11 or newer")
	}
	return nil, 0, fmt.Errorf("unsupported platform %q", platform)
}

func supportsPythonVersion(path string, prefixArguments []string, condition string) bool {
	arguments := append([]string(nil), prefixArguments...)
	arguments = append(arguments, "-c", "import sys; sys.exit(0 if "+condition+" else 1)")
	command := exec.Command(path, arguments...)
	configureNonInteractive(command)
	return command.Run() == nil
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

// CurrentEnvironment detects the supported operating environment.
//
// Args: None.
//
// Returns:
//   - string: linux-native, linux-wsl, or windows.
//   - error: Unsupported operating system or unreadable Linux kernel metadata.
func CurrentEnvironment() (string, error) {
	return detectEnvironment(runtime.GOOS, "/proc/sys/kernel/osrelease")
}

// detectEnvironment detects WSL from Linux kernel release metadata.
//
// Args:
//   - goos: Go operating-system identifier. Accepted values are linux and windows.
//   - osreleasePath: Linux kernel osrelease file. It is read only when goos is linux.
//
// Returns:
//   - string: linux-native, linux-wsl, or windows.
//   - error: Unsupported operating system or an osrelease read failure.
func detectEnvironment(goos, osreleasePath string) (string, error) {
	switch goos {
	case "windows":
		return "windows", nil
	case "linux":
		content, err := os.ReadFile(osreleasePath)
		if err != nil {
			return "", fmt.Errorf("detect Linux environment from %s: %w", osreleasePath, err)
		}
		if strings.Contains(strings.ToLower(string(content)), "microsoft") {
			return "linux-wsl", nil
		}
		return "linux-native", nil
	default:
		return "", fmt.Errorf("unsupported operating system %s", goos)
	}
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
