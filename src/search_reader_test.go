package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestSearchReaderEnterPreservesSearchState(t *testing.T) {
	model := newSearchModel([]Article{{Category: "git", Title: "Reset Git", Content: "# Reset Git\n"}}, 50, 10)
	model.input.SetValue("git")
	model.refreshResults()
	model.input.SetCursor(1)
	view := model.View().Content
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	model.readerCommand = func(Article) (*exec.Cmd, error) {
		return exec.Command(executable), nil
	}

	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	opened := updated.(searchModel)
	if command == nil {
		t.Fatal("Enter did not return a child-process command")
	}
	if opened.input.Value() != "git" || opened.input.Position() != 1 || !opened.input.Focused() || !reflect.DeepEqual(opened.results, model.results) || opened.View().Content != view {
		t.Fatal("opening the child process changed the search state")
	}
}

func TestArticleReaderInvocationUsesOnlyBundledPathAndLiteralSourceArgument(t *testing.T) {
	tests := []struct {
		name, executable, targetOS, source, wantPath string
	}{
		{"linux", "/opt/my toolbox/tb", "linux", "/articles/guide;$(echo nope).md", "/opt/my toolbox/libexec/tb-markdown-reader"},
		{"windows", `C:\My Toolbox\tb.exe`, "windows", `C:\Articles\guide & notes.md`, `C:\My Toolbox\libexec\tb-markdown-reader.exe`},
		{"windows UNC", `\\server\share\toolbox\tb.exe`, "windows", `\\server\share\article.md`, `\\server\share\toolbox\libexec\tb-markdown-reader.exe`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path, args := articleReaderInvocation(test.executable, test.targetOS, Article{SourcePath: test.source, Content: "must not be passed"})
			if path != test.wantPath || !reflect.DeepEqual(args, []string{"--tb-embedded", test.source}) {
				t.Fatalf("invocation = %q %q, want %q with literal source argument", path, args, test.wantPath)
			}
		})
	}
}

func TestArticleReaderCommandDoesNotUsePATHShellOrRenderedContent(t *testing.T) {
	root := t.TempDir()
	writeTestArticle(t, root, "guide;$(echo nope).md", "# Raw source\n")
	article := Article{SourcePath: filepath.Join(root, "guide;$(echo nope).md"), Content: "different loaded content"}
	sidecar := installTestSidecar(t, root)
	t.Setenv("PATH", "")
	command, err := buildArticleReaderCommand(filepath.Join(root, "tb"), runtime.GOOS, article)
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{sidecar, "--tb-embedded", article.SourcePath}
	if command.Path != sidecar || !reflect.DeepEqual(command.Args, wantArgs) || command.Err != nil || command.Stdin != nil {
		t.Fatalf("command = %#v, want direct bundled invocation with inherited stdin", command)
	}
}

