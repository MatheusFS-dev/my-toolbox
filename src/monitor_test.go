//go:build !windows

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

func TestMonitorHelpDocumentsPublicCommandsAndSequentialTargets(t *testing.T) {
	output := &bytes.Buffer{}
	if code := runMonitorCLI("/payload", "1.2.3", []string{"--help"}, strings.NewReader(""), output, output); code != 0 {
		t.Fatalf("runMonitorCLI() = %d", code)
	}
	for _, text := range []string{"monitor <script.py> [more.py ...]", "monitor config", "sequentially"} {
		if !strings.Contains(output.String(), text) {
			t.Fatalf("help is missing %q: %q", text, output.String())
		}
	}
}

func TestResolveMonitorRunTitlesOffersQueueIndividualAndFilenameModes(t *testing.T) {
	targets := []string{"/work/first.py", "/work/second.py"}
	tests := []struct {
		name    string
		mode    string
		entered []string
		want    []string
	}{
		{name: "queue title", mode: "queue", entered: []string{"Experiment"}, want: []string{"Experiment", "Experiment"}},
		{name: "blank queue title", mode: "queue", entered: []string{"  "}, want: []string{"first.py", "first.py"}},
		{name: "individual titles", mode: "individual", entered: []string{"Warmup", ""}, want: []string{"Warmup", "second.py"}},
		{name: "filenames", mode: "filename", want: []string{"first.py", "second.py"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			titles, err := resolveMonitorRunTitles(targets, test.mode, test.entered)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(titles, "|") != strings.Join(test.want, "|") {
				t.Fatalf("titles = %#v, want %#v", titles, test.want)
			}
		})
	}
}

