//go:build windows

package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

func TestRemoveWindowsPathEntriesRemovesEveryManagedVariantAndPreservesText(t *testing.T) {
	managed := `C:\Users\Example\AppData\Local\my-toolbox\bin`
	for _, test := range []struct {
		name      string
		pathValue string
		want      string
	}{
		{name: "single", pathValue: `C:\Before;C:\Users\Example\AppData\Local\my-toolbox\bin;C:\After`, want: `C:\Before;C:\After`},
		{name: "multiple variants and empty entries", pathValue: `;C:\Before;;"C:\USERS\EXAMPLE\APPDATA\LOCAL\MY-TOOLBOX\BIN\";C:\After;C:\Users\Example\AppData\Local\my-toolbox\bin/;`, want: `;C:\Before;;C:\After;`},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, changed := removeWindowsPathEntries(test.pathValue, managed)
			if !changed || got != test.want {
				t.Fatalf("removeWindowsPathEntries() = %q, %t, want %q, true", got, changed, test.want)
			}
		})
	}
}

func TestRemoveWindowsPathEntriesLeavesMissingEntryUnchanged(t *testing.T) {
	pathValue := `;C:\Before;;C:\After;`

	got, changed := removeWindowsPathEntries(pathValue, `C:\Managed\bin`)
	if changed || got != pathValue {
		t.Fatalf("removeWindowsPathEntries() = %q, %t, want %q, false", got, changed, pathValue)
	}
}

func TestWindowsUninstallRemovesManagedUserPathAndNotifies(t *testing.T) {
	localAppData := t.TempDir()
	dataRoot := filepath.Join(localAppData, "my-toolbox")
	binRoot := filepath.Join(dataRoot, "bin")
	wrapper := filepath.Join(binRoot, "tb.cmd")
	t.Setenv("LOCALAPPDATA", localAppData)
	if err := os.MkdirAll(binRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrapper, []byte(windowsToolboxWrapper), 0o644); err != nil {
		t.Fatal(err)
	}

	originalRead := readWindowsUserPath
	originalWrite := writeWindowsUserPath
	originalNotify := notifyWindowsEnvironmentChange
	t.Cleanup(func() {
		readWindowsUserPath = originalRead
		writeWindowsUserPath = originalWrite
		notifyWindowsEnvironmentChange = originalNotify
	})
	readWindowsUserPath = func() (string, uint32, bool, error) {
		return `C:\Before;` + strings.ToUpper(binRoot) + `\;` + binRoot + `;C:\After`, registry.EXPAND_SZ, true, nil
	}
	writtenPath := ""
	writtenType := uint32(0)
	writeWindowsUserPath = func(value string, valueType uint32) error {
		writtenPath = value
		writtenType = valueType
		return nil
	}
	notifications := 0
	notifyWindowsEnvironmentChange = func() error {
		notifications++
		return nil
	}

	if err := cleanupAndReinstallWindows(dataRoot, wrapper, ""); err != nil {
		t.Fatal(err)
	}
	if writtenPath != `C:\Before;C:\After` {
		t.Fatalf("written user PATH = %q", writtenPath)
	}
	if writtenType != registry.EXPAND_SZ {
		t.Fatalf("written registry type = %d, want %d", writtenType, registry.EXPAND_SZ)
	}
	if notifications != 1 {
		t.Fatalf("environment notifications = %d, want 1", notifications)
	}
}

func TestWindowsUninstallMissingUserPathEntryIsNoOp(t *testing.T) {
	originalRead := readWindowsUserPath
	originalWrite := writeWindowsUserPath
	originalNotify := notifyWindowsEnvironmentChange
	t.Cleanup(func() {
		readWindowsUserPath = originalRead
		writeWindowsUserPath = originalWrite
		notifyWindowsEnvironmentChange = originalNotify
	})
	readWindowsUserPath = func() (string, uint32, bool, error) {
		return `;C:\Before;;C:\After;`, registry.SZ, true, nil
	}
	writes := 0
	writeWindowsUserPath = func(value string, valueType uint32) error {
		writes++
		return nil
	}
	notifications := 0
	notifyWindowsEnvironmentChange = func() error {
		notifications++
		return nil
	}

	if err := removeWindowsUserPathEntry(`C:\Managed\bin`); err != nil {
		t.Fatal(err)
	}
	if writes != 0 || notifications != 0 {
		t.Fatalf("missing entry caused %d writes and %d notifications", writes, notifications)
	}
}

