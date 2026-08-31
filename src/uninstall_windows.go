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
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const windowsCleanupMode = "toolbox-windows-cleanup"

const maxInstallerOutputBytes = 16 * 1024

var (
	readWindowsUserPath            = readWindowsUserPathRegistry
	writeWindowsUserPath           = writeWindowsUserPathRegistry
	notifyWindowsEnvironmentChange = broadcastWindowsEnvironmentChange
	windowsDocumentsPath           = resolveWindowsDocumentsPath
	sendMessageTimeout             = windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
)

// boundedInstallerOutput retains only the final installer diagnostics.
type boundedInstallerOutput struct {
	content   []byte
	truncated bool
}

// Write adds installer output while retaining at most the configured tail.
//
// Args:
//   - content: Installer stdout or stderr bytes. Earlier bytes are discarded
//     when the combined output exceeds maxInstallerOutputBytes.
//
// Returns:
//   - int: Full input length because truncation is intentional, not a short write.
//   - error: Always nil.
func (output *boundedInstallerOutput) Write(content []byte) (int, error) {
	written := len(content)
	if len(content) >= maxInstallerOutputBytes {
		output.content = append(output.content[:0], content[len(content)-maxInstallerOutputBytes:]...)
		output.truncated = true
		return written, nil
	}
	overflow := len(output.content) + len(content) - maxInstallerOutputBytes
	if overflow > 0 {
		copy(output.content, output.content[overflow:])
		output.content = output.content[:len(output.content)-overflow]
		output.truncated = true
	}
	output.content = append(output.content, content...)
	return written, nil
}

// String formats retained installer diagnostics for the deferred status file.
//
// Args: None.
//
// Returns:
//   - string: Trimmed output tail, prefixed with a truncation marker when
//     earlier installer output was omitted.
func (output *boundedInstallerOutput) String() string {
	detail := strings.TrimSpace(string(output.content))
	if !output.truncated {
		return detail
	}
	if detail == "" {
		return "[earlier installer output omitted]"
	}
	return "[earlier installer output omitted]\n" + detail
}

// uninstallPlatform starts a temporary Go cleanup helper for Windows.
//
// Args:
//   - dataRoot: Validated toolbox data directory containing the running binary.
//   - wrapper: Stable wrapper path inside dataRoot.
//   - installerPath: Empty for uninstall only, or a downloaded PowerShell
//     installer owned by the detached helper.
//   - output: Unused because deferred work reports through the status file.
//
// Returns:
//   - string: Status file that will contain success or an error message.
//   - error: Helper-copy or process-startup failure.
func uninstallPlatform(dataRoot, wrapper, installerPath string, _ io.Writer) (string, error) {
	return scheduleWindowsRemoval(dataRoot, wrapper, installerPath, os.Getpid())
}

