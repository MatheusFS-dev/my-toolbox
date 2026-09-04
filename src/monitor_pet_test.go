//go:build !windows

package main

import (
	"os"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

var legacyMonitorRobotFramesForTests = []string{
	"     ╭─────╮\n     │ ◉ ◉ │\n     ╰──┬──╯\n     ──┤▣├──\n       │ │\n      ╱   ╲\n     ┴     ┴",
	"     ╭─────╮\n     │ ◉ ◉ │\n     ╰──┬──╯\n     ╲─┤▣├──\n       │ │\n      ╱  │\n     ┴   ┴",
	"     ╭─────╮\n     │ ◉ ◉ │\n     ╰──┬──╯\n     ╲─┤▣├─╱\n       │ │\n       │ │\n       ┴ ┴",
	"     ╭─────╮\n     │ ◉ ◉ │\n     ╰──┬──╯\n     ──┤▣├─╱\n       │ │\n       │  ╲\n       ┴   ┴",
	"     ╭─────╮\n     │ ◉ ◉ │\n     ╰──┬──╯\n     ─╲┤▣├╱─\n       │ │\n      ╱   ╲\n     ┴     ┴",
	"     ╭─────╮\n     │ ◉ ◉ │\n     ╰──┬──╯\n     ╱─┤▣├──\n       │ │\n       │  ╲\n       ┴   ┴",
	"     ╭─────╮\n     │ ◉ ◉ │\n     ╰──┬──╯\n     ╱─┤▣├─╲\n       │ │\n       │ │\n       ┴ ┴",
	"     ╭─────╮\n     │ ◉ ◉ │\n     ╰──┬──╯\n     ──┤▣├─╲\n       │ │\n      ╱  │\n     ┴   ┴",
}

// TestMain preserves the legacy robot-specific assertions while the monitor uses the new pet in production.
func TestMain(m *testing.M) {
	monitorRobotFrames = legacyMonitorRobotFramesForTests
	os.Exit(m.Run())
}

// TestMonitorSideCatFramesStayLyingAndStable verifies the pet remains a compact side-view lying cat through the full idle loop.
func TestMonitorSideCatFramesStayLyingAndStable(t *testing.T) {
	if len(monitorSideCatFrames) != 8 {
		t.Fatalf("side cat has %d frames, want 8", len(monitorSideCatFrames))
	}

	seen := map[string]bool{}
	for index, frame := range monitorSideCatFrames {
		lines := strings.Split(frame, "\n")
		if len(lines) != 6 {
			t.Fatalf("side cat frame %d has %d lines, want 6:\n%s", index, len(lines), frame)
		}
		if !strings.Contains(frame, "/\\_") || !strings.Contains(frame, "<") {
			t.Fatalf("side cat frame %d lost its side-profile head:\n%s", index, frame)
		}
		if strings.ContainsAny(frame, "╭╮╰╯▣◉┬┤┴") {
			t.Fatalf("side cat frame %d contains robot glyphs:\n%s", index, frame)
		}
		if lipgloss.Width(frame) > 30 {
			t.Fatalf("side cat frame %d is too wide at %d columns:\n%s", index, lipgloss.Width(frame), frame)
		}
		seen[frame] = true
	}
	if len(seen) != 8 {
		t.Fatalf("side cat exposes %d distinct stages, want 8", len(seen))
	}
}
