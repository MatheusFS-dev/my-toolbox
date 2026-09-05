package main

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
)

type codeCopyResultMsg struct {
	blockID    int
	generation int
	err        error
}

type clearCodeCopyMsg struct {
	blockID    int
	generation int
}

type searchModel struct {
	articles           []Article
	results            []articleResult
	input              textinput.Model
	resultsViewport    viewport.Model
	reader             viewport.Model
	cursor             int
	rowStarts          []int
	rowEnds            []int
	width              int
	readerWidth        int
	height             int
	reading            bool
	current            Article
	rendered           renderedArticle
	copyState          codeCopyState
	copyGeneration     int
	readerNotice       string
	clipboardWriteFunc func(string) error
	cancelled          bool
	err                error
}

func newSearchModel(articles []Article, terminalWidth int, terminalHeight int) searchModel {
	input := textinput.New()
	input.Prompt = "› "
	input.Placeholder = "Type to search guides"
	input.Focus()
	model := searchModel{
		articles:           append([]Article(nil), articles...),
		input:              input,
		copyState:          codeCopyState{blockID: -1},
		clipboardWriteFunc: clipboard.WriteAll,
	}
	model.resize(terminalWidth, terminalHeight)
	model.refreshResults()
	return model
}

func (model searchModel) Init() tea.Cmd {
	return model.input.Focus()
}

func (model searchModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if window, valid := message.(tea.WindowSizeMsg); valid {
		percent := model.reader.ScrollPercent()
		model.resize(window.Width, window.Height)
		if model.reading {
			model.renderReaderAt(&percent)
		} else {
			model.rebuildResults()
		}
		return model, nil
	}
	if result, valid := message.(codeCopyResultMsg); valid {
		if result.generation != model.copyGeneration || result.blockID != model.copyState.blockID {
			return model, nil
		}
		if result.err != nil {
			model.copyState.feedback = codeCopySent
		} else {
			model.copyState.feedback = codeCopyCopied
		}
		model.rerenderReaderPreservingOffset()
		generation := result.generation
		blockID := result.blockID
		return model, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
			return clearCodeCopyMsg{blockID: blockID, generation: generation}
		})
	}
	if clear, valid := message.(clearCodeCopyMsg); valid {
		if clear.generation != model.copyGeneration {
			return model, nil
		}
		if clear.blockID >= 0 && clear.blockID != model.copyState.blockID {
			return model, nil
		}
		model.copyState = codeCopyState{blockID: -1}
		model.readerNotice = ""
		model.rerenderReaderPreservingOffset()
		return model, nil
	}
	key, isKey := message.(tea.KeyPressMsg)
	if isKey && key.Code == 'c' && key.Mod.Contains(tea.ModCtrl) {
		model.cancelled = true
		return model, tea.Quit
	}
	if model.reading {
		if isKey {
			switch key.Code {
			case tea.KeyEscape:
				model.reading = false
				model.copyGeneration++
				model.copyState = codeCopyState{blockID: -1}
				model.readerNotice = ""
				model.input.Focus()
				return model, nil
			case 'c':
				return model.copyFirstVisibleCodeBlock()
			case tea.KeyUp:
				model.reader.ScrollUp(1)
				return model, nil
			case tea.KeyDown:
				model.reader.ScrollDown(1)
				return model, nil
			case tea.KeyPgUp:
				model.reader.PageUp()
				return model, nil
			case tea.KeyPgDown:
				model.reader.PageDown()
				return model, nil
			case tea.KeyHome:
				model.reader.GotoTop()
				return model, nil
			case tea.KeyEnd:
				model.reader.GotoBottom()
				return model, nil
			}
		}
		if mouse, valid := message.(tea.MouseClickMsg); valid && mouse.Button == tea.MouseLeft {
			contentLine := model.reader.YOffset() + mouse.Y - 1
			if mouse.Y >= 1 && mouse.Y <= model.reader.Height() {
				for _, block := range model.rendered.codeBlocks {
					if contentLine == block.headerLine && mouse.X >= block.copyStartColumn && mouse.X < block.copyEndColumn {
						return model.startCodeCopy(block)
					}
				}
			}
			return model, nil
		}
		updated, command := model.reader.Update(message)
		model.reader = updated
		return model, command
	}
	if isKey {
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
			if len(model.results) > 0 {
				model.current = model.results[model.cursor].Article
				model.reading = true
				model.copyGeneration++
				model.copyState = codeCopyState{blockID: -1}
				model.readerNotice = ""
				model.input.Blur()
				model.renderReader()
				if model.err != nil {
					return model, tea.Quit
				}
			}
			return model, nil
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
	if model.reading {
		footer := model.readerFooter()
		view := tea.NewView(model.readerHeader() + "\n" + model.reader.View() + "\n" + footer)
		view.MouseMode = tea.MouseModeCellMotion
		return view
	}
	title := presentationStyle("SEARCH GUIDES", ansiBoldWhite, true)
	footer := model.searchFooter()
	return tea.NewView(title + "\n" + model.input.View() + "\n\n" + model.resultsViewport.View() + "\n" + footer)
}

