package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// HuhUI renders interactive selection, article search, and typed forms.
type HuhUI struct {
	stdout    io.Writer
	stderr    io.Writer
	runSearch func(tea.Model) (tea.Model, error)
	runMacros func(tea.Model) (tea.Model, error)
}

// BrowseMacros opens the bundled macro browser.
func (ui HuhUI) BrowseMacros(macros []Macro, runEnabled bool) (MacroSelection, error) {
	run := ui.runMacros
	if run == nil {
		run = func(model tea.Model) (tea.Model, error) {
			output := ui.stderr
			if output == nil {
				output = os.Stderr
			}
			return tea.NewProgram(model, tea.WithOutput(output)).Run()
		}
	}
	final, err := run(newMacroModel(macros, runEnabled, maxPresentationWidth, 24))
	if err != nil {
		return MacroSelection{}, err
	}
	model, ok := final.(macroModel)
	if !ok {
		return MacroSelection{}, fmt.Errorf("macro browser returned an invalid model")
	}
	return model.result()
}

// Search opens the bundled article browser.
//
// Args:
//   - articles: Loaded release articles to browse.
//
// Returns:
//   - error: ErrCancelled for Ctrl+C or Escape, a terminal error, or a fallback
//     output error. Reader failures return nil when raw Markdown is delivered.
func (ui HuhUI) Search(articles []Article) error {
	run := ui.runSearch
	if run == nil {
		run = func(model tea.Model) (tea.Model, error) {
			output := ui.stderr
			if output == nil {
				output = os.Stderr
			}
			return tea.NewProgram(model, tea.WithOutput(output)).Run()
		}
	}
	finalModel, err := run(newSearchModel(articles, maxPresentationWidth, 24))
	if err != nil {
		return err
	}
	model, valid := finalModel.(searchModel)
	if !valid {
		return fmt.Errorf("article search returned an invalid model")
	}
	if model.fallback != nil {
		return writeArticleReaderFallback(*model.fallback, ui.stdout, ui.stderr)
	}
	return model.resultError()
}

// Select renders the catalog as a checkbox list.
//
// Args:
//   - commands: Catalog-ordered tools with package and description metadata.
//
// Returns:
//   - []string: Selected command names.
//   - error: ErrCancelled for Ctrl+C or a terminal rendering error.
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
	title := questionTitle(question)
	required := question.Required != nil && *question.Required
	switch question.Type {
	case "text":
		answer, _ := question.Default.(string)
		input := huh.NewInput().Title(title).Value(&answer)
		if required {
			input.Validate(validateRequiredText)
		}
		field = input
		value = &answer
	case "confirm":
		answer, _ := question.Default.(bool)
		field = huh.NewConfirm().Title(title).Value(&answer)
		value = &answer
	case "single":
		answer, _ := question.Default.(string)
		selectField := huh.NewSelect[string]().Title(title).Options(huhOptions(question.Options)...).Value(&answer)
		if required {
			selectField.Validate(func(value string) error {
				if value == "" {
					return errors.New("A value is required; select a value to continue.")
				}
				return nil
			})
		}
		field = selectField
		value = &answer
	case "multiple":
		answer := questionStringSlice(question.Default)
		multiple := huh.NewMultiSelect[string]().Title(title).Options(huhOptions(question.Options)...).Value(&answer)
		if required {
			multiple.Validate(func(value []string) error {
				if len(value) == 0 {
					return errors.New("A value is required; select at least one value to continue.")
				}
				return nil
			})
		}
		field = multiple
		value = &answer
	default:
		return nil, fmt.Errorf("unsupported question type %q", question.Type)
	}
	if err := toolboxHuhForm(field).Run(); err != nil {
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

func questionTitle(question Question) string {
	if question.Required == nil || *question.Required || question.Default == nil {
		return question.Title
	}
	label := fmt.Sprint(question.Default)
	switch value := question.Default.(type) {
	case bool:
		if value {
			return question.Title + " [Y/n] (default: Y):"
		} else {
			return question.Title + " [y/N] (default: n):"
		}
	case []any:
		if len(value) == len(question.Options) {
			label = "all"
		} else {
			label = strings.Join(questionStringSlice(value), ", ")
		}
	case []string:
		if len(value) == len(question.Options) {
			label = "all"
		} else {
			label = strings.Join(value, ", ")
		}
	}
	return question.Title + " [default: " + label + "]"
}

func questionStringSlice(value any) []string {
	switch values := value.(type) {
	case []string:
		return append([]string(nil), values...)
	case []any:
		result := make([]string, 0, len(values))
		for _, raw := range values {
			if text, valid := raw.(string); valid {
				result = append(result, text)
			}
		}
		return result
	default:
		return []string{}
	}
}

func validateRequiredText(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("A value is required; enter a value to continue.")
	}
	return nil
}

