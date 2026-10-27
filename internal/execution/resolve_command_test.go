package execution

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveUserCommandFindsOfficialOpenCodeLinuxInstallation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Linux OpenCode installation path is not used on Windows")
	}
	home := t.TempDir()
	binary := filepath.Join(home, ".opencode", "bin", "opencode")
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	path, err := ResolveUserCommand("opencode", "linux-amd64")
	if err != nil {
		t.Fatal(err)
	}
	if path != binary {
		t.Fatalf("path = %q, want %q", path, binary)
	}
}
