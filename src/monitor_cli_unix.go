//go:build !windows

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/charmbracelet/x/term"
)

const monitorHelp = `Monitor supervises non-interactive Python scripts for the current Linux/WSL user.

Usage:
  monitor <script.py> [more.py ...]  Review and run scripts sequentially
  monitor config                       Edit email and runtime defaults
  monitor --help                       Show this help
  monitor --version                    Show the Monitor version
`

func runMonitorCLI(root, monitorVersion string, arguments []string, input io.Reader, output, errorOutput io.Writer) int {
	if len(arguments) == 1 && (arguments[0] == "--help" || arguments[0] == "-h") {
		fmt.Fprint(output, monitorHelp)
		return 0
	}
	if len(arguments) == 1 && arguments[0] == "--version" {
		fmt.Fprintf(output, "monitor %s\n", monitorVersion)
		return 0
	}
	if len(arguments) == 1 && arguments[0] == "config" {
		if err := configureMonitor(root, output); err != nil {
			fmt.Fprintln(errorOutput, err)
			return 1
		}
		return 0
	}
	if len(arguments) == 0 {
		fmt.Fprint(errorOutput, monitorHelp)
		return 2
	}
	targets, err := validateMonitorTargets(arguments)
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 2
	}
	interpreter, err := resolveMonitorInterpreter("")
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	stateRoot, _, _, err := monitorPaths()
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	configPath := filepath.Join(stateRoot, "config.json")
	runConfigPath := configPath
	titles, _ := resolveMonitorRunTitles(targets, "filename", nil)
	var temporaryRunConfig string
	defer func() {
		if temporaryRunConfig != "" {
			_ = os.Remove(temporaryRunConfig)
		}
	}()
	if err := monitorSetupComplete(configPath, filepath.Join(stateRoot, "credentials.json")); err != nil {
		fmt.Fprintln(output, "First-run email setup is required before launching a target.")
		if err := configureMonitor(root, output); err != nil {
			fmt.Fprintln(errorOutput, err)
			return 1
		}
	}
	for terminalInteractive() {
		fmt.Fprintf(output, "\nQueue: %d script(s)\nInterpreter: %s\nEnvironment: %s\n", len(targets), interpreter.Path, interpreter.Environment)
		for index, target := range targets {
			fmt.Fprintf(output, "  %d. %s\n", index+1, target)
		}
		action := "run"
		err := huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Title("Review Monitor launch").Options(
			huh.NewOption("Run", "run"), huh.NewOption("Edit this run", "edit"),
			huh.NewOption("Config", "config"), huh.NewOption("Cancel", "cancel"),
		).Value(&action))).Run()
		if err != nil || action == "cancel" {
			return 130
		}
		if action == "config" {
			if err := configureMonitor(root, output); err != nil {
				fmt.Fprintln(errorOutput, err)
				return 1
			}
			runConfigPath = configPath
			continue
		}
		if action == "edit" {
			if temporaryRunConfig != "" {
				_ = os.Remove(temporaryRunConfig)
			}
			interpreter, temporaryRunConfig, err = editMonitorRun(runConfigPath, interpreter)
			if err != nil {
				fmt.Fprintln(errorOutput, err)
				continue
			}
			runConfigPath = temporaryRunConfig
			continue
		}
		titles, err = promptMonitorRunTitles(targets)
		if err != nil {
			return 130
		}
		break
	}
	return invokeMonitorRuntime(targets, titles, interpreter, runConfigPath, output, errorOutput)
}

func promptMonitorRunTitles(targets []string) ([]string, error) {
	return promptMonitorRunTitlesWith(targets, runMonitorConfigStep)
}

func promptMonitorRunTitlesWith(targets []string, runStep func(huh.Field) error) ([]string, error) {
	if len(targets) == 1 {
		value := ""
		if err := runStep(huh.NewInput().
			Key("run-title").
			Title("Run title").
			Description("Leave empty to use " + filepath.Base(targets[0]) + ".").
			Value(&value)); err != nil {
			return nil, err
		}
		return resolveMonitorRunTitles(targets, "queue", []string{value})
	}
	mode := "queue"
	if err := runStep(huh.NewSelect[string]().
		Key("title-mode").
		Title("Choose how to title this run").
		Description("Titles identify the run in Monitor emails, the dashboard, and the final summary.").
		Options(
			huh.NewOption("One title for the whole queue", "queue"),
			huh.NewOption("A separate title for each script", "individual"),
			huh.NewOption("Use script filenames", "filename"),
		).
		Value(&mode)); err != nil {
		return nil, err
	}
	entered := []string{}
	switch mode {
	case "queue":
		value := ""
		if err := runStep(huh.NewInput().
			Key("queue-title").
			Title("Run title").
			Description("Leave empty to use the first script filename.").
			Value(&value)); err != nil {
			return nil, err
		}
		entered = append(entered, value)
	case "individual":
		for _, target := range targets {
			value := ""
			if err := runStep(huh.NewInput().
				Key("script-title").
				Title("Title for " + filepath.Base(target)).
				Description("Leave empty to use this script's filename.").
				Value(&value)); err != nil {
				return nil, err
			}
			entered = append(entered, value)
		}
	}
	return resolveMonitorRunTitles(targets, mode, entered)
}

func resolveMonitorRunTitles(targets []string, mode string, entered []string) ([]string, error) {
	filenames := make([]string, len(targets))
	for index, target := range targets {
		filenames[index] = filepath.Base(target)
	}
	switch mode {
	case "filename":
		return filenames, nil
	case "queue":
		if len(entered) != 1 {
			return nil, fmt.Errorf("queue title requires one value")
		}
		title := strings.TrimSpace(entered[0])
		if title == "" && len(filenames) > 0 {
			title = filenames[0]
		}
		titles := make([]string, len(targets))
		for index := range titles {
			titles[index] = title
		}
		return titles, nil
	case "individual":
		if len(entered) != len(targets) {
			return nil, fmt.Errorf("individual titles must match the script count")
		}
		titles := make([]string, len(targets))
		for index, value := range entered {
			titles[index] = strings.TrimSpace(value)
			if titles[index] == "" {
				titles[index] = filenames[index]
			}
		}
		return titles, nil
	default:
		return nil, fmt.Errorf("unsupported title mode")
	}
}