// toolboxHuhForm applies the toolbox focus and selection colors to one field.
func toolboxHuhForm(field huh.Field) *huh.Form {
	theme := huh.Theme(huh.ThemeFunc(toolboxHuhStyles))
	if _, multiple := field.(*huh.MultiSelect[string]); multiple {
		theme = huh.ThemeFunc(toolboxHuhMultiSelectStyles)
	}
	return huh.NewForm(huh.NewGroup(field)).WithTheme(theme)
}

func toolboxHuhStyles(isDark bool) *huh.Styles {
	styles := huh.ThemeCharm(isDark)
	blue := lipgloss.Color("4")
	orange := lipgloss.Color("208")
	styles.Focused.Title = styles.Focused.Title.Foreground(blue)
	styles.Focused.NoteTitle = styles.Focused.NoteTitle.Foreground(blue)
	styles.Focused.Directory = styles.Focused.Directory.Foreground(blue)
	styles.Focused.SelectSelector = styles.Focused.SelectSelector.Foreground(blue)
	styles.Focused.NextIndicator = styles.Focused.NextIndicator.Foreground(blue)
	styles.Focused.PrevIndicator = styles.Focused.PrevIndicator.Foreground(blue)
	styles.Focused.MultiSelectSelector = styles.Focused.MultiSelectSelector.Foreground(blue)
	styles.Focused.SelectedOption = styles.Focused.SelectedOption.Foreground(blue)
	styles.Focused.FocusedButton = styles.Focused.FocusedButton.Background(blue)
	styles.Focused.Next = styles.Focused.Next.Background(blue)
	styles.Focused.TextInput.Cursor = styles.Focused.TextInput.Cursor.Foreground(blue)
	styles.Focused.TextInput.Prompt = styles.Focused.TextInput.Prompt.Foreground(blue)
	styles.Focused.ErrorIndicator = styles.Focused.ErrorIndicator.Foreground(orange)
	styles.Focused.ErrorMessage = styles.Focused.ErrorMessage.Foreground(orange)
	styles.Group.Title = styles.Focused.Title
	return styles
}

func toolboxHuhMultiSelectStyles(isDark bool) *huh.Styles {
	styles := toolboxHuhStyles(isDark)
	green := lipgloss.Color("2")
	styles.Focused.SelectedPrefix = styles.Focused.SelectedPrefix.Foreground(green)
	styles.Focused.SelectedOption = styles.Focused.SelectedOption.Foreground(green)
	return styles
}

func huhOptions(options []Option) []huh.Option[string] {
	values := make([]huh.Option[string], 0, len(options))
	for _, option := range options {
		values = append(values, huh.NewOption(option.Label, option.Value))
	}
	return values
}

type selectorModel struct {
	commands              []Command
	selected              []bool
	input                 textinput.Model
	visibleCommandIndexes []int
	cursor                int
	width                 int
	height                int
	viewport              viewport.Model
	rowStarts             []int
	rowEnds               []int
	confirming            bool
	confirmRun            bool
	completed             bool
	cancelled             bool
}

