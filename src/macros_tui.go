package main

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

type macroResult struct{ Macro Macro }

func searchMacros(macros []Macro, query string) []macroResult {
	query = strings.ToLower(normalizeText(query))
	results := []macroResult{}
	for _, macro := range macros {
		haystack := strings.ToLower(strings.Join([]string{macro.Title, macro.Description, macro.RelativePath, macro.Content}, " "))
		matched := true
		for _, token := range strings.Fields(query) {
			if !strings.Contains(haystack, token) {
				matched = false
				break
			}
		}
		if matched {
			results = append(results, macroResult{Macro: macro})
		}
	}
	return results
}

type macroModel struct {
	macros                                  []Macro
	results                                 []macroResult
	input                                   textinput.Model
	viewport                                viewport.Model
	cursor, action, width, height           int
	rowStarts, rowEnds                      []int
	modal, completed, cancelled, runEnabled bool
	selection                               MacroSelection
}

func newMacroModel(macros []Macro, runEnabled bool, width, height int) macroModel {
	input := textinput.New()
	input.Prompt = "› "
	input.Placeholder = "Type to search macros"
	input.Focus()
	model := macroModel{macros: append([]Macro(nil), macros...), input: input, runEnabled: runEnabled}
	model.resize(width, height)
	model.refresh()
	return model
}
func (model macroModel) Init() tea.Cmd { return model.input.Focus() }
func (model macroModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := message.(tea.WindowSizeMsg); ok {
		model.resize(size.Width, size.Height)
		model.rebuild()
		return model, nil
	}
	if key, ok := message.(tea.KeyPressMsg); ok {
		if key.Code == 'c' && key.Mod.Contains(tea.ModCtrl) {
			model.cancelled = true
			return model, tea.Quit
		}
		if model.modal {
			switch key.Code {
			case tea.KeyEscape:
				model.modal = false
				model.resize(model.width, model.height)
				model.rebuild()
				return model, nil
			case tea.KeyUp, tea.KeyDown:
				if model.runEnabled {
					model.action = 1 - model.action
				}
				return model, nil
			case tea.KeyEnter:
				action := MacroActionRun
				if !model.runEnabled || model.action == 1 {
					action = MacroActionDownload
				}
				model.selection = MacroSelection{Macro: model.results[model.cursor].Macro, Action: action}
				model.completed = true
				return model, tea.Quit
			}
			return model, nil
		}
		switch key.Code {
		case tea.KeyEscape:
			model.cancelled = true
			return model, tea.Quit
		case tea.KeyUp:
			if model.cursor > 0 {
				model.cursor--
				model.rebuild()
			}
			return model, nil
		case tea.KeyDown:
			if model.cursor+1 < len(model.results) {
				model.cursor++
				model.rebuild()
			}
			return model, nil
		case tea.KeyEnter:
			if len(model.results) > 0 {
				model.modal = true
				if !model.runEnabled {
					model.action = 1
				}
				model.resize(model.width, model.height)
				model.rebuild()
			}
			return model, nil
		}
	}
	previous := model.input.Value()
	input, command := model.input.Update(message)
	model.input = input
	if previous != model.input.Value() {
		model.refresh()
	}
	return model, command
}
func (model macroModel) View() tea.View {
	title := presentationStyle("MACROS", ansiBoldWhite, true)
	body := model.viewport.View()
	footer := presentationStyle("↑/↓ move • enter actions • esc exit", ansiGray, true)
	if model.modal {
		run := "Run"
		pointerRun, pointerDownload := "  ", "  "
		if model.action == 0 {
			pointerRun = "› "
		} else {
			pointerDownload = "› "
		}
		body += "\n\n" + pointerRun + run + "\n" + pointerDownload + "Download"
		if !model.runEnabled {
			body += "\n" + presentationStyle("Run unavailable on Linux ARM64.", ansiGray, true)
		}
	}
	return tea.NewView(title + "\n" + model.input.View() + "\n\n" + body + "\n" + footer)
}
func (model *macroModel) resize(width, height int) {
	model.width = min(max(1, width), maxPresentationWidth)
	model.height = max(1, height)
	model.input.SetWidth(max(1, model.width-2))
	model.viewport.SetWidth(model.width)
	reserved := 5
	if model.modal {
		reserved = 9
	}
	model.viewport.SetHeight(max(1, model.height-reserved))
}
func (model *macroModel) refresh() {
	model.results = searchMacros(model.macros, model.input.Value())
	model.cursor = 0
	model.viewport.GotoTop()
	model.rebuild()
}
func (model *macroModel) rebuild() {
	if len(model.results) == 0 {
		model.rowStarts = nil
		model.rowEnds = nil
		model.viewport.SetContent(presentationStyle("No matching macros.", ansiGray, true))
		return
	}
	lines := []string{}
	model.rowStarts = make([]int, len(model.results))
	model.rowEnds = make([]int, len(model.results))
	category := ""
	for i, result := range model.results {
		if result.Macro.Subpackage != category {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			category = result.Macro.Subpackage
			lines = append(lines, "  "+strings.ToUpper(category), "")
		}
		model.rowStarts[i] = len(lines)
		pointer := "    "
		titleColor := ansiWhite
		if i == model.cursor {
			pointer = "  › "
			titleColor = ansiBlue
		}
		for lineIndex, line := range wrapText(result.Macro.Title, max(1, model.width-4)) {
			prefix := "    "
			if lineIndex == 0 {
				prefix = pointer
			}
			lines = append(lines, prefix+presentationStyle(line, titleColor, true))
		}
		if result.Macro.Description != "" {
			for _, line := range wrapText(result.Macro.Description, max(1, model.width-6)) {
				lines = append(lines, "      "+line)
			}
		}
		model.rowEnds[i] = len(lines) - 1
	}
	model.viewport.SetContent(strings.Join(lines, "\n"))
	model.keepResultVisible()
}
func (model *macroModel) keepResultVisible() {
	if model.cursor >= len(model.rowStarts) {
		return
	}
	top := model.viewport.YOffset()
	bottom := top + model.viewport.Height() - 1
	if model.rowStarts[model.cursor] < top {
		model.viewport.SetYOffset(model.rowStarts[model.cursor])
	} else if model.rowEnds[model.cursor] > bottom {
		model.viewport.SetYOffset(model.rowEnds[model.cursor] - model.viewport.Height() + 1)
	}
}
func (model macroModel) result() (MacroSelection, error) {
	if model.cancelled || !model.completed {
		return MacroSelection{}, ErrCancelled
	}
	return model.selection, nil
}

var _ tea.Model = macroModel{}
