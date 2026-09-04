//go:build !windows

package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallMonitorRefusesUnrecognizedWrapper(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	wrapper := filepath.Join(home, ".local", "bin", "monitor")
	if err := os.MkdirAll(filepath.Dir(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\necho unrelated\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	builtins := NewToolboxBuiltins(t.TempDir(), "linux-amd64", "1.2.3", io.Discard)
	err := builtins.installMonitor()
	if err == nil || !strings.Contains(err.Error(), "unrecognized monitor wrapper") {
		t.Fatalf("installMonitor() error = %v", err)
	}
	content, readErr := os.ReadFile(wrapper)
	if readErr != nil || string(content) != "#!/bin/sh\necho unrelated\n" {
		t.Fatalf("unrelated wrapper changed: %q, %v", content, readErr)
	}
}

func TestRecognizedMonitorWrapperIsStableAndOwnerExecutable(t *testing.T) {
	content := monitorWrapper("/opt/toolbox")
	if !isOwnedMonitorWrapperContent([]byte(content)) {
		t.Fatal("generated wrapper was not recognized")
	}
	if isOwnedMonitorWrapperContent([]byte("#!/bin/sh\nexec something-else\n")) {
		t.Fatal("unrelated wrapper was recognized")
	}
}

func TestRemoveMonitorPreservesConfigurationAndLogs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	state := filepath.Join(home, ".monitor")
	if err := os.MkdirAll(filepath.Join(state, "runtime"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "config.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "runtime", "owned.json"), []byte(monitorRuntimeMarker), 0o600); err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(home, ".local", "bin", "monitor")
	if err := os.MkdirAll(filepath.Dir(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrapper, []byte(monitorWrapper("/opt/toolbox")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := removeMonitor(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(state, "config.json")); err != nil {
		t.Fatalf("configuration was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(state, "runtime")); !os.IsNotExist(err) {
		t.Fatalf("runtime remains: %v", err)
	}
	if _, err := os.Stat(wrapper); !os.IsNotExist(err) {
		t.Fatalf("wrapper remains: %v", err)
	}
}

func TestInstallMonitorPublishesPrivateRuntimeAndWrapper(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	source := filepath.Join(root, "packages", "monitor_runtime")
	if err := os.MkdirAll(filepath.Join(source, "monitor_runtime"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "requirements.txt"), []byte("dependency\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "monitor_runtime", "__init__.py"), []byte("VERSION = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fakePython := filepath.Join(bin, "python3")
	fake := "#!/bin/sh\n" +
		"if [ \"$1\" = -c ]; then exit 0; fi\n" +
		"if [ \"$1 $2\" = '-m venv' ]; then mkdir -p \"$3/bin\"; cp \"$0\" \"$3/bin/python\"; exit 0; fi\n" +
		"if [ \"$1 $2 $3\" = '-m pip install' ]; then exit 0; fi\n" +
		"exit 1\n"
	if err := os.WriteFile(fakePython, []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")
	builtins := NewToolboxBuiltins(root, "linux-amd64", "1.2.3", io.Discard)
	if err := builtins.installMonitor(); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(home, ".monitor")
	if mode := fileMode(t, state); mode != 0o700 {
		t.Fatalf("state mode = %o", mode)
	}
	if mode := fileMode(t, filepath.Join(home, ".local", "bin", "monitor")); mode != 0o755 {
		t.Fatalf("wrapper mode = %o", mode)
	}
	if _, err := os.Stat(filepath.Join(state, "runtime", "app", "monitor_runtime", "__init__.py")); err != nil {
		t.Fatalf("runtime package missing: %v", err)
	}
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}
