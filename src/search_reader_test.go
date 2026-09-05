package main

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestSearchReaderEnablesMouseOnlyWhileReadingAndRendersFixedHeader(t *testing.T) {
	model := newSearchModel([]Article{{Category: "git", Title: "Fallback", Content: "# Reset History\n\nParagraph."}}, 50, 10)
	if model.View().MouseMode != tea.MouseModeNone {
		t.Fatal("search view enabled mouse reporting")
	}
	model = openTestReader(t, model)
	view := model.View()
	if view.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("reader mouse mode = %v", view.MouseMode)
	}
	firstLine := ansi.Strip(strings.Split(view.Content, "\n")[0])
	if !strings.Contains(firstLine, "GIT / Reset History") || !strings.Contains(firstLine, "%") {
		t.Fatalf("reader header = %q", firstLine)
	}
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(searchModel)
	if model.View().MouseMode != tea.MouseModeNone {
		t.Fatal("mouse reporting remained enabled after leaving reader")
	}
}

func TestSearchReaderOmitsProgressWhenHeaderIsTooNarrow(t *testing.T) {
	model := openTestReader(t, newSearchModel([]Article{{Category: "long-category", Title: "Long title", Content: "Text."}}, 12, 6))
	firstLine := ansi.Strip(strings.Split(model.View().Content, "\n")[0])
	if strings.Contains(firstLine, "%") || len([]rune(firstLine)) > 12 {
		t.Fatalf("narrow reader header = %q", firstLine)
	}
}

func TestSearchReaderWheelScrollsAndClickCopiesExactBlock(t *testing.T) {
	article := Article{Category: "test", Title: "Mouse", Content: "Intro.\n\n```go\nfmt.Println(1)\n```\n\n" + strings.Repeat("more text\n\n", 20)}
	model := openTestReader(t, newSearchModel([]Article{article}, 32, 8))
	block := model.rendered.codeBlocks[0]
	nativeCopies := []string{}
	model.clipboardWriteFunc = func(value string) error {
		nativeCopies = append(nativeCopies, value)
		return nil
	}

	updated, _ := model.Update(tea.MouseWheelMsg{X: 1, Y: 3, Button: tea.MouseWheelDown})
	model = updated.(searchModel)
	if model.reader.YOffset() == 0 {
		t.Fatal("mouse wheel did not scroll the reader")
	}
	model.reader.GotoTop()
	screenY := 1 + block.headerLine
	updated, command := model.Update(tea.MouseClickMsg{X: block.copyStartColumn, Y: screenY, Button: tea.MouseLeft})
	model = updated.(searchModel)
	if command == nil || model.copyState.feedback != codeCopyCopying {
		t.Fatalf("copy click state = %#v, command = %v", model.copyState, command)
	}
	runCopyBatch(t, command, &model)
	if len(nativeCopies) != 1 || nativeCopies[0] != "fmt.Println(1)\n" {
		t.Fatalf("native copies = %#v", nativeCopies)
	}

	currentBlock := model.rendered.codeBlocks[0]
	updated, missCommand := model.Update(tea.MouseClickMsg{X: currentBlock.copyStartColumn - 1, Y: screenY, Button: tea.MouseLeft})
	model = updated.(searchModel)
	if missCommand != nil {
		t.Fatal("click outside Copy hit region triggered a command")
	}
}

func TestSearchReaderKeyboardCopiesFirstVisibleCodeBlock(t *testing.T) {
	article := Article{Category: "test", Title: "Keyboard", Content: "```go\nfirst\n```\n\n```sh\nsecond\n```"}
	model := openTestReader(t, newSearchModel([]Article{article}, 30, 5))
	copied := ""
	model.clipboardWriteFunc = func(value string) error { copied = value; return nil }
	model.reader.SetYOffset(model.rendered.codeBlocks[1].headerLine)

	updated, command := model.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	model = updated.(searchModel)
	if command == nil || model.copyState.blockID != 1 {
		t.Fatalf("keyboard selected copy state %#v", model.copyState)
	}
	runCopyBatch(t, command, &model)
	if copied != "second\n" {
		t.Fatalf("copied = %q", copied)
	}
}

func TestSearchReaderReportsWhenNoCodeBlockIsVisible(t *testing.T) {
	article := Article{Category: "test", Title: "No visible", Content: strings.Repeat("paragraph\n\n", 15) + "```go\nlate\n```"}
	model := openTestReader(t, newSearchModel([]Article{article}, 30, 7))

	updated, command := model.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	model = updated.(searchModel)
	if command == nil || !strings.Contains(ansi.Strip(model.View().Content), "No code block visible") {
		t.Fatalf("missing no-code feedback: %q", model.View().Content)
	}
}

