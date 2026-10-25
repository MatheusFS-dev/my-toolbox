package ui

import (
	"charm.land/huh/v2"
	"github.com/MatheusFS-dev/my-toolbox/internal/features/macros"
	"github.com/MatheusFS-dev/my-toolbox/internal/features/search"
	"github.com/MatheusFS-dev/my-toolbox/internal/protocol"
)

type Question = protocol.Question
type Option = protocol.Option

var ErrCancelled = protocol.ErrCancelled

type Article = search.Article
type articleResult = search.ArticleResult

func searchArticles(articles []Article, query string) []articleResult {
	return search.SearchArticles(articles, query)
}
func normalizeText(text string) string { return search.NormalizeText(text) }

type Macro = macros.Macro
type MacroSelection = macros.MacroSelection
type MacroAction = macros.MacroAction

const (
	MacroActionRun      = macros.MacroActionRun
	MacroActionDownload = macros.MacroActionDownload
)

type macroDriver = macros.MacroDriver

var macroDrivers = macros.MacroDrivers

func macroSupportsOS(macro Macro, goos string) bool { return macros.MacroSupportsOS(macro, goos) }

const maxPresentationWidth = MaxPresentationWidth
const (
	ansiWhite     = ANSIWhite
	ansiBlue      = ANSIBlue
	ansiGreen     = ANSIGreen
	ansiGray      = ANSIGray
	ansiBoldWhite = ANSIBoldWhite
	ansiBrightRed = ANSIBrightRed
)

func presentationStyle(text, code string, enabled bool) string {
	return PresentationStyle(text, code, enabled)
}
func wrapText(text string, width int) []string        { return WrapText(text, width) }
func groupCommands(commands []Command) []CommandGroup { return GroupCommands(commands) }
func filteredCommands(catalog Catalog, environment string, includeDirect bool) []Command {
	return FilteredCommands(catalog, environment, includeDirect)
}

func toolboxHuhForm(field huh.Field) *huh.Form { return ToolboxHuhForm(field) }
