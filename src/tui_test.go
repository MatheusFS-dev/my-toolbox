package main

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	model = updated.(selectorModel)
	if !model.selected[1] || model.selectedCountText() != "1 tool selected" {
		t.Fatalf("selected = %v, count = %q", model.selected, model.selectedCountText())
	}
	if command != nil || model.completed || model.confirming {
		t.Fatalf("Tab changed selector completion state: command = %v, completed = %t, confirming = %t", command, model.completed, model.confirming)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	model = updated.(selectorModel)
	if model.cursor != 0 {
		t.Fatalf("cursor = %d, want first command", model.cursor)
	}
}

func TestSelectorUsesSpaceToEditTheFocusedSearchInput(t *testing.T) {
	model := newSelectorModel([]Command{{Name: "one", Category: "Test", Description: "One."}}, 72, 10)

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	model = updated.(selectorModel)

	if model.selected[0] {
		t.Fatal("Space selected the focused command")
	}
	if model.input.Value() != " " || !model.input.Focused() {
		t.Fatalf("search input = %q, focused = %t", model.input.Value(), model.input.Focused())
	}
}

func TestSelectorFiltersCaseInsensitiveWordPrefixTokensAcrossNamesAndDescriptions(t *testing.T) {
	commands := []Command{
		{Name: "install-codex", Category: "Agents", Description: "Install Codex for the current user on Linux or Windows. Skips installation when codex exists."},
		{Name: "setup-terminal", Category: "Terminal", Description: "Configure a terminal."},
		{Name: "configure-terminal", Category: "Terminal", Description: "Set up a Windows terminal."},
		{Name: "mount-drive", Category: "Storage", Description: "Mount a filesystem."},
	}
	model := newSelectorModel(commands, 72, 14)
	model.input.SetValue("SeTuP")
	model.refreshVisibleCommands()

	if got := model.visibleCommandIndexes; len(got) != 1 || got[0] != 1 {
		t.Fatalf("visible command indexes = %v, want [1]", got)
	}
	if model.cursor != 1 || !strings.Contains(model.View().Content, "setup-terminal") {
		t.Fatalf("cursor = %d, view = %q", model.cursor, model.View().Content)
	}

	model.input.SetValue("set win")
	model.refreshVisibleCommands()
	if got := model.visibleCommandIndexes; len(got) != 1 || got[0] != 2 {
		t.Fatalf("visible command indexes = %v, want [2]", got)
	}
}

func TestSelectorFilteringPreservesCatalogOrderCategoriesAndHiddenSelections(t *testing.T) {
	commands := []Command{
		{Name: "alpha", Category: "First", Description: "Alpha tool."},
		{Name: "beta", Category: "Second", Description: "Matches term."},
		{Name: "gamma", Category: "First", Description: "Matches term too."},
	}
	model := newSelectorModel(commands, 72, 16)
	model.selected[0] = true
	model.input.SetValue("term")
	model.refreshVisibleCommands()

	if got := model.visibleCommandIndexes; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("visible command indexes = %v, want [1 2]", got)
	}
	plainView := ansi.Strip(model.View().Content)
	if strings.Index(plainView, "Second") > strings.Index(plainView, "First") {
		t.Fatalf("filtered categories are out of catalog order: %q", plainView)
	}
	if model.selectedCountText() != "1 tool selected" || strings.Join(model.selectedNames(), ",") != "alpha" {
		t.Fatalf("hidden selection was lost: count = %q, names = %v", model.selectedCountText(), model.selectedNames())
	}
}

