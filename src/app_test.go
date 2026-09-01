package main

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeUI struct {
	selected []string
	answers  map[string]any
	err      error
	commands []Command
}

func (ui *fakeUI) Select(commands []Command) ([]string, error) {
	ui.commands = append([]Command(nil), commands...)
	return ui.selected, ui.err
}

func (ui *fakeUI) Ask(question Question) (any, error) {
	if ui.err != nil {
		return nil, ui.err
	}
	return ui.answers[question.ID], nil
}

type fakeExecutor struct {
	responses         map[string][]ProtocolResponse
	runs              []string
	preflights        []string
	arguments         map[string][]string
	fail              string
	preflightFailures map[string]error
	installed         map[string]bool
}

func (executor *fakeExecutor) Preflight(command Command) error {
	executor.preflights = append(executor.preflights, command.Name)
	if command.Name == "plugin" && !executor.installed["agent"] {
		return errors.New("missing agent plugin management")
	}
	return executor.preflightFailures[command.Name]
}

func (executor *fakeExecutor) Questions(command Command, _ map[string]any, _ []string) (ProtocolResponse, error) {
	responses := executor.responses[command.Name]
	if len(responses) == 0 {
		return ProtocolResponse{Status: "ready"}, nil
	}
	response := responses[0]
	executor.responses[command.Name] = responses[1:]
	return response, nil
}

func (executor *fakeExecutor) Run(command Command, _ map[string]any, arguments []string) error {
	executor.runs = append(executor.runs, command.Name)
	executor.arguments[command.Name] = append([]string(nil), arguments...)
	if command.Name == executor.fail {
		return errors.New("planned failure")
	}
	if command.Name == "agent" {
		executor.installed["agent"] = true
	}
	return nil
}

func testCatalog(names ...string) Catalog {
	commands := make([]Command, 0, len(names))
	for _, name := range names {
		commands = append(commands, Command{Name: name, Category: "Test", Description: name, Package: "test", Visibility: "list", Protocol: "builtin", Environments: []string{"linux-native"}})
	}
	return Catalog{Commands: commands}
}

func TestListAndHelpExcludeCommandsFromOtherEnvironments(t *testing.T) {
	catalog := testCatalog("native", "wsl", "windows")
	catalog.Commands[1].Environments = []string{"linux-wsl"}
	catalog.Commands[2].Environments = []string{"windows"}
	ui := &fakeUI{selected: []string{"native"}}
	output := &bytes.Buffer{}
	app := App{Catalog: catalog, Environment: "linux-native", UI: ui, Executor: &fakeExecutor{responses: map[string][]ProtocolResponse{}, arguments: map[string][]string{}}, Output: output}

	if err := app.Execute([]string{"list"}); err != nil {
		t.Fatal(err)
	}
	if len(ui.commands) != 1 || ui.commands[0].Name != "native" {
		t.Fatalf("selector commands = %#v", ui.commands)
	}
	output.Reset()
	if err := app.Execute([]string{"help"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "    native\n") || strings.Contains(output.String(), "    wsl\n") || strings.Contains(output.String(), "    windows\n") {
		t.Fatalf("help = %q", output.String())
	}
}

func TestCompletePrintsSortedBuiltinsAndSupportedCatalogCommands(t *testing.T) {
	catalog := testCatalog("zeta", "direct", "other-environment", "alpha")
	catalog.Commands[1].Visibility = "direct"
	catalog.Commands[2].Environments = []string{"windows"}
	output := &bytes.Buffer{}
	app := App{Catalog: catalog, Environment: "linux-native", Output: output}

	if err := app.Execute([]string{"__complete"}); err != nil {
		t.Fatal(err)
	}
	want := "alpha\ndirect\nhelp\nlist\nuninstall\nupdate\nversion\nzeta\n"
	if output.String() != want {
		t.Fatalf("completion output = %q, want %q", output.String(), want)
	}
}

func TestRepositoryCompletionCandidatesMatchEachEnvironment(t *testing.T) {
	catalog, err := LoadCatalogFile("../commands.json")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		environment string
		want        string
	}{
		{
			environment: "linux-native",
			want:        "bootstrap-python-from-venv\nchange-grub-order\ncreate-env-alias\ncreate-project-template\nhelp\ninstall-antigravity\ninstall-claude\ninstall-codex\ninstall-gh\ninstall-superpowers-antigravity\ninstall-superpowers-claude\ninstall-superpowers-codex\ninstall-uv\nlist\nsetup-agents-antigravity\nsetup-agents-claude\nsetup-agents-codex\nsetup-agents-project\nsetup-alacritty\nsetup-kitty\nsetup-venv\ntoggle-nopasswd-sudo\nuninstall\nupdate\nversion\n",
		},
		{
			environment: "linux-wsl",
			want:        "bootstrap-python-from-venv\ncreate-env-alias\ncreate-project-template\nhelp\ninstall-antigravity\ninstall-claude\ninstall-codex\ninstall-gh\ninstall-superpowers-antigravity\ninstall-superpowers-claude\ninstall-superpowers-codex\ninstall-uv\nlist\nset-default-cwd\nsetup-agents-antigravity\nsetup-agents-claude\nsetup-agents-codex\nsetup-agents-project\nsetup-venv\nsetup-wsl\ntoggle-nopasswd-sudo\nuninstall\nupdate\nversion\n",
		},
		{
			environment: "windows",
			want:        "create-project-template\nhelp\ninstall-antigravity\ninstall-claude\ninstall-codex\ninstall-gh\ninstall-superpowers-antigravity\ninstall-superpowers-claude\ninstall-superpowers-codex\ninstall-uv\nlist\nset-terminal-hotkey\nset-vscode-wsl-cwd\nsetup-agents-antigravity\nsetup-agents-claude\nsetup-agents-codex\nsetup-agents-project\nsetup-windows\nuninstall\nupdate\nversion\n",
		},
	}
	for _, test := range tests {
		t.Run(test.environment, func(t *testing.T) {
			output := &bytes.Buffer{}
			app := App{Catalog: catalog, Environment: test.environment, Output: output}
			if err := app.Execute([]string{"__complete"}); err != nil {
				t.Fatal(err)
			}
			if output.String() != test.want {
				t.Fatalf("completion output = %q, want %q", output.String(), test.want)
			}
		})
	}
}

