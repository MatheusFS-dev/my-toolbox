package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// articleReaderResultMsg also records the selected article for output after exit.
type articleReaderResultMsg struct {
	article Article
	err     error
}

// articleReaderInvocation describes the installed layout independently of the host OS.
func articleReaderInvocation(executable string, targetOS string, article Article) (string, []string) {
	args := []string{"--tb-embedded", article.SourcePath}
	if targetOS == "windows" {
		executable = strings.ReplaceAll(executable, "/", `\`)
		directory := executable[:strings.LastIndex(executable, `\`)+1]
		return directory + `libexec\tb-markdown-reader.exe`, args
	}
	return path.Join(path.Dir(executable), "libexec", "tb-markdown-reader"), args
}

func newArticleReaderCommand(article Article) (*exec.Cmd, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate toolbox executable: %w", err)
	}
	return buildArticleReaderCommand(executable, runtime.GOOS, article)
}

func buildArticleReaderCommand(executable string, targetOS string, article Article) (*exec.Cmd, error) {
	if !filepath.IsAbs(executable) {
		return nil, fmt.Errorf("toolbox executable path must be absolute: %q", executable)
	}
	if !filepath.IsAbs(article.SourcePath) {
		return nil, fmt.Errorf("article source path must be absolute: %q", article.SourcePath)
	}
	source, err := os.Stat(article.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("inspect article source %s: %w", article.SourcePath, err)
	}
	if !source.Mode().IsRegular() {
		return nil, fmt.Errorf("article source is not a regular file: %s", article.SourcePath)
	}
	sidecar, args := articleReaderInvocation(executable, targetOS, article)
	info, err := os.Stat(sidecar)
	if err != nil {
		return nil, fmt.Errorf("inspect bundled Markdown reader %s: %w", sidecar, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("bundled Markdown reader is not a regular file: %s", sidecar)
	}
	if targetOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return nil, fmt.Errorf("bundled Markdown reader is not executable: %s", sidecar)
	}
	return exec.Command(sidecar, args...), nil
}

func writeArticleReaderFallback(result articleReaderResultMsg, stdout io.Writer, stderr io.Writer) error {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	warning := fmt.Sprintf("warning: Markdown reader failed: %v; printing article source\n", result.err)
	n, warningErr := io.WriteString(stderr, warning)
	if warningErr == nil && n != len(warning) {
		warningErr = io.ErrShortWrite
	}
	if warningErr != nil {
		warningErr = fmt.Errorf("write reader warning: %w", warningErr)
	}
	content := result.article.Content
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	n, err := io.WriteString(stdout, content)
	if err == nil && n != len(content) {
		err = io.ErrShortWrite
	}
	if err != nil {
		err = fmt.Errorf("write article source: %w", err)
	}
	return errors.Join(warningErr, err)
}