func TestRemoveWindowsUserPathEntryPreservesRegistryValueType(t *testing.T) {
	for _, test := range []struct {
		name      string
		valueType uint32
	}{
		{name: "string", valueType: registry.SZ},
		{name: "expand string", valueType: registry.EXPAND_SZ},
	} {
		t.Run(test.name, func(t *testing.T) {
			originalRead := readWindowsUserPath
			originalWrite := writeWindowsUserPath
			originalNotify := notifyWindowsEnvironmentChange
			t.Cleanup(func() {
				readWindowsUserPath = originalRead
				writeWindowsUserPath = originalWrite
				notifyWindowsEnvironmentChange = originalNotify
			})
			readWindowsUserPath = func() (string, uint32, bool, error) {
				return `C:\Managed\bin`, test.valueType, true, nil
			}
			writtenType := uint32(0)
			writeWindowsUserPath = func(value string, valueType uint32) error {
				writtenType = valueType
				return nil
			}
			notifyWindowsEnvironmentChange = func() error { return nil }

			if err := removeWindowsUserPathEntry(`C:\Managed\bin`); err != nil {
				t.Fatal(err)
			}
			if writtenType != test.valueType {
				t.Fatalf("written registry type = %d, want %d", writtenType, test.valueType)
			}
		})
	}
}

func TestWindowsCleanupRemovesToolboxAndPreservesGitHubCLI(t *testing.T) {
	localAppData := t.TempDir()
	dataRoot := filepath.Join(localAppData, "my-toolbox")
	versionsRoot := filepath.Join(dataRoot, "versions", "0.1.1")
	binRoot := filepath.Join(dataRoot, "bin")
	wrapper := filepath.Join(binRoot, "tb.cmd")
	githubCLI := filepath.Join(binRoot, "gh.exe")
	currentFile := filepath.Join(dataRoot, "current.txt")
	unrelated := filepath.Join(localAppData, "unrelated")
	t.Setenv("LOCALAPPDATA", localAppData)
	for _, path := range []string{versionsRoot, binRoot, unrelated} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		wrapper:     windowsToolboxWrapper,
		githubCLI:   "installed gh",
		currentFile: "0.1.1\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := cleanupWindowsPaths(dataRoot, wrapper); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{versionsRoot, currentFile, wrapper} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("managed path still exists: %s", path)
		}
	}
	content, err := os.ReadFile(githubCLI)
	if err != nil || string(content) != "installed gh" {
		t.Fatalf("uninstall changed GitHub CLI: %q, %v", content, err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("uninstall removed unrelated path: %v", err)
	}
}