type selectorControlHint struct {
	key    string
	action string
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
	input := textinput.New()
	input.Prompt = "› "
	input.Placeholder = "Type to filter tools"
	input.Focus()
	model := selectorModel{
		commands: append([]Command(nil), commands...),
		selected: make([]bool, len(commands)),
		input:    input,
	}
	model.resize(terminalWidth, terminalHeight)
	model.refreshVisibleCommands()
	return model
}

// Init satisfies the Bubble Tea model contract without starting background work.
//
// Args:
//   - None.
//
// Returns:
//   - tea.Cmd: Input-focus command for the search field.
//
// Raises:
//   - None.
func (model selectorModel) Init() tea.Cmd {
	return model.input.Focus()
}

// Update applies the selector's supported keyboard and resize events.
//
// Args:
//   - message: Bubble Tea event. Typing filters tools, Up and Down move, Tab
//     toggles, Enter opens run confirmation, Escape dismisses or clears,
//     Ctrl+C cancels, and terminal resize events update the layout.
//
// Returns:
//   - tea.Model: Updated selector value.
//   - tea.Cmd: tea.Quit for confirmed completion or cancellation, otherwise
//     the search input command or nil.
//
// Raises:
//   - None.
func (model selectorModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.resize(message.Width, message.Height)
		return model, nil
	case tea.KeyPressMsg:
		if message.Code == 'c' && message.Mod.Contains(tea.ModCtrl) {
			model.cancelled = true
			return model, tea.Quit
		}
		if model.confirming {
			return model.updateConfirmation(message)
		}
		switch message.Code {
		case tea.KeyUp:
			model.moveCursor(-1)
			return model, nil
		case tea.KeyDown:
			model.moveCursor(1)
			return model, nil
		case tea.KeyTab:
			model.toggleFocusedCommand()
			return model, nil
		case tea.KeyEnter:
			if model.selectedCount() > 0 {
				model.confirming = true
				model.confirmRun = false
			}
			return model, nil
		case tea.KeyEscape:
			if model.input.Value() != "" {
				model.input.SetValue("")
				model.refreshVisibleCommands()
			}
			return model, nil
		}
		previous := model.input.Value()
		updatedInput, command := model.input.Update(message)
		model.input = updatedInput
		if model.input.Value() != previous {
			model.refreshVisibleCommands()
		}
		return model, command
	}
	return model, nil
}

func (model selectorModel) updateConfirmation(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch message.Code {
	case tea.KeyEscape:
		model.confirming = false
	case tea.KeyLeft, tea.KeyRight, tea.KeyTab:
		model.confirmRun = !model.confirmRun
	case tea.KeyEnter:
		if model.confirmRun {
			model.completed = true
			return model, tea.Quit
		}
		model.confirming = false
	}
	return model, nil
}

func (model *selectorModel) moveCursor(delta int) {
	position := model.focusedVisiblePosition()
	if position < 0 {
		return
	}
	position = min(max(0, position+delta), len(model.visibleCommandIndexes)-1)
	model.cursor = model.visibleCommandIndexes[position]
	model.rebuildContent()
}

func (model *selectorModel) toggleFocusedCommand() {
	if model.focusedVisiblePosition() < 0 {
		return
	}
	model.selected[model.cursor] = !model.selected[model.cursor]
	model.rebuildContent()
}

// View renders the fixed title/footer and scrolling categorized tool rows.
//
// Args:
//   - None.
//
// Returns:
//   - tea.View: ANSI-styled selector. The confirmation state layers a bordered
//     modal over the list viewport.
//
// Raises:
//   - None.
func (model selectorModel) View() tea.View {
	title := presentationStyle("SELECT TOOLS", ansiBoldWhite, true)
	footer := model.footer()
	base := title + "\n" + model.input.View() + "\n\n" + model.viewport.View() + "\n" + footer
	if model.confirming {
		return tea.NewView(model.selectorModalView(base))
	}
	return tea.NewView(base)
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
	count := model.selectedCount()
	noun := "tools"
	if count == 1 {
		noun = "tool"
	}
	return fmt.Sprintf("%d %s selected", count, noun)
}