func monitorSetupComplete(configPath, credentialsPath string) error {
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var config map[string]any
	if err := json.Unmarshal(configContent, &config); err != nil {
		return err
	}
	if numberValue(config["schema_version"], 0) != 1 {
		return fmt.Errorf("unsupported Monitor configuration schema")
	}
	recipients, valid := config["recipients"].([]any)
	if !valid || len(recipients) == 0 {
		return fmt.Errorf("Monitor recipients are missing")
	}
	credentialContent, err := os.ReadFile(credentialsPath)
	if err != nil {
		return err
	}
	var credentials map[string]any
	if err := json.Unmarshal(credentialContent, &credentials); err != nil {
		return err
	}
	for _, key := range []string{"host", "port", "security", "sender", "password"} {
		if fmt.Sprint(credentials[key]) == "" || fmt.Sprint(credentials[key]) == "<nil>" {
			return fmt.Errorf("Monitor SMTP credentials are incomplete")
		}
	}
	if security := fmt.Sprint(credentials["security"]); security != "starttls" && security != "tls" {
		return fmt.Errorf("Monitor SMTP security is invalid")
	}
	return nil
}

func editMonitorRun(configPath string, interpreter monitorInterpreter) (monitorInterpreter, string, error) {
	return editMonitorRunWith(configPath, interpreter, runMonitorConfigStep)
}

