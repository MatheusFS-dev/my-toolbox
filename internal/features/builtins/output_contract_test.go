package builtins

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func fixedOutputClock() time.Time {
	return time.Date(2026, 10, 22, 9, 32, 23, 0, time.Local)
}

func TestBuiltinLifecycleWritesProgressBeforeActionAndOneFinalSuccess(t *testing.T) {
	var destination bytes.Buffer
	builtins := &ToolboxBuiltins{output: &destination, now: fixedOutputClock, color: colorModeNever}
	observedProgress := false

	err := builtins.withToolOutput("install-gh", func() error {
		observedProgress = strings.Contains(destination.String(), "[09:32:23 install-gh INFO] Installing...")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !observedProgress {
		t.Fatal("action began before progress was written")
	}
	if count := strings.Count(destination.String(), " SUCCESS]"); count != 1 {
		t.Fatalf("success count = %d, output = %q", count, destination.String())
	}
}

func TestBuiltinLifecycleOmitsSuccessOnFailure(t *testing.T) {
	var destination bytes.Buffer
	builtins := &ToolboxBuiltins{output: &destination, now: fixedOutputClock, color: colorModeNever}
	want := errors.New("network unavailable")

	err := builtins.withToolOutput("install-gh", func() error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(destination.String(), " SUCCESS]") {
		t.Fatalf("failed output contains success: %q", destination.String())
	}
}