func TestSelectorGroupsInterleavedCategoriesWithoutLosingCommandIndexes(t *testing.T) {
	commands := []Command{
		{Name: "alpha", Category: "First", Description: "First alpha."},
		{Name: "beta", Category: "Second", Description: "Second beta."},
		{Name: "gamma", Category: "First", Description: "First gamma."},
	}
	model := newSelectorModel(commands, 72, 16)
	if got := model.visibleCommandIndexes; len(got) != 3 || got[0] != 0 || got[1] != 2 || got[2] != 1 {
		t.Fatalf("visible command indexes = %v, want [0 2 1]", got)
	}

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(selectorModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	model = updated.(selectorModel)
	if !model.selected[2] || model.selected[1] {
		t.Fatalf("selected commands = %v, want only gamma selected", model.selected)
	}
	plainView := ansi.Strip(model.View().Content)
	if strings.Index(plainView, "alpha") > strings.Index(plainView, "gamma") || strings.Index(plainView, "gamma") > strings.Index(plainView, "beta") {
		t.Fatalf("grouped command order = %q", plainView)
	}
}

func TestSelectorClearsQueryAndHandlesEmptyResultsWithoutCancelling(t *testing.T) {
	model := newSelectorModel([]Command{{Name: "one", Category: "Test", Description: "Only tool."}}, 72, 12)
	model.input.SetValue("absent")
	model.refreshVisibleCommands()
	if !strings.Contains(model.View().Content, "No matching tools.") {
		t.Fatalf("no-results view = %q", model.View().Content)
	}

	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(selectorModel)
	if command != nil || model.input.Value() != "" || model.cancelled || len(model.visibleCommandIndexes) != 1 {
		t.Fatalf("escape changed selector unexpectedly: query = %q, cancelled = %t, visible = %v", model.input.Value(), model.cancelled, model.visibleCommandIndexes)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(selectorModel)
	if command != nil || model.cancelled {
		t.Fatalf("empty-query escape cancelled selector: command = %v, cancelled = %t", command, model.cancelled)
	}
}

func TestSelectorMovesAcrossVisibleCommandsAndResizes(t *testing.T) {
	commands := []Command{
		{Name: "alpha", Category: "Tools", Description: "Matches filter."},
		{Name: "beta", Category: "Tools", Description: "Ignored."},
		{Name: "gamma", Category: "Tools", Description: "Matches filter."},
	}
	model := newSelectorModel(commands, 72, 12)
	model.input.SetValue("match")
	model.refreshVisibleCommands()
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(selectorModel)
	if model.cursor != 2 {
		t.Fatalf("cursor = %d, want original command index 2", model.cursor)
	}
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 30, Height: 8})
	model = updated.(selectorModel)
	if model.width != 30 || model.viewport.Width() != 30 {
		t.Fatalf("resized widths = %d and %d", model.width, model.viewport.Width())
	}
	for _, line := range strings.Split(model.View().Content, "\n") {
		if lipgloss.Width(line) > 30 {
			t.Fatalf("line width = %d, want at most 30: %q", lipgloss.Width(line), line)
		}
	}
}

func TestSelectorEnterOpensConfirmationForSelectedTools(t *testing.T) {
	model := newSelectorModel([]Command{{Name: "one", Category: "Test", Description: "One."}}, 72, 14)
	model.selected[0] = true

	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(selectorModel)

	if command != nil || !model.confirming || model.completed || !model.selected[0] {
		t.Fatalf("Enter state = command %v, confirming %t, completed %t, selected %v", command, model.confirming, model.completed, model.selected)
	}
}

func TestSelectorEnterDoesNothingWithoutSelectedTools(t *testing.T) {
	model := newSelectorModel([]Command{{Name: "one", Category: "Test", Description: "One."}}, 72, 14)

	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(selectorModel)

	if command != nil || model.confirming || model.completed || model.selected[0] {
		t.Fatalf("empty Enter state = command %v, confirming %t, completed %t, selected %v", command, model.confirming, model.completed, model.selected)
	}
}

func TestSelectorConfirmationDefaultsToNoAndPreservesListOnDismissal(t *testing.T) {
	model := newSelectorModel([]Command{{Name: "one", Category: "Test", Description: "One."}}, 72, 16)
	model.selected[0] = true
	model.input.SetValue("one")
	model.refreshVisibleCommands()
	model.confirming = true
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "RUN SELECTED TOOLS?") || !strings.Contains(view, "[No]") || !strings.Contains(view, "←/→ or tab choice") {
		t.Fatalf("confirmation view = %q", view)
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(selectorModel)
	if command != nil || model.confirming || model.completed || model.input.Value() != "one" || !model.selected[0] {
		t.Fatalf("No confirmation changed list: confirming %t, completed %t, query %q, selected %v", model.confirming, model.completed, model.input.Value(), model.selected)
	}

	model.confirming = true
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(selectorModel)
	if model.confirming || model.input.Value() != "one" || !model.selected[0] {
		t.Fatalf("escape changed list: confirming %t, query %q, selected %v", model.confirming, model.input.Value(), model.selected)
	}
}