func editMonitorRunWith(configPath string, interpreter monitorInterpreter, runStep func(huh.Field) error) (monitorInterpreter, string, error) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return interpreter, "", err
	}
	var config map[string]any
	if err := json.Unmarshal(content, &config); err != nil {
		return interpreter, "", err
	}
	restart, restartValid := config["restart"].(map[string]any)
	notifications, notificationsValid := config["notifications"].(map[string]any)
	leak, leakValid := config["leak_detection"].(map[string]any)
	if !restartValid || !notificationsValid || !leakValid {
		return interpreter, "", fmt.Errorf("Monitor configuration has invalid nested settings")
	}
	normalizeMonitorRestart(restart)
	path := interpreter.Path
	samplingInterval := strconv.FormatFloat(numberValue(config["sampling_interval_seconds"], 1), 'f', -1, 64)
	crashRetries := strconv.Itoa(int(numberValue(restart["crash_retries"], 10)))
	baseDelaySeconds := strconv.FormatFloat(numberValue(restart["base_delay_seconds"], 3), 'f', -1, 64)
	backoffMultiplier := strconv.FormatFloat(numberValue(restart["backoff_multiplier"], 1.2), 'f', -1, 64)
	maxDelaySeconds := strconv.FormatFloat(numberValue(restart["max_delay_seconds"], 30), 'f', -1, 64)
	rapidCrashSeconds := strconv.FormatFloat(numberValue(restart["rapid_crash_seconds"], 60), 'f', -1, 64)
	scheduledMinutes := strconv.Itoa(int(numberValue(restart["scheduled_interval_minutes"], 0)))
	memoryLimitGB := strconv.FormatFloat(numberValue(restart["memory_limit_gb"], 1), 'f', -1, 64)
	automaticRestartsEnabled := boolValue(restart["memory_aware"], false) || numberValue(restart["scheduled_interval_minutes"], 0) > 0
	automaticRestartMode := "time"
	if boolValue(restart["memory_aware"], false) {
		automaticRestartMode = "memory"
	}
	heartbeatEnabled := boolValue(notifications["heartbeat"], false)
	heartbeatMinutes := strconv.Itoa(int(numberValue(config["heartbeat_interval_minutes"], 60)))
	recoveryEnabled := boolValue(notifications["recovery"], true)
	scheduledNotificationEnabled := boolValue(notifications["scheduled_restart"], true)
	finalFailureEnabled := boolValue(notifications["final_failure"], true)
	completionEnabled := boolValue(notifications["completion"], true)
	leakNotificationEnabled := boolValue(notifications["possible_leak"], true)
	codeErrorNotificationEnabled := boolValue(notifications["possible_code_error"], true)
	leakEnabled := boolValue(leak["enabled"], true)
	leakWarmupSeconds := strconv.FormatFloat(numberValue(leak["warmup_seconds"], 300), 'f', -1, 64)
	leakWindowSeconds := strconv.FormatFloat(numberValue(leak["window_seconds"], 300), 'f', -1, 64)
	leakMinimumGrowthMiB := strconv.FormatFloat(numberValue(leak["minimum_growth_mib"], 100), 'f', -1, 64)
	leakMinimumSlope := strconv.FormatFloat(numberValue(leak["minimum_slope_mib_per_minute"], 5), 'f', -1, 64)
	reportsEnabled := boolValue(config["reports_enabled"], true)
	viewerEnabled := boolValue(config["gui_viewer"], false)

	if err := runStep(huh.NewInput().Key("python-interpreter").Title("Python 3 interpreter").Description("This interpreter runs every target in the queue.").Value(&path)); err != nil {
		return interpreter, "", err
	}
	if err := runStep(huh.NewInput().Key("sampling-interval").Title("Resource sampling interval (seconds)").Description("How often Monitor measures CPU, RAM, and GPU usage.").Value(&samplingInterval).Validate(validateMonitorPositiveDecimal("sampling interval"))); err != nil {
		return interpreter, "", err
	}
	if err := runStep(huh.NewInput().Key("crash-retries").Title("Crash retries for this run").Description("How many restarts are allowed after a nonzero exit or signal.").Value(&crashRetries).Validate(validateMonitorNonNegative("crash retries"))); err != nil {
		return interpreter, "", err
	}
	if err := runStep(huh.NewInput().Key("base-retry-delay").Title("Initial crash-retry delay (seconds)").Description("How long Monitor waits before the first crash restart.").Value(&baseDelaySeconds).Validate(validateMonitorNonNegativeDecimal("initial crash-retry delay"))); err != nil {
		return interpreter, "", err
	}
	if err := runStep(huh.NewInput().Key("retry-backoff").Title("Crash-retry backoff multiplier").Description("The factor applied to the delay after each consecutive crash.").Value(&backoffMultiplier).Validate(validateMonitorPositiveDecimal("crash-retry backoff multiplier"))); err != nil {
		return interpreter, "", err
	}
	if err := runStep(huh.NewInput().Key("max-retry-delay").Title("Maximum crash-retry delay (seconds)").Description("The upper limit for the delay between crash restarts.").Value(&maxDelaySeconds).Validate(validateMonitorNonNegativeDecimal("maximum crash-retry delay"))); err != nil {
		return interpreter, "", err
	}
	if err := runStep(huh.NewInput().Key("rapid-crash-threshold").Title("Rapid-crash threshold for this run (seconds)").Description("A failed attempt shorter than this is identified as a possible code error.").Value(&rapidCrashSeconds).Validate(validateMonitorPositiveDecimal("rapid-crash threshold"))); err != nil {
		return interpreter, "", err
	}
	if err := runStep(huh.NewConfirm().Key("automatic-restarts").Title("Enable automatic restarts for this run?").Description("Automatic restarts can use a memory limit or elapsed time.").Value(&automaticRestartsEnabled)); err != nil {
		return interpreter, "", err
	}
	if automaticRestartsEnabled {
		if err := runStep(huh.NewSelect[string]().Key("automatic-restart-type").Title("Choose automatic restart type for this run").Description("Memory-aware uses process-tree RAM. Time scheduled uses elapsed minutes.").Options(huh.NewOption("Memory-aware", "memory"), huh.NewOption("Time scheduled", "time")).Value(&automaticRestartMode)); err != nil {
			return interpreter, "", err
		}
		if automaticRestartMode == "memory" {
			if err := runStep(huh.NewInput().Key("memory-restart-limit").Title("Memory restart limit for this run (GB)").Description("Restart when the target process tree reaches this many decimal gigabytes of RAM.").Value(&memoryLimitGB).Validate(validateMonitorPositiveDecimal("memory restart limit"))); err != nil {
				return interpreter, "", err
			}
		} else if err := runStep(huh.NewInput().Key("time-restart-interval").Title("Time restart interval for this run (minutes)").Description("Elapsed time before restarting the target.").Value(&scheduledMinutes).Validate(validateMonitorPositive("time restart interval"))); err != nil {
			return interpreter, "", err
		}
	}
	if err := runStep(huh.NewConfirm().Key("heartbeat-enabled").Title("Enable heartbeat emails for this run?").Description("Heartbeats confirm that the target is still running and include current metrics and recent output.").Value(&heartbeatEnabled)); err != nil {
		return interpreter, "", err
	}
	if heartbeatEnabled {
		if err := runStep(huh.NewInput().Key("heartbeat-interval").Title("Heartbeat interval (minutes)").Description("How much time passes between heartbeat emails.").Value(&heartbeatMinutes).Validate(validateMonitorPositive("heartbeat interval"))); err != nil {
			return interpreter, "", err
		}
	}
	notificationSteps := []struct {
		key         string
		title       string
		description string
		value       *bool
	}{
		{"notification-recovery", "Email after recovery?", "Send an email when a target succeeds after one or more crashes.", &recoveryEnabled},
		{"notification-scheduled-restart", "Email after an automatic restart?", "Send an email after a memory-aware or time-scheduled restart.", &scheduledNotificationEnabled},
		{"notification-final-failure", "Email after final failure?", "Send an email when the crash-retry budget is exhausted.", &finalFailureEnabled},
		{"notification-completion", "Email after successful completion?", "Send an email when the target exits successfully.", &completionEnabled},
		{"notification-possible-leak", "Email about a possible memory leak?", "Send an email when sustained process-tree RAM growth crosses the configured leak thresholds.", &leakNotificationEnabled},
		{"notification-possible-code-error", "Email about a possible code error?", "Send one email after rapid crashes exhaust all crash retries.", &codeErrorNotificationEnabled},
	}
	for _, step := range notificationSteps {
		if err := runStep(huh.NewConfirm().Key(step.key).Title(step.title).Description(step.description).Value(step.value)); err != nil {
			return interpreter, "", err
		}
	}
	if err := runStep(huh.NewConfirm().Key("leak-detection-enabled").Title("Enable memory-leak detection for this run?").Description("Monitor evaluates sustained target process-tree RAM growth against the configured thresholds.").Value(&leakEnabled)); err != nil {
		return interpreter, "", err
	}
	if leakEnabled {
		leakSteps := []struct {
			key         string
			title       string
			description string
			value       *string
			validator   func(string) error
		}{
			{"leak-warmup", "Memory-leak warm-up (seconds)", "How long Monitor waits before checking for sustained RAM growth.", &leakWarmupSeconds, validateMonitorPositiveDecimal("memory-leak warm-up")},
			{"leak-window", "Memory-leak analysis window (seconds)", "The span of resource samples used to evaluate RAM growth.", &leakWindowSeconds, validateMonitorPositiveDecimal("memory-leak analysis window")},
			{"leak-minimum-growth", "Minimum memory growth (MiB)", "The RAM increase that must be reached within the analysis window.", &leakMinimumGrowthMiB, validateMonitorPositiveDecimal("minimum memory growth")},
			{"leak-minimum-slope", "Minimum memory-growth rate (MiB/minute)", "The sustained RAM growth rate that must be reached within the analysis window.", &leakMinimumSlope, validateMonitorPositiveDecimal("minimum memory-growth rate")},
		}
		for _, step := range leakSteps {
			if err := runStep(huh.NewInput().Key(step.key).Title(step.title).Description(step.description).Value(step.value).Validate(step.validator)); err != nil {
				return interpreter, "", err
			}
		}
	}
	if err := runStep(huh.NewConfirm().Key("reports-enabled").Title("Write detailed reports and graphs for this run?").Description("Create metric samples, summaries, and CPU, RAM, and GPU graphs beside the run logs.").Value(&reportsEnabled)); err != nil {
		return interpreter, "", err
	}
	if err := runStep(huh.NewConfirm().Key("viewer-enabled").Title("Open a separate live log viewer for this run?").Description("When a supported graphical terminal is available, open a read-only live tail while retaining the dashboard.").Value(&viewerEnabled)); err != nil {
		return interpreter, "", err
	}

	selected, err := resolveMonitorInterpreter(path)
	if err != nil {
		return interpreter, "", err
	}
	crashRetryCount, crashErr := strconv.Atoi(crashRetries)
	samplingIntervalCount, samplingErr := strconv.ParseFloat(samplingInterval, 64)
	baseDelayCount, baseDelayErr := strconv.ParseFloat(baseDelaySeconds, 64)
	backoffCount, backoffErr := strconv.ParseFloat(backoffMultiplier, 64)
	maxDelayCount, maxDelayErr := strconv.ParseFloat(maxDelaySeconds, 64)
	rapidCrashSecondCount, rapidCrashErr := strconv.ParseFloat(rapidCrashSeconds, 64)
	if crashErr != nil || crashRetryCount < 0 || samplingErr != nil || samplingIntervalCount <= 0 || baseDelayErr != nil || baseDelayCount < 0 || backoffErr != nil || backoffCount <= 0 || maxDelayErr != nil || maxDelayCount < 0 || rapidCrashErr != nil || rapidCrashSecondCount <= 0 {
		return interpreter, "", fmt.Errorf("per-run restart values are invalid")
	}
	config["sampling_interval_seconds"] = samplingIntervalCount
	restart["crash_retries"] = crashRetryCount
	restart["base_delay_seconds"] = baseDelayCount
	restart["backoff_multiplier"] = backoffCount
	restart["max_delay_seconds"] = maxDelayCount
	restart["rapid_crash_seconds"] = rapidCrashSecondCount
	automaticValue := 0.0
	if automaticRestartMode == "memory" {
		automaticValue, err = strconv.ParseFloat(memoryLimitGB, 64)
	} else {
		var scheduledValue int
		scheduledValue, err = strconv.Atoi(scheduledMinutes)
		automaticValue = float64(scheduledValue)
	}
	if err != nil || applyMonitorRestartMode(restart, automaticRestartsEnabled, automaticRestartMode, automaticValue) != nil {
		return interpreter, "", fmt.Errorf("per-run automatic restart value is invalid")
	}
	heartbeatMinuteCount, heartbeatErr := strconv.Atoi(heartbeatMinutes)
	if heartbeatErr != nil || heartbeatMinuteCount <= 0 {
		return interpreter, "", fmt.Errorf("per-run heartbeat interval is invalid")
	}
	notifications["heartbeat"] = heartbeatEnabled
	notifications["recovery"] = recoveryEnabled
	notifications["scheduled_restart"] = scheduledNotificationEnabled
	notifications["final_failure"] = finalFailureEnabled
	notifications["completion"] = completionEnabled
	notifications["possible_leak"] = leakNotificationEnabled
	notifications["possible_code_error"] = codeErrorNotificationEnabled
	config["heartbeat_interval_minutes"] = heartbeatMinuteCount
	leakWarmupCount, warmupErr := strconv.ParseFloat(leakWarmupSeconds, 64)
	leakWindowCount, windowErr := strconv.ParseFloat(leakWindowSeconds, 64)
	leakGrowthCount, growthErr := strconv.ParseFloat(leakMinimumGrowthMiB, 64)
	leakSlopeCount, slopeErr := strconv.ParseFloat(leakMinimumSlope, 64)
	if warmupErr != nil || leakWarmupCount <= 0 || windowErr != nil || leakWindowCount <= 0 || growthErr != nil || leakGrowthCount <= 0 || slopeErr != nil || leakSlopeCount <= 0 {
		return interpreter, "", fmt.Errorf("per-run memory-leak settings are invalid")
	}
	leak["enabled"] = leakEnabled
	leak["warmup_seconds"] = leakWarmupCount
	leak["window_seconds"] = leakWindowCount
	leak["minimum_growth_mib"] = leakGrowthCount
	leak["minimum_slope_mib_per_minute"] = leakSlopeCount
	config["reports_enabled"] = reportsEnabled
	config["gui_viewer"] = viewerEnabled
	temporary, err := os.CreateTemp(filepath.Dir(configPath), ".run-config-*.json")
	if err != nil {
		return interpreter, "", err
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return interpreter, "", err
	}
	_ = os.Remove(temporaryPath)
	if err := writeMonitorJSON(temporaryPath, config); err != nil {
		return interpreter, "", err
	}
	return selected, temporaryPath, nil
}

