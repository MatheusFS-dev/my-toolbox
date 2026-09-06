package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

func TestHuhUISearchProgramBoundary(t *testing.T) {
	if scenario := os.Getenv("TB_TEST_SEARCH_PROGRAM"); scenario != "" {
		source := os.Getenv("TB_TEST_SEARCH_SOURCE")
		content, err := os.ReadFile(source)
		if err == nil {
			err = (HuhUI{}).Search([]Article{{Title: "Selected", SourcePath: source, Content: string(content)}})
		}
		if err != nil && !(scenario == "success" && errors.Is(err, ErrCancelled)) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	for _, scenario := range []string{"missing", "nonzero", "success"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			sourceExecutable, err := os.Open(executable)
			if err != nil {
				t.Fatal(err)
			}
			defer sourceExecutable.Close()
			installedExecutable := filepath.Join(root, "tb")
			destination, err := os.OpenFile(installedExecutable, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
			if err != nil {
				t.Fatal(err)
			}
			_, copyErr := io.Copy(destination, sourceExecutable)
			closeErr := destination.Close()
			if err := errors.Join(copyErr, closeErr); err != nil {
				t.Fatal(err)
			}
			const content = "# Exact **Markdown**\r\n\nraw\t\xff"
			source := filepath.Join(root, "article.md")
			if err := os.WriteFile(source, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(root, "reader-ran")
			if scenario != "missing" {
				if err := os.Mkdir(filepath.Join(root, "libexec"), 0o755); err != nil {
					t.Fatal(err)
				}
				status := "0"
				if scenario == "nonzero" {
					status = "17"
				}
				fixture := "#!/bin/sh\nprintf 'interactive reader output\\n'\n: > \"$TB_TEST_READER_MARKER\"\nexit " + status + "\n"
				if err := os.WriteFile(filepath.Join(root, "libexec", "tb-markdown-reader"), []byte(fixture), 0o755); err != nil {
					t.Fatal(err)
				}
			}

			master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer master.Close()
			if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
				t.Fatal(err)
			}
			number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
			if err != nil {
				t.Fatal(err)
			}
			terminal, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|unix.O_NOCTTY, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer terminal.Close()
			if _, err := term.MakeRaw(terminal.Fd()); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, installedExecutable, "-test.run=^TestHuhUISearchProgramBoundary$")
			command.Env = append(os.Environ(), "TB_TEST_SEARCH_PROGRAM="+scenario, "TB_TEST_SEARCH_SOURCE="+source, "TB_TEST_READER_MARKER="+marker)
			var stdout, stderr bytes.Buffer
			command.Stdin, command.Stdout, command.Stderr = terminal, &stdout, &stderr
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			if _, err := master.Write([]byte("\r")); err != nil {
				t.Fatal(err)
			}
			if scenario == "success" {
				for {
					if _, err := os.Stat(marker); err == nil {
						break
					}
					select {
					case <-ctx.Done():
						t.Fatal("reader did not run")
					case <-time.After(10 * time.Millisecond):
					}
				}
				if _, err := master.Write([]byte{3}); err != nil {
					t.Fatal(err)
				}
			}
			if err := command.Wait(); err != nil {
				t.Fatalf("search failed: %v; stderr=%q", err, stderr.String())
			}
			want := content + "\n"
			if scenario == "success" {
				want = ""
			} else if !strings.Contains(stderr.String(), "warning: Markdown reader failed:") {
				t.Fatalf("missing fallback warning: %q", stderr.String())
			}
			if stdout.String() != want {
				t.Fatalf("stdout = %q, want %q", stdout.String(), want)
			}
		})
	}
}
