package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadArticlesDiscoversMarkdownRecursivelyAndExtractsMetadata(t *testing.T) {
	root := t.TempDir()
	writeTestArticle(t, root, "git/history/reset.md", "# Reset Git History\n\n## Safety\n\nRewrite commits safely.\n\n# Troubleshooting\n")
	writeTestArticle(t, root, "python/create_uv_requirements.md", "Create a lock file.\n")
	writeTestArticle(t, root, "python/ignored.txt", "# Not Markdown\n")

	articles, err := loadArticles(root, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if len(articles) != 2 {
		t.Fatalf("articles = %#v, want two Markdown articles", articles)
	}
	if articles[0].Category != "history" || articles[0].Title != "Reset Git History" || articles[0].RelativePath != "git/history/reset.md" {
		t.Fatalf("first article = %#v", articles[0])
	}
	if strings.Join(articles[0].Headings, ",") != "Safety,Troubleshooting" {
		t.Fatalf("indexed headings = %v", articles[0].Headings)
	}
	if articles[1].Category != "python" || articles[1].Title != "Create Uv Requirements" {
		t.Fatalf("fallback article = %#v", articles[1])
	}
}

func TestLoadArticlesSortsDeterministicallyByCategoryTitleAndPath(t *testing.T) {
	root := t.TempDir()
	writeTestArticle(t, root, "zeta/b.md", "# Same\n\nSecond.\n")
	writeTestArticle(t, root, "alpha/c.md", "# Zebra\n\nThird.\n")
	writeTestArticle(t, root, "alpha/a.md", "# Same\n\nFirst.\n")
	writeTestArticle(t, root, "alpha/b.md", "# Same\n\nSecond.\n")

	articles, err := loadArticles(root, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, article := range articles {
		got = append(got, article.RelativePath)
	}
	want := "alpha/a.md,alpha/b.md,alpha/c.md,zeta/b.md"
	if strings.Join(got, ",") != want {
		t.Fatalf("paths = %q, want %q", strings.Join(got, ","), want)
	}
}

func TestLoadArticlesRecordsAbsoluteCanonicalSourcePath(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	content := "# Original\r\n\r\nRaw **Markdown**.\n"
	writeTestArticle(t, root, "articles/git/guide.md", content)
	if err := os.Symlink(filepath.Join(root, "articles", "git", "guide.md"), filepath.Join(root, "articles", "git", "linked.md")); err != nil {
		t.Fatal(err)
	}

	articles, err := loadArticles("articles", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(articles) != 2 {
		t.Fatalf("loaded %d articles, want the source and its symlink", len(articles))
	}
	want, err := filepath.EvalSymlinks(filepath.Join(root, "articles", "git", "guide.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, article := range articles {
		if !filepath.IsAbs(article.SourcePath) || article.SourcePath != want {
			t.Fatalf("source path = %q, want canonical absolute path %q", article.SourcePath, want)
		}
		loaded, err := os.ReadFile(article.SourcePath)
		if err != nil || string(loaded) != content || article.Content != content {
			t.Fatalf("source does not identify loaded Markdown: content=%q error=%v", loaded, err)
		}
	}
}

func TestLoadArticlesWarnsAndSkipsUnreadableMarkdown(t *testing.T) {
	root := t.TempDir()
	writeTestArticle(t, root, "git/readable.md", "# Readable\n\nText.\n")
	if err := os.Symlink(filepath.Join(root, "missing-target"), filepath.Join(root, "git", "broken.md")); err != nil {
		t.Fatal(err)
	}
	warnings := &bytes.Buffer{}

	articles, err := loadArticles(root, warnings)
	if err != nil {
		t.Fatal(err)
	}
	if len(articles) != 1 || articles[0].Title != "Readable" {
		t.Fatalf("articles = %#v", articles)
	}
	if !strings.Contains(warnings.String(), "warning:") || !strings.Contains(warnings.String(), "git/broken.md") {
		t.Fatalf("warnings = %q", warnings.String())
	}
}

func TestLoadArticlesFailsForMissingOrUnreadableLibrary(t *testing.T) {
	tests := []struct {
		name string
		make func(string)
		want string
	}{
		{name: "missing", make: func(string) {}, want: "article library"},
		{name: "empty", make: func(root string) { os.MkdirAll(root, 0o755) }, want: "no readable articles"},
		{name: "only unreadable", make: func(root string) {
			os.MkdirAll(filepath.Join(root, "git"), 0o755)
			os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "git", "broken.md"))
		}, want: "no readable articles"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "articles")
			test.make(root)
			_, err := loadArticles(root, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("loadArticles() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestSearchArticlesRanksTitleThenHeadingThenBody(t *testing.T) {
	articles := []Article{
		testIndexedArticle("body.md", "Unrelated", nil, "alpha needle omega"),
		testIndexedArticle("heading.md", "Unrelated", []string{"Needle Setup"}, "other text"),
		testIndexedArticle("title.md", "Needle Guide", nil, "other text"),
	}

	results := searchArticles(articles, "needle")
	got := []string{results[0].Article.RelativePath, results[1].Article.RelativePath, results[2].Article.RelativePath}
	if strings.Join(got, ",") != "title.md,heading.md,body.md" {
		t.Fatalf("ranking = %v", got)
	}
}

func TestSearchArticlesKeepsFieldPriorityAheadOfContiguousBodyMatches(t *testing.T) {
	articles := []Article{
		testIndexedArticle("body.md", "Unrelated", nil, "git reset is contiguous here"),
		testIndexedArticle("title.md", "Git Advanced Reset", nil, "other text"),
	}

	results := searchArticles(articles, "git reset")
	if len(results) != 2 || results[0].Article.RelativePath != "title.md" {
		t.Fatalf("ranking = %#v", results)
	}
}

func TestSearchArticlesRequiresEveryTokenAndPrefersContiguousMatches(t *testing.T) {
	articles := []Article{
		testIndexedArticle("scattered.md", "Git Advanced Reset", nil, "reference"),
		testIndexedArticle("contiguous.md", "Git Reset Guide", nil, "reference"),
		testIndexedArticle("partial.md", "Git Guide", nil, "reference"),
		testIndexedArticle("fuzzy.md", "Giant Reset Notes", nil, "reference"),
	}

	results := searchArticles(articles, "git reset")
	got := []string{}
	for _, result := range results {
		got = append(got, result.Article.RelativePath)
	}
	if strings.Join(got, ",") != "contiguous.md,scattered.md,fuzzy.md" {
		t.Fatalf("ranking = %v", got)
	}
}

func TestSearchArticlesUsesStablePathTieBreakAndBuildsExcerpts(t *testing.T) {
	articles := []Article{
		testIndexedArticle("b.md", "Same", []string{"Install"}, "First paragraph.\n\nNeedle body paragraph with useful details."),
		testIndexedArticle("a.md", "Same", []string{"Install"}, "Opening paragraph.\n\nNeedle body paragraph with useful details."),
	}

	empty := searchArticles(articles, "")
	if len(empty) != 2 || empty[0].Article.RelativePath != "a.md" || empty[0].Excerpt != "Opening paragraph." {
		t.Fatalf("empty results = %#v", empty)
	}
	heading := searchArticles(articles, "install")
	if heading[0].Excerpt != "Install" {
		t.Fatalf("heading excerpt = %q", heading[0].Excerpt)
	}
	body := searchArticles(articles, "needle")
	if body[0].Excerpt != "Needle body paragraph with useful details." {
		t.Fatalf("body excerpt = %q", body[0].Excerpt)
	}
	if results := searchArticles(articles, "absent"); len(results) != 0 {
		t.Fatalf("no-match results = %#v", results)
	}
}

func TestSearchArticlesChoosesTheBestMatchingExcerpt(t *testing.T) {
	article := testIndexedArticle(
		"guide.md",
		"Guide",
		nil,
		"Needless unrelated setup notes.\n\nNeedle setup details are here.",
	)

	results := searchArticles([]Article{article}, "needle setup")
	if len(results) != 1 || results[0].Excerpt != "Needle setup details are here." {
		t.Fatalf("results = %#v", results)
	}
}

func writeTestArticle(t *testing.T, root string, relative string, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testIndexedArticle(path string, title string, headings []string, body string) Article {
	return Article{Title: title, RelativePath: path, Headings: headings, Content: body}
}