func TestSelectorConfirmationCanRunCancelAndTruncateSelectedNames(t *testing.T) {
	commands := []Command{}
	for _, name := range []string{"one", "two", "three", "four", "five"} {
		commands = append(commands, Command{Name: name, Category: "Test", Description: "Tool."})
	}
	model := newSelectorModel(commands, 72, 11)
	for index := range model.selected {
		model.selected[index] = true
	}
	model.confirming = true
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "5 tools selected") || !strings.Contains(view, "and 2 more") {
		t.Fatalf("truncated confirmation = %q", view)
	}

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	model = updated.(selectorModel)
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(selectorModel)
	if command == nil || !model.completed {
		t.Fatalf("Yes confirmation did not complete: command = %v, completed = %t", command, model.completed)
	}

	model = newSelectorModel(commands, 72, 20)
	model.confirming = true
	updated, command = model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	model = updated.(selectorModel)
	if command == nil || !model.cancelled || !errors.Is(model.resultError(), ErrCancelled) {
		t.Fatalf("Ctrl+C did not cancel modal: command = %v, cancelled = %t", command, model.cancelled)
	}
}

func TestSelectorConfirmationShowsAllSelectedNamesWhenHeightAllows(t *testing.T) {
	commands := []Command{}
	for _, name := range []string{"one", "two", "three", "four", "five"} {
		commands = append(commands, Command{Name: name, Category: "Test", Description: "Tool."})
	}
	model := newSelectorModel(commands, 72, 20)
	for index := range model.selected {
		model.selected[index] = true
	}
	model.confirming = true
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "five") || strings.Contains(view, "and 2 more") {
		t.Fatalf("spacious confirmation truncated names: %q", view)
	}
}

func TestSelectorCancelsOnlyOnControlC(t *testing.T) {
	model := newSelectorModel([]Command{{Name: "one", Category: "Test", Description: "Tool."}}, 72, 10)
	updated, command := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	model = updated.(selectorModel)
	if command == nil || !model.cancelled || !errors.Is(model.resultError(), ErrCancelled) {
		t.Fatalf("Ctrl+C state: command = %v, cancelled = %t, error = %v", command, model.cancelled, model.resultError())
	}
}

func TestSelectorRendersRequiredStylesWithoutBordersOrBackgrounds(t *testing.T) {
	model := newSelectorModel([]Command{{Name: "selected-tool", Category: "Tools", Description: "A gray description."}}, 72, 10)
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	model = updated.(selectorModel)
	view := model.View().Content

	for _, sequence := range []string{"\x1b[1;37mSELECT TOOLS", "\x1b[34m›", "\x1b[32m◉", "\x1b[32mselected-tool", "\x1b[90mA gray description.", "\x1b[32m1\x1b[0m\x1b[37m tool selected"} {
		if !strings.Contains(view, sequence) {
			t.Fatalf("selector view is missing %q: %q", sequence, view)
		}
	}
	plainView := ansi.Strip(view)
	if strings.Contains(plainView, "type filter") || !strings.Contains(plainView, "↑/↓ move") || !strings.Contains(plainView, "tab toggle") || !strings.Contains(plainView, "enter run") || !strings.Contains(plainView, "esc clear") {
		t.Fatalf("selector controls are not rendered in the documented order: %q", view)
	}
	if strings.Contains(view, "\x1b[4") || strings.ContainsAny(view, "│┌┐└┘") {
		t.Fatalf("selector view contains background or border styling: %q", view)
	}
}

func TestSelectorRendersBoldKeyHints(t *testing.T) {
	model := newSelectorModel([]Command{{Name: "tool", Category: "Tools", Description: "A tool."}}, 72, 14)
	view := model.View().Content
	for _, sequence := range []string{
		"\x1b[1;37m↑/↓\x1b[0m\x1b[90m move",
		"\x1b[1;37mtab\x1b[0m\x1b[90m toggle",
		"\x1b[1;37menter\x1b[0m\x1b[90m run",
		"\x1b[1;37mctrl+c\x1b[0m\x1b[90m cancel",
	} {
		if !strings.Contains(view, sequence) {
			t.Fatalf("selector view is missing bold key hint %q: %q", sequence, view)
		}
	}

	model.selected[0] = true
	model.confirming = true
	view = model.confirmationContent(48)
	if !strings.Contains(view, "\x1b[1;37m←/→ or tab\x1b[0m\x1b[90m choice") {
		t.Fatalf("confirmation content is missing bold key hint: %q", view)
	}
}

