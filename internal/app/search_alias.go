package app

import (
	"github.com/MatheusFS-dev/my-toolbox/internal/features/search"
	"io"
)

type Article = search.Article
type articleResult = search.ArticleResult

func loadArticles(root string, warnings io.Writer) ([]Article, error) {
	return search.LoadArticles(root, warnings)
}
func searchArticles(articles []Article, query string) []articleResult {
	return search.SearchArticles(articles, query)
}
func normalizeText(text string) string     { return search.NormalizeText(text) }
func filenameTitle(slug string) string     { return search.FilenameTitle(slug) }
func warningPath(root, path string) string { return search.WarningPath(root, path) }