func (model selectorModel) selectedCount() int {
	count := 0
	for _, selected := range model.selected {
		if selected {
			count++
		}
	}
	return count
}

// resultError maps internal cancellation state to the public cancellation error.
//
// Args:
//   - None.
//
// Returns:
//   - error: ErrCancelled after Ctrl+C, otherwise nil.
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
	model.input.SetWidth(max(1, model.width-3))
	footerHeight := len(strings.Split(model.footer(), "\n"))
	viewportHeight := max(1, model.height-footerHeight-3)
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
	if len(model.visibleCommandIndexes) == 0 {
		model.rowStarts = nil
		model.rowEnds = nil
		model.viewport.SetContent(presentationStyle("No matching tools.", ansiGray, true))
		return
	}
	lines := []string{}
	model.rowStarts = make([]int, len(model.commands))
	model.rowEnds = make([]int, len(model.commands))
	visiblePosition := 0
	for groupIndex, group := range groupCommands(model.visibleCommands()) {
		if groupIndex > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "  "+presentationStyle(group.Category, ansiBoldWhite, true))
		for _, command := range group.Commands {
			commandIndex := model.visibleCommandIndexes[visiblePosition]
			model.rowStarts[commandIndex] = len(lines)
			lines = append(lines, model.commandLines(command, commandIndex)...)
			model.rowEnds[commandIndex] = len(lines) - 1
			visiblePosition++
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
		cursor = presentationStyle("›", ansiBlue, true) + " "
	}
	color := ansiWhite
	marker := "◯"
	if index == model.cursor {
		color = ansiBlue
	}
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
	if model.focusedVisiblePosition() < 0 {
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
//   - string: Wrapped controls with bold keys followed by a green count and
//     white count grammar.
//
// Raises:
//   - None.
func (model selectorModel) footer() string {
	lines := selectorControlHintLines(model.width, []selectorControlHint{
		{key: "↑/↓", action: "move"},
		{key: "tab", action: "toggle"},
		{key: "enter", action: "run"},
		{key: "esc", action: "clear"},
		{key: "ctrl+c", action: "cancel"},
	})
	countText := model.selectedCountText()
	separator := strings.IndexByte(countText, ' ')
	count := countText[:separator]
	grammar := countText[separator:]
	lines = append(lines, presentationStyle(count, ansiGreen, true)+presentationStyle(grammar, ansiWhite, true))
	return strings.Join(lines, "\n")
}

func selectorControlHintLines(width int, hints []selectorControlHint) []string {
	lines := []string{}
	plainLine := ""
	styledLine := ""
	for index, hint := range hints {
		plainHint := hint.key + " " + hint.action
		styledHint := presentationStyle(hint.key, ansiBoldWhite, true) + presentationStyle(" "+hint.action, ansiGray, true)
		if index > 0 {
			plainHint = "• " + plainHint
			styledHint = presentationStyle("• ", ansiGray, true) + styledHint
		}
		if plainLine != "" && lipgloss.Width(plainLine+" "+plainHint) > width {
			lines = append(lines, styledLine)
			plainLine = ""
			styledLine = ""
			if index > 0 {
				plainHint = strings.TrimPrefix(plainHint, "• ")
				styledHint = strings.TrimPrefix(styledHint, presentationStyle("• ", ansiGray, true))
			}
		}
		if plainLine != "" {
			plainLine += " "
			styledLine += " "
		}
		plainLine += plainHint
		styledLine += styledHint
	}
	if styledLine != "" {
		lines = append(lines, styledLine)
	}
	return lines
}

func (model *selectorModel) refreshVisibleCommands() {
	tokens := selectorSearchWords(model.input.Value())
	indexesByCategory := map[string][]int{}
	categories := []string{}
	for index, command := range model.commands {
		if commandMatchesSelectorQuery(command, tokens) {
			if _, exists := indexesByCategory[command.Category]; !exists {
				categories = append(categories, command.Category)
			}
			indexesByCategory[command.Category] = append(indexesByCategory[command.Category], index)
		}
	}
	model.visibleCommandIndexes = model.visibleCommandIndexes[:0]
	for _, category := range categories {
		model.visibleCommandIndexes = append(model.visibleCommandIndexes, indexesByCategory[category]...)
	}
	if model.focusedVisiblePosition() < 0 {
		if len(model.visibleCommandIndexes) > 0 {
			model.cursor = model.visibleCommandIndexes[0]
		} else {
			model.cursor = 0
		}
	}
	model.viewport.GotoTop()
	model.rebuildContent()
}

func commandMatchesSelectorQuery(command Command, tokens []string) bool {
	words := append(selectorSearchWords(command.Name), selectorSearchWords(command.Description)...)
	for _, token := range tokens {
		matched := false
		for _, word := range words {
			if strings.HasPrefix(word, token) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func selectorSearchWords(text string) []string {
	return strings.FieldsFunc(strings.ToLower(normalizeText(text)), func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsNumber(character)
	})
}

func (model selectorModel) visibleCommands() []Command {
	commands := make([]Command, 0, len(model.visibleCommandIndexes))
	for _, index := range model.visibleCommandIndexes {
		commands = append(commands, model.commands[index])
	}
	return commands
}

func (model selectorModel) focusedVisiblePosition() int {
	for position, index := range model.visibleCommandIndexes {
		if index == model.cursor {
			return position
		}
	}
	return -1
}

func (model selectorModel) selectorModalView(base string) string {
	contentWidth := max(1, min(48, model.width-6))
	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ansiBlue)).
		Background(lipgloss.Color("0")).
		Padding(0, 1).
		Width(contentWidth).
		Render(model.confirmationContent(contentWidth))
	modalWidth := lipgloss.Width(modal)
	modalHeight := lipgloss.Height(modal)
	baseLayer := lipgloss.NewLayer(base).Z(1)
	modalLayer := lipgloss.NewLayer(modal).
		X(max(0, (model.width-modalWidth)/2)).
		Y(max(0, (model.height-modalHeight)/2)).
		Z(2)
	canvas := lipgloss.NewCanvas(model.width, model.height)
	canvas.Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(strings.Repeat(" ", model.width)).Y(max(0, model.height-1)),
		baseLayer,
		modalLayer,
	))
	return canvas.Render()
}

func (model selectorModel) confirmationContent(width int) string {
	lines := []string{presentationStyle("RUN SELECTED TOOLS?", ansiBoldWhite, true), model.selectedCountText()}
	names := model.selectedNames()
	maxNames := min(len(names), max(1, model.height-8))
	for _, name := range names[:min(len(names), maxNames)] {
		for _, line := range wrapText("• "+name, width) {
			lines = append(lines, presentationStyle(line, ansiGreen, true))
		}
	}
	if remaining := len(names) - maxNames; remaining > 0 {
		lines = append(lines, presentationStyle(fmt.Sprintf("and %d more", remaining), ansiGray, true))
	}
	lines = append(lines, "")
	yes := " Yes "
	no := " No "
	if model.confirmRun {
		yes = presentationStyle("[Yes]", ansiGreen, true)
	} else {
		no = presentationStyle("[No]", ansiGreen, true)
	}
	lines = append(lines, yes+"  "+no)
	lines = append(lines, selectorControlHintLines(width, []selectorControlHint{
		{key: "←/→ or tab", action: "choice"},
		{key: "enter", action: "apply"},
		{key: "esc", action: "back"},
		{key: "ctrl+c", action: "cancel"},
	})...)
	return strings.Join(lines, "\n")
}
