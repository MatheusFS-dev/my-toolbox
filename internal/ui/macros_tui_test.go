package ui

import (
	tea "charm.land/bubbletea/v2"
	"strings"
	"testing"
)

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
