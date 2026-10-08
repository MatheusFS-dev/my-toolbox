package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const stressGPUOwnership = "{\"owner\":\"my-toolbox-stress-gpu\",\"schema\":1}\n"
const stressGPUVersionCondition = "(3, 10) <= sys.version_info[:2] < (3, 15)"

// stressGPUPaths resolves independent user-owned runtime and launcher locations.
// Args: platform (string): Toolbox platform identifier.
// Returns: string: State root. string: Wrapper path. error: Home lookup failure.
// Raises: None.
func stressGPUPaths(platform string) (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	state := filepath.Join(home, ".stress-gpu")
	wrapper := filepath.Join(home, ".local", "bin", "stress-gpu")
	if platform == "windows-amd64" {
		local := os.Getenv("LOCALAPPDATA")
		if local == "" {
			local = filepath.Join(home, "AppData", "Local")
		}
		wrapper = filepath.Join(local, "my-toolbox", "bin", "stress-gpu.cmd")
	}
	return state, wrapper, nil
}

// stressGPUWrapper creates a stable launcher independent of installed tb versions.
// Args: platform (string): Toolbox platform identifier.
// Returns: string: Wrapper source.
// Raises: None.
func stressGPUWrapper(platform string) string {
	if platform == "windows-amd64" {
		return "@echo off\r\nrem my-toolbox-stress-gpu wrapper v2\r\nsetlocal EnableExtensions DisableDelayedExpansion\r\nif \"%~1\"==\"--uninstall\" if \"%~2\"==\"\" (set \"TOOLBOX_ROOT=%LOCALAPPDATA%\\my-toolbox\" & setlocal EnableDelayedExpansion & set /p toolbox_version=<\"!TOOLBOX_ROOT!\\current.txt\" & \"!TOOLBOX_ROOT!\\versions\\!toolbox_version!\\tb.exe\" uninstall-stress-gpu & exit /b !errorlevel!)\r\nset \"STRESS_STATE=%USERPROFILE%\\.stress-gpu\"\r\nsetlocal EnableDelayedExpansion\r\nset \"generation=\"\r\nset /p generation=<\"!STRESS_STATE!\\current.txt\"\r\necho(!generation!|findstr /r /x \"runtime-[0-9][0-9]*\" >nul\r\nif errorlevel 1 (echo Invalid stress-gpu runtime. Run tb install-stress-gpu. >&2 & exit /b 1)\r\nset \"PYTHONPATH=!STRESS_STATE!\\!generation!\\app\"\r\nset \"PYTHONNOUSERSITE=1\"\r\n\"!STRESS_STATE!\\!generation!\\venv\\Scripts\\python.exe\" -s -m stress_gpu_runtime %*\r\nexit /b !errorlevel!\r\n"
	}
	return "#!/bin/sh\n# my-toolbox-stress-gpu wrapper v2\nif [ \"$#\" -eq 1 ] && [ \"$1\" = --uninstall ]; then\n  data_root=\"${XDG_DATA_HOME:-$HOME/.local/share}/my-toolbox\"\n  IFS= read -r toolbox_version < \"$data_root/current.txt\"\n  exec \"$data_root/versions/$toolbox_version/tb\" uninstall-stress-gpu\nfi\nstate=\"$HOME/.stress-gpu\"\ngeneration=$(cat \"$state/current.txt\") || exit 1\ncase \"$generation\" in runtime-*) ;; *) echo 'Invalid stress-gpu runtime. Run tb install-stress-gpu.' >&2; exit 1 ;; esac\ncase \"$generation\" in *[!a-zA-Z0-9-]*) echo 'Invalid stress-gpu runtime.' >&2; exit 1 ;; esac\nexport PYTHONPATH=\"$state/$generation/app\"\nexport PYTHONNOUSERSITE=1\nexec \"$state/$generation/venv/bin/python\" -s -m stress_gpu_runtime \"$@\"\n"
}

