package main

import (
	"fmt"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	terminalansi "github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	emoji "github.com/yuin/goldmark-emoji"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

const (
	codeHeaderBackground = "#24283b"
	codeBodyBackground   = "#1a1b26"
)

type codeCopyFeedback int

const (
	codeCopyIdle codeCopyFeedback = iota
	codeCopyCopying
	codeCopyCopied
	codeCopySent
	codeCopyNoVisible
)

type codeCopyState struct {
	blockID  int
	feedback codeCopyFeedback
}

type renderedArticle struct {
	title      string
	lines      []string
	codeBlocks []renderedCodeBlock
}

type renderedCodeBlock struct {
	id              int
	rawCode         string
	language        string
	headerLine      int
	bodyStartLine   int
	bodyEndLine     int
	copyStartColumn int
	copyEndColumn   int
}

type sourceCodeBlock struct {
	rawCode  string
	language string
}

// renderMarkdownArticle renders one article and records interactive code-card geometry.
//
// Args:
//   - article: Markdown article to render.
//   - width: Maximum terminal width in cells.
//   - copyState: Feedback to show on the active code card.
//
// Returns:
//   - renderedArticle: Rendered lines, title, and code-block metadata.
//   - error: Markdown renderer configuration or rendering failure.
func renderMarkdownArticle(article Article, width int, copyState codeCopyState) (renderedArticle, error) {
	width = max(1, width)
	source := []byte(article.Content)
	title, titleStart, titleEnd, blocks := inspectMarkdown(source, article.Title)
	body := removeSourceRange(source, titleStart, titleEnd)
	startMarker, endMarker := codeMarkers(string(body))

	style := styles.TokyoNightStyleConfig
	zero := uint(0)
	style.Document.Margin = &zero
	style.CodeBlock.Margin = &zero
	style.CodeBlock.BlockPrefix = startMarker + "\n"
	style.CodeBlock.BlockSuffix = "\n" + endMarker
	style.CodeBlock.BackgroundColor = stringPointer(codeBodyBackground)
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
		glamour.WithTableWrap(true),
	)
	if err != nil {
		return renderedArticle{}, fmt.Errorf("configure article reader: %w", err)
	}
	defer renderer.Close()
	rendered, err := renderer.RenderBytes(body)
	if err != nil {
		return renderedArticle{}, fmt.Errorf("render article %s: %w", article.RelativePath, err)
	}

	lines, metadata := replaceCodeMarkers(strings.Split(strings.TrimRight(string(rendered), "\n"), "\n"), blocks, width, copyState, startMarker, endMarker)
	return renderedArticle{title: title, lines: lines, codeBlocks: metadata}, nil
}

func inspectMarkdown(source []byte, fallbackTitle string) (string, int, int, []sourceCodeBlock) {
	markdown := goldmark.New(goldmark.WithExtensions(extension.GFM, extension.DefinitionList, emoji.New()))
	document := markdown.Parser().Parse(text.NewReader(source))
	title := fallbackTitle
	titleStart := -1
	titleEnd := -1
	blocks := []sourceCodeBlock{}
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch typed := node.(type) {
		case *ast.Heading:
			if typed.Level == 1 && titleStart < 0 {
				title = strings.TrimSpace(string(typed.Text(source)))
				titleStart, titleEnd = headingSourceRange(source, typed)
			}
		case *ast.FencedCodeBlock:
			blocks = append(blocks, sourceCodeBlock{rawCode: nodeLines(source, typed.Lines()), language: normalizeCodeLanguage(string(typed.Language(source)))})
		case *ast.CodeBlock:
			blocks = append(blocks, sourceCodeBlock{rawCode: nodeLines(source, typed.Lines()), language: "CODE"})
		}
		return ast.WalkContinue, nil
	})
	if strings.TrimSpace(title) == "" {
		title = "Untitled"
	}
	return title, titleStart, titleEnd, blocks
}

func headingSourceRange(source []byte, heading *ast.Heading) (int, int) {
	if heading.Lines().Len() == 0 {
		return -1, -1
	}
	start := heading.Lines().At(0).Start
	for start > 0 && source[start-1] != '\n' {
		start--
	}
	last := heading.Lines().At(heading.Lines().Len() - 1)
	end := last.Stop
	for end < len(source) && source[end] != '\n' {
		end++
	}
	if end < len(source) {
		end++
	}
	// Setext headings store only their text line in Lines; remove the underline too.
	if end < len(source) {
		nextEnd := end
		for nextEnd < len(source) && source[nextEnd] != '\n' {
			nextEnd++
		}
		next := strings.TrimSpace(string(source[end:nextEnd]))
		if next != "" && strings.Trim(next, "=") == "" {
			end = nextEnd
			if end < len(source) {
				end++
			}
		}
	}
	return start, end
}

