package main

import (
	"io"
	"testing"
)

type unusedBuiltins struct{}

func (unusedBuiltins) SkipReason(string) (string, error) {
	return "", nil
}

func (unusedBuiltins) Run(string, []string) error {
	return nil
}

func TestProcessExecutorExchangesRealAdapterQuestions(t *testing.T) {
	// A broken process boundary would fail to pass package context, accumulated
	// answers, or strict JSON between the Go binary and Python adapter.
	t.Setenv("HOME", t.TempDir())
	catalog, err := LoadCatalogFile("../commands.json")
	if err != nil {
		t.Fatal(err)
	}
	command, exists := catalog.Find("setup-agents-codex")
	if !exists {
		t.Fatal("setup-agents-codex is missing")
	}
	executor := ProcessExecutor{
		Root:     "..",
		Platform: "linux-amd64",
		Builtins: unusedBuiltins{},
		Output:   io.Discard,
	}
	response, err := executor.Questions(command, map[string]any{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != "question" || response.Question == nil || response.Question.ID != "profiles" {
		t.Fatalf("first response = %#v", response)
	}
	response, err = executor.Questions(command, map[string]any{"profiles": []string{}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != "ready" {
		t.Fatalf("completed response = %#v", response)
	}
}
