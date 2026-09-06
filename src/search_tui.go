package main

import (
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

type searchModel struct {
	articles        []Article
	results         []articleResult
	input           textinput.Model
	resultsViewport viewport.Model
	cursor          int
	rowStarts       []int
	rowEnds         []int
	width           int
	height          int
	readerCommand   func(Article) (*exec.Cmd, error)
	fallback        *articleReaderResultMsg
	cancelled       bool
}

func newSearchModel(articles []Article, terminalWidth int, terminalHeight int) searchModel {
	input := textinput.New()
	input.Prompt = "› "
	input.Placeholder = "Type to search guides"
	input.Focus()
	model := searchModel{
		articles:      append([]Article(nil), articles...),
		input:         input,
		readerCommand: newArticleReaderCommand,
	}
	model.resize(terminalWidth, terminalHeight)
	model.refreshResults()
	return model
}

func (model searchModel) Init() tea.Cmd {
	return model.input.Focus()
}

func (model searchModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.resize(message.Width, message.Height)
		model.rebuildResults()
		return model, nil
	case articleReaderResultMsg:
		if message.err != nil {
			model.fallback = &message
			return model, tea.Quit
		}
		return model, nil
	}

	if key, isKey := message.(tea.KeyPressMsg); isKey {
		if key.Code == 'c' && key.Mod.Contains(tea.ModCtrl) {
			model.cancelled = true
			return model, tea.Quit
		}
		switch key.Code {
		case tea.KeyEscape:
			model.cancelled = true
			return model, tea.Quit
		case tea.KeyUp:
			if model.cursor > 0 {
				model.cursor--
				model.rebuildResults()
			}
			return model, nil
		case tea.KeyDown:
			if model.cursor+1 < len(model.results) {
				model.cursor++
				model.rebuildResults()
			}
			return model, nil
		case tea.KeyEnter:
			if len(model.results) == 0 {
				return model, nil
			}
			article := model.results[model.cursor].Article
			command, err := model.readerCommand(article)
			complete := func(err error) tea.Msg {
				return articleReaderResultMsg{article: article, err: err}
			}
			if err != nil {
				return model, func() tea.Msg { return complete(err) }
			}
			return model, tea.ExecProcess(command, complete)
		}
	}
	previous := model.input.Value()
	updatedInput, command := model.input.Update(message)
	model.input = updatedInput
	if model.input.Value() != previous {
		model.refreshResults()
	}
	return model, command
}

func (model searchModel) View() tea.View {
	title := presentationStyle("SEARCH GUIDES", ansiBoldWhite, true)
	footer := model.searchFooter()
	return tea.NewView(title + "\n" + model.input.View() + "\n\n" + model.resultsViewport.View() + "\n" + footer)
}

func (model searchModel) resultError() error {
	if model.cancelled {
		return ErrCancelled
	}
	return nil
}

func (model *searchModel) resize(terminalWidth int, terminalHeight int) {
	model.width = min(max(1, terminalWidth), maxPresentationWidth)
	model.height = max(1, terminalHeight)
	model.input.SetWidth(max(1, model.width-2))
	model.resultsViewport.SetWidth(model.width)
	searchFooterHeight := len(strings.Split(model.searchFooter(), "\n"))
	model.resultsViewport.SetHeight(max(1, model.height-3-searchFooterHeight))
}

func (model searchModel) searchFooter() string {
	return presentationStyle(strings.Join(wrapText("↑/↓ move • enter open • esc exit", model.width), "\n"), ansiGray, true)
}

func (model *searchModel) refreshResults() {
	model.results = searchArticles(model.articles, model.input.Value())
	model.cursor = 0
	model.resultsViewport.GotoTop()
	model.rebuildResults()
}

func (model *searchModel) rebuildResults() {
	if len(model.results) == 0 {
		model.rowStarts = nil
		model.rowEnds = nil
		model.resultsViewport.SetContent(presentationStyle("No matching guides.", ansiGray, true))
		return
	}
	lines := []string{}
	model.rowStarts = make([]int, len(model.results))
	model.rowEnds = make([]int, len(model.results))
	previousCategory := ""
	emptyQuery := normalizeText(model.input.Value()) == ""
	for index, result := range model.results {
		if emptyQuery && result.Article.Category != previousCategory {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, "  "+presentationStyle(strings.ToUpper(result.Article.Category), ansiBoldWhite, true), "")
			previousCategory = result.Article.Category
		}
		model.rowStarts[index] = len(lines)
		cursor := "  "
		if index == model.cursor {
			cursor = presentationStyle("›", ansiBlue, true) + " "
		}
		indent := 2
		if emptyQuery {
			indent = 4
			cursor = "    "
			if index == model.cursor {
				cursor = "  " + presentationStyle("›", ansiBlue, true) + " "
			}
		}
		titleColor := ansiWhite
		if index == model.cursor {
			titleColor = ansiBlue
		}
		for lineIndex, line := range wrapText(result.Article.Title, max(1, model.width-indent)) {
			prefix := strings.Repeat(" ", indent)
			if lineIndex == 0 {
				prefix = cursor
			}
			lines = append(lines, prefix+presentationStyle(line, titleColor, true))
		}
		if !emptyQuery && result.Excerpt != "" {
			for _, line := range wrapText(result.Excerpt, max(1, model.width-4)) {
				lines = append(lines, "    "+presentationStyle(line, ansiGray, true))
			}
		}
		model.rowEnds[index] = len(lines) - 1
	}
	model.resultsViewport.SetContent(strings.Join(lines, "\n"))
	model.keepResultVisible()
}

func (model *searchModel) keepResultVisible() {
	if model.cursor >= len(model.rowStarts) {
		return
	}
	top := model.resultsViewport.YOffset()
	bottom := top + model.resultsViewport.Height() - 1
	if model.rowStarts[model.cursor] < top {
		model.resultsViewport.SetYOffset(model.rowStarts[model.cursor])
	} else if model.rowEnds[model.cursor] > bottom {
		model.resultsViewport.SetYOffset(model.rowEnds[model.cursor] - model.resultsViewport.Height() + 1)
	}
}

var _ tea.Model = searchModel{}