func removeSourceRange(source []byte, start int, end int) []byte {
	if start < 0 || end < start {
		return source
	}
	result := make([]byte, 0, len(source)-(end-start))
	result = append(result, source[:start]...)
	result = append(result, source[end:]...)
	return result
}

func nodeLines(source []byte, lines *text.Segments) string {
	var result strings.Builder
	for index := 0; index < lines.Len(); index++ {
		segment := lines.At(index)
		result.WriteString(strings.Repeat(" ", segment.Padding))
		result.Write(source[segment.Start:segment.Stop])
	}
	return result.String()
}

func normalizeCodeLanguage(language string) string {
	language = strings.TrimSpace(language)
	if language == "" {
		return "CODE"
	}
	return strings.ToUpper(language)
}

func codeMarkers(source string) (string, string) {
	seed := "§"
	for strings.Contains(source, seed+"S§") || strings.Contains(source, seed+"E§") {
		seed += "§"
	}
	return seed + "S§", seed + "E§"
}

func replaceCodeMarkers(lines []string, blocks []sourceCodeBlock, width int, copyState codeCopyState, startMarker string, endMarker string) ([]string, []renderedCodeBlock) {
	result := []string{}
	metadata := make([]renderedCodeBlock, 0, len(blocks))
	blockIndex := 0
	for lineIndex := 0; lineIndex < len(lines); lineIndex++ {
		plain := terminalansi.Strip(lines[lineIndex])
		if !strings.Contains(plain, startMarker) || blockIndex >= len(blocks) {
			if !strings.Contains(plain, endMarker) {
				result = append(result, fitRenderedLine(lines[lineIndex], width))
			}
			continue
		}

		block := blocks[blockIndex]
		header, copyStart, copyEnd := renderCodeHeader(block.language, width, copyButtonLabel(copyState, blockIndex))
		entry := renderedCodeBlock{id: blockIndex, rawCode: block.rawCode, language: block.language, headerLine: len(result), copyStartColumn: copyStart, copyEndColumn: copyEnd}
		result = append(result, header)
		entry.bodyStartLine = len(result)
		lineIndex++
		bodyLines := []string{}
		for lineIndex < len(lines) && !strings.Contains(terminalansi.Strip(lines[lineIndex]), endMarker) {
			wrapped := terminalansi.Hardwrap(lines[lineIndex], width, true)
			bodyLines = append(bodyLines, strings.Split(wrapped, "\n")...)
			lineIndex++
		}
		if len(bodyLines) == 0 {
			bodyLines = []string{""}
		}
		bodyStyle := lipgloss.NewStyle().Background(lipgloss.Color(codeBodyBackground)).Width(width)
		for _, bodyLine := range bodyLines {
			result = append(result, bodyStyle.Render(terminalansi.Truncate(bodyLine, width, "")))
		}
		entry.bodyEndLine = len(result)
		metadata = append(metadata, entry)
		blockIndex++
	}
	return result, metadata
}

func copyButtonLabel(state codeCopyState, blockID int) string {
	if state.blockID != blockID {
		return "[ Copy ]"
	}
	switch state.feedback {
	case codeCopyCopying:
		return "[ Copying ]"
	case codeCopyCopied:
		return "[ Copied! ]"
	case codeCopySent:
		return "[ Sent ]"
	default:
		return "[ Copy ]"
	}
}

func renderCodeHeader(language string, width int, button string) (string, int, int) {
	buttonWidth := min(width, lipgloss.Width(button))
	visibleButton := terminalansi.Truncate(button, buttonWidth, "")
	languageWidth := max(0, width-buttonWidth)
	visibleLanguage := terminalansi.Truncate(language, languageWidth, "")
	gap := width - lipgloss.Width(visibleLanguage) - lipgloss.Width(visibleButton)
	plain := visibleLanguage + strings.Repeat(" ", gap) + visibleButton
	copyStart := width - lipgloss.Width(visibleButton)
	header := lipgloss.NewStyle().Foreground(lipgloss.Color("#a9b1d6")).Background(lipgloss.Color(codeHeaderBackground)).Width(width).Render(plain)
	return header, copyStart, width
}

func fitRenderedLine(line string, width int) string {
	if lipgloss.Width(line) <= width {
		return line
	}
	return terminalansi.Truncate(line, width, "")
}

func stringPointer(value string) *string {
	return &value
}