func legacyStressGPUWrapper(platform string) string {
	if platform == "windows-amd64" {
		return "@echo off\r\nrem my-toolbox-stress-gpu wrapper v1\r\nsetlocal EnableExtensions DisableDelayedExpansion\r\nset \"STRESS_STATE=%USERPROFILE%\\.stress-gpu\"\r\nsetlocal EnableDelayedExpansion\r\nset \"generation=\"\r\nset /p generation=<\"!STRESS_STATE!\\current.txt\"\r\necho(!generation!|findstr /r /x \"runtime-[0-9][0-9]*\" >nul\r\nif errorlevel 1 (echo Invalid stress-gpu runtime. Run tb install-stress-gpu. >&2 & exit /b 1)\r\nset \"PYTHONPATH=!STRESS_STATE!\\!generation!\\app\"\r\nset \"PYTHONNOUSERSITE=1\"\r\n\"!STRESS_STATE!\\!generation!\\venv\\Scripts\\python.exe\" -s -m stress_gpu_runtime %*\r\nexit /b !errorlevel!\r\n"
	}
	return "#!/bin/sh\n# my-toolbox-stress-gpu wrapper v1\nstate=\"$HOME/.stress-gpu\"\ngeneration=$(cat \"$state/current.txt\") || exit 1\ncase \"$generation\" in runtime-*) ;; *) echo 'Invalid stress-gpu runtime. Run tb install-stress-gpu.' >&2; exit 1 ;; esac\ncase \"$generation\" in *[!a-zA-Z0-9-]*) echo 'Invalid stress-gpu runtime.' >&2; exit 1 ;; esac\nexport PYTHONPATH=\"$state/$generation/app\"\nexport PYTHONNOUSERSITE=1\nexec \"$state/$generation/venv/bin/python\" -s -m stress_gpu_runtime \"$@\"\n"
}

func isOwnedStressGPUWrapper(content []byte, platform string) bool {
	return string(content) == stressGPUWrapper(platform) || string(content) == legacyStressGPUWrapper(platform)
}

// stressGPUPython selects a supported system Python without installing it.
// Args: platform (string): Toolbox platform identifier.
// Returns: []string: Executable and optional launcher prefix. error: Unsupported Python.
// Raises: None.
func stressGPUPython(platform string) ([]string, error) {
	candidates := [][]string{{"python3"}}
	if platform == "windows-amd64" {
		candidates = [][]string{{"py", "-3"}, {"python"}, {"python3"}}
	}
	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate[0]); err == nil && supportsPythonVersion(path, candidate[1:], stressGPUVersionCondition) {
			return append([]string{path}, candidate[1:]...), nil
		}
	}
	return nil, fmt.Errorf("stress-gpu requires Python 3.10 through 3.14 with venv support")
}

// runStressGPUCommand runs a noninteractive dependency or validation command.
// Args: path (string): Interpreter. arguments ([]string): Arguments. output (io.Writer): Status stream.
// Returns: error: Process failure.
// Raises: None.
func runStressGPUCommand(path string, arguments []string, output io.Writer) error {
	return runStressGPUCommandContext(context.Background(), path, arguments, output)
}

// runStressGPUCommandContext cancels and waits for detached dependency processes.
// Args: ctx (context.Context): Cancellation source. path (string): Executable. arguments ([]string): Arguments. output (io.Writer): Output.
// Returns: error: Cancellation or process failure.
// Raises: None.
func runStressGPUCommandContext(ctx context.Context, path string, arguments []string, output io.Writer) error {
	command := exec.CommandContext(ctx, path, arguments...)
	configureNonInteractive(command)
	command.Stdout, command.Stderr = output, output
	command.Cancel = func() error { return cancelStressGPUCommand(command) }
	command.WaitDelay = 5 * time.Second
	err := command.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// installStressGPU installs or repairs the isolated standalone application.
// Args: None.
// Returns: error: Python, ownership, dependency, or activation failure.
// Raises: None.
func (builtins *ToolboxBuiltins) installStressGPU() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	python, err := stressGPUPython(builtins.platform)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	run := func(path string, arguments []string, output io.Writer) error {
		return runStressGPUCommandContext(ctx, path, arguments, output)
	}
	rename := func(source, destination string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return os.Rename(source, destination)
	}
	return builtins.installStressGPUWith(python, run, rename)
}

