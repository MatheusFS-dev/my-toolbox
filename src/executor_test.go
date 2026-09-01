package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

func TestPreflightAggregatesMissingCapabilitiesWithExactRemediation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test controls a POSIX PATH")
	}
	t.Setenv("PATH", t.TempDir())
	command := Command{
		Name: "setup", Protocol: "interactive-script", Elevation: "sudo",
		Environments: []string{"linux-native"},
	}
	executor := ProcessExecutor{Platform: "linux-amd64", Environment: "linux-native", Builtins: unusedBuiltins{}}
	err := executor.Preflight(command)
	if err == nil {
		t.Fatal("Preflight() succeeded")
	}
	for _, text := range []string{"- Bash", "Install Bash and ensure 'bash' is on PATH.", "- sudo", "Install sudo and ensure 'sudo' is on PATH."} {
		if !strings.Contains(err.Error(), text) {
			t.Fatalf("Preflight() error = %q, want containing %q", err, text)
		}
	}
}

func TestPreflightRequiresExactPython27TomlFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake Python executable is a POSIX shell script")
	}
	bin := t.TempDir()
	python := filepath.Join(bin, "python2.7")
	script := "#!/bin/sh\ncase \"$*\" in *toml*) exit 1;; *) exit 0;; esac\n"
	if err := os.WriteFile(python, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	command := Command{
		Name: "workspace", Protocol: "interactive-python", Environments: []string{"linux-native"},
		Entrypoints: map[string][]string{"linux-amd64": {"python-script", "python3.py", "python2.py"}},
	}
	executor := ProcessExecutor{Platform: "linux-amd64", Environment: "linux-native", Builtins: unusedBuiltins{}}
	err := executor.Preflight(command)
	if err == nil || !strings.Contains(err.Error(), "Python 3.11+, or Python 2.7 with toml==0.10.2") {
		t.Fatalf("Preflight() error = %v", err)
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
			output := strings.ReplaceAll(stdout.String(), "\r\n", "\n")
			if !strings.Contains(output, test.want) {
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
	standardOutput := strings.ReplaceAll(stdout.String(), "\r\n", "\n")
	standardError := strings.ReplaceAll(stderr.String(), "\r\n", "\n")
	if !strings.Contains(standardOutput, "stdin=terminal input") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(standardOutput, "arguments=\n") {
		t.Fatalf("stdout contains unexpected script arguments: %q", stdout.String())
	}
	if standardError != "script stderr\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestInteractiveBashScriptForwardsArgumentsAndAttachedStreams(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test requires a POSIX Bash executable")
	}
	root := t.TempDir()
	script := filepath.Join(root, "script.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nread value\nprintf 'stdin=%s\\n' \"$value\"\nprintf 'arguments=%s\\n' \"$*\"\nprintf 'script stderr\\n' >&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	command := Command{Name: "script", Protocol: "interactive-script", Entrypoints: map[string][]string{"linux-amd64": {"bash-script", "script.sh"}}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	executor := ProcessExecutor{Root: root, Platform: "linux-amd64", Builtins: unusedBuiltins{}, Input: strings.NewReader("terminal input\n"), Output: stdout, Error: stderr}

	if err := executor.Run(command, map[string]any{}, []string{"one", "two words"}); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "stdin=terminal input\narguments=one two words\n" || stderr.String() != "script stderr\n" {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestInteractiveSudoScriptUsesSudoBashSeparator(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake sudo executable is a POSIX shell script")
	}
	root := t.TempDir()
	bin := t.TempDir()
	recorder := filepath.Join(root, "arguments.txt")
	if err := os.WriteFile(filepath.Join(bin, "sudo"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$RECORDER\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("RECORDER", recorder)
	command := Command{Name: "script", Protocol: "interactive-script", Elevation: "sudo", Entrypoints: map[string][]string{"linux-amd64": {"bash-script", "script.sh"}}}
	executor := ProcessExecutor{Root: root, Platform: "linux-amd64", Builtins: unusedBuiltins{}, Input: strings.NewReader(""), Output: io.Discard, Error: io.Discard}

	if err := executor.Run(command, map[string]any{}, []string{"argument"}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(recorder)
	if err != nil {
		t.Fatal(err)
	}
	want := "--\nbash\n" + filepath.Join(root, "script.sh") + "\nargument\n"
	if string(content) != want {
		t.Fatalf("sudo arguments = %q, want %q", content, want)
	}
}

func TestInteractivePowerShellScriptUsesPortableInvocation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake PowerShell executable is a POSIX shell script")
	}
	root := t.TempDir()
	bin := t.TempDir()
	recorder := filepath.Join(root, "arguments.txt")
	if err := os.WriteFile(filepath.Join(bin, "powershell.exe"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$RECORDER\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("RECORDER", recorder)
	command := Command{Name: "script", Protocol: "interactive-script", Entrypoints: map[string][]string{"windows-amd64": {"powershell-script", "script.ps1"}}}
	executor := ProcessExecutor{Root: root, Platform: "windows-amd64", Builtins: unusedBuiltins{}, Input: strings.NewReader(""), Output: io.Discard, Error: io.Discard}

	if err := executor.Run(command, map[string]any{}, []string{"argument"}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(recorder)
	if err != nil {
		t.Fatal(err)
	}
	want := "-NoLogo\n-NoProfile\n-ExecutionPolicy\nBypass\n-File\n" + filepath.Join(root, "script.ps1") + "\nargument\n"
	if string(content) != want {
		t.Fatalf("PowerShell arguments = %q, want %q", content, want)
	}
}

func TestDetectEnvironmentDistinguishesNativeLinuxWSLAndWindows(t *testing.T) {
	root := t.TempDir()
	osrelease := filepath.Join(root, "osrelease")
	if err := os.WriteFile(osrelease, []byte("6.8.0-generic\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	environment, err := detectEnvironment("linux", osrelease)
	if err != nil || environment != "linux-native" {
		t.Fatalf("detectEnvironment(native) = %q, %v", environment, err)
	}
	if err := os.WriteFile(osrelease, []byte("5.15.153.1-microsoft-standard-WSL2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	environment, err = detectEnvironment("linux", osrelease)
	if err != nil || environment != "linux-wsl" {
		t.Fatalf("detectEnvironment(WSL) = %q, %v", environment, err)
	}
	environment, err = detectEnvironment("windows", osrelease)
	if err != nil || environment != "windows" {
		t.Fatalf("detectEnvironment(Windows) = %q, %v", environment, err)
	}
	if _, err := detectEnvironment("linux", filepath.Join(root, "missing")); err == nil || !strings.Contains(err.Error(), "detect Linux environment") {
		t.Fatalf("detectEnvironment(missing) error = %v", err)
	}
}

func TestInteractiveCommandSelectsPlatformInterpreterAndFallbackScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake Python launchers are POSIX shell scripts")
	}
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

func TestSelectPythonPrefersSupportedPython3OnLinux(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake Python interpreters are POSIX shell scripts")
	}
	bin := t.TempDir()
	writePythonProbe(t, bin, "python3", true)
	writePythonProbe(t, bin, "python2.7", true)
	t.Setenv("PATH", bin)

	interpreter, scriptIndex, err := selectPython("linux-amd64", []string{"python-script", "python3.py", "python2.py"})
	if err != nil {
		t.Fatal(err)
	}
	if interpreter[0] != filepath.Join(bin, "python3") || len(interpreter) != 1 || scriptIndex != 1 {
		t.Fatalf("selectPython() = %v, %d, want supported Python 3", interpreter, scriptIndex)
	}
}

func TestSelectPythonFallsBackWhenPython3IsOlderThan311(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake Python interpreters are POSIX shell scripts")
	}
	bin := t.TempDir()
	writePythonProbe(t, bin, "python3", false)
	writePythonProbe(t, bin, "python2.7", true)
	t.Setenv("PATH", bin)

	interpreter, scriptIndex, err := selectPython("linux-amd64", []string{"python-script", "python3.py", "python2.py"})
	if err != nil {
		t.Fatal(err)
	}
	if interpreter[0] != filepath.Join(bin, "python2.7") || len(interpreter) != 1 || scriptIndex != 2 {
		t.Fatalf("selectPython() = %v, %d, want Python 2.7 fallback", interpreter, scriptIndex)
	}
}

func TestSelectPythonReportsAcceptedLinuxVersions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake Python interpreter is a POSIX shell script")
	}
	bin := t.TempDir()
	writePythonProbe(t, bin, "python3", false)
	t.Setenv("PATH", bin)

	_, _, err := selectPython("linux-amd64", []string{"python-script", "python3.py", "python2.py"})
	if err == nil || !strings.Contains(err.Error(), "Python 3.11 or newer, or Python 2.7") {
		t.Fatalf("selectPython() error = %v, want accepted Linux versions", err)
	}
}

func TestSelectPythonRequiresPython311OnWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake Python launchers are POSIX shell scripts")
	}
	bin := t.TempDir()
	writePythonProbe(t, bin, "py", false)
	writePythonProbe(t, bin, "python", false)
	t.Setenv("PATH", bin)

	_, _, err := selectPython("windows-amd64", []string{"python-script", "windows.py"})
	if err == nil || !strings.Contains(err.Error(), "Python 3.11 or newer") {
		t.Fatalf("selectPython() error = %v, want Windows Python 3.11 requirement", err)
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

func writePythonProbe(t *testing.T, bin, name string, supported bool) {
	t.Helper()
	exitStatus := "1"
	if supported {
		exitStatus = "0"
	}
	script := []byte("#!/bin/sh\ncase \"$*\" in *' < (4, 0)'*) exit 1 ;; esac\nexit " + exitStatus + "\n")
	if err := os.WriteFile(filepath.Join(bin, name), script, 0o755); err != nil {
		t.Fatal(err)
	}
}