// scheduleWindowsRemoval copies tb outside its locked installation and starts it.
//
// Args:
//   - dataRoot: Validated toolbox directory containing managed runtime files.
//   - wrapper: Stable wrapper path to revalidate immediately before removal.
//   - installerPath: Empty for uninstall only, or a downloaded PowerShell
//     installer for the helper to run and remove after cleanup.
//   - processID: Running tb process identifier that must exit before removal.
//
// Returns:
//   - string: Status file that will contain success or an error message.
//   - error: Helper-copy or process-startup failure.
func scheduleWindowsRemoval(dataRoot, wrapper, installerPath string, processID int) (string, error) {
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
		"TOOLBOX_REINSTALL_SCRIPT="+installerPath,
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
	installerPath := os.Getenv("TOOLBOX_REINSTALL_SCRIPT")
	processID, err := strconv.Atoi(os.Getenv("TOOLBOX_UNINSTALL_PID"))
	if err != nil || processID <= 0 {
		err = fmt.Errorf("invalid Windows uninstall process ID")
	} else if err = waitForWindowsProcess(processID); err == nil {
		err = cleanupAndReinstallWindows(dataRoot, wrapper, installerPath)
	}
	if installerPath != "" {
		if removeErr := os.Remove(installerPath); removeErr != nil && !os.IsNotExist(removeErr) && err == nil {
			err = fmt.Errorf("remove temporary installer: %w", removeErr)
		}
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

// cleanupAndReinstallWindows removes managed paths before running an optional installer.
//
// Args:
//   - dataRoot: Validated toolbox directory containing managed runtime files.
//   - wrapper: Stable wrapper path to revalidate during removal.
//   - installerPath: Empty for uninstall only, or a downloaded PowerShell
//     installer that must run after managed removal completes.
//
// Returns:
//   - error: Managed cleanup or installer execution failure.
func cleanupAndReinstallWindows(dataRoot, wrapper, installerPath string) error {
	if err := removeWindowsCompletionProfiles(); err != nil {
		return err
	}
	if err := cleanupWindowsPaths(dataRoot, wrapper); err != nil {
		return err
	}
	if installerPath == "" {
		return removeWindowsUserPathEntry(filepath.Dir(wrapper))
	}
	installerOutput := &boundedInstallerOutput{}
	if err := runClosedInput("powershell", []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", installerPath}, nil, installerOutput); err != nil {
		if detail := installerOutput.String(); detail != "" {
			return fmt.Errorf("reinstall toolbox: %s: %w", detail, err)
		}
		return fmt.Errorf("reinstall toolbox: %w", err)
	}
	return nil
}

// removeWindowsCompletionProfiles removes exact CurrentUserAllHosts blocks.
//
// Args: None.
//
// Returns:
//   - error: Documents resolution, malformed-marker, profile write, or rollback failure.
func removeWindowsCompletionProfiles() error {
	documents, err := windowsDocumentsPath()
	if err != nil {
		return fmt.Errorf("resolve Windows Documents folder for completion cleanup: %w", err)
	}
	if documents == "" {
		return fmt.Errorf("resolve Windows Documents folder for completion cleanup: empty path")
	}
	sourceLine := `. (Join-Path $env:LOCALAPPDATA 'my-toolbox\completions\tb.ps1')`
	return removeCompletionProfileBlocks([]completionProfileRequest{
		{
			path:       filepath.Join(documents, "WindowsPowerShell", "profile.ps1"),
			sourceLine: sourceLine,
			newline:    "\r\n",
		},
		{
			path:       filepath.Join(documents, "PowerShell", "profile.ps1"),
			sourceLine: sourceLine,
			newline:    "\r\n",
		},
	})
}

// resolveWindowsDocumentsPath resolves the current user's Documents known folder.
//
// Args: None.
//
// Returns:
//   - string: Absolute Documents known-folder path for the current user.
//   - error: Windows known-folder resolution failure.
func resolveWindowsDocumentsPath() (string, error) {
	return windows.KnownFolderPath(windows.FOLDERID_Documents, windows.KF_FLAG_DEFAULT)
}

// removeWindowsPathEntries removes every equivalent managed directory entry.
// Unrelated entries, their order, and empty entries are preserved exactly.
//
// Args:
//   - value: Existing semicolon-delimited Windows PATH text.
//   - managed: Managed wrapper directory to remove.
//
// Returns:
//   - string: PATH text with managed entries removed.
//   - bool: True when at least one entry was removed.
func removeWindowsPathEntries(value, managed string) (string, bool) {
	managed = normalizeWindowsPathEntry(managed)
	entries := strings.Split(value, ";")
	kept := make([]string, 0, len(entries))
	changed := false
	for _, entry := range entries {
		if strings.EqualFold(normalizeWindowsPathEntry(entry), managed) {
			changed = true
			continue
		}
		kept = append(kept, entry)
	}
	if !changed {
		return value, false
	}
	return strings.Join(kept, ";"), true
}

// normalizeWindowsPathEntry normalizes only syntax ignored for PATH identity.
func normalizeWindowsPathEntry(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}
	return strings.TrimRight(value, `\/`)
}

// removeWindowsUserPathEntry removes the managed wrapper directory from the
// persisted user PATH and notifies running Windows applications of the change.
func removeWindowsUserPathEntry(managed string) error {
	value, valueType, exists, err := readWindowsUserPath()
	if err != nil {
		return fmt.Errorf("read Windows user PATH: %w", err)
	}
	if !exists {
		return nil
	}
	updated, changed := removeWindowsPathEntries(value, managed)
	if !changed {
		return nil
	}
	if err := writeWindowsUserPath(updated, valueType); err != nil {
		return fmt.Errorf("write Windows user PATH: %w", err)
	}
	if err := notifyWindowsEnvironmentChange(); err != nil {
		return fmt.Errorf("notify Windows environment change: %w", err)
	}
	return nil
}

// readWindowsUserPathRegistry reads PATH without expanding registry variables.
func readWindowsUserPathRegistry() (string, uint32, bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	defer key.Close()
	value, valueType, err := key.GetStringValue("Path")
	if errors.Is(err, registry.ErrNotExist) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", valueType, false, err
	}
	return value, valueType, true, nil
}

// writeWindowsUserPathRegistry writes PATH using its original registry type.
func writeWindowsUserPathRegistry(value string, valueType uint32) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	switch valueType {
	case registry.SZ:
		return key.SetStringValue("Path", value)
	case registry.EXPAND_SZ:
		return key.SetExpandStringValue("Path", value)
	default:
		return fmt.Errorf("unsupported PATH registry type %d", valueType)
	}
}

// broadcastWindowsEnvironmentChange tells running applications to refresh
// environment settings after the persisted user PATH changes.
func broadcastWindowsEnvironmentChange() error {
	environment, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return err
	}
	var messageResult uintptr
	result, _, callErr := sendMessageTimeout.Call(
		0xffff,
		0x001a,
		0,
		uintptr(unsafe.Pointer(environment)),
		0x0002,
		5000,
		uintptr(unsafe.Pointer(&messageResult)),
	)
	if result != 0 {
		return nil
	}
	if callErr == windows.ERROR_SUCCESS {
		return fmt.Errorf("SendMessageTimeoutW returned no result")
	}
	return callErr
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
	if err := toolboxRoot.RemoveAll("completions"); err != nil {
		return fmt.Errorf("remove Windows toolbox completions: %w", err)
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
