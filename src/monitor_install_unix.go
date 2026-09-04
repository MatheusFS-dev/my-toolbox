//go:build !windows

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const monitorRuntimeMarker = "{\"owner\":\"my-toolbox\",\"schema_version\":1}\n"
const monitorWrapperMarker = "# my-toolbox monitor wrapper v1\n"

func monitorWrapper(_ string) string {
	return `#!/bin/sh
# my-toolbox monitor wrapper v1
set -eu
data_root="${XDG_DATA_HOME:-$HOME/.local/share}/my-toolbox"
IFS= read -r current < "$data_root/current.txt"
exec "$data_root/versions/$current/tb" __monitor "$@"
`
}

func isOwnedMonitorWrapperContent(content []byte) bool {
	normalized := bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	return strings.HasPrefix(string(normalized), "#!/bin/sh\n"+monitorWrapperMarker) && string(normalized) == monitorWrapper("")
}

func monitorPaths() (stateRoot, runtimeRoot, wrapper string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", "", fmt.Errorf("resolve user home: %w", err)
	}
	stateRoot = filepath.Join(home, ".monitor")
	return stateRoot, filepath.Join(stateRoot, "runtime"), filepath.Join(home, ".local", "bin", "monitor"), nil
}

func (builtins *ToolboxBuiltins) installMonitor() error {
	if !strings.HasPrefix(builtins.platform, "linux-") {
		return fmt.Errorf("Monitor supports Linux and WSL only")
	}
	stateRoot, runtimeRoot, wrapper, err := monitorPaths()
	if err != nil {
		return err
	}
	if content, readErr := os.ReadFile(wrapper); readErr == nil && !isOwnedMonitorWrapperContent(content) {
		return fmt.Errorf("refusing to replace unrecognized monitor wrapper %s", wrapper)
	} else if readErr != nil && !os.IsNotExist(readErr) {
		return fmt.Errorf("inspect monitor wrapper: %w", readErr)
	}
	sourceRoot := filepath.Join(builtins.root, "packages", "monitor_runtime")
	if !regularFile(filepath.Join(sourceRoot, "requirements.txt")) || !regularFile(filepath.Join(sourceRoot, "monitor_runtime", "__init__.py")) {
		return fmt.Errorf("packaged Monitor runtime is incomplete")
	}
	python, err := exec.LookPath("python3")
	if err != nil || !supportsPythonVersion(python, nil, "sys.version_info[:2] >= (3, 9)") {
		return fmt.Errorf("Monitor requires Python 3.9 or newer")
	}
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		return fmt.Errorf("create Monitor state directory: %w", err)
	}
	if err := os.Chmod(stateRoot, 0o700); err != nil {
		return fmt.Errorf("secure Monitor state directory: %w", err)
	}
	stage, err := os.MkdirTemp(stateRoot, ".runtime-stage-")
	if err != nil {
		return fmt.Errorf("stage Monitor runtime: %w", err)
	}
	defer os.RemoveAll(stage)
	if err := runInstallerCommand(python, []string{"-m", "venv", filepath.Join(stage, "venv")}, builtins.output); err != nil {
		return fmt.Errorf("create Monitor supervisor environment: %w", err)
	}
	pip := filepath.Join(stage, "venv", "bin", "python")
	if err := runInstallerCommand(pip, []string{"-m", "pip", "install", "--disable-pip-version-check", "--no-input", "-r", filepath.Join(sourceRoot, "requirements.txt")}, builtins.output); err != nil {
		return fmt.Errorf("install Monitor runtime dependencies: %w", err)
	}
	if err := copyMonitorTree(filepath.Join(sourceRoot, "monitor_runtime"), filepath.Join(stage, "app", "monitor_runtime")); err != nil {
		return err
	}
	requirements, err := os.ReadFile(filepath.Join(sourceRoot, "requirements.txt"))
	if err != nil {
		return fmt.Errorf("read Monitor dependency manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stage, "requirements.txt"), requirements, 0o600); err != nil {
		return fmt.Errorf("write Monitor dependency manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stage, "owned.json"), []byte(monitorRuntimeMarker), 0o600); err != nil {
		return fmt.Errorf("write Monitor ownership marker: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(wrapper), 0o755); err != nil {
		return fmt.Errorf("create user binary directory: %w", err)
	}
	wrapperStage, err := os.CreateTemp(filepath.Dir(wrapper), ".monitor-wrapper-")
	if err != nil {
		return fmt.Errorf("stage Monitor wrapper: %w", err)
	}
	wrapperStagePath := wrapperStage.Name()
	defer os.Remove(wrapperStagePath)
	if _, err := io.WriteString(wrapperStage, monitorWrapper(builtins.root)); err != nil {
		wrapperStage.Close()
		return fmt.Errorf("write Monitor wrapper: %w", err)
	}
	if err := wrapperStage.Chmod(0o755); err != nil {
		wrapperStage.Close()
		return fmt.Errorf("set Monitor wrapper permissions: %w", err)
	}
	if err := wrapperStage.Close(); err != nil {
		return fmt.Errorf("close Monitor wrapper: %w", err)
	}
	backup, err := os.MkdirTemp(stateRoot, ".runtime-backup-")
	if err != nil {
		return fmt.Errorf("reserve Monitor runtime backup: %w", err)
	}
	if err := os.Remove(backup); err != nil {
		return fmt.Errorf("prepare Monitor runtime backup: %w", err)
	}
	defer os.RemoveAll(backup)
	if _, statErr := os.Stat(runtimeRoot); statErr == nil {
		marker, markerErr := os.ReadFile(filepath.Join(runtimeRoot, "owned.json"))
		if markerErr != nil || string(marker) != monitorRuntimeMarker {
			return fmt.Errorf("refusing to replace unrecognized Monitor runtime %s", runtimeRoot)
		}
		if err := os.Rename(runtimeRoot, backup); err != nil {
			return fmt.Errorf("preserve installed Monitor runtime: %w", err)
		}
	}
	if err := os.Rename(stage, runtimeRoot); err != nil {
		_ = os.Rename(backup, runtimeRoot)
		return fmt.Errorf("publish Monitor runtime: %w", err)
	}
	if err := os.Rename(wrapperStagePath, wrapper); err != nil {
		_ = os.RemoveAll(runtimeRoot)
		_ = os.Rename(backup, runtimeRoot)
		return fmt.Errorf("publish Monitor wrapper: %w", err)
	}
	_, err = fmt.Fprintf(builtins.output, "Installed monitor to %s\n", wrapper)
	return err
}

func runInstallerCommand(path string, arguments []string, output io.Writer) error {
	command := exec.Command(path, arguments...)
	command.Stdin = strings.NewReader("")
	command.Stdout = output
	command.Stderr = output
	return command.Run()
}

func copyMonitorTree(source, destination string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if info.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Monitor runtime contains unsupported file %s", path)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, content, 0o600); err != nil {
			return err
		}
		return nil
	})
}

func removeMonitor() error {
	_, runtimeRoot, wrapper, err := monitorPaths()
	if err != nil {
		return err
	}
	if content, readErr := os.ReadFile(wrapper); readErr == nil {
		if isOwnedMonitorWrapperContent(content) {
			if err := os.Remove(wrapper); err != nil {
				return fmt.Errorf("remove Monitor wrapper: %w", err)
			}
		}
	} else if !os.IsNotExist(readErr) {
		return readErr
	}
	marker, markerErr := os.ReadFile(filepath.Join(runtimeRoot, "owned.json"))
	if markerErr == nil && string(marker) == monitorRuntimeMarker {
		if err := os.RemoveAll(runtimeRoot); err != nil {
			return fmt.Errorf("remove Monitor runtime: %w", err)
		}
	}
	return nil
}
