package main

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestRenderMarkdownArticlePreservesRichMarkdownAndMovesFirstTitle(t *testing.T) {
	article := Article{
		Category: "docs",
		Title:    "Fallback",
		Content: "# Actual Title\n\n## Features\n\n**bold** and *italic*, [site](https://example.com), `code`, and :rocket:.\n\n" +
			"> quote\n\n- [x] done\n\nTerm\n: definition\n\n| A | B |\n|---|---|\n| 1 | 2 |\n\n![diagram](https://example.com/image.png)\n\n---\n",
	}

	rendered, err := renderMarkdownArticle(article, 80, codeCopyState{})
	if err != nil {
		t.Fatal(err)
	}
	plain := ansi.Strip(strings.Join(rendered.lines, "\n"))
	if rendered.title != "Actual Title" {
		t.Fatalf("title = %q, want Actual Title", rendered.title)
	}
	if strings.Contains(plain, "# Actual Title") {
		t.Fatalf("first H1 remains in article body: %q", plain)
	}
	for _, want := range []string{"Features", "bold", "italic", "site", "https://example.com", "code", "🚀", "quote", "done", "Term", "definition", "A", "B", "diagram", "https://example.com/image.png"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered Markdown missing %q: %q", want, plain)
		}
	}
}

func TestRenderMarkdownArticleUsesFallbackTitleWithoutAddingItToBody(t *testing.T) {
	article := Article{Title: "Filename Title", Content: "A paragraph without a heading."}

	rendered, err := renderMarkdownArticle(article, 40, codeCopyState{})
	if err != nil {
		t.Fatal(err)
	}
	if rendered.title != "Filename Title" {
		t.Fatalf("title = %q", rendered.title)
	}
	if strings.Contains(ansi.Strip(strings.Join(rendered.lines, "\n")), "Filename Title") {
		t.Fatal("fallback title was duplicated into the body")
	}
}

func TestRenderMarkdownArticleExtractsExactCodeAndBuildsOrderedCards(t *testing.T) {
	article := Article{Title: "Code", Content: "Before.\n\n```go\nfmt.Println(\"hi\")\n```\n\n    indented\n    code\n\n```mystery-lang\nunknown()\n```\n\n```txt\n```\n\n~~~sh\nprintf eof"}

	rendered, err := renderMarkdownArticle(article, 34, codeCopyState{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered.codeBlocks) != 5 {
		t.Fatalf("code blocks = %#v, want 5", rendered.codeBlocks)
	}
	wantRaw := []string{"fmt.Println(\"hi\")\n", "indented\ncode\n", "unknown()\n", "", "printf eof"}
	wantLanguages := []string{"GO", "CODE", "MYSTERY-LANG", "TXT", "SH"}
	for index, block := range rendered.codeBlocks {
		if block.id != index || block.rawCode != wantRaw[index] || block.language != wantLanguages[index] {
			t.Fatalf("block %d = %#v, want raw %q language %q", index, block, wantRaw[index], wantLanguages[index])
		}
		if block.headerLine < 0 || block.bodyStartLine != block.headerLine+1 || block.bodyEndLine <= block.bodyStartLine {
			t.Fatalf("invalid line range for block %d: %#v", index, block)
		}
		if block.copyStartColumn < 0 || block.copyEndColumn <= block.copyStartColumn || block.copyEndColumn > 34 {
			t.Fatalf("invalid copy range for block %d: %#v", index, block)
		}
	}
	output := strings.Join(rendered.lines, "\n")
	plain := ansi.Strip(output)
	if strings.Contains(plain, "TB_CODE_") {
		t.Fatalf("temporary marker leaked: %q", plain)
	}
	if !strings.Contains(output, "\x1b[") || !strings.Contains(plain, "fmt.Println") || !strings.Contains(plain, "unknown()") {
		t.Fatalf("code output lacks highlighting or fallback content: %q", output)
	}
}

func TestRenderMarkdownArticleWrapsCodeAndKeepsVisibleCopyHitRange(t *testing.T) {
	article := Article{Title: "Narrow", Content: "```an-extraordinarily-long-language-name\nabcdefghijklmnopqrstuvwxyz0123456789\n```"}

	for _, width := range []int{9, 16, 24} {
		rendered, err := renderMarkdownArticle(article, width, codeCopyState{})
		if err != nil {
			t.Fatal(err)
		}
		if len(rendered.codeBlocks) != 1 {
			t.Fatalf("width %d: blocks = %#v", width, rendered.codeBlocks)
		}
		block := rendered.codeBlocks[0]
		for _, line := range rendered.lines {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("width %d: rendered line is %d cells: %q", width, got, line)
			}
		}
		header := ansi.Strip(rendered.lines[block.headerLine])
		if block.copyEndColumn > lipgloss.Width(header) || block.copyEndColumn <= block.copyStartColumn {
			t.Fatalf("width %d: button range %#v does not match header %q", width, block, header)
		}
	}
}

func TestRenderMarkdownArticleShowsCopyFeedbackForOnlySelectedBlock(t *testing.T) {
	article := Article{Title: "Copy", Content: "```go\none\n```\n\n```sh\ntwo\n```"}

	rendered, err := renderMarkdownArticle(article, 30, codeCopyState{blockID: 1, feedback: codeCopyCopied})
	if err != nil {
		t.Fatal(err)
	}
	first := ansi.Strip(rendered.lines[rendered.codeBlocks[0].headerLine])
	second := ansi.Strip(rendered.lines[rendered.codeBlocks[1].headerLine])
	if !strings.Contains(first, "[ Copy ]") || !strings.Contains(second, "[ Copied! ]") {
		t.Fatalf("copy feedback headers = %q and %q", first, second)
	}
}

func TestRenderMarkdownArticlePreservesCodeLineEndingsExactly(t *testing.T) {
	article := Article{Title: "CRLF", Content: "```text\r\nfirst\r\nsecond\r\n```\r\n"}

	rendered, err := renderMarkdownArticle(article, 30, codeCopyState{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered.codeBlocks) != 1 || rendered.codeBlocks[0].rawCode != "first\r\nsecond\r\n" {
		t.Fatalf("raw code = %q, want original CRLF line endings", rendered.codeBlocks[0].rawCode)
	}
}
