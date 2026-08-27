package main

import "testing"

func TestSelectorLabelUsesRequestedLayoutAndGrayDescription(t *testing.T) {
	command := Command{Name: "tool-name", Package: "package", Description: "Tool description."}
	want := "tool-name  [package]\n    \x1b[38;5;8mTool description.\x1b[0m\n\n"
	if got := selectorLabel(command); got != want {
		t.Fatalf("selectorLabel() = %q, want %q", got, want)
	}
}
