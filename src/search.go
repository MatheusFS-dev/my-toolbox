package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/sahilm/fuzzy"
)

// Article is one Markdown document in the installed toolbox library.
type Article struct {
	Category     string
	Title        string
	RelativePath string
	SourcePath   string
	Content      string
	Headings     []string
	bodyText     string
	paragraphs   []string
}

type articleResult struct {
	Article Article
	Excerpt string
	score   int
}

// loadArticles recursively loads readable Markdown files from one library.
func loadArticles(root string, warnings io.Writer) ([]Article, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("article library is missing: %s", root)
		}
		return nil, fmt.Errorf("inspect article library %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("article library is not a directory: %s", root)
	}
	if warnings == nil {
		warnings = io.Discard
	}
	articles := []Article{}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			relative := warningPath(root, path)
			fmt.Fprintf(warnings, "warning: skip article %s: %v\n", relative, walkErr)
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			return nil
		}
		sourcePath, sourceErr := filepath.Abs(path)
		if sourceErr == nil {
			sourcePath, sourceErr = filepath.EvalSymlinks(sourcePath)
		}
		if sourceErr != nil {
			fmt.Fprintf(warnings, "warning: skip article %s: %v\n", warningPath(root, path), sourceErr)
			return nil
		}
		content, readErr := os.ReadFile(sourcePath)
		if readErr != nil {
			fmt.Fprintf(warnings, "warning: skip article %s: %v\n", warningPath(root, path), readErr)
			return nil
		}
		relative, relativeErr := filepath.Rel(root, path)
		if relativeErr != nil {
			fmt.Fprintf(warnings, "warning: skip article %s: %v\n", path, relativeErr)
			return nil
		}
		headings, paragraphs := articleSections(string(content))
		title := ""
		titleHeadingIndex := -1
		for index, heading := range headings {
			if heading.level == 1 {
				title = heading.text
				titleHeadingIndex = index
				break
			}
		}
		if title == "" {
			title = filenameTitle(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
		}
		indexedHeadings := make([]string, 0, len(headings))
		for index, heading := range headings {
			if index != titleHeadingIndex {
				indexedHeadings = append(indexedHeadings, heading.text)
			}
		}
		articles = append(articles, Article{
			Category:     filepath.Base(filepath.Dir(path)),
			Title:        title,
			RelativePath: filepath.ToSlash(relative),
			SourcePath:   sourcePath,
			Content:      string(content),
			Headings:     indexedHeadings,
			bodyText:     strings.Join(paragraphs, " "),
			paragraphs:   paragraphs,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover article library %s: %w", root, err)
	}
	if len(articles) == 0 {
		return nil, fmt.Errorf("article library contains no readable articles: %s", root)
	}
	sort.SliceStable(articles, func(left int, right int) bool {
		leftKey := strings.ToLower(articles[left].Category) + "\x00" + strings.ToLower(articles[left].Title) + "\x00" + articles[left].RelativePath
		rightKey := strings.ToLower(articles[right].Category) + "\x00" + strings.ToLower(articles[right].Title) + "\x00" + articles[right].RelativePath
		return leftKey < rightKey
	})
	return articles, nil
}

type articleHeading struct {
	level int
	text  string
}

func articleSections(content string) ([]articleHeading, []string) {
	headings := []articleHeading{}
	paragraphs := []string{}
	paragraphLines := []string{}
	inFence := false
	flushParagraph := func() {
		paragraph := normalizeText(strings.Join(paragraphLines, " "))
		if paragraph != "" {
			paragraphs = append(paragraphs, paragraph)
		}
		paragraphLines = nil
	}
	for _, sourceLine := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(sourceLine)
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			flushParagraph()
			inFence = !inFence
			continue
		}
		if !inFence {
			level, text := markdownHeading(line)
			if level > 0 {
				flushParagraph()
				headings = append(headings, articleHeading{level: level, text: text})
				continue
			}
		}
		if line == "" {
			flushParagraph()
			continue
		}
		paragraphLines = append(paragraphLines, line)
	}
	flushParagraph()
	return headings, paragraphs
}

func markdownHeading(line string) (int, string) {
	level := 0
	for level < len(line) && level < 6 && line[level] == '#' {
		level++
	}
	if level == 0 || level >= len(line) || line[level] != ' ' {
		return 0, ""
	}
	text := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(line[level+1:]), "#"))
	if text == "" {
		return 0, ""
	}
	return level, text
}

func filenameTitle(slug string) string {
	words := strings.FieldsFunc(slug, func(character rune) bool {
		return character == '-' || character == '_'
	})
	for index, word := range words {
		runes := []rune(strings.ToLower(word))
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
		}
		words[index] = string(runes)
	}
	return strings.Join(words, " ")
}