func TestSingleScriptTitlePromptSkipsModeSelection(t *testing.T) {
	keys := []string{}
	titles, err := promptMonitorRunTitlesWith([]string{"/work/job.py"}, func(field huh.Field) error {
		keys = append(keys, field.GetKey())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(keys, "|") != "run-title" {
		t.Fatalf("prompted fields = %#v", keys)
	}
	if len(titles) != 1 || titles[0] != "job.py" {
		t.Fatalf("titles = %#v", titles)
	}
}

func TestEditMonitorRunOffersEveryRuntimeSettingWithoutChangingSavedDefaults(t *testing.T) {
	stateRoot := t.TempDir()
	configPath := filepath.Join(stateRoot, "config.json")
	config := monitorDefaultConfig([]string{"alerts@example.com"})
	restart := config["restart"].(map[string]any)
	restart["memory_aware"] = true
	restart["memory_limit_gb"] = 100.0
	leak := config["leak_detection"].(map[string]any)
	leak["enabled"] = true
	config["notifications"].(map[string]any)["heartbeat"] = true
	if err := writeMonitorJSON(configPath, config); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	interpreter, err := resolveMonitorInterpreter("")
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	selected, temporaryPath, err := editMonitorRunWith(configPath, interpreter, func(field huh.Field) error {
		keys = append(keys, field.GetKey())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(temporaryPath)
	if selected.Path != interpreter.Path {
		t.Fatalf("selected interpreter = %q, want %q", selected.Path, interpreter.Path)
	}
	wantKeys := []string{
		"python-interpreter", "sampling-interval", "crash-retries", "base-retry-delay",
		"retry-backoff", "max-retry-delay", "rapid-crash-threshold", "automatic-restarts",
		"automatic-restart-type", "memory-restart-limit", "heartbeat-enabled", "heartbeat-interval",
		"notification-recovery", "notification-scheduled-restart", "notification-final-failure",
		"notification-completion", "notification-possible-leak", "notification-possible-code-error",
		"leak-detection-enabled", "leak-warmup", "leak-window", "leak-minimum-growth",
		"leak-minimum-slope", "reports-enabled", "viewer-enabled",
	}
	if strings.Join(keys, "|") != strings.Join(wantKeys, "|") {
		t.Fatalf("run editor fields = %#v, want %#v", keys, wantKeys)
	}
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("editing a run changed the saved configuration")
	}
	content, err := os.ReadFile(temporaryPath)
	if err != nil {
		t.Fatal(err)
	}
	var temporary map[string]any
	if err := json.Unmarshal(content, &temporary); err != nil {
		t.Fatal(err)
	}
	if numberValue(temporary["sampling_interval_seconds"], 0) != 1 || !boolValue(temporary["reports_enabled"], false) || boolValue(temporary["gui_viewer"], true) {
		t.Fatalf("temporary run settings = %#v", temporary)
	}
}

func TestMonitorMessageColorUsesSeverityInsteadOfKeywords(t *testing.T) {
	plain := "target output says failure warning success"
	if colored := colorMonitorMessage(plain, monitorNeutral); colored != plain {
		t.Fatalf("neutral target output was colorized: %q", colored)
	}
	warning := "log viewer is unavailable"
	colored := colorMonitorMessage(warning, monitorWarning)
	if colored == warning {
		t.Fatalf("warning message was not colorized: %q", colored)
	}
	if visible := strings.Join(sanitizeMonitorLines(colored, -1), "\n"); visible != warning {
		t.Fatalf("color changed visible message: %q", visible)
	}
	if lipgloss.Width(colored) != lipgloss.Width(warning) {
		t.Fatalf("colored width = %d, plain width = %d", lipgloss.Width(colored), lipgloss.Width(warning))
	}
}

func TestMonitorFinalSummaryShowsRunDetailsAndEmailResult(t *testing.T) {
	dashboard := newMonitorDashboard([]string{"/work/job.py"}, monitorInterpreter{Path: "/venv/bin/python3", Environment: "venv"})
	dashboard.apply(monitorEvent{Type: "run_created", Title: "Experiment A", Script: "/work/job.py", RunDirectory: "/work/runs/one"})
	dashboard.apply(monitorEvent{Type: "attempt_started", Attempt: 2, PID: 42})
	dashboard.apply(monitorEvent{Type: "restart_decision", CrashCount: 1, ScheduledCount: 2, MemoryCount: 3})
	dashboard.apply(monitorEvent{Type: "email_result", Kind: "completion", Success: true})
	dashboard.apply(monitorEvent{Type: "final_outcome", Title: "Experiment A", Outcome: "success", Elapsed: 65, Attempt: 2, CrashCount: 1, ScheduledCount: 2, MemoryCount: 3})

	summary := strings.Join(sanitizeMonitorLines(dashboard.finalSummary(), -1), "\n")
	for _, expected := range []string{"MONITOR RUN COMPLETE", "Experiment A", "SUCCESS", "job.py", "1m05s", "2", "crash 1", "time 2", "memory 3", "completion sent", "/work/runs/one"} {
		if !strings.Contains(summary, expected) {
			t.Fatalf("final summary is missing %q:\n%s", expected, summary)
		}
	}
}

func TestDashboardRetainsOnlyTenLatestOutputLines(t *testing.T) {
	dashboard := newMonitorDashboard([]string{"a.py"}, monitorInterpreter{Path: "/venv/bin/python3", Environment: "venv"})
	for index := 0; index < 12; index++ {
		dashboard.apply(monitorEvent{Type: "target_output", Text: fmt.Sprintf("line-%d", index)})
	}
	if len(dashboard.Output) != 10 || dashboard.Output[0] != "line-2" || dashboard.Output[9] != "line-11" {
		t.Fatalf("dashboard output = %v", dashboard.Output)
	}
}

func TestMonitorSetupRequiresBothConfigurationAndCredentials(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "config.json")
	credentials := filepath.Join(root, "credentials.json")
	if err := writeMonitorJSON(config, monitorDefaultConfig([]string{"person@example.com"})); err != nil {
		t.Fatal(err)
	}
	if err := monitorSetupComplete(config, credentials); err == nil {
		t.Fatal("setup was accepted without credentials")
	}
	value := map[string]any{"host": "smtp.example.com", "port": 587, "security": "starttls", "sender": "person@example.com", "password": "secret"}
	if err := writeMonitorJSON(credentials, value); err != nil {
		t.Fatal(err)
	}
	if err := monitorSetupComplete(config, credentials); err != nil {
		t.Fatalf("complete setup rejected: %v", err)
	}
}

func TestResolveMonitorInterpreterPrefersVirtualEnv(t *testing.T) {
	root := t.TempDir()
	virtual := filepath.Join(root, "chosen")
	conda := filepath.Join(root, "conda")
	for _, directory := range []string{virtual, conda} {
		if err := os.MkdirAll(filepath.Join(directory, "bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		python := filepath.Join(directory, "bin", "python3")
		if err := os.WriteFile(python, []byte("#!/bin/sh\nprintf '3\\n'\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("VIRTUAL_ENV", virtual)
	t.Setenv("CONDA_PREFIX", conda)
	interpreter, err := resolveMonitorInterpreter("")
	if err != nil {
		t.Fatal(err)
	}
	if interpreter.Path != filepath.Join(virtual, "bin", "python3") || interpreter.Environment != "chosen" {
		t.Fatalf("interpreter = %#v", interpreter)
	}
}

func TestValidateMonitorTargetsRejectsDirectoriesAndNonPythonFiles(t *testing.T) {
	root := t.TempDir()
	text := filepath.Join(root, "target.txt")
	if err := os.WriteFile(text, []byte("pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{root, text} {
		_, err := validateMonitorTargets([]string{target})
		if err == nil {
			t.Fatalf("validateMonitorTargets(%q) succeeded", target)
		}
	}
}

func TestSanitizeTerminalTextRemovesControlsAndBoundsLines(t *testing.T) {
	lines := sanitizeMonitorLines("one\n\x1b[31mtwo\x1b[0m\nthree\nfour", 3)
	if got := strings.Join(lines, "|"); got != "two|three|four" {
		t.Fatalf("sanitized lines = %q", got)
	}
}

func TestGmailPresetSuppliesSMTPSettings(t *testing.T) {
	credentials := monitorEmailCredentials("gmail", "person@gmail.com", "abcd efgh ijkl mnop", monitorEmailSettings{})
	if credentials.Host != "smtp.gmail.com" || credentials.Port != "587" || credentials.Security != "starttls" {
		t.Fatalf("Gmail SMTP settings = %#v", credentials)
	}
	if credentials.Sender != "person@gmail.com" || credentials.Password != "abcdefghijklmnop" {
		t.Fatalf("Gmail account settings = %#v", credentials)
	}
}

func TestBlankPasswordKeepsExistingCredential(t *testing.T) {
	existing := monitorEmailSettings{Password: "saved-app-password"}
	credentials := monitorEmailCredentials("gmail", "person@gmail.com", "", existing)
	if credentials.Password != "saved-app-password" {
		t.Fatalf("password = %q", credentials.Password)
	}
}

func TestConfigurationSummaryNeverContainsPassword(t *testing.T) {
	credentials := monitorEmailSettings{
		Provider: "gmail", Sender: "person@gmail.com", Password: "top-secret",
		Host: "smtp.gmail.com", Port: "587", Security: "starttls",
	}
	summary := monitorConfigurationSummary(credentials, []string{"alerts@example.com"}, true)
	if strings.Contains(summary, "top-secret") {
		t.Fatalf("summary exposed password: %q", summary)
	}
	for _, expected := range []string{"Gmail", "person@gmail.com", "alerts@example.com", "default monitoring settings"} {
		if !strings.Contains(summary, expected) {
			t.Fatalf("summary is missing %q: %q", expected, summary)
		}
	}
}

func TestMonitorRestartModeDisablesAutomaticRestarts(t *testing.T) {
	restart := monitorDefaultConfig(nil)["restart"].(map[string]any)
	if err := applyMonitorRestartMode(restart, false, "memory", 512); err != nil {
		t.Fatal(err)
	}
	if boolValue(restart["memory_aware"], true) || numberValue(restart["scheduled_interval_minutes"], -1) != 0 {
		t.Fatalf("restart settings = %#v", restart)
	}
}

func TestMonitorRestartModeSelectsOnlyMemoryLimit(t *testing.T) {
	restart := monitorDefaultConfig(nil)["restart"].(map[string]any)
	if err := applyMonitorRestartMode(restart, true, "memory", 1.5); err != nil {
		t.Fatal(err)
	}
	if !boolValue(restart["memory_aware"], false) || numberValue(restart["memory_limit_gb"], 0) != 1.5 || numberValue(restart["scheduled_interval_minutes"], -1) != 0 {
		t.Fatalf("restart settings = %#v", restart)
	}
	if _, legacy := restart["memory_limit_mib"]; legacy {
		t.Fatalf("restart settings retained legacy MiB key: %#v", restart)
	}
}

func TestMonitorRestartModeSelectsOnlyTimeInterval(t *testing.T) {
	restart := monitorDefaultConfig(nil)["restart"].(map[string]any)
	if err := applyMonitorRestartMode(restart, true, "time", 45); err != nil {
		t.Fatal(err)
	}
	if boolValue(restart["memory_aware"], true) || numberValue(restart["scheduled_interval_minutes"], 0) != 45 {
		t.Fatalf("restart settings = %#v", restart)
	}
}

func TestMonitorRestartModeRejectsNonPositiveValues(t *testing.T) {
	for _, mode := range []string{"memory", "time"} {
		restart := monitorDefaultConfig(nil)["restart"].(map[string]any)
		if err := applyMonitorRestartMode(restart, true, mode, 0); err == nil {
			t.Fatalf("mode %q accepted zero", mode)
		}
	}
}

func TestMonitorLeakWarmupConvertsMinutesToSeconds(t *testing.T) {
	leak := monitorDefaultConfig(nil)["leak_detection"].(map[string]any)
	if err := applyMonitorLeakWarmup(leak, true, 7); err != nil {
		t.Fatal(err)
	}
	if !boolValue(leak["enabled"], false) || numberValue(leak["warmup_seconds"], 0) != 420 {
		t.Fatalf("leak settings = %#v", leak)
	}
	if err := applyMonitorLeakWarmup(leak, true, 0); err == nil {
		t.Fatal("zero warm-up was accepted")
	}
}

func TestMonitorDashboardShowsConfigurationAndGraphicalResources(t *testing.T) {
	config := monitorDefaultConfig([]string{"alerts@example.com"})
	restart := config["restart"].(map[string]any)
	restart["crash_retries"] = 4
	restart["memory_aware"] = true
	restart["memory_limit_gb"] = 100.0
	config["notifications"].(map[string]any)["heartbeat"] = true
	config["heartbeat_interval_minutes"] = 12
	leak := config["leak_detection"].(map[string]any)
	leak["warmup_seconds"] = 420
	dashboard := newMonitorDashboard([]string{"/work/job.py"}, monitorInterpreter{Path: "/venv/bin/python3", Environment: "venv"}, config)
	dashboard.Width = 120
	dashboard.Height = 42
	dashboard.apply(monitorEvent{Type: "run_created", Script: "/work/job.py", RunDirectory: "/work/runs/one"})
	dashboard.apply(monitorEvent{Type: "attempt_started", PID: 42, Attempt: 1})
	dashboard.apply(monitorEvent{Type: "resource_sample", Elapsed: 30, CPUPercent: 25, RAMBytes: 453_500_000, RAMMiB: 432.5, SystemRAMTotalBytes: 32_000_000_000, GPUPercent: 50.0, GPUMemoryMiB: 256, GPUMemoryTotalMiB: 8192, GPUScope: "system-wide"})
	resources := dashboard.resourcePanel()
	if !strings.Contains(resources, "0.454 / 32.000 GB") || strings.Contains(resources, "100 GB") {
		t.Fatalf("RAM row includes more than used and system total:\n%s", resources)
	}

	rendered := dashboard.View()
	if !rendered.AltScreen {
		t.Fatal("dashboard did not request the terminal alternate screen")
	}
	view := rendered.Content
	visibleView := strings.Join(sanitizeMonitorLines(view, -1), "\n")
	for _, expected := range []string{
		"╭", "TARGET", "RESOURCES", "RESTART POLICY", "MONITORING", "RECENT ACTIVITY", "LATEST OUTPUT",
		"/venv/bin/python3", "Memory-aware", "100 GB", "Rapid crash", "60s", "0.454 / 32.000 GB", "SYSTEM-WIDE", "0.268 / 8.590 GB", "4", "7m", "12m", "━━", "Ctrl+C stop target and quit",
	} {
		if !strings.Contains(visibleView, expected) {
			t.Fatalf("dashboard is missing %q:\n%s", expected, view)
		}
	}
	if strings.Contains(visibleView, "password") {
		t.Fatalf("dashboard exposed credential vocabulary:\n%s", view)
	}
	if strings.Contains(view, "░") || strings.Contains(view, "█") {
		t.Fatalf("dashboard retained terminal-dependent shade glyphs:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > dashboard.Width {
			t.Fatalf("dashboard line width = %d, terminal width = %d:\n%s", lipgloss.Width(line), dashboard.Width, view)
		}
	}
}

func TestMonitorRobotDancesInsideRestartPolicyPanel(t *testing.T) {
	dashboard := newMonitorDashboard([]string{"job.py"}, monitorInterpreter{}, monitorDefaultConfig(nil))
	first := strings.Join(sanitizeMonitorLines(dashboard.restartPanel(), -1), "\n")
	if strings.Contains(first, "MONITOR BOT") {
		t.Fatalf("restart panel still labels the pet:\n%s", first)
	}
	if monitorPetFrameInterval < 600*time.Millisecond {
		t.Fatalf("robot animation interval = %s, want a slower dance", monitorPetFrameInterval)
	}
	if dashboard.Init() == nil {
		t.Fatal("dashboard did not schedule the robot animation")
	}
	firstWidth, firstHeight := lipgloss.Width(first), lipgloss.Height(first)
	frames := map[string]bool{first: true}
	for index := 1; index < 8; index++ {
		model, command := dashboard.Update(monitorPetTick{})
		if command == nil {
			t.Fatal("robot animation did not schedule its next frame")
		}
		dashboard = model.(*monitorDashboard)
		frame := strings.Join(sanitizeMonitorLines(dashboard.restartPanel(), -1), "\n")
		frames[frame] = true
		if lipgloss.Width(frame) != firstWidth || lipgloss.Height(frame) != firstHeight {
			t.Fatalf("robot dance changed panel content geometry from %dx%d to %dx%d", firstWidth, firstHeight, lipgloss.Width(frame), lipgloss.Height(frame))
		}
	}
	if len(frames) != 8 {
		t.Fatalf("robot dance exposed %d distinct poses, want 8", len(frames))
	}
}

func TestMonitorRobotKeepsACompactConnectedSilhouette(t *testing.T) {
	if len(monitorRobotFrames) != 8 {
		t.Fatalf("robot dance has %d frames, want 8", len(monitorRobotFrames))
	}
	var head []string
	anchor := -1
	for index, frame := range monitorRobotFrames {
		lines := strings.Split(frame, "\n")
		if len(lines) != 7 {
			t.Fatalf("robot frame %d has %d lines, want 7:\n%s", index, len(lines), frame)
		}
		if index == 0 {
			head = append([]string(nil), lines[:3]...)
		} else if !slices.Equal(lines[:3], head) {
			t.Fatalf("robot frame %d moved or disconnected its head:\n%s", index, frame)
		}

		neckByte := strings.Index(lines[2], "┬")
		bodyByte := strings.Index(lines[3], "▣")
		if neckByte < 0 || bodyByte < 0 {
			t.Fatalf("robot frame %d is missing its neck or body:\n%s", index, frame)
		}
		neck := lipgloss.Width(lines[2][:neckByte])
		body := lipgloss.Width(lines[3][:bodyByte])
		if neck != body {
			t.Fatalf("robot frame %d has neck at %d and body at %d:\n%s", index, neck, body, frame)
		}
		if anchor < 0 {
			anchor = neck
		} else if neck != anchor {
			t.Fatalf("robot frame %d shifted its center from %d to %d:\n%s", index, anchor, neck, frame)
		}

		shoulders := strings.TrimRight(lines[3], " ")
		first := len(lines[3]) - len(strings.TrimLeft(lines[3], " "))
		last := lipgloss.Width(shoulders) - 1
		if first < body-4 || last > body+4 {
			t.Fatalf("robot frame %d has oversized arms spanning columns %d..%d around body %d:\n%s", index, first, last, body, frame)
		}

		legRunes := []rune(lines[5])
		leftLegConnected := legRunes[6] == '╱' || legRunes[7] == '│'
		rightLegConnected := (len(legRunes) > 10 && legRunes[10] == '╲') || legRunes[9] == '│'
		if !leftLegConnected || !rightLegConnected {
			t.Fatalf("robot frame %d has a disconnected leg pose:\n%s", index, frame)
		}
	}
}

func TestMonitorResourcePanelReflowsWithoutEllipsesAtNarrowPairWidth(t *testing.T) {
	dashboard := newMonitorDashboard([]string{"job.py"}, monitorInterpreter{}, monitorDefaultConfig(nil))
	dashboard.apply(monitorEvent{
		Type: "resource_sample", CPUPercent: 12.5, RAMBytes: 1_519_000_000,
		SystemRAMTotalBytes: 16_466_000_000, GPUPercent: 25.0,
		GPUMemoryMiB: 236.0, GPUMemoryTotalMiB: 8151.0, GPUScope: "system-wide",
	})
	rendered := dashboard.panel("RESOURCES", dashboard.resourcePanel(), 40)
	visible := strings.Join(sanitizeMonitorLines(rendered, -1), "\n")
	resourceVisible := strings.Join(sanitizeMonitorLines(dashboard.resourcePanel(), -1), "\n")
	if strings.Contains(visible, "…") || strings.Contains(visible, "...") {
		t.Fatalf("resource panel truncated a metric:\n%s", visible)
	}
	for _, expected := range []string{"CPU", "12.5%", "RAM", "1.519 / 16.466 GB", "GPU", "SYSTEM-WIDE", "Utilization", "25.0%", "Memory", "0.247 / 8.547 GB"} {
		if !strings.Contains(resourceVisible, expected) {
			t.Fatalf("resource panel is missing %q:\n%s", expected, visible)
		}
	}
	if meters := strings.Count(resourceVisible, "━") + strings.Count(resourceVisible, "─"); meters != 40 {
		t.Fatalf("resource panel has %d meter cells, want four 10-cell meters:\n%s", meters, visible)
	}
}

func TestPossibleCodeErrorShowsPersistentLargeRedDashboardWarning(t *testing.T) {
	dashboard := newMonitorDashboard([]string{"job.py"}, monitorInterpreter{}, monitorDefaultConfig(nil))
	dashboard.Width = 100
	dashboard.Height = 35
	dashboard.apply(monitorEvent{Type: "lifecycle", State: "possible_code_error_warning", Message: "Target exited with code 1 after 0.420s."})
	dashboard.apply(monitorEvent{Type: "attempt_started", Attempt: 2, PID: 99})

	view := dashboard.View().Content
	visible := strings.Join(sanitizeMonitorLines(view, -1), "\n")
	if !strings.Contains(visible, "POSSIBLE CODE ERROR") || !strings.Contains(visible, "Target exited with code 1 after 0.420s.") {
		t.Fatalf("dashboard warning is missing or did not persist:\n%s", visible)
	}
	if !strings.Contains(view, "\x1b[38;5;196m") || strings.Index(visible, "POSSIBLE CODE ERROR") > strings.Index(visible, "TARGET") {
		t.Fatalf("dashboard warning is not prominent and red:\n%s", view)
	}
}

func TestMonitorDashboardListsAndColorsEveryEmailToggle(t *testing.T) {
	config := monitorDefaultConfig(nil)
	notifications := config["notifications"].(map[string]any)
	notifications["completion"] = true
	notifications["final_failure"] = false
	notifications["possible_leak"] = true
	notifications["recovery"] = false
	notifications["scheduled_restart"] = true
	notifications["heartbeat"] = false
	notifications["possible_code_error"] = true
	dashboard := newMonitorDashboard([]string{"job.py"}, monitorInterpreter{}, config)
	body := dashboard.monitoringPanel()
	for _, expected := range []string{
		"Completion email      on",
		"Final failure email   off",
		"Possible leak email   on",
		"Recovery email        off",
		"Scheduled restart     on",
		"Heartbeat email       off",
		"Code error email      on",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("monitoring panel is missing %q:\n%s", expected, body)
		}
	}
	styled := dashboard.panel("MONITORING", body, 50)
	if !strings.Contains(styled, colorMonitorMessage("on", monitorSuccess)) {
		t.Fatalf("enabled state is not green:\n%s", styled)
	}
	if !strings.Contains(styled, colorMonitorMessage("off", monitorFailure)) {
		t.Fatalf("disabled state is not red:\n%s", styled)
	}
}

func TestMonitorDashboardStacksPanelsWhenNarrow(t *testing.T) {
	dashboard := newMonitorDashboard([]string{"job.py"}, monitorInterpreter{Path: "/bin/python3", Environment: "system"}, monitorDefaultConfig(nil))
	dashboard.Width = 55
	view := dashboard.View().Content
	targetAt := strings.Index(view, "TARGET")
	resourcesAt := strings.Index(view, "RESOURCES")
	if targetAt < 0 || resourcesAt < 0 || !strings.Contains(view[targetAt:resourcesAt], "\n") {
		t.Fatalf("narrow dashboard did not stack panels:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > dashboard.Width {
			t.Fatalf("narrow dashboard line width = %d, terminal width = %d:\n%s", lipgloss.Width(line), dashboard.Width, view)
		}
	}
}

func TestMonitorSideBySidePanelsShareTopAndBottomRows(t *testing.T) {
	dashboard := newMonitorDashboard([]string{"job.py"}, monitorInterpreter{}, monitorDefaultConfig(nil))
	pair := dashboard.panelPair("SHORT", "one", "TALL", "one\ntwo\nthree", 30, 30)
	visible := strings.Join(sanitizeMonitorLines(pair, -1), "\n")
	lines := strings.Split(visible, "\n")
	if strings.Count(lines[0], "╭") != 2 {
		t.Fatalf("panel tops are not aligned:\n%s", visible)
	}
	if strings.Count(lines[len(lines)-1], "╰") != 2 {
		t.Fatalf("panel bottoms are not aligned:\n%s", visible)
	}
	for _, line := range lines[1 : len(lines)-1] {
		if strings.Count(line, "│") != 4 {
			t.Fatalf("panel border does not span shared height:\n%s", visible)
		}
	}
	for _, line := range strings.Split(pair, "\n") {
		if width := lipgloss.Width(line); width != 61 {
			t.Fatalf("paired panel width = %d, want 61:\n%s", width, pair)
		}
	}
}

func TestMonitorFullWidthPanelUsesItsEntireRequestedWidth(t *testing.T) {
	dashboard := newMonitorDashboard([]string{"job.py"}, monitorInterpreter{}, monitorDefaultConfig(nil))
	panel := dashboard.panel("OUTPUT", "one\ntwo", 60)
	for _, line := range strings.Split(panel, "\n") {
		if width := lipgloss.Width(line); width != 60 {
			t.Fatalf("panel width = %d, want 60:\n%s", width, panel)
		}
	}
}

func TestMonitorDashboardScrollsAllPanelsInAStandardTerminalHeight(t *testing.T) {
	dashboard := newMonitorDashboard([]string{"/work/job.py"}, monitorInterpreter{Path: "/venv/bin/python3", Environment: "venv"}, monitorDefaultConfig(nil))
	dashboard.Width = 80
	dashboard.Height = 24
	view := dashboard.View().Content
	if lines := strings.Count(view, "\n") + 1; lines > dashboard.Height {
		t.Fatalf("dashboard height = %d lines, terminal height = %d:\n%s", lines, dashboard.Height, view)
	}
	for _, required := range []string{"TARGET", "RESOURCES", "Ctrl+C stop target and quit"} {
		if !strings.Contains(view, required) {
			t.Fatalf("standard terminal omitted visible %q:\n%s", required, view)
		}
	}
	for _, required := range []string{"RESTART POLICY", "MONITORING", "RECENT ACTIVITY", "LATEST OUTPUT"} {
		if !strings.Contains(dashboard.viewportContent(), required) {
			t.Fatalf("scrollable content omitted %q:\n%s", required, dashboard.viewportContent())
		}
	}
}

func TestMonitorDashboardScrollOffsetSurvivesLiveUpdatesAndClampsOnResize(t *testing.T) {
	dashboard := newMonitorDashboard([]string{"job.py"}, monitorInterpreter{Path: "/bin/python3", Environment: "system"}, monitorDefaultConfig(nil))
	dashboard.Update(tea.WindowSizeMsg{Width: 55, Height: 12})
	dashboard.viewport.SetYOffset(8)
	dashboard.apply(monitorEvent{Type: "resource_sample", RAMBytes: 500_000_000, SystemRAMTotalBytes: 8_000_000_000})
	dashboard.rebuildViewport()
	if dashboard.viewport.YOffset() == 0 {
		t.Fatal("live update reset viewport to top")
	}
	dashboard.Update(tea.WindowSizeMsg{Width: 120, Height: 200})
	if dashboard.viewport.YOffset() != 0 {
		t.Fatalf("large resize did not clamp viewport offset: %d", dashboard.viewport.YOffset())
	}
}

func TestMonitorDashboardCtrlCCancelsExactlyOnceAndWaitsForFinalOutcome(t *testing.T) {
	cancel := make(chan struct{}, 2)
	dashboard := newMonitorDashboard([]string{"job.py"}, monitorInterpreter{}, monitorDefaultConfig(nil))
	dashboard.Cancel = cancel
	for range 2 {
		updated, command := dashboard.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		dashboard = updated.(*monitorDashboard)
		if command != nil {
			t.Fatal("Ctrl+C quit the dashboard before supervisor cleanup")
		}
	}
	if len(cancel) != 1 || !dashboard.Stopping {
		t.Fatalf("cancel signals = %d, stopping = %t", len(cancel), dashboard.Stopping)
	}
	view := strings.Join(sanitizeMonitorLines(dashboard.View().Content, -1), "\n")
	if !strings.Contains(view, "Stopping target and descendants") {
		t.Fatalf("stopping state is not visible:\n%s", dashboard.View().Content)
	}
}

func TestMonitorDashboardBoundsRecentActivityAndLabelsOutputStreams(t *testing.T) {
	dashboard := newMonitorDashboard([]string{"job.py"}, monitorInterpreter{}, monitorDefaultConfig(nil))
	for index := range 20 {
		dashboard.apply(monitorEvent{Type: "lifecycle", State: fmt.Sprintf("state-%d", index)})
	}
	dashboard.apply(monitorEvent{Type: "target_output", Stream: "stderr", Text: "problem"})
	if len(dashboard.Activity) != 8 {
		t.Fatalf("activity count = %d", len(dashboard.Activity))
	}
	if dashboard.Output[len(dashboard.Output)-1] != "[stderr] problem" {
		t.Fatalf("output = %#v", dashboard.Output)
	}
}
