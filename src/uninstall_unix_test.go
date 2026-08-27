//go:build !windows

package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