func TestArticleReaderCommandRejectsInvalidBundleAndSource(t *testing.T) {
	for _, name := range []string{"missing sidecar", "sidecar directory", "nonexecutable sidecar", "absent source", "missing source file", "relative source"} {
		t.Run(name, func(t *testing.T) {
			if name == "nonexecutable sidecar" && runtime.GOOS == "windows" {
				t.Skip("Windows does not use Unix executable permission bits")
			}
			root := t.TempDir()
			writeTestArticle(t, root, "guide.md", "# Guide\n")
			article := Article{SourcePath: filepath.Join(root, "guide.md")}
			sidecar, _ := articleReaderInvocation(filepath.Join(root, "tb"), runtime.GOOS, article)
			if name == "sidecar directory" {
				if err := os.MkdirAll(sidecar, 0o755); err != nil {
					t.Fatal(err)
				}
			} else if name != "missing sidecar" {
				installTestSidecar(t, root)
			}
			if name == "nonexecutable sidecar" {
				if err := os.Chmod(sidecar, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			switch name {
			case "absent source":
				article.SourcePath = ""
			case "missing source file":
				article.SourcePath = filepath.Join(root, "missing.md")
			case "relative source":
				article.SourcePath = "guide.md"
			}
			// Even a same-named executable on PATH must never replace the bundle.
			pathRoot := t.TempDir()
			writeTestArticle(t, pathRoot, filepath.Base(sidecar), "not the bundled executable")
			if err := os.Chmod(filepath.Join(pathRoot, filepath.Base(sidecar)), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", pathRoot)
			command, err := buildArticleReaderCommand(filepath.Join(root, "tb"), runtime.GOOS, article)
			if err == nil || command != nil {
				t.Fatalf("command = %#v, error = %v, want construction failure", command, err)
			}
		})
	}
}

func TestSearchReaderSuccessfulCompletionPreservesQueryCursorViewportAndResults(t *testing.T) {
	articles := []Article{}
	for _, title := range []string{"A", "B", "C", "D", "E", "F", "G", "H"} {
		articles = append(articles, Article{Title: "Guide " + title, Content: "Details for this guide."})
	}
	model := newSearchModel(articles, 40, 8)
	model.input.SetValue("guide")
	model.refreshResults()
	model.input.SetCursor(2)
	model.cursor = 6
	model.rebuildResults()
	before := model.View().Content
	beforeOffset := model.resultsViewport.YOffset()
	if beforeOffset == 0 {
		t.Fatal("test must exercise a scrolled results viewport")
	}
	updated, command := model.Update(articleReaderResultMsg{article: model.results[model.cursor].Article})
	resumed := updated.(searchModel)
	if command != nil || resumed.fallback != nil || resumed.cancelled || resumed.resultError() != nil || resumed.input.Value() != "guide" || resumed.input.Position() != 2 || !resumed.input.Focused() || resumed.cursor != 6 || resumed.resultsViewport.YOffset() != beforeOffset || !reflect.DeepEqual(resumed.results, model.results) || resumed.View().Content != before {
		t.Fatal("successful reader exit did not preserve the search session")
	}
	if resumed.View().MouseMode != tea.MouseModeNone {
		t.Fatal("search enabled reader mouse reporting")
	}
}

func TestSearchReaderConstructionFailureRetainsSelectedArticleAndQuits(t *testing.T) {
	wantErr := errors.New("cannot construct reader")
	articles := []Article{{Title: "First"}, {Title: "Selected", Content: "# Selected\n"}}
	model := newSearchModel(articles, 50, 10)
	model.cursor = 1
	model.readerCommand = func(article Article) (*exec.Cmd, error) {
		if !reflect.DeepEqual(article, articles[1]) {
			t.Fatalf("construction article = %#v, want selected result", article)
		}
		return nil, wantErr
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if command == nil {
		t.Fatal("missing completion command")
	}
	message, ok := command().(articleReaderResultMsg)
	if !ok || !errors.Is(message.err, wantErr) || !reflect.DeepEqual(message.article, articles[1]) {
		t.Fatalf("construction completion = %#v", message)
	}
	updated, command = updated.(searchModel).Update(message)
	model = updated.(searchModel)
	if model.fallback == nil || !errors.Is(model.fallback.err, wantErr) || !reflect.DeepEqual(model.fallback.article, articles[1]) || model.cancelled || model.resultError() != nil {
		t.Fatalf("fallback state = %#v", model.fallback)
	}
	if command == nil {
		t.Fatal("failure did not quit search")
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatal("failure command was not tea.Quit")
	}
}

func TestSearchReaderExecProcessReportsStartFailureExitFailureAndSuccess(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"start failure", "nonzero exit", "success"} {
		t.Run(name, func(t *testing.T) {
			article := Article{Title: "Selected", Content: "# Exact Markdown\n"}
			model := newSearchModel([]Article{article}, 50, 10)
			model.readerCommand = func(Article) (*exec.Cmd, error) {
				if name == "start failure" {
					return exec.Command(filepath.Join(t.TempDir(), "absent-executable")), nil
				}
				command := exec.Command(executable, "-test.run=^TestArticleReaderChildProcess$")
				command.Env = append(os.Environ(), "TB_TEST_READER_CHILD="+name)
				command.Stdout = io.Discard
				command.Stderr = io.Discard
				return command, nil
			}
			harness := &searchReaderHarness{searchModel: model}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			program := tea.NewProgram(harness, tea.WithInput(&bytes.Buffer{}), tea.WithOutput(io.Discard), tea.WithWindowSize(50, 10), tea.WithoutRenderer(), tea.WithoutSignalHandler(), tea.WithContext(ctx))
			if _, err := program.Run(); err != nil {
				t.Fatal(err)
			}
			if harness.completed == nil || !reflect.DeepEqual(harness.completed.article, article) {
				t.Fatalf("missing selected article in process completion: %#v", harness.completed)
			}
			if name == "success" {
				if harness.completed.err != nil || harness.fallback != nil || !harness.input.Focused() {
					t.Fatalf("successful child result = %#v, fallback = %#v", harness.completed, harness.fallback)
				}
				return
			}
			if harness.completed.err == nil || harness.fallback == nil || !errors.Is(harness.fallback.err, harness.completed.err) {
				t.Fatalf("process failure did not reach fallback: %#v", harness.completed)
			}
			if name == "nonzero exit" {
				var exitError *exec.ExitError
				if !errors.As(harness.completed.err, &exitError) || exitError.ExitCode() != 17 {
					t.Fatalf("exit error = %v, want status 17", harness.completed.err)
				}
			}
		})
	}
}

func TestArticleReaderChildProcess(t *testing.T) {
	switch os.Getenv("TB_TEST_READER_CHILD") {
	case "nonzero exit":
		os.Exit(17)
	case "success":
		os.Exit(0)
	}
}

func TestHuhUISearchFallbackWritesExactMarkdownAndWarning(t *testing.T) {
	for _, test := range []struct {
		name, content, want string
	}{
		{"missing newline", "# Heading\r\n\n**raw**\t\xff", "# Heading\r\n\n**raw**\t\xff\n"},
		{"final newline", "# Heading\n", "# Heading\n"},
		{"final blank line", "# Heading\n\n", "# Heading\n\n"},
		{"empty article", "", "\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			ui := HuhUI{stdout: &stdout, stderr: &stderr, runSearch: func(initial tea.Model) (tea.Model, error) {
				model := initial.(searchModel)
				model.fallback = &articleReaderResultMsg{article: Article{Title: "Selected", Content: test.content}, err: errors.New("reader failed")}
				return model, nil
			}}
			if err := ui.Search([]Article{{Title: "Other", Content: "wrong article"}}); err != nil {
				t.Fatalf("fallback should be command-success: %v", err)
			}
			if stdout.String() != test.want {
				t.Fatalf("stdout = %q, want %q", stdout.String(), test.want)
			}
			if !strings.Contains(stderr.String(), "warning:") || !strings.Contains(stderr.String(), "reader failed") {
				t.Fatalf("missing reader warning on stderr: %q", stderr.String())
			}
		})
	}
}

