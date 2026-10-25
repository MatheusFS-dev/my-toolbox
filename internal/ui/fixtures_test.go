package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestArticle(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
func testIndexedArticle(path, title string, headings []string, body string) Article {
	return Article{Title: title, RelativePath: path, Headings: headings, Content: body}
}
