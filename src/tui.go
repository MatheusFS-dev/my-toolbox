package main

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// HuhUI renders Arrow/Space selection and typed configuration forms.
type HuhUI struct{}

// Select renders the catalog as a checkbox list.
//
// Args:
//   - commands: Catalog-ordered tools with package and description metadata.
//
// Returns:
//   - []string: Selected command names.
//   - error: ErrCancelled for Ctrl+C or Escape, or a terminal rendering error.
func (HuhUI) Select(commands []Command) ([]string, error) {
	program := tea.NewProgram(newSelectorModel(commands, maxPresentationWidth, 24))
	finalModel, err := program.Run()
	if err != nil {
		return nil, err
	}
	model, valid := finalModel.(selectorModel)
	if !valid {
		return nil, fmt.Errorf("selector returned an invalid model")
	}
	if err := model.resultError(); err != nil {
		return nil, err
	}
	return model.selectedNames(), nil
}

// Ask renders exactly one typed configuration question in a visible form.
//
// Args:
//   - question: Validated text, confirmation, single, or multiple question.
//
// Returns:
//   - any: String, bool, or string slice matching the question type.
//   - error: ErrCancelled for Ctrl+C or Escape, validation, or terminal error.
func (HuhUI) Ask(question Question) (any, error) {
	var field huh.Field
	var value any
	switch question.Type {
	case "text":
		answer := ""
		field = huh.NewInput().Title(question.Title).Value(&answer)
		value = &answer
	case "confirm":
		answer := false
		field = huh.NewConfirm().Title(question.Title).Value(&answer)
		value = &answer
	case "single":
		answer := ""
		field = huh.NewSelect[string]().Title(question.Title).Options(huhOptions(question.Options)...).Value(&answer)
		value = &answer
	case "multiple":
		answer := []string{}
		field = huh.NewMultiSelect[string]().Title(question.Title).Options(huhOptions(question.Options)...).Value(&answer)
		value = &answer
	default:
		return nil, fmt.Errorf("unsupported question type %q", question.Type)
	}
	if err := huh.NewForm(huh.NewGroup(field)).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, ErrCancelled
		}
		return nil, err
	}
	switch answer := value.(type) {
	case *string:
		return *answer, nil
	case *bool:
		return *answer, nil
	case *[]string:
		return *answer, nil
	default:
		return nil, fmt.Errorf("question %q produced an invalid answer", question.ID)
	}
}

func huhOptions(options []Option) []huh.Option[string] {
	values := make([]huh.Option[string], 0, len(options))
	for _, option := range options {
		values = append(values, huh.NewOption(option.Label, option.Value))
	}
	return values
}

type selectorModel struct {
	commands  []Command
	selected  []bool
	cursor    int
	width     int
	height    int
	viewport  viewport.Model
	rowStarts []int
	rowEnds   []int
	completed bool
	cancelled bool
}

// newSelectorModel creates the categorized interactive selection state.
//
// Args:
//   - commands: Environment-filtered, list-visible commands in catalog order.
//   - terminalWidth: Initial live terminal width in columns. The model applies
//     the shared 72-column content cap and responds to later resize messages.
//   - terminalHeight: Initial live terminal height in rows. Title and footer
//     rows remain fixed while the remaining rows form the scrolling viewport.
//
// Returns:
//   - selectorModel: Bubble Tea model focused on the first command.
//
// Raises:
//   - None.
//
// Example:
//
//	model := newSelectorModel(commands, 72, 24)
func newSelectorModel(commands []Command, terminalWidth int, terminalHeight int) selectorModel {
	model := selectorModel{
		commands: append([]Command(nil), commands...),
		selected: make([]bool, len(commands)),
	}
	model.resize(terminalWidth, terminalHeight)
	return model
}

// Init satisfies the Bubble Tea model contract without starting background work.
//
// Args:
//   - None.
//
// Returns:
//   - tea.Cmd: Always nil because selection is driven only by terminal events.
//
// Raises:
//   - None.
func (model selectorModel) Init() tea.Cmd {
	return nil
}

// Update applies the selector's supported keyboard and resize events.
//
// Args:
//   - message: Bubble Tea event. Only Up, Down, Space, Enter, Escape, Ctrl+C,
//     and terminal resize events change selector state.
//
// Returns:
//   - tea.Model: Updated selector value.
//   - tea.Cmd: tea.Quit for completion or cancellation, otherwise nil.
//
// Raises:
//   - None.
func (model selectorModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.resize(message.Width, message.Height)
	case tea.KeyPressMsg:
		switch {
		case message.Code == tea.KeyUp:
			if model.cursor > 0 {
				model.cursor--
				model.rebuildContent()
			}
		case message.Code == tea.KeyDown:
			if model.cursor+1 < len(model.commands) {
				model.cursor++
				model.rebuildContent()
			}
		case message.Code == tea.KeySpace:
			if len(model.commands) > 0 {
				model.selected[model.cursor] = !model.selected[model.cursor]
				model.rebuildContent()
			}
		case message.Code == tea.KeyEnter:
			model.completed = true
			return model, tea.Quit
		case message.Code == tea.KeyEscape || (message.Code == 'c' && message.Mod.Contains(tea.ModCtrl)):
			model.cancelled = true
			return model, tea.Quit
		}
	}
	return model, nil
}

// View renders the fixed title/footer and scrolling categorized tool rows.
//
// Args:
//   - None.
//
// Returns:
//   - tea.View: ANSI-styled selector without borders or background styling.
//
// Raises:
//   - None.
func (model selectorModel) View() tea.View {
	title := presentationStyle("SELECT TOOLS", ansiBoldWhite, true)
	footer := model.footer()
	return tea.NewView(title + "\n\n" + model.viewport.View() + "\n" + footer)
}