func TestSelectorRendersFocusedCommandLineBlueBeforeSelection(t *testing.T) {
	model := newSelectorModel([]Command{{Name: "focused-tool", Category: "Tools", Description: "Gray description."}}, 72, 10)
	view := model.View().Content

	if !strings.Contains(view, "\x1b[34m›\x1b[0m \x1b[34m◯\x1b[0m \x1b[34mfocused-tool\x1b[0m") {
		t.Fatalf("focused command line is not entirely blue: %q", view)
	}
	if !strings.Contains(view, "\x1b[90mGray description.\x1b[0m") {
		t.Fatalf("focused command description did not remain gray: %q", view)
	}
}

func TestToolboxHuhFormsUseBlueFocusAndGreenCommittedSelections(t *testing.T) {
	choice := "first"
	selectForm := toolboxHuhForm(huh.NewSelect[string]().
		Options(huh.NewOption("First", "first"), huh.NewOption("Second", "second")).
		Value(&choice))
	selectForm.Init()
	selectView := selectForm.View()
	if !strings.Contains(selectView, "\x1b[34m> ") || !strings.Contains(selectView, "\x1b[34mFirst") {
		t.Fatalf("single-select focus is not blue: %q", selectView)
	}

	choices := []string{"first"}
	multiSelectForm := toolboxHuhForm(huh.NewMultiSelect[string]().
		Options(huh.NewOption("First", "first"), huh.NewOption("Second", "second")).
		Value(&choices))
	multiSelectForm.Init()
	multiSelectView := multiSelectForm.View()
	if !strings.Contains(multiSelectView, "\x1b[34m> ") || !strings.Contains(multiSelectView, "\x1b[32m✓ ") || !strings.Contains(multiSelectView, "\x1b[32mFirst") {
		t.Fatalf("multi-select focus and committed selection colors are incorrect: %q", multiSelectView)
	}
}

