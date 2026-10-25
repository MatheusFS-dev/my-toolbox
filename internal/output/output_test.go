package output

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func fixedOutputClock() time.Time { return time.Date(2026, 10, 22, 9, 32, 23, 0, time.Local) }

func TestToolOutputPrefixesEveryMessageAndOpensWithBlankLine(t *testing.T) {
	var destination bytes.Buffer
	output := New(&destination, "install-gh", fixedOutputClock, ColorModeNever)

	if err := output.Info("Checking...\nsecond line"); err != nil {
		t.Fatal(err)
	}
	want := "\n[09:32:23 install-gh INFO] Checking...\n[09:32:23 install-gh INFO] second line\n"
	if destination.String() != want {
		t.Fatalf("output = %q, want %q", destination.String(), want)
	}
}

func TestToolOutputMenuPrefixesOnlyQuestion(t *testing.T) {
	var destination bytes.Buffer
	output := New(&destination, "git-sync", fixedOutputClock, ColorModeNever)
	if err := output.Menu("How should Git remember credentials?", []string{"1. Secure credential manager", "2. Do not save"}); err != nil {
		t.Fatal(err)
	}
	want := "\n[09:32:23 git-sync PROMPT] How should Git remember credentials?\n  1. Secure credential manager\n  2. Do not save\n"
	if destination.String() != want {
		t.Fatalf("menu = %q, want %q", destination.String(), want)
	}
}

func TestToolOutputMenuColorsQuestionAndChoices(t *testing.T) {
	var destination bytes.Buffer
	output := New(&destination, "git-sync", fixedOutputClock, ColorMode256)
	if err := output.Menu("Choose", []string{"1. First"}); err != nil {
		t.Fatal(err)
	}
	want := "\n\x1b[36m[09:32:23 git-sync PROMPT] Choose\x1b[0m\n\x1b[36m  1. First\x1b[0m\n"
	if destination.String() != want {
		t.Fatalf("menu = %q, want %q", destination.String(), want)
	}
}

func TestToolOutputUsesSemanticColorsAndResetsEveryLine(t *testing.T) {
	var destination bytes.Buffer
	output := New(&destination, "setup-wsl", fixedOutputClock, ColorMode256)

	for _, write := range []func(string) error{output.Info, output.Prompt, output.Warn, output.Error, output.Success} {
		if err := write("message"); err != nil {
			t.Fatal(err)
		}
	}
	want := "\n[09:32:23 setup-wsl INFO] message\n" +
		"\x1b[36m[09:32:23 setup-wsl PROMPT] message\x1b[0m\n" +
		"\x1b[38;5;208m[09:32:23 setup-wsl WARN] message\x1b[0m\n" +
		"\x1b[31m[09:32:23 setup-wsl ERROR] message\x1b[0m\n" +
		"\x1b[32m[09:32:23 setup-wsl SUCCESS] message\x1b[0m\n"
	if destination.String() != want {
		t.Fatalf("output = %q, want %q", destination.String(), want)
	}
}

func TestToolOutputFallsBackToYellowWarnings(t *testing.T) {
	var destination bytes.Buffer
	output := New(&destination, "tool", fixedOutputClock, ColorModeBasic)
	if err := output.Warn("Careful."); err != nil {
		t.Fatal(err)
	}
	if got := destination.String(); !strings.Contains(got, "\x1b[33m") || strings.Contains(got, "38;5;208") {
		t.Fatalf("warning = %q", got)
	}
}

func TestToolOutputColorDetectionHonorsRedirectsAndNoColor(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "")
	if mode := DetectColor(&bytes.Buffer{}); mode != ColorModeNever {
		t.Fatalf("redirected mode = %v", mode)
	}

	t.Setenv("NO_COLOR", "1")
	if mode := DetectColor(os.Stdout); mode != ColorModeNever {
		t.Fatalf("NO_COLOR mode = %v", mode)
	}
}

func TestToolOutputExternalBytesPassThroughUnchanged(t *testing.T) {
	var destination bytes.Buffer
	output := New(&destination, "tool", fixedOutputClock, ColorModeNever)
	external := []byte("child stdout\x00\xff\n")

	written, err := output.External().Write(external)
	if err != nil {
		t.Fatal(err)
	}
	if written != len(external) || !bytes.Equal(destination.Bytes(), external) {
		t.Fatalf("external output = %q", destination.Bytes())
	}
}