func warningPath(root string, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(relative)
}

func searchArticles(articles []Article, query string) []articleResult {
	query = normalizeText(query)
	if query == "" {
		results := make([]articleResult, len(articles))
		for index, article := range articles {
			results[index] = articleResult{Article: article, Excerpt: firstArticleParagraph(article)}
		}
		sortArticleResults(results, true)
		return results
	}
	tokens := strings.Fields(strings.ToLower(query))
	results := []articleResult{}
	for _, article := range articles {
		score, matched := articleScore(article, tokens, strings.ToLower(query))
		if !matched {
			continue
		}
		results = append(results, articleResult{Article: article, Excerpt: matchingExcerpt(article, tokens), score: score})
	}
	sortArticleResults(results, false)
	return results
}

func articleScore(article Article, tokens []string, query string) (int, bool) {
	fields := []struct {
		text string
		base int
	}{{text: article.Title, base: 300000000}}
	for _, heading := range article.Headings {
		fields = append(fields, struct {
			text string
			base int
		}{text: heading, base: 200000000})
	}
	body := article.bodyText
	if body == "" {
		_, paragraphs := articleSections(article.Content)
		body = strings.Join(paragraphs, " ")
	}
	fields = append(fields, struct {
		text string
		base int
	}{text: body, base: 100000000})
	score := 0
	for _, token := range tokens {
		best := -1
		for _, field := range fields {
			match := fuzzy.FindNoSort(token, []string{strings.ToLower(normalizeText(field.text))})
			if len(match) > 0 {
				candidate := field.base + match[0].Score
				if candidate > best {
					best = candidate
				}
			}
		}
		if best < 0 {
			return 0, false
		}
		score += best
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(normalizeText(field.text)), query) {
			score += 1000000
			break
		}
	}
	return score, true
}

func matchingExcerpt(article Article, tokens []string) string {
	type excerptCandidate struct {
		text  string
		score int
	}
	candidates := []excerptCandidate{}
	for _, heading := range article.Headings {
		if allTokensMatch(tokens, heading) {
			candidates = append(candidates, excerptCandidate{text: normalizeText(heading), score: excerptScore(heading, tokens) + 100000})
		}
	}
	paragraphs := article.paragraphs
	if len(paragraphs) == 0 {
		_, paragraphs = articleSections(article.Content)
	}
	for _, paragraph := range paragraphs {
		if allTokensMatch(tokens, paragraph) {
			candidates = append(candidates, excerptCandidate{text: paragraph, score: excerptScore(paragraph, tokens)})
		}
	}
	if len(candidates) > 0 {
		sort.SliceStable(candidates, func(left int, right int) bool {
			return candidates[left].score > candidates[right].score
		})
		return candidates[0].text
	}
	if len(paragraphs) > 0 {
		return paragraphs[0]
	}
	return ""
}

func excerptScore(text string, tokens []string) int {
	normalized := strings.ToLower(normalizeText(text))
	score := 0
	for _, token := range tokens {
		matches := fuzzy.FindNoSort(token, []string{normalized})
		if len(matches) > 0 {
			score += matches[0].Score
		}
	}
	if strings.Contains(normalized, strings.Join(tokens, " ")) {
		score += 1000000
	}
	return score
}

func allTokensMatch(tokens []string, text string) bool {
	normalized := strings.ToLower(normalizeText(text))
	for _, token := range tokens {
		if len(fuzzy.FindNoSort(token, []string{normalized})) == 0 {
			return false
		}
	}
	return true
}

func firstArticleParagraph(article Article) string {
	if len(article.paragraphs) > 0 {
		return article.paragraphs[0]
	}
	_, paragraphs := articleSections(article.Content)
	if len(paragraphs) > 0 {
		return paragraphs[0]
	}
	return ""
}

func sortArticleResults(results []articleResult, byMetadata bool) {
	sort.SliceStable(results, func(left int, right int) bool {
		if !byMetadata && results[left].score != results[right].score {
			return results[left].score > results[right].score
		}
		if byMetadata {
			leftKey := strings.ToLower(results[left].Article.Category) + "\x00" + strings.ToLower(results[left].Article.Title) + "\x00" + results[left].Article.RelativePath
			rightKey := strings.ToLower(results[right].Article.Category) + "\x00" + strings.ToLower(results[right].Article.Title) + "\x00" + results[right].Article.RelativePath
			return leftKey < rightKey
		}
		return results[left].Article.RelativePath < results[right].Article.RelativePath
	})
}

func normalizeText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