func TestSelectorRendersBrightRedRequirementInsideCommandRow(t *testing.T) {
	catalog := Catalog{Commands: []Command{{
		Name: "setup", Category: "Tools", Description: "Description.", Visibility: "list",
		Protocol: "interactive-script", Environments: []string{"linux-native"},
	}}}
	model := newSelectorModel(filteredCommands(catalog, "linux-native", false), 30, 14)
	view := model.View().Content
	if !strings.Contains(view, "\x1b[91mRequires: Bash") {
		t.Fatalf("selector lacks bright-red requirement: %q", view)
	}
	if model.rowEnds[0]-model.rowStarts[0]+1 != 3 {
		t.Fatalf("row height = %d, want name, description, requirement", model.rowEnds[0]-model.rowStarts[0]+1)
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

func TestSearchModelKeepsInputFocusedAndFiltersWhileTyping(t *testing.T) {
	articles := []Article{
		testIndexedArticle("git/reset.md", "Reset Git", nil, "Rewrite history."),
		testIndexedArticle("python/release.md", "Release Python", nil, "Publish a package."),
	}
	model := newSearchModel(articles, 72, 14)
	if !model.input.Focused() {
		t.Fatal("search input is not focused")
	}
	for _, character := range "git" {
		updated, _ := model.Update(tea.KeyPressMsg{Code: character, Text: string(character)})
		model = updated.(searchModel)
	}
	if model.input.Value() != "git" || len(model.results) != 1 || model.results[0].Article.Title != "Reset Git" {
		t.Fatalf("query = %q, results = %#v", model.input.Value(), model.results)
	}
	view := model.View().Content
	if !strings.Contains(view, "SEARCH GUIDES") || !strings.Contains(view, "Reset Git") || !strings.Contains(view, "Rewrite history.") {
		t.Fatalf("search view = %q", view)
	}
}

func TestSearchModelShowsAllArticlesByCategoryAndMovesAcrossResults(t *testing.T) {
	articles := []Article{
		{Category: "git", Title: "Branches", RelativePath: "git/branches.md", Content: "First paragraph."},
		{Category: "pip", Title: "Release", RelativePath: "pip/release.md", Content: "Second paragraph."},
	}
	model := newSearchModel(articles, 72, 14)
	view := model.View().Content
	for _, text := range []string{"Branches", "Release"} {
		if !strings.Contains(view, text) {
			t.Fatalf("search view missing %q: %q", text, view)
		}
	}
	if !strings.Contains(view, "\x1b[1;37mGIT\x1b[0m") || !strings.Contains(view, "\x1b[1;37mPIP\x1b[0m") {
		t.Fatalf("category headings are not bold, uppercase, and separated: %q", view)
	}
	plainLines := strings.Split(ansi.Strip(view), "\n")
	for _, category := range []string{"GIT", "PIP"} {
		for index, line := range plainLines {
			if strings.TrimSpace(line) == category && index+1 < len(plainLines) && strings.TrimSpace(plainLines[index+1]) == "" {
				break
			}
			if index == len(plainLines)-1 {
				t.Fatalf("category %s is not separated from its articles: %q", category, view)
			}
		}
	}
	if !strings.Contains(view, "\x1b[34mBranches\x1b[0m") {
		t.Fatalf("focused article title is not blue: %q", view)
	}
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(searchModel)
	if model.cursor != 1 {
		t.Fatalf("cursor = %d, want second result", model.cursor)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	model = updated.(searchModel)
	if model.cursor != 0 {
		t.Fatalf("cursor = %d, want first result", model.cursor)
	}
}

func TestSearchModelScrollsResultsAndKeepsFocusedArticleVisible(t *testing.T) {
	articles := []Article{}
	for index := 0; index < 8; index++ {
		articles = append(articles, Article{
			Category:     "guides",
			Title:        "Guide " + string(rune('A'+index)),
			RelativePath: "guides/guide-" + string(rune('a'+index)) + ".md",
			Content:      "Opening paragraph.",
		})
	}
	model := newSearchModel(articles, 30, 8)
	for index := 1; index < len(articles); index++ {
		updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		model = updated.(searchModel)
	}
	view := model.View().Content
	if !strings.Contains(view, "SEARCH GUIDES") || !strings.Contains(view, "Guide H") || !strings.Contains(view, "enter open") {
		t.Fatalf("focused result is not visible with fixed chrome: %q", view)
	}
}

func TestSearchModelResizesResultsAndKeepsThePresentationWidthCap(t *testing.T) {
	article := Article{Category: "test", Title: "A guide title long enough to wrap at narrow widths", Content: "Text."}
	model := newSearchModel([]Article{article}, 40, 10)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	model = updated.(searchModel)
	if model.width != maxPresentationWidth || model.resultsViewport.Width() != maxPresentationWidth {
		t.Fatalf("resized search width = %d, viewport width = %d", model.width, model.resultsViewport.Width())
	}
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 30, Height: 20})
	model = updated.(searchModel)
	for _, line := range strings.Split(model.resultsViewport.View(), "\n") {
		if lipgloss.Width(line) > 30 {
			t.Fatalf("search line width = %d, want at most 30: %q", lipgloss.Width(line), line)
		}
	}
}

func TestSearchModelEscapeCancelsAndEnterWithoutResultsDoesNothing(t *testing.T) {
	model := newSearchModel(nil, 50, 10)
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(searchModel)
	if command != nil || model.resultError() != nil {
		t.Fatal("Enter without a result changed search")
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(searchModel)
	if command == nil || !errors.Is(model.resultError(), ErrCancelled) {
		t.Fatal("Escape did not cancel search")
	}
}

func TestSearchModelShowsNoResultsAndCancelsFromSearch(t *testing.T) {
	model := newSearchModel([]Article{testIndexedArticle("one.md", "One", nil, "First.")}, 72, 10)
	model.input.SetValue("absent")
	model.refreshResults()
	if !strings.Contains(model.View().Content, "No matching guides.") {
		t.Fatalf("no-results view = %q", model.View().Content)
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	model = updated.(searchModel)
	if !model.cancelled || command == nil || !errors.Is(model.resultError(), ErrCancelled) {
		t.Fatalf("cancelled = %t, command = %v, error = %v", model.cancelled, command, model.resultError())
	}
}
