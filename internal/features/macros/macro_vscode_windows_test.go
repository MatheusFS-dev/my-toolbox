package macros

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

const vscodeWindowsMacroRoot = "../../../assets/payload/packages/macros"

func TestVSCodeWindowsDiscovery(t *testing.T) {
	library, err := LoadMacros(filepath.FromSlash(vscodeWindowsMacroRoot), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, macro := range library {
		if macro.Subpackage != "vscode" {
			continue
		}
		if macro.Title != "VS Code split terminal" || !MacroSupportsOS(macro, "windows") || !MacroSupportsOS(macro, "linux") || MacroSupportsOS(macro, "darwin") {
			t.Fatalf("invalid macro: %+v", macro)
		}
		if !strings.HasSuffix(macro.ScriptFor("windows"), "split_terminal.ps1") || !strings.HasSuffix(macro.ScriptFor("linux"), "split_terminal.sh") || len(macro.DownloadPaths()) != 2 {
			t.Fatalf("invalid variants: %+v", macro.Variants)
		}
		workflow := DefaultMacroWorkflow{GOOS: "darwin", Output: &bytes.Buffer{}}
		if err := workflow.Execute(MacroSelection{Macro: macro, Action: MacroActionRun}); err == nil {
			t.Fatal("ran installer on an unsupported system")
		}
		return
	}
	t.Fatal("Windows VS Code macro missing")
}

// Removing the embedded payloads or the output helper must break this test.
func TestVSCodeWindowsInstaller(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows installer")
	}
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("powershell.exe unavailable")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(bin, "wt.cmd"), "@exit /b 0\r\n")
	write(filepath.Join(bin, "code.cmd"), "@echo off\r\nif not \"%1\"==\"--install-extension\" exit /b 4\r\nif not \"%3\"==\"--force\" exit /b 4\r\nif \"%TEST_CLI_FAIL%\"==\"yes\" exit /b 7\r\ncopy /y \"%~2\" \"%TEST_VSIX%\" >nul\r\n")
	appData := filepath.Join(root, "appdata")
	user := filepath.Join(appData, "Code", "User")
	if err := os.MkdirAll(user, 0700); err != nil {
		t.Fatal(err)
	}
	bindings := []byte("// user comment\r\n[{\"key\":\"ctrl+alt+s\",\"command\":\"other.command\",},]\r\n")
	bindingPath := filepath.Join(user, "keybindings.json")
	write(bindingPath, string(bindings))
	archive := filepath.Join(root, "captured.vsix")
	source, err := filepath.Abs(filepath.Join(filepath.FromSlash(vscodeWindowsMacroRoot), "vscode", "split_terminal.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	path := bin + string(os.PathListSeparator) + filepath.Dir(powershell) + string(os.PathListSeparator) + os.Getenv("PATH")
	run := func(pathValue string, extra ...string) (string, error) {
		t.Helper()
		command := exec.Command(powershell, "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", source)
		command.Dir = root
		command.Env = append(os.Environ(), "APPDATA="+appData, "PATH="+pathValue, "TEST_VSIX="+archive)
		command.Env = append(command.Env, extra...)
		result, err := command.CombinedOutput()
		return string(result), err
	}
	for attempt := 0; attempt < 2; attempt++ {
		result, err := run(path)
		if err != nil {
			t.Fatalf("installation %d: %v: %s", attempt, err, result)
		}
		if strings.Count(result, " SUCCESS]") != 1 || !regexp.MustCompile(`^\r?\n\[\d\d:\d\d:\d\d vscode-split-terminal INFO\]`).MatchString(result) || !strings.Contains(result, " WARN]") || !strings.Contains(result, "Reload") {
			t.Fatal(result)
		}
		reader, err := zip.OpenReader(archive)
		if err != nil {
			t.Fatal(err)
		}
		files := map[string][]byte{}
		for _, file := range reader.File {
			if strings.Contains(file.Name, `\`) {
				t.Fatalf("backslash in archive entry %q", file.Name)
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
		var manifest struct {
			Name, Publisher string
			Contributes     struct{ Keybindings []struct{ Key, When string } }
		}
		if err := json.Unmarshal(files["extension/package.json"], &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest.Name != "split-terminal" || manifest.Publisher != "my-toolbox" || len(manifest.Contributes.Keybindings) != 1 || manifest.Contributes.Keybindings[0].When != "isWindows" {
			t.Fatalf("invalid manifest: %+v", manifest)
		}
		for _, name := range []string{"extension/extension.js", "extension/split.ps1", "extension.vsixmanifest", "[Content_Types].xml"} {
			if len(files[name]) == 0 {
				t.Fatalf("missing %s in %v", name, files)
			}
		}
	}
	after, err := os.ReadFile(bindingPath)
	if err != nil || !bytes.Equal(after, bindings) {
		t.Fatal("installer changed user bindings")
	}
	result, err := run(path, "TEST_CLI_FAIL=yes")
	if err == nil || strings.Contains(result, " SUCCESS]") || !strings.Contains(result, " ERROR]") {
		t.Fatalf("failure handling: %v: %s", err, result)
	}
	result, err = run(bin)
	if err == nil || !strings.Contains(result, "Missing:") || !strings.Contains(result, " ERROR]") {
		t.Fatalf("missing dependency: %v: %s", err, result)
	}
}