func terminalInteractive() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
}

func configureMonitor(root string, output io.Writer) error {
	if !terminalInteractive() {
		return fmt.Errorf("Monitor configuration requires an interactive terminal")
	}
	stateRoot, _, _, err := monitorPaths()
	if err != nil {
		return err
	}
	configPath := filepath.Join(stateRoot, "config.json")
	credentialsPath := filepath.Join(stateRoot, "credentials.json")
	config := monitorDefaultConfig(nil)
	recipients := ""
	if existing, readErr := os.ReadFile(configPath); readErr == nil {
		if err := json.Unmarshal(existing, &config); err != nil {
			return fmt.Errorf("load Monitor configuration: %w", err)
		}
		if numberValue(config["schema_version"], 0) > 1 {
			return fmt.Errorf("Monitor configuration uses a newer schema version")
		}
		if addresses, valid := config["recipients"].([]any); valid {
			parts := make([]string, 0, len(addresses))
			for _, address := range addresses {
				parts = append(parts, fmt.Sprint(address))
			}
			recipients = strings.Join(parts, ", ")
		}
	}
	email := monitorEmailSettings{Provider: "gmail", Port: "587", Security: "starttls"}
	if existing, readErr := os.ReadFile(credentialsPath); readErr == nil {
		var value map[string]any
		if json.Unmarshal(existing, &value) == nil {
			email.Host = fmt.Sprint(value["host"])
			email.Port = fmt.Sprint(value["port"])
			email.Security = fmt.Sprint(value["security"])
			email.Sender = fmt.Sprint(value["sender"])
			email.Password = fmt.Sprint(value["password"])
			email.Provider = fmt.Sprint(value["provider"])
			if email.Provider != "gmail" && email.Provider != "custom" {
				email.Provider = "custom"
				if email.Host == "smtp.gmail.com" {
					email.Provider = "gmail"
				}
			}
		}
	}
	restart, restartValid := config["restart"].(map[string]any)
	notifications, notificationsValid := config["notifications"].(map[string]any)
	leak, leakValid := config["leak_detection"].(map[string]any)
	if !restartValid || !notificationsValid || !leakValid {
		return fmt.Errorf("Monitor configuration has invalid nested settings")
	}
	normalizeMonitorRestart(restart)
	crashRetries := strconv.Itoa(int(numberValue(restart["crash_retries"], 10)))
	rapidCrashSeconds := strconv.Itoa(int(numberValue(restart["rapid_crash_seconds"], 60)))
	scheduledMinutes := strconv.Itoa(int(numberValue(restart["scheduled_interval_minutes"], 0)))
	memoryLimitGB := strconv.FormatFloat(numberValue(restart["memory_limit_gb"], 1), 'f', -1, 64)
	automaticRestartsEnabled := boolValue(restart["memory_aware"], false) || numberValue(restart["scheduled_interval_minutes"], 0) > 0
	automaticRestartMode := "time"
	if boolValue(restart["memory_aware"], false) {
		automaticRestartMode = "memory"
	}
	heartbeatMinutes := strconv.Itoa(int(numberValue(config["heartbeat_interval_minutes"], 60)))
	leakWarmupMinutes := strconv.Itoa(max(1, int(numberValue(leak["warmup_seconds"], 300)/60)))
	heartbeatEnabled := boolValue(notifications["heartbeat"], false)
	recoveryEnabled := boolValue(notifications["recovery"], true)
	scheduledNotificationEnabled := boolValue(notifications["scheduled_restart"], true)
	finalFailureEnabled := boolValue(notifications["final_failure"], true)
	completionEnabled := boolValue(notifications["completion"], true)
	leakNotificationEnabled := boolValue(notifications["possible_leak"], true)
	codeErrorNotificationEnabled := boolValue(notifications["possible_code_error"], true)
	reportsEnabled := boolValue(config["reports_enabled"], true)
	viewerEnabled := boolValue(config["gui_viewer"], false)
	leakEnabled := boolValue(leak["enabled"], true)

	step := 1
	title := func(label string) string {
		value := fmt.Sprintf("Step %d · %s", step, label)
		step++
		return value
	}
	setupMode := "quick"
	if err := runMonitorConfigStep(huh.NewSelect[string]().
		Title(title("Choose setup type")).
		Description("Quick setup configures email and keeps the default monitoring settings. Full setup explains every monitoring option one at a time.").
		Options(huh.NewOption("Quick setup", "quick"), huh.NewOption("Full setup", "advanced")).
		Value(&setupMode)); err != nil {
		return err
	}
	if err := runMonitorConfigStep(huh.NewSelect[string]().
		Title(title("Choose your email provider")).
		Description("Gmail needs only your address and a Google App Password. Custom email server exposes SMTP connection settings.").
		Options(huh.NewOption("Gmail", "gmail"), huh.NewOption("Custom email server", "custom")).
		Value(&email.Provider)); err != nil {
		return err
	}

	enteredPassword := ""
	if email.Provider == "gmail" {
		if err := runMonitorConfigStep(huh.NewInput().
			Title(title("Enter your Gmail address")).
			Description("This account sends Monitor notifications. Example: you@gmail.com").
			Value(&email.Sender).
			Validate(validateMonitorEmail)); err != nil {
			return err
		}
		passwordDescription := "Use a 16-character Google App Password, not your normal Gmail password. First enable 2-Step Verification, then create one at myaccount.google.com/apppasswords. Spaces are accepted."
		if email.Password != "" {
			passwordDescription += " Leave this blank to keep your saved App Password."
		}
		if err := runMonitorConfigStep(huh.NewInput().
			Title(title("Enter your Google App Password")).
			Description(passwordDescription).
			EchoMode(huh.EchoModePassword).
			Value(&enteredPassword).
			Validate(func(value string) error {
				if strings.TrimSpace(value) == "" && email.Password == "" {
					return fmt.Errorf("an App Password is required")
				}
				return nil
			})); err != nil {
			return err
		}
		email = monitorEmailCredentials("gmail", email.Sender, enteredPassword, email)
	} else {
		if err := runMonitorConfigStep(huh.NewInput().
			Title(title("Enter the sender email address")).
			Description("Monitor signs in and sends messages as this address.").
			Value(&email.Sender).
			Validate(validateMonitorEmail)); err != nil {
			return err
		}
		if err := runMonitorConfigStep(huh.NewInput().
			Title(title("Enter the SMTP server")).
			Description("SMTP is the server used to send mail. Your email provider documents this hostname, such as smtp.example.com.").
			Value(&email.Host).
			Validate(validateMonitorRequired("SMTP server"))); err != nil {
			return err
		}
		if err := runMonitorConfigStep(huh.NewInput().
			Title(title("Enter the SMTP port")).
			Description("Enter the port documented by your email provider.").
			Value(&email.Port).
			Validate(validateMonitorPort)); err != nil {
			return err
		}
		if err := runMonitorConfigStep(huh.NewSelect[string]().
			Title(title("Choose connection security")).
			Description("Choose the transport security documented by your email provider.").
			Options(huh.NewOption("STARTTLS", "starttls"), huh.NewOption("Implicit TLS", "tls")).
			Value(&email.Security)); err != nil {
			return err
		}
		passwordDescription := "Enter the SMTP password supplied by your provider. It is stored in an owner-only file and is never shown in summaries or logs."
		if email.Password != "" {
			passwordDescription += " Leave this blank to keep the saved password."
		}
		if err := runMonitorConfigStep(huh.NewInput().
			Title(title("Enter the email password")).
			Description(passwordDescription).
			EchoMode(huh.EchoModePassword).
			Value(&enteredPassword).
			Validate(func(value string) error {
				if strings.TrimSpace(value) == "" && email.Password == "" {
					return fmt.Errorf("an email password is required")
				}
				return nil
			})); err != nil {
			return err
		}
		email = monitorEmailCredentials("custom", email.Sender, enteredPassword, email)
	}

	if recipients == "" {
		recipients = email.Sender
	}
	if err := runMonitorConfigStep(huh.NewInput().
		Title(title("Choose who receives notifications")).
		Description("Enter one or more email addresses separated by commas. The sending address is selected by default.").
		Value(&recipients).
		Validate(func(value string) error {
			_, err := parseMonitorRecipients(value)
			return err
		})); err != nil {
		return err
	}
	addresses, err := parseMonitorRecipients(recipients)
	if err != nil {
		return err
	}

	if setupMode == "quick" {
		config = monitorDefaultConfig(addresses)
		restart = config["restart"].(map[string]any)
		notifications = config["notifications"].(map[string]any)
		leak = config["leak_detection"].(map[string]any)
		crashRetries, rapidCrashSeconds, scheduledMinutes, memoryLimitGB, heartbeatMinutes, leakWarmupMinutes = "10", "60", "0", "1", "60", "5"
		automaticRestartsEnabled, automaticRestartMode = false, "time"
		heartbeatEnabled, recoveryEnabled, scheduledNotificationEnabled = false, true, true
		finalFailureEnabled, completionEnabled, leakNotificationEnabled, codeErrorNotificationEnabled = true, true, true, true
		leakEnabled, reportsEnabled, viewerEnabled = true, true, false
	} else {
		if err := runMonitorConfigStep(huh.NewInput().
			Title(title("Set crash retries")).
			Description("How many times Monitor restarts a script after it crashes. Use 0 to stop after the first crash.").
			Value(&crashRetries).
			Validate(validateMonitorNonNegative("crash retries"))); err != nil {
			return err
		}
		if err := runMonitorConfigStep(huh.NewInput().
			Title(title("Set rapid-crash threshold (seconds)")).
			Description("A failed attempt shorter than this is identified as a possible code error. The alert is sent only once per run.").
			Value(&rapidCrashSeconds).
			Validate(validateMonitorPositive("rapid-crash threshold"))); err != nil {
			return err
		}
		if err := runMonitorConfigStep(huh.NewConfirm().
			Title(title("Enable automatic restarts?")).
			Description("Automatic restarts can be triggered by a memory limit or by elapsed time. They do not consume crash retries.").
			Value(&automaticRestartsEnabled)); err != nil {
			return err
		}
		if automaticRestartsEnabled {
			if err := runMonitorConfigStep(huh.NewSelect[string]().
				Title(title("Choose automatic restart type")).
				Description("Memory-aware uses target process-tree RAM. Time scheduled uses elapsed minutes.").
				Options(huh.NewOption("Memory-aware", "memory"), huh.NewOption("Time scheduled", "time")).
				Value(&automaticRestartMode)); err != nil {
				return err
			}
			if automaticRestartMode == "memory" {
				if err := runMonitorConfigStep(huh.NewInput().
					Title(title("Set memory restart limit (GB)")).
					Description("Monitor restarts the target when total process-tree RAM reaches this decimal-gigabyte limit.").
					Value(&memoryLimitGB).
					Validate(validateMonitorPositiveDecimal("memory restart limit"))); err != nil {
					return err
				}
			} else {
				if err := runMonitorConfigStep(huh.NewInput().
					Title(title("Set time restart interval")).
					Description("Monitor restarts the target after this many minutes.").
					Value(&scheduledMinutes).
					Validate(validateMonitorPositive("time restart interval"))); err != nil {
					return err
				}
			}
		}
		if err := runMonitorConfigStep(huh.NewConfirm().
			Title(title("Enable heartbeat emails?")).
			Description("Heartbeats periodically confirm that the script is alive and include current resource usage and recent output.").
			Value(&heartbeatEnabled)); err != nil {
			return err
		}
		if heartbeatEnabled {
			if err := runMonitorConfigStep(huh.NewInput().
				Title(title("Set heartbeat interval")).
				Description("Minutes between heartbeat emails. This must be greater than zero.").
				Value(&heartbeatMinutes).
				Validate(validateMonitorPositive("heartbeat interval"))); err != nil {
				return err
			}
		}
		selectedNotifications := []string{}
		for name, enabled := range map[string]bool{
			"recovery": recoveryEnabled, "scheduled_restart": scheduledNotificationEnabled,
			"final_failure": finalFailureEnabled, "completion": completionEnabled, "possible_leak": leakNotificationEnabled,
			"possible_code_error": codeErrorNotificationEnabled,
		} {
			if enabled {
				selectedNotifications = append(selectedNotifications, name)
			}
		}
		if err := runMonitorConfigStep(huh.NewMultiSelect[string]().
			Title(title("Choose notification events")).
			Description("Space toggles an event. Enter continues. Runtime email failures never stop your script.").
			Options(
				huh.NewOption("Recovered after a crash", "recovery"),
				huh.NewOption("Scheduled restart", "scheduled_restart"),
				huh.NewOption("Final failure", "final_failure"),
				huh.NewOption("Successful completion", "completion"),
				huh.NewOption("Possible memory leak", "possible_leak"),
				huh.NewOption("Possible code error after a rapid crash", "possible_code_error"),
			).
			Value(&selectedNotifications)); err != nil {
			return err
		}
		recoveryEnabled, scheduledNotificationEnabled = false, false
		finalFailureEnabled, completionEnabled, leakNotificationEnabled, codeErrorNotificationEnabled = false, false, false, false
		for _, notification := range selectedNotifications {
			switch notification {
			case "recovery":
				recoveryEnabled = true
			case "scheduled_restart":
				scheduledNotificationEnabled = true
			case "final_failure":
				finalFailureEnabled = true
			case "completion":
				completionEnabled = true
			case "possible_leak":
				leakNotificationEnabled = true
			case "possible_code_error":
				codeErrorNotificationEnabled = true
			}
		}
		if err := runMonitorConfigStep(huh.NewConfirm().
			Title(title("Enable memory-leak detection?")).
			Description("Monitor watches sustained RAM growth after a warm-up period and can warn about a possible leak.").
			Value(&leakEnabled)); err != nil {
			return err
		}
		if leakEnabled {
			if err := runMonitorConfigStep(huh.NewInput().
				Title(title("Set memory-leak warm-up interval")).
				Description("Monitor waits this many minutes before evaluating RAM growth for a possible leak.").
				Value(&leakWarmupMinutes).
				Validate(validateMonitorPositive("memory-leak warm-up interval"))); err != nil {
				return err
			}
		}
		if err := runMonitorConfigStep(huh.NewConfirm().
			Title(title("Write detailed reports and plots?")).
			Description("Creates metric samples, summaries, and CPU/RAM/GPU graphs beside each script's run logs.").
			Value(&reportsEnabled)); err != nil {
			return err
		}
		if err := runMonitorConfigStep(huh.NewConfirm().
			Title(title("Open a separate live log viewer?")).
			Description("When a supported graphical terminal is available, Monitor opens a read-only live tail. The dashboard always remains active.").
			Value(&viewerEnabled)); err != nil {
			return err
		}
	}

	crashRetryCount, _ := strconv.Atoi(crashRetries)
	rapidCrashSecondCount, _ := strconv.Atoi(rapidCrashSeconds)
	scheduledMinuteCount, _ := strconv.Atoi(scheduledMinutes)
	memoryLimitGBCount, _ := strconv.ParseFloat(memoryLimitGB, 64)
	heartbeatMinuteCount, _ := strconv.Atoi(heartbeatMinutes)
	leakWarmupMinuteCount, _ := strconv.Atoi(leakWarmupMinutes)
	config["recipients"] = addresses
	restart["crash_retries"] = crashRetryCount
	restart["rapid_crash_seconds"] = rapidCrashSecondCount
	automaticValue := float64(scheduledMinuteCount)
	if automaticRestartMode == "memory" {
		automaticValue = memoryLimitGBCount
	}
	if err := applyMonitorRestartMode(restart, automaticRestartsEnabled, automaticRestartMode, automaticValue); err != nil {
		return err
	}
	notifications["heartbeat"] = heartbeatEnabled
	notifications["recovery"] = recoveryEnabled
	notifications["scheduled_restart"] = scheduledNotificationEnabled
	notifications["final_failure"] = finalFailureEnabled
	notifications["completion"] = completionEnabled
	notifications["possible_leak"] = leakNotificationEnabled
	notifications["possible_code_error"] = codeErrorNotificationEnabled
	config["heartbeat_interval_minutes"] = heartbeatMinuteCount
	if err := applyMonitorLeakWarmup(leak, leakEnabled, leakWarmupMinuteCount); err != nil {
		return err
	}
	config["reports_enabled"] = reportsEnabled
	config["gui_viewer"] = viewerEnabled
	portNumber, _ := strconv.Atoi(email.Port)
	credentials := map[string]any{"provider": email.Provider, "host": email.Host, "port": portNumber, "security": email.Security, "sender": email.Sender, "password": email.Password}
	confirm := true
	if err := runMonitorConfigStep(huh.NewConfirm().
		Title(title("Send a test email and save?")).
		Description(monitorConfigurationSummary(email, addresses, setupMode == "quick") + "\n\nMonitor saves these settings only if every recipient receives the test successfully.").
		Affirmative("Send test").
		Negative("Cancel").
		Value(&confirm)); err != nil {
		return err
	}
	if !confirm {
		return ErrCancelled
	}
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(stateRoot, 0o700); err != nil {
		return err
	}
	temporaryConfig := filepath.Join(stateRoot, ".config-test.json")
	temporaryCredentials := filepath.Join(stateRoot, ".credentials-test.json")
	defer os.Remove(temporaryConfig)
	defer os.Remove(temporaryCredentials)
	if err := writeMonitorJSON(temporaryConfig, config); err != nil {
		return err
	}
	if err := writeMonitorJSON(temporaryCredentials, credentials); err != nil {
		return err
	}
	fmt.Fprintln(output, "Sending a test email to every recipient…")
	if code := invokeMonitorTestEmail(temporaryConfig, temporaryCredentials, output); code != 0 {
		return fmt.Errorf("test email failed; configuration was not saved")
	}
	if err := writeMonitorConfigPair(configPath, config, credentialsPath, credentials); err != nil {
		return err
	}
	fmt.Fprintln(output, "Monitor configuration saved after successful test delivery.")
	return nil
}

