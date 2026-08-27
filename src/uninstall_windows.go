//go:build windows

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
)

const windowsCleanupMode = "toolbox-windows-cleanup"

// uninstallPlatform starts a temporary Go cleanup helper for Windows.
//
// Args:
//   - dataRoot: Validated toolbox data directory containing the running binary.
//   - wrapper: Stable wrapper path inside dataRoot.
//
// Returns:
//   - string: Status file that will contain success or an error message.
//   - error: Helper-copy or process-startup failure.
func uninstallPlatform(dataRoot, wrapper string) (string, error) {
	return scheduleWindowsRemoval(dataRoot, wrapper, os.Getpid())
}

// scheduleWindowsRemoval copies tb outside its locked installation and starts it.
//
// Args:
//   - dataRoot: Validated toolbox directory containing managed runtime files.
//   - wrapper: Stable wrapper path to revalidate immediately before removal.
//   - processID: Running tb process identifier that must exit before removal.
//
// Returns:
//   - string: Status file that will contain success or an error message.
//   - error: Helper-copy or process-startup failure.
func scheduleWindowsRemoval(dataRoot, wrapper string, processID int) (string, error) {
	statusPath := filepath.Join(os.TempDir(), fmt.Sprintf("my-toolbox-uninstall-%d.status", processID))
	if err := os.Remove(statusPath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("remove stale Windows uninstall status: %w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve Windows cleanup executable: %w", err)
	}
	source, err := os.Open(executable)
	if err != nil {
		return "", fmt.Errorf("open Windows cleanup executable: %w", err)
	}
	defer source.Close()
	helper, err := os.CreateTemp("", "my-toolbox-cleanup-*.exe")
	if err != nil {
		return "", fmt.Errorf("create Windows cleanup helper: %w", err)
	}
	helperPath := helper.Name()
	removeHelper := true
	defer func() {
		if removeHelper {
			_ = os.Remove(helperPath)
		}
	}()
	if _, err := io.Copy(helper, source); err != nil {
		helper.Close()
		return "", fmt.Errorf("copy Windows cleanup helper: %w", err)
	}
	if err := helper.Close(); err != nil {
		return "", fmt.Errorf("close Windows cleanup helper: %w", err)
	}
	command := exec.Command(helperPath)
	configureNonInteractive(command)
	command.Env = append(os.Environ(),
		"TOOLBOX_INTERNAL_MODE="+windowsCleanupMode,
		"TOOLBOX_UNINSTALL_PID="+strconv.Itoa(processID),
		"TOOLBOX_UNINSTALL_ROOT="+dataRoot,
		"TOOLBOX_UNINSTALL_WRAPPER="+wrapper,
		"TOOLBOX_UNINSTALL_STATUS="+statusPath,
	)
	if err := command.Start(); err != nil {
		return "", fmt.Errorf("start Windows uninstall cleanup: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return "", fmt.Errorf("release Windows uninstall cleanup: %w", err)
	}
	removeHelper = false
	return statusPath, nil
}

// runPlatformCleanup executes the hidden cleanup mode from the temporary copy.
//
// Args: None.
//
// Returns:
//   - bool: True when the process was started as the Windows cleanup helper.
//   - error: Wait, validation, deletion, status-write, or self-removal failure.
func runPlatformCleanup() (bool, error) {
	if os.Getenv("TOOLBOX_INTERNAL_MODE") != windowsCleanupMode {
		return false, nil
	}
	statusPath := os.Getenv("TOOLBOX_UNINSTALL_STATUS")
	dataRoot := os.Getenv("TOOLBOX_UNINSTALL_ROOT")
	wrapper := os.Getenv("TOOLBOX_UNINSTALL_WRAPPER")
	processID, err := strconv.Atoi(os.Getenv("TOOLBOX_UNINSTALL_PID"))
	if err != nil || processID <= 0 {
		err = fmt.Errorf("invalid Windows uninstall process ID")
	} else if err = waitForWindowsProcess(processID); err == nil {
		err = cleanupWindowsPaths(dataRoot, wrapper)
	}
	status := "success"
	if err != nil {
		status = "error: " + err.Error()
	}
	if removeErr := scheduleWindowsHelperRemoval(); removeErr != nil && err == nil {
		err = removeErr
		status = "error: " + err.Error()
	}
	if writeErr := os.WriteFile(statusPath, []byte(status+"\n"), 0o600); writeErr != nil && err == nil {
		err = fmt.Errorf("write Windows uninstall status: %w", writeErr)
	}
	return true, err
}

// waitForWindowsProcess blocks until the original tb process exits.
//
// Args:
//   - processID: Original tb process identifier.
//
// Returns:
//   - error: Process-handle or wait failure.
func waitForWindowsProcess(processID int) error {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(processID))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open original toolbox process: %w", err)
	}
	defer windows.CloseHandle(handle)
	event, err := windows.WaitForSingleObject(handle, windows.INFINITE)
	if err != nil {
		return fmt.Errorf("wait for original toolbox process: %w", err)
	}
	if event != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("wait for original toolbox process returned event %d", event)
	}
	return nil
}

