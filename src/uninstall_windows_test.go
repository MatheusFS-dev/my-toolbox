//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

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
