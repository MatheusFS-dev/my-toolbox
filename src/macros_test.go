package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func writeTestMacro(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMacroSearchAndModalActions(t *testing.T) {
	macros := []Macro{{Subpackage: "autohotkey", Title: "Timer", Description: "Wait", RelativePath: "autohotkey/timer.ahk", Content: "unique needle"}}
	if results := searchMacros(macros, "needle"); len(results) != 1 {
		t.Fatalf("results = %#v", results)
	}
	if results := searchMacros(macros, "absent"); len(results) != 0 {
		t.Fatalf("results = %#v", results)
	}
	model := newMacroModel(macros, true, 72, 24)
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(macroModel)
	if !model.modal || !strings.Contains(model.View().Content, "Run") || !strings.Contains(model.View().Content, "Download") {
		t.Fatalf("modal view = %q", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(macroModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(macroModel)
	selection, err := model.result()
	if err != nil || selection.Action != MacroActionDownload {
		t.Fatalf("selection = %#v, %v", selection, err)
	}
}

func TestMacroModalDisablesRunOnLinuxARM64(t *testing.T) {
	model := newMacroModel([]Macro{{Subpackage: "autohotkey", Title: "X"}}, false, 30, 8)
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(macroModel)
	if !strings.Contains(model.View().Content, "unavailable on Linux ARM64") {
		t.Fatalf("view = %q", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(macroModel)
	selection, err := model.result()
	if err != nil || selection.Action != MacroActionDownload {
		t.Fatalf("selection = %#v, %v", selection, err)
	}
}

func TestDownloadMacroExpandsHomeAndCopiesOnlyScript(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	writeTestMacro(t, sourceRoot, "macro.ahk", "script\n")
	ui := &fakeUI{answers: map[string]any{"macro-download-directory": "~"}}
	output := &bytes.Buffer{}
	workflow := DefaultMacroWorkflow{UI: ui, Home: home, Output: output}
	if err := workflow.Execute(MacroSelection{Macro: Macro{Title: "Macro", SourcePath: filepath.Join(sourceRoot, "macro.ahk")}, Action: MacroActionDownload}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(home, "macro.ahk"))
	if err != nil || string(content) != "script\n" {
		t.Fatalf("download = %q, %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(home, "macro.json")); !os.IsNotExist(err) {
		t.Fatalf("metadata was copied: %v", err)
	}
}

func TestProbeAutoHotkeyRejectsV1AndAcceptsV2(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	root := t.TempDir()
	v1 := filepath.Join(root, "v1")
	v2 := filepath.Join(root, "v2")
	if err := os.WriteFile(v1, []byte("#!/bin/sh\nprintf 1.1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v2, []byte("#!/bin/sh\nprintf 2.0.26\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := probeAutoHotkeyV2(v1); err == nil {
		t.Fatal("v1 accepted")
	}
	if err := probeAutoHotkeyV2(v2); err != nil {
		t.Fatal(err)
	}
}

func TestRunMacroUsesOrderedArgumentsAndDetachedProcess(t *testing.T) {
	ui := &fakeUI{answers: map[string]any{"macro-key": "Space", "macro-delay": "25"}}
	var gotRuntime, gotScript string
	var gotArguments []string
	workflow := DefaultMacroWorkflow{UI: ui, GOOS: "linux", GOARCH: "amd64", Output: &bytes.Buffer{}, FindRuntime: func(string, string) (string, error) { return "/bin/ahk", nil }, StartDetached: func(runtimePath, scriptPath string, arguments []string) (int, error) {
		gotRuntime = runtimePath
		gotScript = scriptPath
		gotArguments = append([]string(nil), arguments...)
		return 42, nil
	}}
	macro := Macro{Title: "Timer", SourcePath: "/macros/timer.ahk", Arguments: []MacroArgument{{ID: "key", Prompt: "Key", Type: "text", Default: "Enter"}, {ID: "delay", Prompt: "Delay", Type: "non_negative_integer", Default: "100"}}}
	if err := workflow.Execute(MacroSelection{Macro: macro, Action: MacroActionRun}); err != nil {
		t.Fatal(err)
	}
	if gotRuntime != "/bin/ahk" || gotScript != "/macros/timer.ahk" || !reflect.DeepEqual(gotArguments, []string{"Space", "25"}) {
		t.Fatalf("launch = %q %q %#v", gotRuntime, gotScript, gotArguments)
	}
}

func TestRunMacroInstallsThenRequiresRerun(t *testing.T) {
	ui := &fakeUI{answers: map[string]any{"install-autohotkey": true}}
	probes := 0
	installs := 0
	starts := 0
	output := &bytes.Buffer{}
	workflow := DefaultMacroWorkflow{UI: ui, GOOS: "linux", GOARCH: "amd64", Output: output, FindRuntime: func(string, string) (string, error) {
		probes++
		if probes == 1 {
			return "", os.ErrNotExist
		}
		return "/bin/ahk", nil
	}, InstallRuntime: func(string, string, string) error { installs++; return nil }, StartDetached: func(string, string, []string) (int, error) { starts++; return 1, nil }}
	if err := workflow.Execute(MacroSelection{Macro: Macro{Title: "X"}, Action: MacroActionRun}); err != nil {
		t.Fatal(err)
	}
	if probes != 2 || installs != 1 || starts != 0 || !strings.Contains(output.String(), "again") {
		t.Fatalf("probes=%d installs=%d starts=%d output=%q", probes, installs, starts, output.String())
	}
}

func TestLoadMacrosDiscoversRegisteredAHKRecursivelyAndSorts(t *testing.T) {
	root := t.TempDir()
	writeTestMacro(t, root, "unregistered/ignored.ahk", "ignored")
	writeTestMacro(t, root, "autohotkey/nested/z_last.ahk", "z")
	writeTestMacro(t, root, "autohotkey/a_first.AHK", "a")
	macros, err := loadMacros(root, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if len(macros) != 2 || macros[0].Title != "A First" || macros[1].RelativePath != "autohotkey/nested/z_last.ahk" {
		t.Fatalf("macros = %#v", macros)
	}
	if len(macros[0].Arguments) != 0 {
		t.Fatalf("sidecar-free arguments = %#v", macros[0].Arguments)
	}
}

func TestLoadMacrosUsesStrictSidecarsAndSkipsInvalidEntries(t *testing.T) {
	root := t.TempDir()
	writeTestMacro(t, root, "autohotkey/good.ahk", "good")
	writeTestMacro(t, root, "autohotkey/good.json", `{"title":"Good macro","description":"Does good.","arguments":[{"id":"delay","prompt":"Delay","type":"non_negative_integer","default":"0"}]}`)
	writeTestMacro(t, root, "autohotkey/bad.ahk", "bad")
	writeTestMacro(t, root, "autohotkey/bad.json", `{"title":"Bad","unknown":true}`)
	warnings := &bytes.Buffer{}
	macros, err := loadMacros(root, warnings)
	if err != nil {
		t.Fatal(err)
	}
	if len(macros) != 1 || macros[0].Title != "Good macro" || macros[0].Arguments[0].Default != "0" {
		t.Fatalf("macros = %#v", macros)
	}
	if !strings.Contains(warnings.String(), "warning: skip macro autohotkey/bad.ahk") {
		t.Fatalf("warnings = %q", warnings.String())
	}
}

func TestLoadMacrosRejectsInvalidArgumentMetadata(t *testing.T) {
	cases := []string{
		`{"title":"X","arguments":[{"id":"a","prompt":"A","type":"text","default":""}]}`,
		`{"title":"X","arguments":[{"id":"a","prompt":"A","type":"wat","default":"x"}]}`,
		`{"title":"X","arguments":[{"id":"a","prompt":"A","type":"text","default":"x"},{"id":"a","prompt":"B","type":"text","default":"y"}]}`,
		`{"title":"X","arguments":[{"id":"a","prompt":"A","type":"non_negative_integer","default":"-1"}]}`,
	}
	for index, metadata := range cases {
		root := t.TempDir()
		writeTestMacro(t, root, "autohotkey/x.ahk", "x")
		writeTestMacro(t, root, "autohotkey/x.json", metadata)
		if _, err := loadMacros(root, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "contains no valid macros") {
			t.Fatalf("case %d error = %v", index, err)
		}
	}
}

func TestValidateMacroArgument(t *testing.T) {
	if err := validateMacroArgument(MacroArgument{Type: "text"}, " "); err == nil {
		t.Fatal("empty text accepted")
	}
	if err := validateMacroArgument(MacroArgument{Type: "non_negative_integer"}, "12"); err != nil {
		t.Fatal(err)
	}
	if err := validateMacroArgument(MacroArgument{Type: "non_negative_integer"}, "-1"); err == nil {
		t.Fatal("negative integer accepted")
	}
}
