package main

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

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
	model.goos = "windows"
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

func TestMacroBrowserStylesFocusedTitleBlue(t *testing.T) {
	model := newMacroModel([]Macro{
		{Subpackage: "autohotkey", Title: "Focused macro"},
		{Subpackage: "autohotkey", Title: "Other macro"},
	}, true, 72, 24)
	view := model.View().Content
	if !strings.Contains(view, presentationStyle("Focused macro", ansiBlue, true)) {
		t.Fatalf("focused title is not blue: %q", view)
	}
	if strings.Contains(view, presentationStyle("Other macro", ansiBlue, true)) {
		t.Fatalf("unfocused title is blue: %q", view)
	}
}

func TestMacroModalDisablesUnsupportedRun(t *testing.T) {
	model := newMacroModel([]Macro{{Subpackage: "autohotkey", Title: "X"}}, false, 30, 8)
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(macroModel)
	if !strings.Contains(model.View().Content, "unavailable on this OS") {
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
	workflow := DefaultMacroWorkflow{UI: ui, GOOS: "windows", GOARCH: "amd64", Output: &bytes.Buffer{}, FindRuntime: func(string, string) (string, error) { return "/bin/ahk", nil }, StartDetached: func(runtimePath, scriptPath string, arguments []string) (int, error) {
		gotRuntime = runtimePath
		gotScript = scriptPath
		gotArguments = append([]string(nil), arguments...)
		return 42, nil
	}}
	macro := Macro{Subpackage: "autohotkey", Title: "Timer", SourcePath: "/macros/timer.ahk", Arguments: []MacroArgument{{ID: "key", Prompt: "Key", Type: "text", Default: "Enter"}, {ID: "delay", Prompt: "Delay", Type: "non_negative_integer", Default: "100"}}}
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
	workflow := DefaultMacroWorkflow{UI: ui, GOOS: "windows", GOARCH: "amd64", Output: output, FindRuntime: func(string, string) (string, error) {
		probes++
		if probes == 1 {
			return "", os.ErrNotExist
		}
		return "/bin/ahk", nil
	}, InstallRuntime: func(string, string, string) error { installs++; return nil }, StartDetached: func(string, string, []string) (int, error) { starts++; return 1, nil }}
	if err := workflow.Execute(MacroSelection{Macro: Macro{Subpackage: "autohotkey", Title: "X"}, Action: MacroActionRun}); err != nil {
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

func TestMacroSupportedOSLabels(t *testing.T) {
	model := newMacroModel([]Macro{{Subpackage: "autohotkey", Title: "Windows timer", Description: "Wait then press."}, {Subpackage: "ydotool", Title: "Linux timer", Description: "Wait then press."}}, true, 72, 24)
	view := model.View().Content
	lines := strings.Split(view, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	view = strings.Join(lines, "\n")
	for _, label := range []string{"Supported OSs: Windows", "Supported OSs: Linux"} {
		if !strings.Contains(view, "Wait then press.\n      "+presentationStyle(label, ansiBrightRed, true)) {
			t.Fatalf("missing red OS label below description: %q", view)
		}
	}
}

func TestMacroRejectsUnsupportedOSBeforeRuntimeLookup(t *testing.T) {
	for _, item := range []struct{ driver, goos string }{{"autohotkey", "linux"}, {"ydotool", "windows"}, {"unknown", "linux"}} {
		workflow := DefaultMacroWorkflow{GOOS: item.goos, FindRuntime: func(string, string) (string, error) { t.Fatal("looked up runtime for unsupported OS"); return "", nil }}
		if err := workflow.Execute(MacroSelection{Macro: Macro{Subpackage: item.driver}, Action: MacroActionRun}); err == nil {
			t.Fatal("unsupported macro allowed")
		}
	}
}

func TestLoadYdotoolMacro(t *testing.T) {
	macros, err := loadMacros(filepath.Join("..", "packages", "macros"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, macro := range macros {
		if macro.Subpackage == "ydotool" && len(macro.Arguments) == 2 {
			return
		}
	}
	t.Fatal("bundled ydotool timer missing")
}

func TestYdotoolTimerScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Linux shell macro")
	}
	root := t.TempDir()
	log := filepath.Join(root, "calls")
	t.Setenv("MACRO_TEST_LOG", log)
	for name, content := range map[string]string{
		"ydotool": "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$MACRO_TEST_LOG\"\n",
		"sleep":   "#!/bin/sh\nprintf 'sleep %s\\n' \"$*\" >> \"$MACRO_TEST_LOG\"\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	script := filepath.Join("..", "packages", "macros", "ydotool", "press_key_after_x_ms.sh")
	output, err := exec.Command("sh", script, "Enter", "25").CombinedOutput()
	if err != nil {
		t.Fatalf("timer: %v: %s", err, output)
	}
	calls, _ := os.ReadFile(log)
	if string(calls) != "sleep 0.025\nkey 28:1 28:0\n" {
		t.Fatalf("calls = %q", calls)
	}
	for _, args := range [][]string{{"invalid", "0"}, {"Enter", "-1"}, {"Enter", "oops"}, {"Enter", "0", "extra"}, {"Enter", "9223372036854775808"}, {"768", "0"}, {"0", "0"}} {
		os.Remove(log)
		if err := exec.Command("sh", append([]string{script}, args...)...).Run(); err == nil {
			t.Fatalf("accepted %q", args)
		}
		if _, err := os.Stat(log); !os.IsNotExist(err) {
			t.Fatal("invalid input caused side effects")
		}
	}
}

func TestMacroModalSelectsRunByOS(t *testing.T) {
	for _, goos := range []string{"linux", "windows"} {
		for _, driver := range []string{"ydotool", "autohotkey"} {
			model := newMacroModel([]Macro{{Subpackage: driver}}, true, 72, 24)
			model.goos = goos
			updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			model = updated.(macroModel)
			updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			selection, err := updated.(macroModel).result()
			want := MacroActionDownload
			if goos == "linux" && driver == "ydotool" || goos == "windows" && driver == "autohotkey" {
				want = MacroActionRun
			}
			if err != nil || selection.Action != want {
				t.Fatalf("%s/%s: %v, %v", goos, driver, selection, err)
			}
		}
	}
}

func TestYdotoolRuntimeAndDaemonChecks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixtures")
	}
	root := t.TempDir()
	bin := filepath.Join(root, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	path := filepath.Join(bin, "ydotool")
	for _, item := range []struct {
		script       string
		found, ready bool
	}{
		{"#!/bin/sh\necho legacy\n", false, false},
		{"#!/bin/sh\nif [ \"$1\" = --help ]; then echo YDOTOOL_SOCKET; else echo missing socket; exit 2; fi\n", true, false},
		{"#!/bin/sh\nif [ \"$1\" = --help ]; then echo YDOTOOL_SOCKET; elif [ \"$1\" = key ] && [ \"$#\" = 1 ]; then echo keycode; else exit 9; fi\n", true, true},
	} {
		if err := os.WriteFile(path, []byte(item.script), 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := findYdotool(root)
		if (err == nil) != item.found {
			t.Fatalf("runtime: %v", err)
		}
		if item.found {
			err := probeYdotool(path)
			if (err == nil) != item.ready {
				t.Fatalf("daemon: %v", err)
			}
		}
	}
}

func TestRunYdotoolSchedulesAndLogsErrors(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux macro")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			home := t.TempDir()
			bin := filepath.Join(home, ".local", "bin")
			if err := os.MkdirAll(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			script := "#!/bin/sh\nif [ \"$1\" = --help ]; then echo YDOTOOL_SOCKET; elif [ \"$#\" = 1 ]; then echo keycode; else echo \"$*\" >&2; exit 3; fi\n"
			if err := os.WriteFile(filepath.Join(bin, "ydotool"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			source, err := filepath.Abs(filepath.Join("..", "packages", "macros", "ydotool", "press_key_after_x_ms.sh"))
			if err != nil {
				t.Fatal(err)
			}
			macro, err := loadMacro(filepath.Dir(filepath.Dir(source)), "ydotool", source)
			if err != nil {
				t.Fatal(err)
			}
			output := &bytes.Buffer{}
			ui := &fakeUI{answers: map[string]any{"macro-key": "Space", "macro-delay_ms": "0"}}
			workflow := DefaultMacroWorkflow{UI: ui, GOOS: "linux", GOARCH: arch, Home: home, Output: output}
			if err := workflow.Execute(MacroSelection{Macro: macro, Action: MacroActionRun}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), "Scheduled") {
				t.Fatalf("output: %s", output)
			}
			logs, err := filepath.Glob(filepath.Join(home, ".local", "state", "my-toolbox", "macros", "*.log"))
			if err != nil || len(logs) != 1 {
				t.Fatalf("logs: %v, %v", logs, err)
			}
			deadline := time.Now().Add(3 * time.Second)
			for {
				content, err := os.ReadFile(logs[0])
				if err != nil {
					t.Fatal(err)
				}
				if string(content) == "key 57:1 57:0\n" {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("background error not logged: %q", content)
				}
				time.Sleep(10 * time.Millisecond)
			}
			ui.answers["macro-key"] = "invalid"
			output.Reset()
			if err := workflow.Execute(MacroSelection{Macro: macro, Action: MacroActionRun}); err == nil || !strings.Contains(err.Error(), "Unsupported key") {
				t.Fatalf("invalid key: %v", err)
			}
			if output.Len() != 0 {
				t.Fatalf("invalid key reported launch: %s", output)
			}
		})
	}
}

func TestYdotoolInstallerRejectsCorruptDownload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Linux build tools")
	}
	bin := t.TempDir()
	for _, name := range []string{"cmake", "make", "cc"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 9\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	home := t.TempDir()
	workflow := DefaultMacroWorkflow{HTTP: &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://codeload.github.com/ReimuNotMoe/ydotool/tar.gz/refs/tags/v1.0.4" {
			t.Fatalf("download URL: %s", request.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("corrupt"))}, nil
	})}}
	if err := workflow.installYdotool(home); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("corrupt archive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "bin")); !os.IsNotExist(err) {
		t.Fatal("corrupt archive installed files")
	}
}