func runMonitorConfigStep(field huh.Field) error {
	if err := huh.NewForm(huh.NewGroup(field)).Run(); err != nil {
		return ErrCancelled
	}
	return nil
}

func validateMonitorEmail(value string) error {
	value = strings.TrimSpace(value)
	if !strings.Contains(value, "@") || strings.ContainsAny(value, " \t\r\n") {
		return fmt.Errorf("enter a valid email address")
	}
	return nil
}

func validateMonitorRequired(label string) func(string) error {
	return func(value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", label)
		}
		return nil
	}
}

func validateMonitorPort(value string) error {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("enter a port between 1 and 65535")
	}
	return nil
}

func validateMonitorNonNegative(label string) func(string) error {
	return func(value string) error {
		number, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || number < 0 {
			return fmt.Errorf("%s must be zero or greater", label)
		}
		return nil
	}
}

func validateMonitorPositive(label string) func(string) error {
	return func(value string) error {
		number, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || number <= 0 {
			return fmt.Errorf("%s must be greater than zero", label)
		}
		return nil
	}
}

func validateMonitorPositiveDecimal(label string) func(string) error {
	return func(value string) error {
		number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || math.IsInf(number, 0) || math.IsNaN(number) || number <= 0 {
			return fmt.Errorf("%s must be a positive number", label)
		}
		return nil
	}
}

