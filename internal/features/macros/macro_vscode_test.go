package macros

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVSCodeDiscovery(t *testing.T) {
	library, err := LoadMacros(filepath.Join("..", "..", "..", "assets", "payload", "packages", "macros"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, macro := range library {
		if macro.Subpackage != "vscode" {
			continue
		}
		if macro.Title != "VS Code split terminal" || !MacroSupportsOS(macro, "linux") || MacroSupportsOS(macro, "windows") {
			t.Fatalf("invalid macro: %+v", macro)
		}
		return
	}
	t.Fatal("VS Code macro missing")
}

func TestVSCodeForegroundAndDownload(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh unavailable")
	}
	root := t.TempDir()
	script := filepath.Join(root, "installer.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'foreground complete'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	macro := Macro{Subpackage: "vscode", SourcePath: script, Title: "VS Code split terminal"}
	output := &bytes.Buffer{}
	workflow := DefaultMacroWorkflow{GOOS: "linux", Output: output}
	if err := workflow.Execute(MacroSelection{Macro: macro, Action: MacroActionRun}); err != nil {
		t.Fatal(err)
	}
	if output.String() != "foreground complete" {
		t.Fatal(output.String())
	}
	workflow.GOOS = "windows"
	if err := workflow.Execute(MacroSelection{Macro: macro, Action: MacroActionRun}); err == nil {
		t.Fatal("ran Linux installer on Windows")
	}
	destination := t.TempDir()
	workflow.UI = &fakeUI{answers: map[string]any{"macro-download-directory": destination}}
	if err := workflow.Execute(MacroSelection{Macro: macro, Action: MacroActionDownload}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "installer.sh"))
	if err != nil || !strings.Contains(string(content), "foreground complete") {
		t.Fatalf("download: %s, %v", content, err)
	}
}

// Removing embedded payloads or relying on source-tree siblings must break this test.
func TestVSCodeDownloadedInstaller(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux installer")
	}
	for _, tool := range []string{"node", "zip", "sh"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " unavailable")
		}
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+content), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"xdotool", "wmctrl", "xrandr", "xprop", "x-terminal-emulator"} {
		write(name, "exit 0\n")
	}
	for _, tool := range []string{"node", "zip", "uname"} {
		target, err := exec.LookPath(tool)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(bin, tool)); err != nil {
			t.Fatal(err)
		}
	}
	write("code", "[ \"$1\" = --install-extension ] && [ \"$3\" = --force ] || exit 4\n[ \"${TEST_CLI_FAIL:-}\" != yes ] || exit 7\ncp \"$2\" \"$TEST_VSIX\"\n")
	source := filepath.Join("..", "..", "..", "assets", "payload", "packages", "macros", "vscode", "split_terminal.sh")
	output := &bytes.Buffer{}
	workflow := DefaultMacroWorkflow{Output: output, UI: &fakeUI{answers: map[string]any{"macro-download-directory": root}}}
	macro := Macro{Subpackage: "vscode", Title: "VS Code split terminal", SourcePath: source}
	if err := workflow.Execute(MacroSelection{Macro: macro, Action: MacroActionDownload}); err != nil {
		t.Fatal(err)
	}
	downloaded := filepath.Join(root, "split_terminal.sh")
	archive := filepath.Join(root, "captured.vsix")
	config := filepath.Join(root, "config")
	user := filepath.Join(config, "Code", "User")
	if err := os.MkdirAll(user, 0700); err != nil {
		t.Fatal(err)
	}
	bindings := []byte("// user comment\n[{\"key\":\"ctrl+alt+s\",\"command\":\"other.command\",},]\n")
	bindingPath := filepath.Join(user, "keybindings.json")
	if err := os.WriteFile(bindingPath, bindings, 0600); err != nil {
		t.Fatal(err)
	}
	run := func(extra ...string) (string, error) {
		t.Helper()
		command := exec.Command("sh", downloaded)
		command.Dir = root
		command.Env = append(os.Environ(), "HOME="+root, "XDG_CONFIG_HOME="+config, "XDG_SESSION_TYPE=x11", "DISPLAY=:99", "WAYLAND_DISPLAY=", "TB_OUTPUT_CLOCK=09:32:23", "TB_OUTPUT_COLOR=never", "PATH="+bin+":"+os.Getenv("PATH"), "TEST_VSIX="+archive)
		command.Env = append(command.Env, extra...)
		result, err := command.CombinedOutput()
		return string(result), err
	}
	for attempt := 0; attempt < 2; attempt++ {
		result, err := run()
		if err != nil {
			t.Fatalf("installation %d: %v: %s", attempt, err, result)
		}
		if strings.Count(result, " SUCCESS]") != 1 || !strings.HasPrefix(result, "\n[09:32:23 vscode-split-terminal INFO]") || !strings.Contains(result, " WARN]") || !strings.Contains(result, "Reload") {
			t.Fatal(result)
		}
		reader, err := zip.OpenReader(archive)
		if err != nil {
			t.Fatal(err)
		}
		files := map[string][]byte{}
		for _, file := range reader.File {
			if file.FileInfo().IsDir() {
				continue
			}
			handle, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			files[file.Name], err = io.ReadAll(handle)
			handle.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
		reader.Close()
		var manifest struct{ Name, Publisher string }
		if err := json.Unmarshal(files["extension/package.json"], &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest.Name != "split-terminal" || manifest.Publisher != "my-toolbox" || len(files["extension/extension.js"]) == 0 || len(files["extension.vsixmanifest"]) == 0 || len(files["[Content_Types].xml"]) == 0 {
			t.Fatalf("invalid package: %v", files)
		}
	}
	after, err := os.ReadFile(bindingPath)
	if err != nil || !bytes.Equal(after, bindings) {
		t.Fatal("installer changed user bindings")
	}
	for _, extra := range [][]string{{"TEST_CLI_FAIL=yes"}, {"XDG_SESSION_TYPE=wayland"}, {"DISPLAY="}} {
		result, err := run(extra...)
		if err == nil || strings.Contains(result, " SUCCESS]") || !strings.Contains(result, " ERROR]") {
			t.Fatalf("failure handling: %v: %s", err, result)
		}
	}
	if err := os.Remove(filepath.Join(bin, "wmctrl")); err != nil {
		t.Fatal(err)
	}
	result, err := run("PATH=" + bin)
	if err == nil || !strings.Contains(result, "Missing: wmctrl") || !strings.Contains(result, "sudo apt install") {
		t.Fatalf("missing dependency: %v: %s", err, result)
	}
}
