//go:build !windows

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUninstallRemovesExactCompletionBlocksAndPreservesProfiles(t *testing.T) {
	home := t.TempDir()
	dataBase := filepath.Join(home, "data")
	dataRoot := filepath.Join(dataBase, "my-toolbox")
	versionRoot := filepath.Join(dataRoot, "versions", "0.1.1")
	wrapper := filepath.Join(home, ".local", "bin", "tb")
	zshRoot := filepath.Join(home, "zsh")
	bashProfile := filepath.Join(home, ".bashrc")
	zshProfile := filepath.Join(zshRoot, ".zshrc")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", dataBase)
	t.Setenv("ZDOTDIR", zshRoot)
	for _, path := range []string{versionRoot, filepath.Dir(wrapper), zshRoot, filepath.Join(dataRoot, "completions")} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(wrapper, []byte(linuxToolboxWrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	markerStart := "# >>> my-toolbox completion >>>"
	markerEnd := "# <<< my-toolbox completion <<<"
	bashOriginal := []byte("bash unrelated")
	zshOriginal := []byte("zsh unrelated\n")
	bashBlock := fmt.Sprintf("\n%s\n. '%s/completions/tb.bash'\n%s\n", markerStart, dataRoot, markerEnd)
	zshBlock := fmt.Sprintf("\n%s\nsource '%s/completions/_tb'\n%s\n", markerStart, dataRoot, markerEnd)
	if err := os.WriteFile(bashProfile, append(append([]byte(nil), bashOriginal...), bashBlock...), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(zshProfile, append(append([]byte(nil), zshOriginal...), zshBlock...), 0o644); err != nil {
		t.Fatal(err)
	}

	builtins := NewToolboxBuiltins(versionRoot, "linux-amd64", "0.1.1", io.Discard)
	if err := builtins.uninstall(); err != nil {
		t.Fatal(err)
	}
	for profile, want := range map[string][]byte{bashProfile: bashOriginal, zshProfile: zshOriginal} {
		got, err := os.ReadFile(profile)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Fatalf("profile %s = %q, want %q", profile, got, want)
		}
	}
}

func TestUninstallRejectsMalformedCompletionMarkersBeforeDeletingInstallation(t *testing.T) {
	home := t.TempDir()
	dataBase := filepath.Join(home, "data")
	dataRoot := filepath.Join(dataBase, "my-toolbox")
	versionRoot := filepath.Join(dataRoot, "versions", "0.1.1")
	wrapper := filepath.Join(home, ".local", "bin", "tb")
	bashProfile := filepath.Join(home, ".bashrc")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", dataBase)
	for _, path := range []string{versionRoot, filepath.Dir(wrapper)} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(wrapper, []byte(linuxToolboxWrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	malformed := []byte("unrelated\n# >>> my-toolbox completion >>>\n")
	if err := os.WriteFile(bashProfile, malformed, 0o644); err != nil {
		t.Fatal(err)
	}

	builtins := NewToolboxBuiltins(versionRoot, "linux-amd64", "0.1.1", io.Discard)
	err := builtins.uninstall()
	if err == nil || !strings.Contains(err.Error(), "malformed my-toolbox completion markers") {
		t.Fatalf("uninstall() error = %v", err)
	}
	if _, err := os.Stat(versionRoot); err != nil {
		t.Fatalf("malformed markers removed installation: %v", err)
	}
	if _, err := os.Stat(wrapper); err != nil {
		t.Fatalf("malformed markers removed wrapper: %v", err)
	}
	got, err := os.ReadFile(bashProfile)
	if err != nil || string(got) != string(malformed) {
		t.Fatalf("malformed profile = %q, %v", got, err)
	}
}

func TestUninstallRemovesOnlyToolboxManagedLinuxPaths(t *testing.T) {
	home := t.TempDir()
	dataBase := filepath.Join(home, "data")
	dataRoot := filepath.Join(dataBase, "my-toolbox")
	versionRoot := filepath.Join(dataRoot, "versions", "0.1.1")
	wrapper := filepath.Join(home, ".local", "bin", "tb")
	unrelated := filepath.Join(dataBase, "unrelated")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", dataBase)
	for _, path := range []string{versionRoot, filepath.Dir(wrapper), unrelated} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(wrapper, []byte(linuxToolboxWrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	bashProfile := filepath.Join(home, ".bashrc")
	zshProfile := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(bashProfile, []byte("bash unrelated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(zshProfile, []byte("zsh unrelated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builtins := NewToolboxBuiltins(versionRoot, "linux-amd64", "0.1.1", io.Discard)
	if err := builtins.uninstall(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dataRoot, wrapper} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("managed path still exists: %s", path)
		}
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("uninstall removed unrelated path: %v", err)
	}
	for profile, want := range map[string]string{bashProfile: "bash unrelated", zshProfile: "zsh unrelated\n"} {
		content, err := os.ReadFile(profile)
		if err != nil || string(content) != want {
			t.Fatalf("uninstall changed unmanaged profile %s: %q, %v", profile, content, err)
		}
	}
}

func TestUninstallPreservesUnrecognizedLinuxWrapper(t *testing.T) {
	home := t.TempDir()
	dataBase := filepath.Join(home, "data")
	dataRoot := filepath.Join(dataBase, "my-toolbox")
	versionRoot := filepath.Join(dataRoot, "versions", "0.1.1")
	wrapper := filepath.Join(home, ".local", "bin", "tb")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", dataBase)
	for _, path := range []string{versionRoot, filepath.Dir(wrapper)} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(wrapper, []byte("user-owned executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	builtins := NewToolboxBuiltins(versionRoot, "linux-amd64", "0.1.1", io.Discard)
	if err := builtins.uninstall(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(wrapper)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "user-owned executable" {
		t.Fatalf("uninstall changed unrecognized wrapper: %q", content)
	}
}

func TestUninstallRefusesDevelopmentExecutableRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	builtins := NewToolboxBuiltins(t.TempDir(), "linux-amd64", "development", io.Discard)
	err := builtins.uninstall()
	if err == nil || !strings.Contains(err.Error(), "installed version directory") {
		t.Fatalf("uninstall() error = %v", err)
	}
}