func validateMonitorNonNegativeDecimal(label string) func(string) error {
	return func(value string) error {
		number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || math.IsInf(number, 0) || math.IsNaN(number) || number < 0 {
			return fmt.Errorf("%s must be zero or greater", label)
		}
		return nil
	}
}

func parseMonitorRecipients(value string) ([]string, error) {
	addresses := []string{}
	for _, address := range strings.Split(value, ",") {
		address = strings.TrimSpace(address)
		if err := validateMonitorEmail(address); err != nil {
			return nil, fmt.Errorf("invalid recipient email address: %s", address)
		}
		addresses = append(addresses, address)
	}
	return addresses, nil
}

func numberValue(value any, fallback float64) float64 {
	if number, valid := value.(float64); valid {
		return number
	}
	if number, valid := value.(int); valid {
		return float64(number)
	}
	return fallback
}

func boolValue(value any, fallback bool) bool {
	if boolean, valid := value.(bool); valid {
		return boolean
	}
	return fallback
}

func monitorDefaultConfig(recipients []string) map[string]any {
	return map[string]any{
		"schema_version": 1, "recipients": recipients, "sampling_interval_seconds": 1,
		"heartbeat_interval_minutes": 60, "reports_enabled": true, "gui_viewer": false,
		"notifications":  map[string]any{"heartbeat": false, "recovery": true, "scheduled_restart": true, "final_failure": true, "completion": true, "possible_leak": true, "possible_code_error": true},
		"restart":        map[string]any{"crash_retries": 10, "base_delay_seconds": 3, "backoff_multiplier": 1.2, "max_delay_seconds": 30, "rapid_crash_seconds": 60, "scheduled_interval_minutes": 0, "memory_aware": false, "memory_limit_gb": 1.0},
		"leak_detection": map[string]any{"enabled": true, "warmup_seconds": 300, "window_seconds": 300, "minimum_growth_mib": 100, "minimum_slope_mib_per_minute": 5},
	}
}