func (model searchModel) resultError() error {
	if model.err != nil {
		return model.err
	}
	if model.cancelled {
		return ErrCancelled
	}
	return nil
}

func (model *searchModel) resize(terminalWidth int, terminalHeight int) {
	model.width = min(max(1, terminalWidth), maxPresentationWidth)
	model.readerWidth = max(1, terminalWidth)
	model.height = max(1, terminalHeight)
	model.input.SetWidth(max(1, model.width-2))
	model.resultsViewport.SetWidth(model.width)
	searchFooterHeight := len(strings.Split(model.searchFooter(), "\n"))
	model.resultsViewport.SetHeight(max(1, model.height-3-searchFooterHeight))
	model.reader.SetWidth(model.readerWidth)
	readerFooterHeight := len(strings.Split(model.readerFooter(), "\n"))
	model.reader.SetHeight(max(1, model.height-1-readerFooterHeight))
}

func (model searchModel) searchFooter() string {
	return presentationStyle(strings.Join(wrapText("↑/↓ move • enter open • esc exit", model.width), "\n"), ansiGray, true)
}

func (model searchModel) readerFooter() string {
	text := "↑/↓ or wheel scroll • pgup/pgdn page • home/end jump • c copy code • esc back"
	if model.readerNotice != "" {
		text = model.readerNotice
	}
	return presentationStyle(ansi.Truncate(text, model.readerWidth, ""), ansiGray, true)
}

func (model searchModel) readerHeader() string {
	label := strings.ToUpper(model.current.Category) + " / " + model.rendered.title
	progress := fmt.Sprintf("%d%%", int(model.reader.ScrollPercent()*100+0.5))
	if lipgloss.Width(label)+1+lipgloss.Width(progress) <= model.readerWidth {
		label = label + strings.Repeat(" ", model.readerWidth-lipgloss.Width(label)-lipgloss.Width(progress)) + progress
	} else {
		label = ansi.Truncate(label, model.readerWidth, "")
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#c0caf5")).Background(lipgloss.Color(codeHeaderBackground)).Bold(true).Width(model.readerWidth).Render(label)
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

func (model *searchModel) renderReader() {
	model.renderReaderAt(nil)
}

func (model *searchModel) renderReaderAt(percent *float64) {
	rendered, err := renderMarkdownArticle(model.current, model.readerWidth, model.copyState)
	if err != nil {
		model.err = err
		return
	}
	model.err = nil
	model.rendered = rendered
	model.reader.SetContent(strings.Join(rendered.lines, "\n"))
	if percent == nil {
		model.reader.GotoTop()
		return
	}
	maxOffset := max(0, model.reader.TotalLineCount()-model.reader.Height())
	model.reader.SetYOffset(int(*percent*float64(maxOffset) + 0.5))
}

func (model *searchModel) rerenderReaderPreservingOffset() {
	offset := model.reader.YOffset()
	rendered, err := renderMarkdownArticle(model.current, model.readerWidth, model.copyState)
	if err != nil {
		model.err = err
		return
	}
	model.err = nil
	model.rendered = rendered
	model.reader.SetContent(strings.Join(rendered.lines, "\n"))
	model.reader.SetYOffset(offset)
}

func (model searchModel) copyFirstVisibleCodeBlock() (tea.Model, tea.Cmd) {
	top := model.reader.YOffset()
	bottom := top + model.reader.Height()
	for _, block := range model.rendered.codeBlocks {
		if block.headerLine < bottom && block.bodyEndLine > top {
			return model.startCodeCopy(block)
		}
	}
	model.copyGeneration++
	model.copyState = codeCopyState{blockID: -1, feedback: codeCopyNoVisible}
	model.readerNotice = "No code block visible"
	generation := model.copyGeneration
	return model, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return clearCodeCopyMsg{blockID: -1, generation: generation}
	})
}

func (model searchModel) startCodeCopy(block renderedCodeBlock) (tea.Model, tea.Cmd) {
	model.copyGeneration++
	model.copyState = codeCopyState{blockID: block.id, feedback: codeCopyCopying}
	model.readerNotice = ""
	model.rerenderReaderPreservingOffset()
	generation := model.copyGeneration
	write := model.clipboardWriteFunc
	if write == nil {
		write = clipboard.WriteAll
	}
	native := func() tea.Msg {
		return codeCopyResultMsg{blockID: block.id, generation: generation, err: write(block.rawCode)}
	}
	return model, tea.Batch(native, tea.SetClipboard(block.rawCode))
}

var _ tea.Model = searchModel{}