// cleanupWindowsPaths removes managed files without following reparse points.
//
// Args:
//   - dataRoot: Candidate toolbox data root from the cleanup environment.
//   - wrapper: Candidate stable wrapper path from the cleanup environment.
//
// Returns:
//   - error: Path validation or managed-file removal failure.
func cleanupWindowsPaths(dataRoot, wrapper string) error {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return fmt.Errorf("LOCALAPPDATA is not set")
	}
	expectedRoot, err := filepath.Abs(filepath.Join(localAppData, "my-toolbox"))
	if err != nil {
		return fmt.Errorf("resolve expected Windows toolbox root: %w", err)
	}
	actualRoot, err := filepath.Abs(dataRoot)
	if err != nil {
		return fmt.Errorf("resolve Windows toolbox root: %w", err)
	}
	expectedWrapper := filepath.Join(expectedRoot, "bin", "tb.cmd")
	actualWrapper, err := filepath.Abs(wrapper)
	if err != nil {
		return fmt.Errorf("resolve Windows toolbox wrapper: %w", err)
	}
	if !strings.EqualFold(filepath.Clean(actualRoot), filepath.Clean(expectedRoot)) ||
		!strings.EqualFold(filepath.Clean(actualWrapper), filepath.Clean(expectedWrapper)) {
		return fmt.Errorf("refusing Windows cleanup outside %s", expectedRoot)
	}
	parentRoot, err := os.OpenRoot(localAppData)
	if err != nil {
		return fmt.Errorf("open Windows local application data root: %w", err)
	}
	defer parentRoot.Close()
	// Opening the toolbox directory relative to a held os.Root handle rejects
	// junctions and keeps later operations bound to the same directory even if
	// another process renames or replaces its path.
	toolboxRoot, err := parentRoot.OpenRoot("my-toolbox")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open Windows toolbox root without reparse points: %w", err)
	}
	defer toolboxRoot.Close()
	if err := toolboxRoot.RemoveAll("versions"); err != nil {
		return fmt.Errorf("remove Windows toolbox versions: %w", err)
	}
	if err := toolboxRoot.Remove("current.txt"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove Windows toolbox current version: %w", err)
	}
	if err := cleanupWindowsWrapper(toolboxRoot); err != nil {
		return err
	}
	if err := toolboxRoot.Remove("bin"); err != nil && !os.IsNotExist(err) && !errors.Is(err, windows.ERROR_DIR_NOT_EMPTY) {
		return fmt.Errorf("remove empty Windows toolbox bin directory: %w", err)
	}
	return nil
}

// cleanupWindowsWrapper atomically isolates and validates tb.cmd before deletion.
//
// Args:
//   - root: Held toolbox directory root that cannot escape through junctions.
//
// Returns:
//   - error: Wrapper rename, read, restore, or removal failure.
func cleanupWindowsWrapper(root *os.Root) error {
	const wrapper = "bin/tb.cmd"
	const isolatedWrapper = "bin/.tb-uninstall.cmd"
	if _, err := root.Stat(isolatedWrapper); err == nil {
		return fmt.Errorf("refusing to replace existing Windows uninstall isolation path %s", isolatedWrapper)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect Windows uninstall isolation path: %w", err)
	}
	if err := root.Rename(wrapper, isolatedWrapper); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("isolate Windows toolbox wrapper: %w", err)
	}
	content, err := root.ReadFile(isolatedWrapper)
	if err != nil {
		return fmt.Errorf("read isolated Windows toolbox wrapper: %w", err)
	}
	if !isOwnedToolboxWrapperContent(content, windowsToolboxWrapper) {
		if err := root.Rename(isolatedWrapper, wrapper); err != nil {
			return fmt.Errorf("restore unrecognized Windows wrapper: %w", err)
		}
		return nil
	}
	if err := root.Remove(isolatedWrapper); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove Windows toolbox wrapper: %w", err)
	}
	return nil
}

// scheduleWindowsHelperRemoval deletes the temporary helper after it exits.
//
// Args: None.
//
// Returns:
//   - error: PowerShell startup or process-release failure.
func scheduleWindowsHelperRemoval() error {
	helperPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve temporary Windows cleanup helper: %w", err)
	}
	script := `Wait-Process -Id ([int]$env:TOOLBOX_HELPER_PID) -ErrorAction SilentlyContinue; Remove-Item -LiteralPath $env:TOOLBOX_HELPER_PATH -Force -ErrorAction SilentlyContinue`
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
	configureNonInteractive(command)
	command.Env = append(os.Environ(),
		"TOOLBOX_HELPER_PID="+strconv.Itoa(os.Getpid()),
		"TOOLBOX_HELPER_PATH="+helperPath,
	)
	if err := command.Start(); err != nil {
		return fmt.Errorf("start Windows helper removal: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("release Windows helper removal: %w", err)
	}
	return nil
}
