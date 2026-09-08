package main

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestYdotoolAutomaticallyStartsDaemonAndPassesSocket(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux daemon")
	}
	home := t.TempDir()
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(home, "daemon-started")
	t.Setenv("TB_DAEMON_MARKER", marker)
	t.Setenv("YDOTOOL_SOCKET", "")
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	scripts := map[string]string{
		"ydotool":  "#!/bin/sh\nif [ \"$1\" = --help ]; then echo YDOTOOL_SOCKET; elif [ -f \"$TB_DAEMON_MARKER\" ] && [ -n \"$YDOTOOL_SOCKET\" ]; then echo keycode; else exit 2; fi\n",
		"ydotoold": "#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$TB_DAEMON_MARKER\"\n",
		"sudo":     "#!/bin/sh\nif [ \"$1\" = -v ]; then exit 0; fi\n[ \"$1\" = -n ] && shift\n[ \"$1\" = -b ] && shift\n[ \"$1\" = -- ] && shift\nexec \"$@\"\n",
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// This fixture records the environment actually inherited by the detached macro.
	script := filepath.Join(home, "timer.sh")
	record := filepath.Join(home, "socket")
	t.Setenv("TB_SOCKET_RECORD", record)
	if err := os.WriteFile(script, []byte("#!/bin/sh\n[ \"${1:-}\" = --check ] && exit 0\nprintf '%s' \"$YDOTOOL_SOCKET\" > \"$TB_SOCKET_RECORD\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workflow := DefaultMacroWorkflow{GOOS: "linux", Home: home, Output: &bytes.Buffer{}}
	macro := Macro{Subpackage: "ydotool", SourcePath: script}
	if err := workflow.Execute(MacroSelection{Macro: macro, Action: MacroActionRun}); err != nil {
		t.Fatal(err)
	}
	arguments, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(arguments), "--socket-perm=0600") || !strings.Contains(string(arguments), "--socket-own=") {
		t.Fatalf("daemon arguments: %s", arguments)
	}
	// Reusing the daemon must not start another process.
	if err := workflow.Execute(MacroSelection{Macro: macro, Action: MacroActionRun}); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(marker)
	if string(again) != string(arguments) {
		t.Fatal("daemon started twice")
	}
	// Poll using a bounded helper process to avoid leaving detached test children behind.
	output, err := exec.Command("sh", "-c", "i=0; while [ ! -s \"$TB_SOCKET_RECORD\" ] && [ $i -lt 100 ]; do sleep 0.01; i=$((i+1)); done; cat \"$TB_SOCKET_RECORD\"").CombinedOutput()
	if err != nil || string(output) != filepath.Join(home, ".ydotool_socket") {
		t.Fatalf("macro socket: %s, %v", output, err)
	}
	if os.Getenv("YDOTOOL_SOCKET") != "" {
		t.Fatal("changed parent environment")
	}
}

func TestYdotoolPreservesExplicitWorkingSocket(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux daemon")
	}
	home := t.TempDir()
	client := filepath.Join(home, "ydotool")
	t.Setenv("YDOTOOL_SOCKET", "/custom/ydotool.sock")
	if err := os.WriteFile(client, []byte("#!/bin/sh\n[ \"$YDOTOOL_SOCKET\" = /custom/ydotool.sock ] || exit 2\necho keycode\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	environment, err := (DefaultMacroWorkflow{}).ensureYdotoolDaemon(client, home)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(environment, "\n"), "YDOTOOL_SOCKET=/custom/ydotool.sock") {
		t.Fatal("lost configured socket")
	}
	if _, err := os.Stat(filepath.Join(home, ".local")); !os.IsNotExist(err) {
		t.Fatal("created daemon state despite working socket")
	}
}

func TestYdotoolDoesNotReplaceUnrelatedSocketPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux daemon")
	}
	home := t.TempDir()
	client := filepath.Join(home, "ydotool")
	socket := filepath.Join(home, "socket")
	t.Setenv("YDOTOOL_SOCKET", socket)
	if err := os.WriteFile(client, []byte("#!/bin/sh\nexit 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(socket, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (DefaultMacroWorkflow{}).ensureYdotoolDaemon(client, home); err == nil || !strings.Contains(err.Error(), "not a socket") {
		t.Fatalf("regular file: %v", err)
	}
	contents, _ := os.ReadFile(socket)
	if string(contents) != "preserve" {
		t.Fatal("replaced unrelated file")
	}
}

func TestYdotoolSudoFailureStopsStartup(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux daemon")
	}
	home := t.TempDir()
	t.Setenv("PATH", home)
	if err := os.WriteFile(filepath.Join(home, "sudo"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := startYdotoolDaemon("/unused/daemon", filepath.Join(home, "socket"), home, true, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "authorize") {
		t.Fatalf("sudo failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "socket")); !os.IsNotExist(err) {
		t.Fatal("started after failed authorization")
	}
}

func TestYdotoolContinuesMacroAfterInstallation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux macro")
	}
	home := t.TempDir()
	client := filepath.Join(home, "ydotool")
	script := filepath.Join(home, "macro.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output := &bytes.Buffer{}
	workflow := DefaultMacroWorkflow{GOOS: "linux", Home: home, Output: output, UI: &fakeUI{answers: map[string]any{"install-ydotool": true}},
		FindRuntime: func(_, _ string) (string, error) { _, err := os.Stat(client); return client, err },
		InstallRuntime: func(goos, goarch, location string) error {
			return os.WriteFile(client, []byte("#!/bin/sh\necho keycode\n"), 0o755)
		},
	}
	if err := workflow.Execute(MacroSelection{Macro: Macro{Subpackage: "ydotool", SourcePath: script}, Action: MacroActionRun}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Scheduled") {
		t.Fatalf("installation did not continue: %s", output)
	}
}

func TestYdotoolDoesNotReplaceFailedCustomSocket(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux sockets")
	}
	home := t.TempDir()
	socket := filepath.Join(home, "service.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client := filepath.Join(home, "ydotool")
	if err := os.WriteFile(client, []byte("#!/bin/sh\nexit 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", home)
	t.Setenv("YDOTOOL_SOCKET", socket)
	_, err = (DefaultMacroWorkflow{}).ensureYdotoolDaemon(client, home)
	if err == nil || !strings.Contains(err.Error(), "refusing to replace") {
		t.Fatalf("custom socket: %v", err)
	}
	connection, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatalf("unrelated socket disrupted: %v", err)
	}
	connection.Close()
}

func TestYdotoolUsesPasswordlessSudoWithoutValidation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux daemon")
	}
	home := t.TempDir()
	marker := filepath.Join(home, "started")
	t.Setenv("TB_DAEMON_MARKER", marker)
	t.Setenv("PATH", home)
	sudo := "#!/bin/sh\n[ \"$1\" = -v ] && exit 1\n[ \"$1\" = -n ] || exit 9\nshift\n[ \"$1\" = -b ] || exit 9\nshift\n[ \"$1\" = -- ] || exit 9\nshift\nexec \"$@\"\n"
	if err := os.WriteFile(filepath.Join(home, "sudo"), []byte(sudo), 0o755); err != nil {
		t.Fatal(err)
	}
	daemon := filepath.Join(home, "ydotoold")
	if err := os.WriteFile(daemon, []byte("#!/bin/sh\nprintf started > \"$TB_DAEMON_MARKER\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := startYdotoolDaemon(daemon, filepath.Join(home, "socket"), home, true, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "started" {
		t.Fatalf("daemon did not start: %s %v", data, err)
	}
}