func TestCompleteRejectsArgumentsAndStaysHiddenFromHelp(t *testing.T) {
	output := &bytes.Buffer{}
	app := App{Catalog: testCatalog("tool"), Environment: "linux-native", Output: output, Version: "1.2.3"}

	err := app.Execute([]string{"__complete", "unexpected"})
	if err == nil || !strings.Contains(err.Error(), "tb __complete does not accept arguments") {
		t.Fatalf("Execute() error = %v", err)
	}
	if err := app.Execute([]string{"help"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "__complete") {
		t.Fatalf("hidden completion command appeared in help: %q", output.String())
	}
}

func TestDirectCommandRejectsUnsupportedEnvironment(t *testing.T) {
	catalog := testCatalog("wsl-only")
	catalog.Commands[0].Environments = []string{"linux-wsl"}
	executor := &fakeExecutor{responses: map[string][]ProtocolResponse{}, arguments: map[string][]string{}}
	app := App{Catalog: catalog, Environment: "linux-native", UI: &fakeUI{}, Executor: executor}

	err := app.Execute([]string{"wsl-only"})
	if err == nil || !strings.Contains(err.Error(), `command "wsl-only" is not supported in linux-native`) {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(executor.runs) != 0 {
		t.Fatalf("unsupported command ran: %v", executor.runs)
	}
}

func TestListExcludesDirectToolsWithoutBlockingDirectExecution(t *testing.T) {
	catalog := testCatalog("listed", "direct")
	catalog.Commands[1].Visibility = "direct"
	ui := &fakeUI{selected: []string{"listed"}}
	executor := &fakeExecutor{responses: map[string][]ProtocolResponse{}, arguments: map[string][]string{}}
	app := App{Catalog: catalog, Environment: "linux-native", UI: ui, Executor: executor}

	if err := app.Execute([]string{"list"}); err != nil {
		t.Fatal(err)
	}
	if len(ui.commands) != 1 || ui.commands[0].Name != "listed" {
		t.Fatalf("selector commands = %#v, want listed only", ui.commands)
	}
	if err := app.Execute([]string{"direct"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(executor.runs, []string{"listed", "direct"}) {
		t.Fatalf("runs = %v", executor.runs)
	}
}

func TestHelpPrintsEveryCatalogToolOnceInCatalogOrder(t *testing.T) {
	catalog := testCatalog("first", "direct", "last")
	catalog.Commands[0].Description = "First description"
	catalog.Commands[1].Description = "Direct description"
	catalog.Commands[1].Visibility = "direct"
	catalog.Commands[2].Description = "Last description"
	output := &bytes.Buffer{}
	app := App{Catalog: catalog, Environment: "linux-native", UI: &fakeUI{}, Executor: &fakeExecutor{}, Output: output, Version: "1.2.3"}

	if err := app.Execute([]string{"help"}); err != nil {
		t.Fatal(err)
	}
	help := output.String()
	if !strings.HasPrefix(help, "TOOLBOX 1.2.3\n") {
		t.Fatalf("help = %q", help)
	}
	previous := -1
	for _, text := range []string{"    first\n      First description", "    direct\n      Direct description", "    last\n      Last description"} {
		if strings.Count(help, text) != 1 {
			t.Fatalf("help occurrence for %q = %d; help = %q", text, strings.Count(help, text), help)
		}
		index := strings.Index(help, text)
		if index <= previous {
			t.Fatalf("help is not in catalog order: %q", help)
		}
		previous = index
	}
}

func TestRepositoryHelpIncludesDirectProjectTool(t *testing.T) {
	catalog, err := LoadCatalogFile("../commands.json")
	if err != nil {
		t.Fatal(err)
	}
	output := &bytes.Buffer{}
	app := App{Catalog: catalog, Environment: "linux-native", Output: output}
	if err := app.Execute([]string{"help"}); err != nil {
		t.Fatal(err)
	}
	entry := "    setup-agents-project\n"
	if strings.Count(output.String(), entry) != 1 {
		t.Fatalf("project help entry count = %d; help = %q", strings.Count(output.String(), entry), output.String())
	}
}

func TestBareInvocationIsInvalidAndDoesNotExecute(t *testing.T) {
	executor := &fakeExecutor{arguments: map[string][]string{}}
	app := App{Catalog: testCatalog("one"), Environment: "linux-native", UI: &fakeUI{}, Executor: executor}
	err := app.Execute(nil)
	if err == nil || !strings.Contains(err.Error(), "tb list") {
		t.Fatalf("Execute(nil) error = %v, want tb list guidance", err)
	}
	if len(executor.runs) != 0 {
		t.Fatalf("bare invocation ran tools: %v", executor.runs)
	}
}

func TestListConfiguresEveryToolBeforeCatalogOrderedExecution(t *testing.T) {
	question := ProtocolResponse{Status: "question", Question: &Question{ID: "confirm", Type: "confirm", Title: "Continue?"}}
	executor := &fakeExecutor{
		responses: map[string][]ProtocolResponse{"first": {question, {Status: "ready"}}, "second": {{Status: "ready"}}},
		arguments: map[string][]string{},
	}
	ui := &fakeUI{selected: []string{"second", "first"}, answers: map[string]any{"confirm": true}}
	app := App{Catalog: testCatalog("first", "second", "third"), Environment: "linux-native", UI: ui, Executor: executor}

	if err := app.Execute([]string{"list"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(executor.runs, []string{"first", "second"}) {
		t.Fatalf("runs = %v, want catalog order", executor.runs)
	}
}

func TestCancellationDuringConfigurationRunsNothing(t *testing.T) {
	executor := &fakeExecutor{
		responses: map[string][]ProtocolResponse{"first": {{Status: "question", Question: &Question{ID: "value", Type: "text", Title: "Value"}}}},
		arguments: map[string][]string{},
	}
	app := App{
		Catalog:     testCatalog("first"),
		Environment: "linux-native",
		UI:          &fakeUI{selected: []string{"first"}, err: ErrCancelled},
		Executor:    executor,
	}
	if err := app.Execute([]string{"list"}); !errors.Is(err, ErrCancelled) {
		t.Fatalf("Execute() error = %v, want cancellation", err)
	}
	if len(executor.runs) != 0 {
		t.Fatalf("cancelled batch ran tools: %v", executor.runs)
	}
}

func TestFailureStopsBatchAndPrintsSummary(t *testing.T) {
	output := &bytes.Buffer{}
	executor := &fakeExecutor{responses: map[string][]ProtocolResponse{}, arguments: map[string][]string{}, fail: "second"}
	app := App{
		Catalog:     testCatalog("first", "second", "third"),
		Environment: "linux-native",
		UI:          &fakeUI{selected: []string{"first", "second", "third"}},
		Executor:    executor,
		Output:      output,
	}
	err := app.Execute([]string{"list"})
	if err == nil || !strings.Contains(err.Error(), "second") {
		t.Fatalf("Execute() error = %v, want second failure", err)
	}
	if !reflect.DeepEqual(executor.runs, []string{"first", "second"}) {
		t.Fatalf("runs = %v, want stop after second", executor.runs)
	}
	for _, text := range []string{"executed: first", "failed: second", "not run: third"} {
		if !strings.Contains(output.String(), text) {
			t.Fatalf("summary %q missing %q", output.String(), text)
		}
	}
	if strings.Contains(strings.ToLower(output.String()), "rollback") {
		t.Fatalf("summary makes an unsupported rollback claim: %q", output.String())
	}
}

func TestPreflightFailureRunsNoPartOfToolAndStopsRemainder(t *testing.T) {
	output := &bytes.Buffer{}
	executor := &fakeExecutor{
		responses:         map[string][]ProtocolResponse{},
		arguments:         map[string][]string{},
		preflightFailures: map[string]error{"second": errors.New("missing requirements:\n- Bash\n  Install Bash")},
	}
	app := App{Catalog: testCatalog("first", "second", "third"), Environment: "linux-native", UI: &fakeUI{selected: []string{"first", "second", "third"}}, Executor: executor, Output: output}
	err := app.Execute([]string{"list"})
	if err == nil || !strings.Contains(err.Error(), "missing requirements") {
		t.Fatalf("Execute() error = %v", err)
	}
	if !reflect.DeepEqual(executor.preflights, []string{"first", "second"}) {
		t.Fatalf("preflights = %v", executor.preflights)
	}
	if !reflect.DeepEqual(executor.runs, []string{"first"}) {
		t.Fatalf("runs = %v, failing tool must not run", executor.runs)
	}
	for _, text := range []string{"executed: first", "failed: second", "not run: third"} {
		if !strings.Contains(output.String(), text) {
			t.Fatalf("summary %q missing %q", output.String(), text)
		}
	}
}

func TestEarlierInstallerCanSatisfyLaterToolPreflight(t *testing.T) {
	executor := &fakeExecutor{responses: map[string][]ProtocolResponse{}, arguments: map[string][]string{}, installed: map[string]bool{}}
	app := App{Catalog: testCatalog("agent", "plugin"), Environment: "linux-native", UI: &fakeUI{selected: []string{"agent", "plugin"}}, Executor: executor}
	if err := app.Execute([]string{"list"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(executor.runs, []string{"agent", "plugin"}) {
		t.Fatalf("runs = %v", executor.runs)
	}
}

func TestLinuxUpdatePreflightsBashBeforeRunningBuiltin(t *testing.T) {
	executor := &fakeExecutor{arguments: map[string][]string{}, preflightFailures: map[string]error{"update": errors.New("Bash is unavailable")}}
	app := App{Catalog: testCatalog("tool"), Environment: "linux-native", Executor: executor}
	err := app.Execute([]string{"update"})
	if err == nil || !strings.Contains(err.Error(), "Bash is unavailable") {
		t.Fatalf("Execute(update) error = %v", err)
	}
	if !reflect.DeepEqual(executor.preflights, []string{"update"}) || len(executor.runs) != 0 {
		t.Fatalf("preflights = %v, runs = %v", executor.preflights, executor.runs)
	}
}

func TestFailureSummaryKeepsPreconfiguredSkipsSeparate(t *testing.T) {
	// A broken summary loop would classify a known skipped tool as not run when
	// an earlier ready tool fails.
	output := &bytes.Buffer{}
	executor := &fakeExecutor{
		responses: map[string][]ProtocolResponse{
			"second": {{Status: "skipped", Reason: "replacement declined: /conflict"}},
		},
		arguments: map[string][]string{},
		fail:      "first",
	}
	app := App{
		Catalog:     testCatalog("first", "second", "third"),
		Environment: "linux-native",
		UI:          &fakeUI{selected: []string{"first", "second", "third"}},
		Executor:    executor,
		Output:      output,
	}
	if err := app.Execute([]string{"list"}); err == nil {
		t.Fatal("Execute() succeeded, want first failure")
	}
	if !strings.Contains(output.String(), "skipped: second (replacement declined: /conflict)") {
		t.Fatalf("summary = %q", output.String())
	}
	if !strings.Contains(output.String(), "not run: third") || strings.Contains(output.String(), "not run: second") {
		t.Fatalf("summary = %q", output.String())
	}
}

func TestSkippedToolDoesNotPreventLaterExecution(t *testing.T) {
	executor := &fakeExecutor{
		responses: map[string][]ProtocolResponse{
			"first": {{Status: "skipped", Reason: "replacement declined: /conflict"}},
		},
		arguments: map[string][]string{},
	}
	output := &bytes.Buffer{}
	app := App{Catalog: testCatalog("first", "second"), Environment: "linux-native", UI: &fakeUI{selected: []string{"first", "second"}}, Executor: executor, Output: output}
	if err := app.Execute([]string{"list"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(executor.runs, []string{"second"}) {
		t.Fatalf("runs = %v, want only second", executor.runs)
	}
	if !strings.Contains(output.String(), "skipped: first (replacement declined: /conflict)") {
		t.Fatalf("summary = %q", output.String())
	}
}

func TestDirectCommandForwardsArguments(t *testing.T) {
	executor := &fakeExecutor{responses: map[string][]ProtocolResponse{}, arguments: map[string][]string{}}
	app := App{Catalog: testCatalog("tool"), Environment: "linux-native", UI: &fakeUI{}, Executor: executor}
	if err := app.Execute([]string{"tool", "one", "two"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(executor.arguments["tool"], []string{"one", "two"}) {
		t.Fatalf("arguments = %v", executor.arguments["tool"])
	}
}

func TestRepeatedQuestionIDFailsExplicitly(t *testing.T) {
	response := ProtocolResponse{Status: "question", Question: &Question{ID: "same", Type: "text", Title: "Value"}}
	executor := &fakeExecutor{responses: map[string][]ProtocolResponse{"tool": {response, response}}, arguments: map[string][]string{}}
	app := App{Catalog: testCatalog("tool"), Environment: "linux-native", UI: &fakeUI{answers: map[string]any{"same": "answer"}}, Executor: executor}
	err := app.Execute([]string{"tool"})
	if err == nil || !strings.Contains(err.Error(), "repeated question ID") {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestUninstallRequiresConfirmationBeforeExecution(t *testing.T) {
	executor := &fakeExecutor{arguments: map[string][]string{}}
	app := App{
		Catalog:     testCatalog("tool"),
		Environment: "linux-native",
		UI:          &fakeUI{answers: map[string]any{"confirm-uninstall": true}},
		Executor:    executor,
	}
	if err := app.Execute([]string{"uninstall"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(executor.runs, []string{"uninstall"}) {
		t.Fatalf("runs = %v, want uninstall", executor.runs)
	}
}

func TestUninstallDeclinedLeavesInstallationUntouched(t *testing.T) {
	output := &bytes.Buffer{}
	executor := &fakeExecutor{arguments: map[string][]string{}}
	app := App{
		Catalog:     testCatalog("tool"),
		Environment: "linux-native",
		UI:          &fakeUI{answers: map[string]any{"confirm-uninstall": false}},
		Executor:    executor,
		Output:      output,
	}
	if err := app.Execute([]string{"uninstall"}); err != nil {
		t.Fatal(err)
	}
	if len(executor.runs) != 0 {
		t.Fatalf("declined uninstall ran commands: %v", executor.runs)
	}
	if !strings.Contains(output.String(), "cancelled") {
		t.Fatalf("output = %q, want cancellation", output.String())
	}
}

func TestUninstallRejectsArguments(t *testing.T) {
	executor := &fakeExecutor{arguments: map[string][]string{}}
	app := App{Catalog: testCatalog("tool"), Environment: "linux-native", UI: &fakeUI{}, Executor: executor}
	err := app.Execute([]string{"uninstall", "now"})
	if err == nil || !strings.Contains(err.Error(), "does not accept arguments") {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(executor.runs) != 0 {
		t.Fatalf("invalid uninstall ran commands: %v", executor.runs)
	}
}

func TestUninstallCancellationRunsNothing(t *testing.T) {
	executor := &fakeExecutor{arguments: map[string][]string{}}
	app := App{Catalog: testCatalog("tool"), Environment: "linux-native", UI: &fakeUI{err: ErrCancelled}, Executor: executor}
	if err := app.Execute([]string{"uninstall"}); !errors.Is(err, ErrCancelled) {
		t.Fatalf("Execute() error = %v, want cancellation", err)
	}
	if len(executor.runs) != 0 {
		t.Fatalf("cancelled uninstall ran commands: %v", executor.runs)
	}
}