// selectedNames returns checked command names in unchanged catalog order.
//
// Args:
//   - None.
//
// Returns:
//   - []string: Selected public command names.
//
// Raises:
//   - None.
func (model selectorModel) selectedNames() []string {
	names := []string{}
	for index, command := range model.commands {
		if model.selected[index] {
			names = append(names, command.Name)
		}
	}
	return names
}

// selectedCountText renders the selected-count grammar without ANSI styling.
//
// Args:
//   - None.
//
// Returns:
//   - string: "1 tool selected" for one selection and plural grammar otherwise.
//
// Raises:
//   - None.
func (model selectorModel) selectedCountText() string {
	count := 0
	for _, selected := range model.selected {
		if selected {
			count++
		}
	}
	noun := "tools"
	if count == 1 {
		noun = "tool"
	}
	return fmt.Sprintf("%d %s selected", count, noun)
}

// resultError maps internal cancellation state to the public cancellation error.
//
// Args:
//   - None.
//
// Returns:
//   - error: ErrCancelled after Escape or Ctrl+C, otherwise nil.
//
// Raises:
//   - None.
func (model selectorModel) resultError() error {
	if model.cancelled {
		return ErrCancelled
	}
	return nil
}

// resize applies the content cap and reserves terminal rows for fixed chrome.
//
// Args:
//   - terminalWidth: Current live width in columns.
//   - terminalHeight: Current live height in rows.
//
// Returns:
//   - None.
//
// Raises:
//   - None.
func (model *selectorModel) resize(terminalWidth int, terminalHeight int) {
	model.width = min(max(1, terminalWidth), maxPresentationWidth)
	model.height = max(1, terminalHeight)
	footerHeight := len(strings.Split(model.footer(), "\n"))
	viewportHeight := max(1, model.height-footerHeight-2)
	model.viewport.SetWidth(model.width)
	model.viewport.SetHeight(viewportHeight)
	model.rebuildContent()
}

// rebuildContent redraws groups and keeps the focused command inside the viewport.
//
// Args:
//   - None.
//
// Returns:
//   - None.
//
// Raises:
//   - None.
func (model *selectorModel) rebuildContent() {
	lines := []string{}
	model.rowStarts = make([]int, len(model.commands))
	model.rowEnds = make([]int, len(model.commands))
	commandIndex := 0
	for groupIndex, group := range groupCommands(model.commands) {
		if groupIndex > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "  "+presentationStyle(group.Category, ansiBoldWhite, true))
		for _, command := range group.Commands {
			model.rowStarts[commandIndex] = len(lines)
			lines = append(lines, model.commandLines(command, commandIndex)...)
			model.rowEnds[commandIndex] = len(lines) - 1
			commandIndex++
		}
	}
	model.viewport.SetContent(strings.Join(lines, "\n"))
	model.keepFocusVisible()
}

// commandLines renders one selectable command and its wrapped description.
//
// Args:
//   - command: Catalog metadata for the row.
//   - index: Command index used for focus and selected state.
//
// Returns:
//   - []string: Styled row lines with descriptions beginning in column five.
//
// Raises:
//   - None. The index is supplied only from the model's command iteration.
func (model selectorModel) commandLines(command Command, index int) []string {
	cursor := "  "
	if index == model.cursor {
		cursor = presentationStyle("›", ansiGreen, true) + " "
	}
	color := ansiWhite
	marker := "◯"
	if model.selected[index] {
		color = ansiGreen
		marker = "◉"
	}
	nameLines := wrapText(command.Name, max(1, model.width-4))
	lines := []string{cursor + presentationStyle(marker, color, true) + " " + presentationStyle(nameLines[0], color, true)}
	for _, line := range nameLines[1:] {
		lines = append(lines, "    "+presentationStyle(line, color, true))
	}
	for _, line := range wrapText(command.Description, max(1, model.width-4)) {
		lines = append(lines, "    "+presentationStyle(line, ansiGray, true))
	}
	if requirements := command.requirementText(); requirements != "" {
		for _, line := range wrapText(requirements, max(1, model.width-4)) {
			lines = append(lines, "    "+presentationStyle(line, ansiBrightRed, true))
		}
	}
	return lines
}

// keepFocusVisible adjusts the viewport so the focused row remains visible.
//
// Args:
//   - None.
//
// Returns:
//   - None.
//
// Raises:
//   - None.
func (model *selectorModel) keepFocusVisible() {
	if len(model.commands) == 0 {
		return
	}
	start := model.rowStarts[model.cursor]
	end := model.rowEnds[model.cursor]
	height := model.viewport.Height()
	if end-start+1 > height || start < model.viewport.YOffset() {
		model.viewport.SetYOffset(start)
		return
	}
	if end >= model.viewport.YOffset()+height {
		model.viewport.SetYOffset(end - height + 1)
	}
}

// footer renders fixed controls and the styled selected count responsively.
//
// Args:
//   - None.
//
// Returns:
//   - string: Gray wrapped controls followed by a count whose number is green.
//
// Raises:
//   - None.
func (model selectorModel) footer() string {
	controls := "↑/↓ move • space select • enter run • esc cancel"
	lines := wrapText(controls, model.width)
	for index, line := range lines {
		lines[index] = presentationStyle(line, ansiGray, true)
	}
	countText := model.selectedCountText()
	separator := strings.IndexByte(countText, ' ')
	count := countText[:separator]
	grammar := countText[separator:]
	lines = append(lines, presentationStyle(count, ansiGreen, true)+presentationStyle(grammar, ansiGray, true))
	return strings.Join(lines, "\n")
}