func TestWindowsCleanupReinstallsAfterManagedRemoval(t *testing.T) {
	localAppData := t.TempDir()
	dataRoot := filepath.Join(localAppData, "my-toolbox")
	versionRoot := filepath.Join(dataRoot, "versions", "0.1.1")
	wrapper := filepath.Join(dataRoot, "bin", "tb.cmd")
	installerPath := filepath.Join(t.TempDir(), "install.ps1")
	marker := filepath.Join(t.TempDir(), "installed")
	t.Setenv("LOCALAPPDATA", localAppData)
	t.Setenv("TOOLBOX_TEST_DATA_ROOT", dataRoot)
	t.Setenv("TOOLBOX_TEST_MARKER", marker)
	if err := os.MkdirAll(versionRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrapper, []byte(windowsToolboxWrapper), 0o644); err != nil {
		t.Fatal(err)
	}
	originalRead := readWindowsUserPath
	t.Cleanup(func() { readWindowsUserPath = originalRead })
	pathReads := 0
	readWindowsUserPath = func() (string, uint32, bool, error) {
		pathReads++
		return "", registry.SZ, true, nil
	}
	installer := "if (Test-Path -LiteralPath (Join-Path $env:TOOLBOX_TEST_DATA_ROOT 'versions')) { exit 2 }\n[IO.File]::WriteAllText($env:TOOLBOX_TEST_MARKER, 'installed')\n"
	if err := os.WriteFile(installerPath, []byte(installer), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := cleanupAndReinstallWindows(dataRoot, wrapper, installerPath); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(marker)
	if err != nil || string(content) != "installed" {
		t.Fatalf("installer marker = %q, %v", content, err)
	}
	if pathReads != 0 {
		t.Fatalf("reinstall read user PATH %d times", pathReads)
	}
}

func TestWindowsReinstallReportsInstallerOutput(t *testing.T) {
	localAppData := t.TempDir()
	dataRoot := filepath.Join(localAppData, "my-toolbox")
	wrapper := filepath.Join(dataRoot, "bin", "tb.cmd")
	installerPath := filepath.Join(t.TempDir(), "install.ps1")
	t.Setenv("LOCALAPPDATA", localAppData)
	if err := os.MkdirAll(filepath.Dir(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrapper, []byte(windowsToolboxWrapper), 0o644); err != nil {
		t.Fatal(err)
	}
	installer := "[Console]::Error.WriteLine('release download failed')\nexit 1\n"
	if err := os.WriteFile(installerPath, []byte(installer), 0o600); err != nil {
		t.Fatal(err)
	}

	err := cleanupAndReinstallWindows(dataRoot, wrapper, installerPath)
	if err == nil || !strings.Contains(err.Error(), "release download failed") {
		t.Fatalf("cleanupAndReinstallWindows() error = %v", err)
	}
}

func TestBoundedInstallerOutputKeepsTheFailureTail(t *testing.T) {
	output := &boundedInstallerOutput{}
	content := strings.Repeat("earlier output\n", maxInstallerOutputBytes) + "final failure"
	written, err := output.Write([]byte(content))
	if err != nil || written != len(content) {
		t.Fatalf("Write() = %d, %v", written, err)
	}
	detail := output.String()
	if !strings.Contains(detail, "earlier installer output omitted") || !strings.HasSuffix(detail, "final failure") {
		t.Fatalf("bounded output = %q", detail)
	}
	if len(detail) > maxInstallerOutputBytes+100 {
		t.Fatalf("bounded output length = %d", len(detail))
	}
}

func TestWindowsUpdateRefusesUnrecognizedWrapperBeforeSchedulingRemoval(t *testing.T) {
	localAppData := t.TempDir()
	dataRoot := filepath.Join(localAppData, "my-toolbox")
	versionRoot := filepath.Join(dataRoot, "versions", "0.1.1")
	wrapper := filepath.Join(dataRoot, "bin", "tb.cmd")
	t.Setenv("LOCALAPPDATA", localAppData)
	if err := os.MkdirAll(versionRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrapper, []byte("user-owned wrapper"), 0o644); err != nil {
		t.Fatal(err)
	}
	builtins := NewToolboxBuiltins(versionRoot, "windows-amd64", "0.1.1", io.Discard)

	_, err := builtins.remove(filepath.Join(t.TempDir(), "install.ps1"))
	if err == nil || !strings.Contains(err.Error(), "refusing to update with unrecognized wrapper") {
		t.Fatalf("remove() error = %v", err)
	}
	content, readErr := os.ReadFile(wrapper)
	if readErr != nil || string(content) != "user-owned wrapper" {
		t.Fatalf("update changed unrecognized wrapper: %q, %v", content, readErr)
	}
}

func TestWindowsCleanupDoesNotFollowDirectoryJunctions(t *testing.T) {
	localAppData := t.TempDir()
	dataRoot := filepath.Join(localAppData, "my-toolbox")
	versionsRoot := filepath.Join(dataRoot, "versions")
	wrapper := filepath.Join(dataRoot, "bin", "tb.cmd")
	external := t.TempDir()
	externalFile := filepath.Join(external, "preserved.txt")
	junction := filepath.Join(versionsRoot, "external")
	t.Setenv("LOCALAPPDATA", localAppData)
	if err := os.MkdirAll(versionsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(externalFile, []byte("preserved"), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", junction, external).CombinedOutput(); err != nil {
		t.Fatalf("create directory junction: %s: %v", output, err)
	}
	if err := cleanupWindowsPaths(dataRoot, wrapper); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(externalFile)
	if err != nil || string(content) != "preserved" {
		t.Fatalf("cleanup followed junction: %q, %v", content, err)
	}
}

func TestWindowsCleanupRejectsToolboxRootJunction(t *testing.T) {
	localAppData := t.TempDir()
	dataRoot := filepath.Join(localAppData, "my-toolbox")
	wrapper := filepath.Join(dataRoot, "bin", "tb.cmd")
	external := t.TempDir()
	externalFile := filepath.Join(external, "preserved.txt")
	t.Setenv("LOCALAPPDATA", localAppData)
	if err := os.WriteFile(externalFile, []byte("preserved"), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", dataRoot, external).CombinedOutput(); err != nil {
		t.Fatalf("create toolbox root junction: %s: %v", output, err)
	}
	if err := cleanupWindowsPaths(dataRoot, wrapper); err == nil {
		t.Fatal("cleanup accepted a toolbox root junction")
	}
	content, err := os.ReadFile(externalFile)
	if err != nil || string(content) != "preserved" {
		t.Fatalf("cleanup followed toolbox root junction: %q, %v", content, err)
	}
}

func TestWindowsCleanupPreservesExistingIsolationPath(t *testing.T) {
	localAppData := t.TempDir()
	dataRoot := filepath.Join(localAppData, "my-toolbox")
	binRoot := filepath.Join(dataRoot, "bin")
	wrapper := filepath.Join(binRoot, "tb.cmd")
	isolationPath := filepath.Join(binRoot, ".tb-uninstall.cmd")
	t.Setenv("LOCALAPPDATA", localAppData)
	if err := os.MkdirAll(binRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrapper, []byte(windowsToolboxWrapper), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(isolationPath, []byte("unrelated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cleanupWindowsPaths(dataRoot, wrapper); err == nil {
		t.Fatal("cleanup replaced an existing isolation path")
	}
	content, err := os.ReadFile(isolationPath)
	if err != nil || string(content) != "unrelated" {
		t.Fatalf("cleanup changed existing isolation path: %q, %v", content, err)
	}
}

func TestWaitForWindowsProcessBlocksUntilExit(t *testing.T) {
	process := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "Start-Sleep -Seconds 2")
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- waitForWindowsProcess(process.Process.Pid)
	}()
	select {
	case err := <-done:
		t.Fatalf("wait returned before process exit: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	if err := process.Wait(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("wait did not return after process exit")
	}
}
