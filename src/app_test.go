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
}

func (ui *fakeUI) Select(_ []Command) ([]string, error) {
	return ui.selected, ui.err
}

func (ui *fakeUI) Ask(question Question) (any, error) {
	if ui.err != nil {
		return nil, ui.err
	}
	return ui.answers[question.ID], nil
}

type fakeExecutor struct {
	responses map[string][]ProtocolResponse
	runs      []string
	arguments map[string][]string
	fail      string
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
	return nil
}

func testCatalog(names ...string) Catalog {
	commands := make([]Command, 0, len(names))
	for _, name := range names {
		commands = append(commands, Command{Name: name, Description: name, Package: "test", Protocol: "builtin"})
	}
	return Catalog{Commands: commands}
}

func TestBareInvocationIsInvalidAndDoesNotExecute(t *testing.T) {
	executor := &fakeExecutor{arguments: map[string][]string{}}
	app := App{Catalog: testCatalog("one"), UI: &fakeUI{}, Executor: executor}
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
	app := App{Catalog: testCatalog("first", "second", "third"), UI: ui, Executor: executor}

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
		Catalog:  testCatalog("first"),
		UI:       &fakeUI{selected: []string{"first"}, err: ErrCancelled},
		Executor: executor,
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
		Catalog:  testCatalog("first", "second", "third"),
		UI:       &fakeUI{selected: []string{"first", "second", "third"}},
		Executor: executor,
		Output:   output,
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
		Catalog:  testCatalog("first", "second", "third"),
		UI:       &fakeUI{selected: []string{"first", "second", "third"}},
		Executor: executor,
		Output:   output,
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
	app := App{Catalog: testCatalog("first", "second"), UI: &fakeUI{selected: []string{"first", "second"}}, Executor: executor, Output: output}
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
	app := App{Catalog: testCatalog("tool"), UI: &fakeUI{}, Executor: executor}
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
	app := App{Catalog: testCatalog("tool"), UI: &fakeUI{answers: map[string]any{"same": "answer"}}, Executor: executor}
	err := app.Execute([]string{"tool"})
	if err == nil || !strings.Contains(err.Error(), "repeated question ID") {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestUninstallRequiresConfirmationBeforeExecution(t *testing.T) {
	executor := &fakeExecutor{arguments: map[string][]string{}}
	app := App{
		Catalog:  testCatalog("tool"),
		UI:       &fakeUI{answers: map[string]any{"confirm-uninstall": true}},
		Executor: executor,
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
		Catalog:  testCatalog("tool"),
		UI:       &fakeUI{answers: map[string]any{"confirm-uninstall": false}},
		Executor: executor,
		Output:   output,
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
	app := App{Catalog: testCatalog("tool"), UI: &fakeUI{}, Executor: executor}
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
	app := App{Catalog: testCatalog("tool"), UI: &fakeUI{err: ErrCancelled}, Executor: executor}
	if err := app.Execute([]string{"uninstall"}); !errors.Is(err, ErrCancelled) {
		t.Fatalf("Execute() error = %v, want cancellation", err)
	}
	if len(executor.runs) != 0 {
		t.Fatalf("cancelled uninstall ran commands: %v", executor.runs)
	}
}
