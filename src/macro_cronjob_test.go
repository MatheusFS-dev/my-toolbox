package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type retryTimeUI struct {
	fakeUI
	values []string
	asked  int
}

func (ui *retryTimeUI) Ask(question Question) (any, error) {
	ui.asked++
	if len(ui.values) == 0 {
		return nil, ErrCancelled
	}
	value := ui.values[0]
	ui.values = ui.values[1:]
	return value, nil
}

func TestCronjobInvalidTimePromptsAgain(t *testing.T) {
	for _, ending := range []string{"08:30", "", "cancel"} {
		t.Run(ending, func(t *testing.T) {
			values := []string{"25:00", "bad"}
			if ending != "cancel" {
				values = append(values, ending)
			}
			ui := &retryTimeUI{values: values}
			output := &bytes.Buffer{}
			workflow := DefaultMacroWorkflow{UI: ui, Output: output}
			arguments, err := workflow.macroArguments(Macro{Arguments: []MacroArgument{{ID: "time", Prompt: "Daily local time (HH:MM)", Type: "time_24h", Default: "07:00"}}})
			if ending == "cancel" {
				if !errors.Is(err, ErrCancelled) {
					t.Fatalf("error = %v", err)
				}
			} else {
				want := ending
				if want == "" {
					want = "07:00"
				}
				if err != nil || len(arguments) != 1 || arguments[0] != want {
					t.Fatalf("arguments = %v, error = %v", arguments, err)
				}
			}
			if ui.asked != 3 || strings.Count(output.String(), "must be a 24-hour time HH:MM") != 2 {
				t.Fatalf("prompts = %d, output = %q", ui.asked, output.String())
			}
		})
	}
}

func TestCronjobWorkflowRunsInstallerInForeground(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh unavailable")
	}
	root := t.TempDir()
	script := filepath.Join(root, "installer.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n[ \"$1\" = --check ] && exit 0\nprintf 'installed %s' \"$1\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	output := &bytes.Buffer{}
	workflow := DefaultMacroWorkflow{GOOS: "linux", Output: output, UI: &fakeUI{answers: map[string]any{"macro-time": ""}}}
	macro := Macro{Subpackage: "cronjob", SourcePath: script, Arguments: []MacroArgument{{ID: "time", Prompt: "Time", Type: "time_24h", Default: "07:00"}}}
	if err := workflow.Execute(MacroSelection{Macro: macro, Action: MacroActionRun}); err != nil {
		t.Fatal(err)
	}
	if output.String() != "installed 07:00" {
		t.Fatalf("installer output = %q", output.String())
	}
	workflow.GOOS = "windows"
	if err := workflow.Execute(MacroSelection{Macro: macro, Action: MacroActionRun}); err == nil {
		t.Fatal("allowed cronjob on Windows")
	}
}

func TestCronjobMetadataAndTime(t *testing.T) {
	macros, err := loadMacros(filepath.Join("..", "packages", "macros"), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, macro := range macros {
		if macro.Subpackage == "cronjob" {
			found = true
		}
	}
	if !found {
		t.Fatal("cronjob macro missing")
	}
	for _, value := range []string{"07:00", "00:00", "23:59"} {
		if err := validateMacroArgument(MacroArgument{Type: "time_24h"}, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []string{"7:00", "24:00", "12:60", "aa:bb"} {
		if err := validateMacroArgument(MacroArgument{Type: "time_24h"}, value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}

func TestCronjobScriptValidation(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh unavailable")
	}
	script := filepath.Join("..", "packages", "macros", "cronjob", "codex_hi.sh")
	if out, err := exec.Command(shell, script, "--check", "07:00").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	for _, value := range []string{"7:00", "24:00", "12:60", "$(touch bad)"} {
		if err := exec.Command(shell, script, "--check", value).Run(); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}
