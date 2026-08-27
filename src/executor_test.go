package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type unusedBuiltins struct{}

func (unusedBuiltins) SkipReason(string) (string, error) {
	return "", nil
}

func (unusedBuiltins) Run(string, []string) error {
	return nil
}

func TestInteractiveCatalogCommandIsReadyWithoutDefaultArguments(t *testing.T) {
	catalog, err := LoadCatalogFile("../commands.json")
	if err != nil {
		t.Fatal(err)
	}
	command, exists := catalog.Find("setup-agents-codex")
	if !exists {
		t.Fatal("setup-agents-codex is missing")
	}
	executor := ProcessExecutor{Builtins: unusedBuiltins{}}
	response, err := executor.Questions(command, map[string]any{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != "ready" {
		t.Fatalf("response = %#v, want ready", response)
	}
}

func TestInteractiveCommandRejectsDirectUserArguments(t *testing.T) {
	command := testInteractiveCommand(nil)
	executor := ProcessExecutor{Builtins: unusedBuiltins{}}
	if _, err := executor.Questions(command, map[string]any{}, []string{"unexpected"}); err == nil || !strings.Contains(err.Error(), "does not accept arguments") {
		t.Fatalf("Questions() error = %v", err)
	}
}

func TestInteractiveCommandRequestsAndValidatesDefaultModeConfirmation(t *testing.T) {
	command := testInteractiveCommand([]string{"--defaults"})
	executor := ProcessExecutor{Builtins: unusedBuiltins{}}
	response, err := executor.Questions(command, map[string]any{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != "question" || response.Question == nil {
		t.Fatalf("response = %#v, want question", response)
	}
	if response.Question.ID != "use-default-arguments" || response.Question.Type != "confirm" || response.Question.Title != "Use all default values for setup?" {
		t.Fatalf("question = %#v", response.Question)
	}
	answers := map[string]any{response.Question.ID: "yes"}
	if _, err := executor.Questions(command, answers, nil); err == nil || !strings.Contains(err.Error(), "boolean") {
		t.Fatalf("Questions() error = %v, want boolean validation", err)
	}
	for _, confirmed := range []bool{false, true} {
		response, err := executor.Questions(command, map[string]any{"use-default-arguments": confirmed}, nil)
		if err != nil || response.Status != "ready" {
			t.Fatalf("Questions(%t) = %#v, %v, want ready", confirmed, response, err)
		}
	}
}

func TestInteractiveCommandForwardsDefaultArgumentsOnlyWhenConfirmed(t *testing.T) {
	root := t.TempDir()
	writeInteractiveTestScript(t, root)
	command := testInteractiveCommand([]string{"--defaults"})

	for _, test := range []struct {
		name   string
		answer bool
		want   string
	}{
		{name: "confirmed", answer: true, want: "arguments=--defaults\n"},
		{name: "declined", answer: false, want: "arguments=\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			stdout := &bytes.Buffer{}
			executor := ProcessExecutor{
				Root:     root,
				Platform: "linux-amd64",
				Builtins: unusedBuiltins{},
				Input:    strings.NewReader("input\n"),
				Output:   stdout,
				Error:    io.Discard,
			}
			answers := map[string]any{"use-default-arguments": test.answer}
			if err := executor.Run(command, answers, nil); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(stdout.String(), test.want) {
				t.Fatalf("stdout = %q, want containing %q", stdout.String(), test.want)
			}
		})
	}
}

func TestInteractiveCommandUsesAttachedStandardStreams(t *testing.T) {
	root := t.TempDir()
	writeInteractiveTestScript(t, root)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	executor := ProcessExecutor{
		Root:     root,
		Platform: "linux-amd64",
		Builtins: unusedBuiltins{},
		Input:    strings.NewReader("terminal input\n"),
		Output:   stdout,
		Error:    stderr,
	}
	if err := executor.Run(testInteractiveCommand(nil), map[string]any{}, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "stdin=terminal input") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "arguments=\n") {
		t.Fatalf("stdout contains unexpected script arguments: %q", stdout.String())
	}
	if stderr.String() != "script stderr\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestInteractiveCommandSelectsPlatformInterpreterAndFallbackScript(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	interpreter := []byte("#!/bin/sh\nprintf 'arguments=%s\\n' \"$*\"\n")
	for _, name := range []string{"python2.7", "py"} {
		if err := os.WriteFile(filepath.Join(bin, name), interpreter, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	command := testInteractiveCommand(nil)
	command.Entrypoints["windows-amd64"] = []string{"python-script", "windows.py"}

	for _, test := range []struct {
		name     string
		platform string
		want     string
	}{
		{name: "Linux Python 2.7 fallback", platform: "linux-amd64", want: filepath.Join(root, "script.py") + "\n"},
		{name: "Windows py launcher", platform: "windows-amd64", want: "-3 " + filepath.Join(root, "windows.py") + "\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			stdout := &bytes.Buffer{}
			executor := ProcessExecutor{
				Root:     root,
				Platform: test.platform,
				Builtins: unusedBuiltins{},
				Input:    strings.NewReader(""),
				Output:   stdout,
				Error:    io.Discard,
			}
			if err := executor.Run(command, map[string]any{}, nil); err != nil {
				t.Fatal(err)
			}
			if stdout.String() != "arguments="+test.want {
				t.Fatalf("stdout = %q, want %q", stdout.String(), "arguments="+test.want)
			}
		})
	}
}

func testInteractiveCommand(defaultArguments []string) Command {
	return Command{
		Name:             "setup",
		Package:          "test",
		Visibility:       "list",
		Protocol:         "interactive-python",
		DefaultArguments: defaultArguments,
		Entrypoints: map[string][]string{
			"linux-amd64": {"python-script", "script.py", "script.py"},
		},
	}
}

func writeInteractiveTestScript(t *testing.T, root string) {
	t.Helper()
	script := []byte("import sys\nvalue = input()\nprint('stdin=' + value)\nprint('arguments=' + ' '.join(sys.argv[1:]))\nprint('script stderr', file=sys.stderr)\n")
	if err := os.WriteFile(filepath.Join(root, "script.py"), script, 0o644); err != nil {
		t.Fatal(err)
	}
}