func TestSearchReaderAlwaysSendsOSC52AndHandlesNativeFailure(t *testing.T) {
	article := Article{Category: "test", Title: "Fallback", Content: "```txt\ncopy me\n```"}
	model := openTestReader(t, newSearchModel([]Article{article}, 30, 7))
	model.clipboardWriteFunc = func(string) error { return errors.New("clipboard unavailable") }

	updated, command := model.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	model = updated.(searchModel)
	batch, ok := command().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("copy command = %#v, want native and OSC 52 commands", command())
	}
	var result codeCopyResultMsg
	for _, child := range batch {
		if message, valid := child().(codeCopyResultMsg); valid {
			result = message
		}
	}
	updated, _ = model.Update(result)
	model = updated.(searchModel)
	if model.copyState.feedback != codeCopySent || !strings.Contains(ansi.Strip(model.View().Content), "[ Sent ]") {
		t.Fatalf("native failure feedback = %#v", model.copyState)
	}
}

func TestSearchReaderRejectsStaleCopyResultsAndTimers(t *testing.T) {
	article := Article{Category: "test", Title: "Rapid", Content: "```go\none\n```\n\n```sh\ntwo\n```"}
	model := openTestReader(t, newSearchModel([]Article{article}, 30, 7))
	model.clipboardWriteFunc = func(string) error { return nil }

	updated, _ := model.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	model = updated.(searchModel)
	firstGeneration := model.copyGeneration
	updated, _ = model.startCodeCopy(model.rendered.codeBlocks[1])
	model = updated.(searchModel)
	secondGeneration := model.copyGeneration

	updated, _ = model.Update(codeCopyResultMsg{blockID: 0, generation: firstGeneration})
	model = updated.(searchModel)
	if model.copyState.blockID != 1 || model.copyState.feedback != codeCopyCopying {
		t.Fatalf("stale result overwrote newer copy: %#v", model.copyState)
	}
	updated, _ = model.Update(codeCopyResultMsg{blockID: 1, generation: secondGeneration})
	model = updated.(searchModel)
	updated, _ = model.Update(clearCodeCopyMsg{blockID: 0, generation: firstGeneration})
	model = updated.(searchModel)
	if model.copyState.feedback != codeCopyCopied {
		t.Fatalf("stale timer cleared feedback: %#v", model.copyState)
	}
}

func TestSearchReaderLeavingReaderInvalidatesPendingCopy(t *testing.T) {
	article := Article{Category: "test", Title: "Leave", Content: "```go\none\n```"}
	model := openTestReader(t, newSearchModel([]Article{article}, 30, 7))
	model.clipboardWriteFunc = func(string) error { return nil }

	updated, _ := model.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	model = updated.(searchModel)
	pending := codeCopyResultMsg{blockID: 0, generation: model.copyGeneration}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(searchModel)
	updated, _ = model.Update(pending)
	model = updated.(searchModel)
	if model.reading || model.copyState.feedback != codeCopyIdle {
		t.Fatalf("pending copy survived reader exit: reading=%t state=%#v", model.reading, model.copyState)
	}
}

func TestSearchReaderResizePreservesScrollPercentageAndRecalculatesHitBoxes(t *testing.T) {
	content := strings.Repeat("A paragraph with enough words to wrap substantially.\n\n", 20) + "```go\nabcdefghijklmnopqrstuvwxyz\n```\n" + strings.Repeat("tail\n\n", 20)
	model := openTestReader(t, newSearchModel([]Article{{Category: "test", Title: "Resize", Content: content}}, 45, 9))
	model.reader.SetYOffset(model.reader.TotalLineCount() / 2)
	before := model.reader.ScrollPercent()
	oldHeader := model.rendered.codeBlocks[0].headerLine

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 24, Height: 12})
	model = updated.(searchModel)
	after := model.reader.ScrollPercent()
	if difference(before, after) > 0.12 {
		t.Fatalf("scroll percentage changed from %.2f to %.2f", before, after)
	}
	block := model.rendered.codeBlocks[0]
	if block.headerLine == oldHeader || block.copyEndColumn > 24 {
		t.Fatalf("code geometry was not recalculated: old header %d, new %#v", oldHeader, block)
	}
}

func openTestReader(t *testing.T, model searchModel) searchModel {
	t.Helper()
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	opened := updated.(searchModel)
	if !opened.reading || opened.err != nil {
		t.Fatalf("open reader: reading=%t err=%v", opened.reading, opened.err)
	}
	return opened
}

func runCopyBatch(t *testing.T, command tea.Cmd, model *searchModel) {
	t.Helper()
	batch, ok := command().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("copy command did not include native and OSC 52 operations: %#v", command())
	}
	for _, child := range batch {
		if result, valid := child().(codeCopyResultMsg); valid {
			updated, _ := model.Update(result)
			*model = updated.(searchModel)
		}
	}
}

func difference(left float64, right float64) float64 {
	if left > right {
		return left - right
	}
	return right - left
}
