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

	for _, sequence := range []string{"\x1b[1;37mSELECT TOOLS", "\x1b[34m›", "\x1b[32m◉", "\x1b[32mselected-tool", "\x1b[90mA gray description.", "\x1b[90m↑/↓ move", "\x1b[32m1\x1b[0m\x1b[90m tool selected"} {
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
	model := newSelectorModel(filteredCommands(catalog, "linux-native", false), 30, 8)
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

func TestSearchModelOpensReaderAndEscapeRestoresSearchState(t *testing.T) {
	article := testIndexedArticle("git/reset.md", "Reset Git", []string{"Safety"}, "Keep backups before rewriting history.")
	model := newSearchModel([]Article{article}, 60, 14)
	updated, _ := model.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	model = updated.(searchModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(searchModel)
	if !model.reading || !strings.Contains(model.View().Content, "Reset") || !strings.Contains(model.reader.GetContent(), "Keep backups") {
		t.Fatalf("reader state = reading %t, view %q", model.reading, model.View().Content)
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(searchModel)
	if command != nil || model.reading || !model.input.Focused() || model.input.Value() != "g" || len(model.results) != 1 {
		t.Fatalf("restored state = reading %t, query %q, results %d", model.reading, model.input.Value(), len(model.results))
	}
}

func TestSearchReaderScrollsAndRerendersOnResize(t *testing.T) {
	lines := []string{"# Long Guide"}
	for index := 0; index < 80; index++ {
		lines = append(lines, "", "Paragraph with enough words to wrap across a narrow reader viewport and continue well beyond the old seventy-two-column presentation limit.")
	}
	article := Article{Category: "test", Title: "Long Guide", RelativePath: "test/long.md", Content: strings.Join(lines, "\n")}
	model := newSearchModel([]Article{article}, 40, 10)
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(searchModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	model = updated.(searchModel)
	if model.reader.YOffset() == 0 {
		t.Fatal("page down did not scroll reader")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	model = updated.(searchModel)
	if !model.reader.AtBottom() {
		t.Fatal("end did not move reader to bottom")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	model = updated.(searchModel)
	if model.reader.YOffset() != 0 {
		t.Fatalf("home offset = %d", model.reader.YOffset())
	}
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	model = updated.(searchModel)
	if model.width != maxPresentationWidth || model.reader.Width() != 120 {
		t.Fatalf("resized width = %d, reader width = %d", model.width, model.reader.Width())
	}
	usesWideReader := false
	for _, line := range strings.Split(ansi.Strip(model.reader.GetContent()), "\n") {
		if lipgloss.Width(strings.TrimSpace(line)) > maxPresentationWidth {
			usesWideReader = true
			break
		}
	}
	if !usesWideReader {
		t.Fatalf("reader content did not expand beyond %d columns", maxPresentationWidth)
	}
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 30, Height: 20})
	model = updated.(searchModel)
	for _, line := range strings.Split(model.View().Content, "\n") {
		if lipgloss.Width(line) > 30 {
			t.Fatalf("reader line width = %d, want at most 30: %q", lipgloss.Width(line), line)
		}
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
