package macros

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMultiOSMacroDownloadsEveryVariant(t *testing.T) {
	source := t.TempDir()
	linux := filepath.Join(source, "tool.sh")
	windows := filepath.Join(source, "tool.ps1")
	for path, content := range map[string]string{linux: "linux", windows: "windows"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	macro := Macro{Subpackage: "vscode", Title: "Tool", SourcePath: linux, Variants: map[string]string{"linux": linux, "windows": windows}}
	for goos, want := range map[string]bool{"linux": true, "windows": true, "darwin": false} {
		if MacroSupportsOS(macro, goos) != want {
			t.Fatalf("support for %s should be %v", goos, want)
		}
	}
	destination := t.TempDir()
	workflow := DefaultMacroWorkflow{GOOS: "darwin", UI: &fakeUI{answers: map[string]any{"macro-download-directory": destination}}}
	if err := workflow.Execute(MacroSelection{Macro: macro, Action: MacroActionDownload}); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"tool.sh": "linux", "tool.ps1": "windows"} {
		got, err := os.ReadFile(filepath.Join(destination, name))
		if err != nil || string(got) != want {
			t.Fatalf("%s: %q, %v", name, got, err)
		}
	}
}