func TestHuhUISearchReturnsFallbackWriteFailure(t *testing.T) {
	wantErr := errors.New("stdout unavailable")
	for _, test := range []struct {
		name      string
		writerErr error
		wantErr   error
	}{
		{"write error", wantErr, wantErr},
		{"short write", nil, io.ErrShortWrite},
	} {
		t.Run(test.name, func(t *testing.T) {
			ui := HuhUI{stdout: failingReaderOutput{err: test.writerErr}, stderr: io.Discard, runSearch: func(initial tea.Model) (tea.Model, error) {
				model := initial.(searchModel)
				model.fallback = &articleReaderResultMsg{article: Article{Content: "# Guide"}, err: errors.New("reader unavailable")}
				return model, nil
			}}
			if err := ui.Search(nil); !errors.Is(err, test.wantErr) {
				t.Fatalf("Search error = %v, want stdout failure %v", err, test.wantErr)
			}
		})
	}
}

func TestHuhUISearchDefaultsFallbackToProcessStreams(t *testing.T) {
	if os.Getenv("TB_TEST_READER_CHILD") == "fallback" {
		ui := HuhUI{runSearch: func(initial tea.Model) (tea.Model, error) {
			model := initial.(searchModel)
			model.fallback = &articleReaderResultMsg{article: Article{Content: "# Default output"}, err: errors.New("missing reader")}
			return model, nil
		}}
		if err := ui.Search(nil); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(executable, "-test.run=^TestHuhUISearchDefaultsFallbackToProcessStreams$")
	command.Env = append(os.Environ(), "TB_TEST_READER_CHILD=fallback")
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "# Default output\n" || !strings.Contains(stderr.String(), "warning:") || !strings.Contains(stderr.String(), "missing reader") {
		t.Fatalf("default stdout=%q stderr=%q", &stdout, &stderr)
	}
}

func TestHuhUISearchKeepsCancellationAndProgramErrors(t *testing.T) {
	wantErr := errors.New("terminal unavailable")
	for _, name := range []string{"cancel", "program failure", "invalid model"} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			ui := HuhUI{stdout: &stdout, stderr: &stderr, runSearch: func(initial tea.Model) (tea.Model, error) {
				if name == "program failure" {
					return nil, wantErr
				}
				if name == "invalid model" {
					return nil, nil
				}
				model := initial.(searchModel)
				model.cancelled = true
				return model, nil
			}}
			err := ui.Search(nil)
			if err == nil || (name == "cancel" && !errors.Is(err, ErrCancelled)) || (name == "program failure" && !errors.Is(err, wantErr)) || stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("Search error=%v stdout=%q stderr=%q", err, &stdout, &stderr)
			}
		})
	}
}

type failingReaderOutput struct{ err error }

func (writer failingReaderOutput) Write([]byte) (int, error) { return 0, writer.err }

type searchReaderHarness struct {
	searchModel
	completed *articleReaderResultMsg
}

func (model *searchReaderHarness) Init() tea.Cmd {
	updated, command := model.searchModel.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model.searchModel = updated.(searchModel)
	return command
}

func (model *searchReaderHarness) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	updated, command := model.searchModel.Update(message)
	model.searchModel = updated.(searchModel)
	if result, ok := message.(articleReaderResultMsg); ok {
		model.completed = &result
		// The harness exits on success; the production model keeps browsing.
		if command == nil {
			command = tea.Quit
		}
	}
	return model, command
}

func installTestSidecar(t *testing.T, root string) string {
	t.Helper()
	name := "libexec/tb-markdown-reader"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	writeTestArticle(t, root, name, "not executable machine code")
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