// installStressGPUWith stages and activates a validated runtime generation.
// Args: python ([]string): Interpreter and prefixes. run (func): Process runner. rename (func): Atomic publication callback.
// Returns: error: Ownership, installation, validation or publication failure.
// Raises: None.
// Notes: Generation directories are never relocated, preserving venv absolute paths.
func (builtins *ToolboxBuiltins) installStressGPUWith(python []string, run func(string, []string, io.Writer) error, rename func(string, string) error) error {
	state, wrapper, err := stressGPUPaths(builtins.platform)
	if err != nil {
		return err
	}
	expected := []byte(stressGPUWrapper(builtins.platform))
	if info, statErr := os.Lstat(wrapper); statErr == nil {
		content, readErr := os.ReadFile(wrapper)
		if !info.Mode().IsRegular() || readErr != nil || !isOwnedStressGPUWrapper(content, builtins.platform) {
			return fmt.Errorf("refusing to replace unrecognized stress-gpu wrapper %s", wrapper)
		}
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	markerPath := filepath.Join(state, "owned.json")
	if info, statErr := os.Lstat(state); statErr == nil {
		marker, readErr := os.ReadFile(markerPath)
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || readErr != nil || string(marker) != stressGPUOwnership {
			return fmt.Errorf("refusing to replace unrecognized stress-gpu state %s", state)
		}
	} else if os.IsNotExist(statErr) {
		if err := os.MkdirAll(state, 0700); err != nil {
			return err
		}
		if err := os.WriteFile(markerPath, []byte(stressGPUOwnership), 0600); err != nil {
			return err
		}
	} else {
		return statErr
	}
	if err := os.Chmod(state, 0700); err != nil {
		return err
	}
	releaseLock, err := acquireStressGPULock(filepath.Join(state, ".install-lock-file"))
	if err != nil {
		return err
	}
	defer releaseLock()
	source := filepath.Join(builtins.root, "packages", "stress_gpu_runtime")
	if !regularFile(filepath.Join(source, "requirements.txt")) || !regularFile(filepath.Join(source, "stress_gpu_runtime", "__init__.py")) {
		return fmt.Errorf("packaged stress-gpu runtime is incomplete")
	}
	currentPath := filepath.Join(state, "current.txt")
	if info, statErr := os.Lstat(currentPath); statErr == nil {
		current, readErr := os.ReadFile(currentPath)
		generation := strings.TrimSpace(string(current))
		if !info.Mode().IsRegular() || readErr != nil || !validStressGPUGeneration(generation) {
			return fmt.Errorf("unrecognized stress-gpu current-runtime pointer")
		}
		marker, markerErr := os.ReadFile(filepath.Join(state, generation, "owned.json"))
		if markerErr != nil || string(marker) != stressGPUOwnership {
			return fmt.Errorf("unrecognized active stress-gpu runtime")
		}
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	stage, err := os.MkdirTemp(state, "runtime-")
	if err != nil {
		return err
	}
	activated := false
	defer func() {
		if !activated {
			os.RemoveAll(stage)
		}
	}()
	if err := os.WriteFile(filepath.Join(stage, "owned.json"), []byte(stressGPUOwnership), 0600); err != nil {
		return err
	}
	arguments := append(append([]string{}, python[1:]...), "-m", "venv", filepath.Join(stage, "venv"))
	if err := run(python[0], arguments, builtins.output); err != nil {
		return fmt.Errorf("create stress-gpu virtual environment: %w", err)
	}
	venvPython := filepath.Join(stage, "venv", "bin", "python")
	if builtins.platform == "windows-amd64" {
		venvPython = filepath.Join(stage, "venv", "Scripts", "python.exe")
	}
	if err := run(venvPython, []string{"-m", "pip", "install", "--disable-pip-version-check", "--no-input", "--only-binary=:all:", "torch==2.10.0", "--index-url", "https://download.pytorch.org/whl/cu128"}, builtins.output); err != nil {
		return fmt.Errorf("install CUDA PyTorch: a compatible Python/platform wheel and NVIDIA driver are required: %w", err)
	}
	if err := run(venvPython, []string{"-m", "pip", "install", "--disable-pip-version-check", "--no-input", "--only-binary=:all:", "-r", filepath.Join(source, "requirements.txt")}, builtins.output); err != nil {
		return fmt.Errorf("install NVIDIA telemetry dependencies: %w", err)
	}
	if err := copyStressGPUTree(filepath.Join(source, "stress_gpu_runtime"), filepath.Join(stage, "app", "stress_gpu_runtime")); err != nil {
		return err
	}
	validation := "import sys; sys.path.insert(0, " + fmt.Sprintf("%q", filepath.Join(stage, "app")) + "); import torch, pynvml; import stress_gpu_runtime.__main__; assert torch.version.cuda is not None, 'CUDA-enabled PyTorch is required'"
	if err := run(venvPython, []string{"-s", "-c", validation}, builtins.output); err != nil {
		return fmt.Errorf("validate stress-gpu runtime: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(wrapper), 0755); err != nil {
		return err
	}
	wrapperCreated := false
	if _, err := os.Stat(wrapper); os.IsNotExist(err) {
		if err := publishStressGPUFile(wrapper, expected, 0755, rename); err != nil {
			return fmt.Errorf("publish stress-gpu wrapper: %w", err)
		}
		wrapperCreated = true
	}
	if err := publishStressGPUFile(currentPath, []byte(filepath.Base(stage)+"\n"), 0600, rename); err != nil {
		if wrapperCreated {
			os.Remove(wrapper)
		}
		return fmt.Errorf("activate stress-gpu runtime: %w", err)
	}
	activated = true
	_, err = fmt.Fprintf(builtins.output, "Installed stress-gpu to %s\nRun stress-gpu to select GPUs and configure the test. Add %s to PATH if needed.\nCUDA 12.8 compatibility is required. NVIDIA drivers and GPU availability are checked at application launch.\n", wrapper, filepath.Dir(wrapper))
	return err
}

// validStressGPUGeneration accepts only generated directory basenames.
// Args: generation (string): Current-runtime pointer.
// Returns: bool: Whether the basename is safe and canonical.
// Raises: None.
func validStressGPUGeneration(generation string) bool {
	if !strings.HasPrefix(generation, "runtime-") || len(generation) <= len("runtime-") {
		return false
	}
	for _, character := range strings.TrimPrefix(generation, "runtime-") {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func stressGPUInstalled(platform string) (bool, error) {
	state, _, err := stressGPUPaths(platform)
	if err != nil {
		return false, err
	}
	marker, err := os.ReadFile(filepath.Join(state, "owned.json"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return string(marker) == stressGPUOwnership, nil
}

func (builtins *ToolboxBuiltins) uninstallStressGPU() error {
	state, wrapper, err := stressGPUPaths(builtins.platform)
	if err != nil {
		return err
	}
	if content, readErr := os.ReadFile(wrapper); readErr == nil {
		if isOwnedStressGPUWrapper(content, builtins.platform) {
			if err := os.Remove(wrapper); err != nil {
				return fmt.Errorf("remove stress-gpu wrapper: %w", err)
			}
		}
	} else if !os.IsNotExist(readErr) {
		return readErr
	}
	installed, err := stressGPUInstalled(builtins.platform)
	if err != nil {
		return err
	}
	if installed {
		if err := os.RemoveAll(state); err != nil {
			return fmt.Errorf("remove stress-gpu runtime: %w", err)
		}
	}
	_, err = fmt.Fprintln(builtins.output, "Removed stress-gpu runtime and launcher.")
	return err
}

// publishStressGPUFile atomically writes a private pointer or launcher.
// Args: path (string): Destination. content ([]byte): Source. mode (os.FileMode): Permissions. rename (func): Publication callback.
// Returns: error: File creation or publication failure.
// Raises: None.
func publishStressGPUFile(path string, content []byte, mode os.FileMode, rename func(string, string) error) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".stress-gpu-publish-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(content); err != nil {
		file.Close()
		return err
	}
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return rename(file.Name(), path)
}

// copyStressGPUTree copies regular runtime sources without generated bytecode.
// Args: source (string): Packaged source. destination (string): Isolated runtime directory.
// Returns: error: Unsupported entry or I/O failure.
// Raises: None.
func copyStressGPUTree(source, destination string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() && info.Name() == "__pycache__" {
			return filepath.SkipDir
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if info.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("stress-gpu source contains unsupported entry %s", path)
		}
		if strings.HasSuffix(path, ".pyc") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, 0600)
	})
}

// openStressGPULock refuses non-regular entries before opening a private lock file.
// Args: path (string): Lock file path.
// Returns: *os.File: Open lock file. error: Ownership or I/O failure.
// Raises: None.
func openStressGPULock(path string) (*os.File, error) {
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("unrecognized stress-gpu lock file %s", path)
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
}