func writeMonitorJSON(path string, value any) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".monitor-json-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func writeMonitorConfigPair(configPath string, config any, credentialsPath string, credentials any) error {
	configStage, err := reserveMonitorPath(filepath.Dir(configPath), ".config-stage-")
	if err != nil {
		return err
	}
	defer os.Remove(configStage)
	credentialsStage, err := reserveMonitorPath(filepath.Dir(credentialsPath), ".credentials-stage-")
	if err != nil {
		return err
	}
	defer os.Remove(credentialsStage)
	if err := writeMonitorJSON(configStage, config); err != nil {
		return err
	}
	if err := writeMonitorJSON(credentialsStage, credentials); err != nil {
		return err
	}
	configBackup, err := reserveMonitorPath(filepath.Dir(configPath), ".config-backup-")
	if err != nil {
		return err
	}
	defer os.Remove(configBackup)
	credentialsBackup, err := reserveMonitorPath(filepath.Dir(credentialsPath), ".credentials-backup-")
	if err != nil {
		return err
	}
	defer os.Remove(credentialsBackup)
	configExisted := false
	if err := os.Rename(configPath, configBackup); err == nil {
		configExisted = true
	} else if !os.IsNotExist(err) {
		return err
	}
	credentialsExisted := false
	if err := os.Rename(credentialsPath, credentialsBackup); err == nil {
		credentialsExisted = true
	} else if !os.IsNotExist(err) {
		if configExisted {
			_ = os.Rename(configBackup, configPath)
		}
		return err
	}
	rollback := func() {
		_ = os.Remove(configPath)
		_ = os.Remove(credentialsPath)
		if configExisted {
			_ = os.Rename(configBackup, configPath)
		}
		if credentialsExisted {
			_ = os.Rename(credentialsBackup, credentialsPath)
		}
	}
	if err := os.Rename(configStage, configPath); err != nil {
		rollback()
		return err
	}
	if err := os.Rename(credentialsStage, credentialsPath); err != nil {
		rollback()
		return err
	}
	return nil
}

