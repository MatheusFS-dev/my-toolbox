package main

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestSelectorMovesOnlyAcrossCommandsAndTogglesSelection(t *testing.T) {
	commands := []Command{
		{Name: "one", Category: "First", Description: "First tool."},
		{Name: "two", Category: "Second", Description: "Second tool."},
	}
	model := newSelectorModel(commands, 72, 12)

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(selectorModel)
	if model.cursor != 1 {
		t.Fatalf("cursor = %d, want second command", model.cursor)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	model = updated.(selectorModel)
	if !model.selected[1] || model.selectedCountText() != "1 tool selected" {
		t.Fatalf("selected = %v, count = %q", model.selected, model.selectedCountText())
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	model = updated.(selectorModel)
	if model.cursor != 0 {
		t.Fatalf("cursor = %d, want first command", model.cursor)
	}
}

func TestSelectorCompletesOrCancelsOnExactKeys(t *testing.T) {
	tests := []struct {
		name      string
		message   tea.KeyPressMsg
		completed bool
		cancelled bool
	}{
		{name: "enter", message: tea.KeyPressMsg{Code: tea.KeyEnter}, completed: true},
		{name: "escape", message: tea.KeyPressMsg{Code: tea.KeyEscape}, cancelled: true},
		{name: "control-c", message: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, cancelled: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := newSelectorModel([]Command{{Name: "one", Category: "Test", Description: "Tool."}}, 72, 10)
			updated, command := model.Update(test.message)
			model = updated.(selectorModel)
			if model.completed != test.completed || model.cancelled != test.cancelled {
				t.Fatalf("completed = %t, cancelled = %t", model.completed, model.cancelled)
			}
			if command == nil {
				t.Fatal("completion key returned no Bubble Tea command")
			}
		})
	}
}

func TestSelectorRendersRequiredStylesWithoutBordersOrBackgrounds(t *testing.T) {
	model := newSelectorModel([]Command{{Name: "selected-tool", Category: "Tools", Description: "A gray description."}}, 72, 10)
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	model = updated.(selectorModel)
	view := model.View().Content

	for _, sequence := range []string{"\x1b[1;37mSELECT TOOLS", "\x1b[32m›", "\x1b[32m◉", "\x1b[32mselected-tool", "\x1b[90mA gray description.", "\x1b[90m↑/↓ move", "\x1b[32m1\x1b[0m\x1b[90m tool selected"} {
		if !strings.Contains(view, sequence) {
			t.Fatalf("selector view is missing %q: %q", sequence, view)
		}
	}
	if !strings.Contains(view, "↑/↓ move • space select • enter run • esc cancel") {
		t.Fatalf("selector controls are not rendered in the documented order: %q", view)
	}
	if strings.Contains(view, "\x1b[4") || strings.ContainsAny(view, "│┌┐└┘") {
		t.Fatalf("selector view contains background or border styling: %q", view)
	}
}

func TestSelectorWrapsToContentWidthAndKeepsFocusVisible(t *testing.T) {
	commands := []Command{}
	for index := 0; index < 6; index++ {
		commands = append(commands, Command{
			Name:        "tool-" + string(rune('a'+index)),
			Category:    "Tools",
			Description: "This description contains enough words to wrap in a narrow terminal.",
		})
	}
	model := newSelectorModel(commands, 30, 9)
	for index := 1; index < len(commands); index++ {
		updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		model = updated.(selectorModel)
	}
	view := model.View().Content
	if !strings.Contains(view, "SELECT TOOLS") || !strings.Contains(view, "tool-f") || !strings.Contains(view, "tools selected") {
		t.Fatalf("focused view does not keep title, focused row, and footer visible: %q", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > 30 {
			t.Fatalf("line width = %d, want at most 30: %q", lipgloss.Width(line), line)
		}
	}
}

func TestSelectorReturnsSelectedNamesInCatalogOrder(t *testing.T) {
	commands := []Command{
		{Name: "one", Category: "Test", Description: "One."},
		{Name: "two", Category: "Test", Description: "Two."},
	}
	model := newSelectorModel(commands, 72, 10)
	model.selected[1] = true
	model.selected[0] = true
	got := model.selectedNames()
	if strings.Join(got, ",") != "one,two" {
		t.Fatalf("selected names = %v, want catalog order", got)
	}
}

func TestSelectorCancellationMapsToPublicError(t *testing.T) {
	model := newSelectorModel([]Command{{Name: "one", Category: "Test", Description: "One."}}, 72, 10)
	model.cancelled = true
	if !errors.Is(model.resultError(), ErrCancelled) {
		t.Fatalf("resultError() = %v, want ErrCancelled", model.resultError())
	}
}