func reserveMonitorPath(directory, pattern string) (string, error) {
	file, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}

func invokeMonitorRuntime(targets, titles []string, interpreter monitorInterpreter, configPath string, output, errorOutput io.Writer) int {
	content, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Fprintln(errorOutput, "load Monitor run configuration:", err)
		return 1
	}
	var config map[string]any
	if err := json.Unmarshal(content, &config); err != nil {
		fmt.Fprintln(errorOutput, "load Monitor run configuration:", err)
		return 1
	}
	request := map[string]any{"protocol_version": 1, "type": "run", "scripts": targets, "titles": titles, "interpreter": interpreter.Path, "config_path": configPath}
	dashboard := newMonitorDashboard(targets, interpreter, config)
	dashboard.Titles = append([]string(nil), titles...)
	return runMonitorProtocol(request, dashboard, output, errorOutput)
}

func invokeMonitorTestEmail(configPath, credentialsPath string, output io.Writer) int {
	request := map[string]any{"protocol_version": 1, "type": "test_email", "config_path": configPath, "credentials_path": credentialsPath}
	return runMonitorProtocol(request, newMonitorDashboard(nil, monitorInterpreter{}), output, output)
}

func runMonitorProtocol(request map[string]any, dashboard *monitorDashboard, output, errorOutput io.Writer) int {
	_, runtimeRoot, _, err := monitorPaths()
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	python := filepath.Join(runtimeRoot, "venv", "bin", "python")
	command := exec.Command(python, "-m", "monitor_runtime")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Env = append(os.Environ(), "PYTHONPATH="+filepath.Join(runtimeRoot, "app"))
	stdin, err := command.StdinPipe()
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	command.Stderr = errorOutput
	if err := command.Start(); err != nil {
		fmt.Fprintln(errorOutput, "Monitor runtime is unavailable; run 'tb install-monitor' to repair it:", err)
		return 1
	}
	interrupts := make(chan os.Signal, 1)
	cancelRequests := make(chan struct{}, 1)
	dashboard.Cancel = cancelRequests
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-interrupts:
			_ = syscall.Kill(command.Process.Pid, syscall.SIGINT)
		case <-cancelRequests:
			_ = syscall.Kill(command.Process.Pid, syscall.SIGINT)
		case <-done:
		}
	}()
	_ = json.NewEncoder(stdin).Encode(request)
	_ = stdin.Close()
	var program *tea.Program
	var dashboardDone chan error
	if terminalInteractive() && request["type"] == "run" {
		program = tea.NewProgram(dashboard, tea.WithOutput(output), tea.WithFPS(20), tea.WithoutSignalHandler())
		dashboardDone = make(chan error, 1)
		go func() {
			_, runErr := program.Run()
			dashboardDone <- runErr
		}()
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	eventIndex := 0
	for scanner.Scan() {
		var event monitorEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			fmt.Fprintln(errorOutput, "invalid Monitor runtime event:", err)
			_ = command.Process.Kill()
			_ = command.Wait()
			return 1
		}
		if event.ProtocolVersion != 1 || !validMonitorEventType(event.Type) || (eventIndex == 0 && event.Type != "handshake") {
			fmt.Fprintln(errorOutput, "invalid Monitor runtime protocol event")
			_ = command.Process.Kill()
			_ = command.Wait()
			return 1
		}
		eventIndex++
		if program != nil {
			program.Send(event)
		} else {
			dashboard.apply(event)
			dashboard.render(output)
		}
	}
	if program != nil {
		program.Quit()
		if err := <-dashboardDone; err != nil {
			fmt.Fprintln(errorOutput, err)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(errorOutput, err)
		_ = command.Process.Kill()
		_ = command.Wait()
		return 1
	}
	if err := command.Wait(); err != nil {
		if request["type"] == "run" {
			fmt.Fprintln(output, dashboard.finalSummary())
		}
		if exit, valid := err.(*exec.ExitError); valid {
			return exit.ExitCode()
		}
		return 1
	}
	if request["type"] == "run" {
		fmt.Fprintln(output, dashboard.finalSummary())
	}
	return 0
}

func validMonitorEventType(eventType string) bool {
	switch eventType {
	case "handshake", "run_created", "attempt_started", "attempt_ended", "target_output", "resource_sample", "lifecycle", "restart_decision", "email_result", "final_outcome":
		return true
	default:
		return false
	}
}
