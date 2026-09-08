//go:build !windows

package main

import (
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type monitorEvent struct {
	ProtocolVersion     int                `json:"protocol_version"`
	Type                string             `json:"type"`
	Title               string             `json:"title"`
	Script              string             `json:"script"`
	State               string             `json:"state"`
	Message             string             `json:"message"`
	Text                string             `json:"text"`
	Stream              string             `json:"stream"`
	Reason              string             `json:"reason"`
	Kind                string             `json:"kind"`
	RunDirectory        string             `json:"run_directory"`
	InterpreterPath     string             `json:"interpreter"`
	PID                 int                `json:"pid"`
	Attempt             int                `json:"attempt"`
	CrashCount          int                `json:"crash_count"`
	ScheduledCount      int                `json:"scheduled_count"`
	MemoryCount         int                `json:"memory_count"`
	QueueIndex          int                `json:"queue_index"`
	QueueTotal          int                `json:"queue_total"`
	CPUPercent          float64            `json:"cpu_percent"`
	RAMBytes            int64              `json:"ram_bytes"`
	RAMMiB              float64            `json:"ram_mib"`
	SystemRAMTotalBytes int64              `json:"system_ram_total_bytes"`
	GPUPercent          any                `json:"gpu_percent"`
	GPUMemoryMiB        any                `json:"gpu_memory_mib"`
	GPUMemoryTotalMiB   any                `json:"gpu_memory_total_mib"`
	GPUDevices          []monitorGPUDevice `json:"gpu_devices"`
	GPUScope            string             `json:"gpu_scope"`
	MemoryLimitGB       float64            `json:"memory_limit_gb"`
	Elapsed             float64            `json:"elapsed_seconds"`
	Outcome             string             `json:"outcome"`
	Error               string             `json:"error"`
	Success             bool               `json:"success"`
	DelaySeconds        float64            `json:"delay_seconds"`
	AttemptDuration     float64            `json:"attempt_duration_seconds"`
	ExitCode            int                `json:"exit_code"`
	Restart             bool               `json:"restart"`
	Scheduled           bool               `json:"scheduled"`
	MemoryRestart       bool               `json:"memory_restart"`
}

type monitorGPUDevice struct {
	Index              int    `json:"index"`
	UUID               string `json:"uuid"`
	Name               string `json:"name"`
	UtilizationPercent any    `json:"utilization_percent"`
	TargetMemoryMiB    any    `json:"target_memory_mib"`
	SystemMemoryMiB    any    `json:"system_memory_mib"`
	TotalMemoryMiB     any    `json:"total_memory_mib"`
	TargetActive       bool   `json:"target_active"`
}

type monitorDashboardSettings struct {
	CrashRetries       int
	BaseDelaySeconds   float64
	BackoffMultiplier  float64
	MaxDelaySeconds    float64
	RapidCrashSeconds  int
	AutomaticMode      string
	ScheduledMinutes   int
	MemoryLimitGB      float64
	SampleSeconds      float64
	LeakEnabled        bool
	LeakWarmupSeconds  int
	ReportsEnabled     bool
	ViewerEnabled      bool
	HeartbeatEnabled   bool
	HeartbeatMinutes   int
	NotificationStates []monitorNotificationState
}

type monitorNotificationState struct {
	Label   string
	Enabled bool
}

type monitorPetTick struct{ animation int }
type monitorViewerResult struct{ Error error }

const monitorPetFrameInterval = 500 * time.Millisecond

const monitorActivityEntries = 8
const monitorLogEntries = monitorActivityEntries * 2

// Frame ranges for the independently selected eye, tail, and ear animations.
var monitorCatFrames = []string{
	// Eyes: 1 → 2 → 3 → 2.
	"      ／＞　 フ\n      | 　_　_|\n    ／` ミ＿xノ\n   /　　　　 |\n  /　 ヽ　　 ﾉ\n  │　　|　|　|\n／￣|　　 |　|\n(￣ヽ＿_ヽ_)__)\n＼二)",
	"      ／＞　 フ\n      | 　o　o|\n    ／` ミ＿xノ\n   /　　　　 |\n  /　 ヽ　　 ﾉ\n  │　　|　|　|\n／￣|　　 |　|\n(￣ヽ＿_ヽ_)__)\n＼二)",
	"      ／＞　 フ\n      | 　_　_|\n    ／` ミ＿xノ\n   /　　　　 |\n  /　 ヽ　　 ﾉ\n  │　　|　|　|\n／￣|　　 |　|\n(￣ヽ＿_ヽ_)__)\n＼二)",
	"      ／＞　 フ\n      | 　o　o|\n    ／` ミ＿xノ\n   /　　　　 |\n  /　 ヽ　　 ﾉ\n  │　　|　|　|\n／￣|　　 |　|\n(￣ヽ＿_ヽ_)__)\n＼二)",
	// Tail: 1 → 2 → 3 → 2.
	"      ／＞　 フ\n      | 　_　_|\n    ／` ミ＿xノ\n   /　　　　 |\n  /　 ヽ　　 ﾉ\n  │　　|　|　|\n／￣|　　 |　|\n(￣ヽ＿_ヽ_)__)\n＼二)",
	"      ／＞　 フ\n      | 　_　_|\n    ／` ミ＿xノ\n   /　　　　 |\n  /　 ヽ　　 ﾉ\n  │　　|　|　|\n／￣|　　 |　|\n(￣ヽ＿_ヽ_)__)\n ＼二＿)",
	"      ／＞　 フ\n      | 　_　_|\n    ／` ミ＿xノ\n   /　　　　 |\n  /　 ヽ　　 ﾉ\n  │　　|　|　|\n／￣|　　 |　|\n(￣ヽ＿_ヽ_)__)\n  ＼＿＿二)",
	"      ／＞　 フ\n      | 　_　_|\n    ／` ミ＿xノ\n   /　　　　 |\n  /　 ヽ　　 ﾉ\n  │　　|　|　|\n／￣|　　 |　|\n(￣ヽ＿_ヽ_)__)\n ＼二＿)",
	// Ears: 1 → 2 → 3 → 2 → 1.
	"      ／＞　 フ\n      | 　_　_|\n    ／` ミ＿xノ\n   /　　　　 |\n  /　 ヽ　　 ﾉ\n  │　　|　|　|\n／￣|　　 |　|\n(￣ヽ＿_ヽ_)__)\n＼二)",
	"      ／〉　 /〉\n      | 　_　_|\n    ／` ミ＿xノ\n   /　　　　 |\n  /　 ヽ　　 ﾉ\n  │　　|　|　|\n／￣|　　 |　|\n(￣ヽ＿_ヽ_)__)\n＼二)",
	"      /∧　 /∧\n      | 　_　_|\n    ／` ミ＿xノ\n   /　　　　 |\n  /　 ヽ　　 ﾉ\n  │　　|　|　|\n／￣|　　 |　|\n(￣ヽ＿_ヽ_)__)\n＼二)",
	"      ／〉　 /〉\n      | 　_　_|\n    ／` ミ＿xノ\n   /　　　　 |\n  /　 ヽ　　 ﾉ\n  │　　|　|　|\n／￣|　　 |　|\n(￣ヽ＿_ヽ_)__)\n＼二)",
	"      ／＞　 フ\n      | 　_　_|\n    ／` ミ＿xノ\n   /　　　　 |\n  /　 ヽ　　 ﾉ\n  │　　|　|　|\n／￣|　　 |　|\n(￣ヽ＿_ヽ_)__)\n＼二)",
}

type monitorDashboard struct {
	Scripts       []string
	Titles        []string
	Interpreter   monitorInterpreter
	Settings      monitorDashboardSettings
	Output        []string
	Activity      []string
	ActivityLevel []monitorMessageSeverity
	MonitorLog    []string
	Current       monitorEvent
	Width         int
	Height        int
	Stopping      bool
	CodeError     bool
	CodeErrorText string
	Cancel        chan<- struct{}
	OpenViewer    func(string) error
	cancelSent    bool
	lastHeartbeat float64
	emailKind     string
	emailSuccess  bool
	emailSeen     bool
	petFrame      int
	petEnd        int
	viewport      viewport.Model
}

func newMonitorDashboard(scripts []string, interpreter monitorInterpreter, configs ...map[string]any) *monitorDashboard {
	config := monitorDefaultConfig(nil)
	if len(configs) > 0 && configs[0] != nil {
		config = configs[0]
	}
	dashboard := &monitorDashboard{
		Scripts: append([]string(nil), scripts...), Interpreter: interpreter,
		Settings: monitorDashboardSettingsFromConfig(config), Width: 80, Height: 24,
		OpenViewer: openMonitorLogViewer,
		viewport:   viewport.New(),
	}
	dashboard.resize(80, 24)
	return dashboard
}

func monitorDashboardSettingsFromConfig(config map[string]any) monitorDashboardSettings {
	restart, _ := config["restart"].(map[string]any)
	normalizeMonitorRestart(restart)
	leak, _ := config["leak_detection"].(map[string]any)
	notifications, _ := config["notifications"].(map[string]any)
	mode := "Disabled"
	if boolValue(restart["memory_aware"], false) {
		mode = "Memory-aware"
	} else if numberValue(restart["scheduled_interval_minutes"], 0) > 0 {
		mode = "Time scheduled"
	}
	notificationStates := []monitorNotificationState{
		{Label: "Completion email", Enabled: boolValue(notifications["completion"], false)},
		{Label: "Final failure email", Enabled: boolValue(notifications["final_failure"], false)},
		{Label: "Possible leak email", Enabled: boolValue(notifications["possible_leak"], false)},
		{Label: "Crash/restart email", Enabled: boolValue(notifications["runtime_crash"], true)},
		{Label: "Code error email", Enabled: boolValue(notifications["possible_code_error"], true)},
		{Label: "Scheduled restart", Enabled: boolValue(notifications["scheduled_restart"], false)},
		{Label: "Heartbeat email", Enabled: boolValue(notifications["heartbeat"], false)},
	}
	return monitorDashboardSettings{
		CrashRetries: int(numberValue(restart["crash_retries"], 10)), BaseDelaySeconds: numberValue(restart["base_delay_seconds"], 3),
		BackoffMultiplier: numberValue(restart["backoff_multiplier"], 1.2), MaxDelaySeconds: numberValue(restart["max_delay_seconds"], 30),
		RapidCrashSeconds: int(numberValue(restart["rapid_crash_seconds"], 60)),
		AutomaticMode:     mode, ScheduledMinutes: int(numberValue(restart["scheduled_interval_minutes"], 0)), MemoryLimitGB: numberValue(restart["memory_limit_gb"], 1),
		SampleSeconds: numberValue(config["sampling_interval_seconds"], 1), LeakEnabled: boolValue(leak["enabled"], true),
		LeakWarmupSeconds: int(numberValue(leak["warmup_seconds"], 300)), ReportsEnabled: boolValue(config["reports_enabled"], true),
		ViewerEnabled: boolValue(config["gui_viewer"], false), HeartbeatEnabled: boolValue(notifications["heartbeat"], false),
		HeartbeatMinutes: int(numberValue(config["heartbeat_interval_minutes"], 60)), NotificationStates: notificationStates,
	}
}

func (dashboard *monitorDashboard) Init() tea.Cmd {
	return monitorPetTickCommand(dashboard.petTickInterval())
}

func (dashboard *monitorDashboard) petTickInterval() time.Duration {
	if dashboard.petEnd == 0 {
		return 2 * time.Second
	}
	return monitorPetFrameInterval + time.Duration(rand.Int64N(int64(2*time.Second-monitorPetFrameInterval)+1))
}

func monitorPetTickCommand(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return monitorPetTick{animation: rand.IntN(3)}
	})
}

func (dashboard *monitorDashboard) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		dashboard.resize(message.Width, message.Height)
	case tea.KeyPressMsg:
		if message.Code == 'v' && dashboard.Settings.ViewerEnabled {
			if dashboard.Current.RunDirectory == "" {
				dashboard.appendMonitorLog("viewer unavailable • waiting for run directory")
				return dashboard, nil
			}
			path := filepath.Join(dashboard.Current.RunDirectory, "output.log")
			opener := dashboard.OpenViewer
			return dashboard, func() tea.Msg {
				return monitorViewerResult{Error: opener(path)}
			}
		}
		if message.Code == 'c' && message.Mod.Contains(tea.ModCtrl) && !dashboard.cancelSent {
			dashboard.cancelSent = true
			dashboard.Stopping = true
			if dashboard.Cancel != nil {
				select {
				case dashboard.Cancel <- struct{}{}:
				default:
				}
			}
			return dashboard, nil
		}
	case monitorPetTick:
		if dashboard.petEnd == 0 {
			bounds := [3][2]int{{0, 4}, {4, 8}, {8, 13}}[message.animation]
			dashboard.petFrame, dashboard.petEnd = bounds[0], bounds[1]
		} else {
			dashboard.petFrame++
			if dashboard.petFrame == dashboard.petEnd {
				dashboard.petFrame, dashboard.petEnd = 0, 0
			}
		}
		dashboard.rebuildViewport()
		return dashboard, monitorPetTickCommand(dashboard.petTickInterval())
	case monitorEvent:
		dashboard.apply(message)
		if message.Type == "final_outcome" && (message.Outcome != "success" || message.QueueIndex >= message.QueueTotal) {
			return dashboard, tea.Quit
		}
	case monitorViewerResult:
		if message.Error != nil {
			dashboard.appendMonitorLog("viewer failed • " + message.Error.Error())
		} else {
			dashboard.appendMonitorLog("viewer opened • press V to reopen it after closing")
		}
	}
	var command tea.Cmd
	dashboard.viewport, command = dashboard.viewport.Update(message)
	return dashboard, command
}

func (dashboard *monitorDashboard) View() tea.View {
	if dashboard.viewport.Width() != max(36, dashboard.Width) || dashboard.viewport.Height() == 0 {
		dashboard.resize(dashboard.Width, dashboard.Height)
	}
	width := max(36, dashboard.Width)
	inner := width - 2
	status := dashboard.status()
	title := fmt.Sprintf(" MONITOR  %s  •  Queue %d/%d  •  %s ", status, dashboard.Current.QueueIndex, max(dashboard.Current.QueueTotal, len(dashboard.Scripts)), formatMonitorDuration(dashboard.Current.Elapsed))
	headerText := wrapMonitorText(title, inner-2)
	headerColor := "86"
	if severity := dashboard.currentSeverity(); severity != monitorNeutral {
		headerColor = monitorSeverityColor(severity)
	}
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(headerColor))
	header := titleStyle.Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Width(inner).Render(headerText)
	footer := "Ctrl+C stop target and quit"
	if dashboard.Stopping {
		footer = "Stopping target and descendants… waiting for cleanup"
		if dashboard.Current.Message != "" && strings.HasPrefix(dashboard.Current.State, "cleanup_") {
			footer = dashboard.Current.Message
		}
	} else {
		viewerHelp := ""
		if dashboard.Settings.ViewerEnabled {
			viewerHelp = " • V reopen viewer"
		}
		footer = fmt.Sprintf("↑/↓ PgUp/PgDn Home/End scroll • %d%%%s • Ctrl+C stop target and quit", int(dashboard.viewport.ScrollPercent()*100), viewerHelp)
	}
	footerColor := "244"
	if dashboard.Stopping {
		footerColor = monitorSeverityColor(monitorWarning)
	}
	footer = lipgloss.NewStyle().Foreground(lipgloss.Color(footerColor)).PaddingLeft(1).Render(wrapMonitorText(footer, width-1))
	if lipgloss.Height(header)+lipgloss.Height(footer) >= dashboard.Height {
		header = titleStyle.Width(width).Render(wrapMonitorText(title, width))
	}
	viewportHeight := max(0, dashboard.Height-lipgloss.Height(header)-lipgloss.Height(footer))
	offset := dashboard.viewport.YOffset()
	dashboard.viewport.SetHeight(viewportHeight)
	dashboard.viewport.SetYOffset(offset)
	sections := []string{header}
	if viewportHeight > 0 {
		sections = append(sections, dashboard.viewport.View())
	}
	sections = append(sections, footer)
	view := tea.NewView(strings.Join(sections, "\n"))
	view.AltScreen = true
	return view
}

func (dashboard *monitorDashboard) resize(width, height int) {
	dashboard.Width = max(36, width)
	dashboard.Height = max(6, height)
	headerHeight := 3
	footerHeight := 1
	dashboard.viewport.SetWidth(dashboard.Width)
	dashboard.viewport.SetHeight(max(1, dashboard.Height-headerHeight-footerHeight))
	dashboard.rebuildViewport()
}

func (dashboard *monitorDashboard) rebuildViewport() {
	offset := dashboard.viewport.YOffset()
	dashboard.viewport.SetContent(dashboard.viewportContent())
	dashboard.viewport.SetYOffset(offset)
}

func (dashboard *monitorDashboard) viewportContent() string {
	inner := max(34, dashboard.Width-2)
	sections := []string{}
	if dashboard.CodeError {
		sections = append(sections, dashboard.codeErrorBanner(inner))
	}
	if dashboard.Width >= 72 {
		leftWidth := (inner - 1) / 2
		rightWidth := inner - leftWidth - 1
		sections = append(sections,
			dashboard.panelPair("TARGET", dashboard.targetPanel(), "RESOURCES", dashboard.resourcePanel(), leftWidth, rightWidth),
			dashboard.panelPair("RESTART POLICY", dashboard.restartPanel(max(8, leftWidth-4)), "MONITORING", dashboard.monitoringPanel(), leftWidth, rightWidth),
		)
	} else {
		sections = append(sections,
			dashboard.panel("TARGET", dashboard.targetPanel(), inner),
			dashboard.panel("RESOURCES", dashboard.resourcePanel(), inner),
			dashboard.panel("RESTART POLICY", dashboard.restartPanel(max(8, inner-4)), inner),
			dashboard.panel("MONITORING", dashboard.monitoringPanel(), inner),
		)
	}
	sections = append(sections,
		dashboard.panel("RECENT ACTIVITY", dashboard.activityPanel(), inner),
		dashboard.panel("MONITOR LOG", dashboard.monitorLogPanel(), inner),
		dashboard.panel("LATEST OUTPUT", dashboard.outputPanel(), inner),
	)
	return strings.Join(sections, "\n")
}

func (dashboard *monitorDashboard) compactConfiguration(width int) string {
	leak := "off"
	if dashboard.Settings.LeakEnabled {
		leak = fmt.Sprintf("%dm", dashboard.Settings.LeakWarmupSeconds/60)
	}
	heartbeat := "off"
	if dashboard.Settings.HeartbeatEnabled {
		heartbeat = fmt.Sprintf("%dm", dashboard.Settings.HeartbeatMinutes)
	}
	line := fmt.Sprintf("CONFIG  restart %s • retries %d • leak %s • heartbeat %s • sample %.1fs",
		dashboard.Settings.AutomaticMode, dashboard.Settings.CrashRetries, leak, heartbeat, dashboard.Settings.SampleSeconds)
	return lipgloss.NewStyle().Foreground(lipgloss.Color("250")).BorderTop(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240")).Width(width).Render(wrapMonitorText(line, width))
}

func (dashboard *monitorDashboard) status() string {
	if dashboard.Stopping {
		return "STOPPING"
	}
	if dashboard.Current.Outcome != "" {
		return strings.ToUpper(dashboard.Current.Outcome)
	}
	if dashboard.Current.State != "" {
		return strings.ToUpper(strings.ReplaceAll(dashboard.Current.State, "_", " "))
	}
	return "STARTING"
}

func (dashboard *monitorDashboard) panel(title, body string, width int) string {
	content := dashboard.panelContent(title, body, width)
	return renderMonitorPanel(content, width, 0)
}

func (dashboard *monitorDashboard) panelPair(leftTitle, leftBody, rightTitle, rightBody string, leftWidth, rightWidth int) string {
	leftContent := dashboard.panelContent(leftTitle, leftBody, leftWidth)
	rightContent := dashboard.panelContent(rightTitle, rightBody, rightWidth)
	contentHeight := max(lipgloss.Height(leftContent), lipgloss.Height(rightContent))
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		renderMonitorPanel(leftContent, leftWidth, contentHeight),
		" ",
		renderMonitorPanel(rightContent, rightWidth, contentHeight),
	)
}

func (dashboard *monitorDashboard) panelContent(title, body string, width int) string {
	heading := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75")).Render(title)
	if title == "RECENT ACTIVITY" {
		lines := strings.Split(body, "\n")
		start := max(0, len(dashboard.ActivityLevel)-len(lines))
		for index := range lines {
			levelIndex := start + index
			if levelIndex < len(dashboard.ActivityLevel) {
				lines[index] = colorMonitorMessage(lines[index], dashboard.ActivityLevel[levelIndex])
			}
		}
		body = strings.Join(lines, "\n")
	} else if title == "MONITORING" {
		body = colorMonitorToggleValues(body)
	} else if title == "RESOURCES" {
		body = colorMonitorResourceBody(body)
	}
	body = wrapMonitorBlock(body, max(8, width-4))
	return heading + "\n" + body
}

func renderMonitorPanel(content string, width, height int) string {
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1).Width(max(10, width))
	if height > 0 {
		style = style.Height(height + style.GetVerticalBorderSize())
	}
	return style.Render(content)
}

func (dashboard *monitorDashboard) targetPanel() string {
	script := dashboard.Current.Script
	if script == "" && len(dashboard.Scripts) > 0 {
		script = dashboard.Scripts[0]
	}
	return fmt.Sprintf("Title        %s\nScript       %s\nWorking dir  %s\nEnvironment  %s\nInterpreter  %s\nPID          %s\nAttempt      %d\nRun logs     %s",
		emptyMonitorValue(dashboard.Current.Title), filepath.Base(script), filepath.Dir(script), dashboard.Interpreter.Environment, dashboard.Interpreter.Path,
		formatMonitorPID(dashboard.Current.PID), dashboard.Current.Attempt, emptyMonitorValue(dashboard.Current.RunDirectory))
}

func (dashboard *monitorDashboard) resourcePanel() string {
	ramBytes := dashboard.Current.RAMBytes
	if ramBytes == 0 && dashboard.Current.RAMMiB > 0 {
		ramBytes = int64(dashboard.Current.RAMMiB * 1024 * 1024)
	}
	ramPercent := 0.0
	if dashboard.Current.SystemRAMTotalBytes > 0 {
		ramPercent = float64(ramBytes) / float64(dashboard.Current.SystemRAMTotalBytes) * 100
	}
	ramMemory := formatMonitorMemoryPair(float64(ramBytes)/1000000000, ramBytes > 0, float64(dashboard.Current.SystemRAMTotalBytes)/1000000000, dashboard.Current.SystemRAMTotalBytes > 0)
	base := fmt.Sprintf("CPU\n  %s %5.1f%%\n\nRAM\n  %s %s",
		monitorMeter(dashboard.Current.CPUPercent), dashboard.Current.CPUPercent,
		monitorMeter(ramPercent), ramMemory)
	if len(dashboard.Current.GPUDevices) > 0 {
		devices := make([]string, 0, len(dashboard.Current.GPUDevices))
		for _, device := range dashboard.Current.GPUDevices {
			status := "IDLE"
			memoryLabel := "System VRAM"
			usedMiB := anyMonitorFloat(device.SystemMemoryMiB)
			usedAvailable := device.SystemMemoryMiB != nil
			if device.TargetActive {
				status = "TARGET"
				memoryLabel = "Target VRAM"
				usedMiB = anyMonitorFloat(device.TargetMemoryMiB)
				usedAvailable = device.TargetMemoryMiB != nil
			}
			usedGB := usedMiB * 1048576 / 1000000000
			totalGB := anyMonitorFloat(device.TotalMemoryMiB) * 1048576 / 1000000000
			memoryPercent := 0.0
			if totalGB > 0 {
				memoryPercent = usedGB / totalGB * 100
			}
			name := strings.TrimSpace(strings.TrimPrefix(device.Name, "NVIDIA GeForce "))
			devices = append(devices, fmt.Sprintf("GPU %d · %s · %s\n  Utilization\n  %s %s\n  %s\n  %s %s",
				device.Index, name, status, monitorMeter(anyMonitorFloat(device.UtilizationPercent)), formatMonitorOptional(device.UtilizationPercent, "%"),
				memoryLabel, monitorMeter(memoryPercent), formatMonitorMemoryPair(usedGB, usedAvailable, totalGB, device.TotalMemoryMiB != nil)))
		}
		return base + "\n\n" + strings.Join(devices, "\n\n")
	}
	gpuScope := dashboard.Current.GPUScope
	if gpuScope == "" {
		gpuScope = "target"
	}
	gpuMemoryUsed := anyMonitorFloat(dashboard.Current.GPUMemoryMiB) * 1048576 / 1000000000
	gpuMemoryTotal := anyMonitorFloat(dashboard.Current.GPUMemoryTotalMiB) * 1048576 / 1000000000
	gpuMemory := formatMonitorMemoryPair(gpuMemoryUsed, dashboard.Current.GPUMemoryMiB != nil, gpuMemoryTotal, dashboard.Current.GPUMemoryTotalMiB != nil)
	gpuMemoryPercent := 0.0
	if gpuMemoryTotal > 0 {
		gpuMemoryPercent = gpuMemoryUsed / gpuMemoryTotal * 100
	}
	return fmt.Sprintf("%s\n\nGPU · %s\n  Utilization\n  %s %s\n  Memory\n  %s %s",
		base,
		strings.ToUpper(gpuScope), monitorMeter(anyMonitorFloat(dashboard.Current.GPUPercent)),
		formatMonitorOptional(dashboard.Current.GPUPercent, "%"), monitorMeter(gpuMemoryPercent), gpuMemory)
}

func (dashboard *monitorDashboard) codeErrorBanner(width int) string {
	red := lipgloss.Color(monitorSeverityColor(monitorFailure))
	contentWidth := max(12, width-4)
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(red).Padding(0, 2).Render("POSSIBLE CODE ERROR")
	subtitle := lipgloss.NewStyle().Bold(true).Foreground(red).Render("RAPID CRASH DETECTED")
	detail := lipgloss.NewStyle().Foreground(red).Width(contentWidth).Align(lipgloss.Center).Render(dashboard.CodeErrorText)
	status := lipgloss.NewStyle().Bold(true).Foreground(red).Render("RETRYING · EMAIL HELD UNTIL CRASH RETRIES ARE EXHAUSTED")
	body := lipgloss.JoinVertical(lipgloss.Center, title, subtitle, "", detail, status)
	return lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(red).Padding(0, 1).Width(width).Align(lipgloss.Center).Render(body)
}

func (dashboard *monitorDashboard) restartPanel(widths ...int) string {
	width := 56
	if len(widths) > 0 {
		width = max(8, widths[0])
	}
	automatic := dashboard.Settings.AutomaticMode
	if automatic == "Memory-aware" {
		automatic += " at " + formatMonitorDecimal(dashboard.Settings.MemoryLimitGB) + " GB"
	} else if automatic == "Time scheduled" {
		automatic += fmt.Sprintf(" every %dm", dashboard.Settings.ScheduledMinutes)
	}
	latest := "—"
	if dashboard.Current.Reason != "" {
		latest = strings.ReplaceAll(dashboard.Current.Reason, "_", " ")
	}
	details := fmt.Sprintf("Crash retries  %d/%d\nRapid crash    under %ds → possible code error\nBackoff        %.1fs × %.2f, cap %.1fs\nAutomatic      %s\nRestarts       crash %d • time %d • memory %d\nLatest         %s",
		dashboard.Current.CrashCount, dashboard.Settings.CrashRetries, dashboard.Settings.RapidCrashSeconds, dashboard.Settings.BaseDelaySeconds,
		dashboard.Settings.BackoffMultiplier, dashboard.Settings.MaxDelaySeconds, automatic,
		dashboard.Current.CrashCount, dashboard.Current.ScheduledCount, dashboard.Current.MemoryCount, latest)
	cat := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81")).Width(width).Align(lipgloss.Center).Render(padMonitorPetFrame(monitorCatFrames[dashboard.petFrame%len(monitorCatFrames)]))
	return details + "\n\n" + cat
}

func padMonitorPetFrame(frame string) string {
	const canvasWidth = 22
	lines := strings.Split(frame, "\n")
	for index, line := range lines {
		lines[index] = line + strings.Repeat(" ", max(0, canvasWidth-lipgloss.Width(line)))
	}
	return strings.Join(lines, "\n")
}

func (dashboard *monitorDashboard) monitoringPanel() string {
	leak := "off"
	if dashboard.Settings.LeakEnabled {
		leak = fmt.Sprintf("on • warm-up %s", formatMonitorDuration(float64(dashboard.Settings.LeakWarmupSeconds)))
	}
	heartbeat := "off"
	if dashboard.Settings.HeartbeatEnabled {
		remaining := float64(dashboard.Settings.HeartbeatMinutes*60) - (dashboard.Current.Elapsed - dashboard.lastHeartbeat)
		heartbeat = fmt.Sprintf("every %dm • next in %s", dashboard.Settings.HeartbeatMinutes, formatMonitorDuration(max(0, remaining)))
	}
	notificationLines := []string{"", "EMAIL NOTIFICATIONS"}
	for _, notification := range dashboard.Settings.NotificationStates {
		notificationLines = append(notificationLines, fmt.Sprintf("%-21s %s", notification.Label, onOff(notification.Enabled)))
	}
	return fmt.Sprintf("Sampling       %.1fs\nLeak detection %s\nHeartbeat      %s\nReports        %s\nLog viewer     %s\n%s",
		dashboard.Settings.SampleSeconds, leak, heartbeat, onOff(dashboard.Settings.ReportsEnabled), onOff(dashboard.Settings.ViewerEnabled), strings.Join(notificationLines, "\n"))
}

func (dashboard *monitorDashboard) activityPanel() string {
	if len(dashboard.Activity) == 0 {
		return "Waiting for runtime events…"
	}
	return strings.Join(dashboard.Activity, "\n")
}

func (dashboard *monitorDashboard) monitorLogPanel() string {
	lines := dashboard.MonitorLog
	if len(lines) > monitorLogEntries {
		lines = lines[len(lines)-monitorLogEntries:]
	}
	if len(lines) == 0 {
		lines = []string{"Waiting for monitor events…"}
	}
	lines = append([]string(nil), lines...)
	for len(lines) < monitorLogEntries {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (dashboard *monitorDashboard) appendMonitorLog(message string) {
	if message == "" {
		return
	}
	dashboard.MonitorLog = append(dashboard.MonitorLog, time.Now().Format("15:04:05")+"  "+message)
	if len(dashboard.MonitorLog) > 100 {
		dashboard.MonitorLog = dashboard.MonitorLog[len(dashboard.MonitorLog)-100:]
	}
	dashboard.rebuildViewport()
}

func (dashboard *monitorDashboard) outputPanel() string {
	if len(dashboard.Output) == 0 {
		return "Waiting for target output…"
	}
	return strings.Join(dashboard.Output, "\n")
}

func (dashboard *monitorDashboard) apply(event monitorEvent) {
	current := dashboard.Current
	current.Type, current.State, current.Message = event.Type, event.State, event.Message
	if event.Title != "" {
		current.Title = event.Title
	}
	current.Text, current.Stream, current.Error = event.Text, event.Stream, event.Error
	current.Outcome, current.Success = event.Outcome, event.Success
	if event.Script != "" {
		current.Script = event.Script
	}
	if event.RunDirectory != "" {
		current.RunDirectory = event.RunDirectory
	}
	if event.QueueTotal != 0 {
		current.QueueIndex, current.QueueTotal = event.QueueIndex, event.QueueTotal
	}
	if event.PID != 0 {
		current.PID = event.PID
	}
	if event.Attempt != 0 {
		current.Attempt = event.Attempt
	}
	if event.Type == "resource_sample" {
		current.Elapsed, current.CPUPercent, current.RAMBytes, current.RAMMiB = event.Elapsed, event.CPUPercent, event.RAMBytes, event.RAMMiB
		current.SystemRAMTotalBytes = event.SystemRAMTotalBytes
		current.GPUPercent, current.GPUMemoryMiB, current.GPUMemoryTotalMiB, current.GPUScope = event.GPUPercent, event.GPUMemoryMiB, event.GPUMemoryTotalMiB, event.GPUScope
		current.GPUDevices = append([]monitorGPUDevice(nil), event.GPUDevices...)
	}
	if event.Type == "restart_decision" || event.Type == "final_outcome" {
		current.CrashCount, current.ScheduledCount, current.MemoryCount = event.CrashCount, event.ScheduledCount, event.MemoryCount
	}
	if event.Type == "restart_decision" {
		current.DelaySeconds, current.Reason = event.DelaySeconds, event.Reason
	}
	if event.Type == "final_outcome" && event.Elapsed > 0 {
		current.Elapsed = event.Elapsed
	}
	if event.Type == "email_result" && event.Kind == "heartbeat" && event.Success {
		dashboard.lastHeartbeat = current.Elapsed
	}
	if event.Type == "email_result" {
		dashboard.emailSeen = true
		dashboard.emailKind = event.Kind
		dashboard.emailSuccess = event.Success
	}
	if event.Type == "run_created" {
		dashboard.CodeError = false
		dashboard.CodeErrorText = ""
	}
	if event.Type == "lifecycle" && event.State == "possible_code_error_warning" {
		dashboard.CodeError = true
		dashboard.CodeErrorText = event.Message
		dashboard.viewport.GotoTop()
	}
	dashboard.Current = current
	if event.Type == "target_output" {
		for _, line := range sanitizeMonitorLines(event.Text, -1) {
			if event.Stream != "" {
				line = fmt.Sprintf("[%s] %s", event.Stream, line)
			}
			dashboard.Output = append(dashboard.Output, line)
		}
		if len(dashboard.Output) > 10 {
			dashboard.Output = dashboard.Output[len(dashboard.Output)-10:]
		}
	}
	if activity := monitorActivityLine(event); activity != "" {
		dashboard.Activity = append(dashboard.Activity, activity)
		dashboard.ActivityLevel = append(dashboard.ActivityLevel, monitorEventSeverity(event))
		if len(dashboard.Activity) > monitorActivityEntries {
			dashboard.Activity = dashboard.Activity[len(dashboard.Activity)-monitorActivityEntries:]
			dashboard.ActivityLevel = dashboard.ActivityLevel[len(dashboard.ActivityLevel)-monitorActivityEntries:]
		}
	}
	if line := monitorLogLine(event); line != "" {
		dashboard.MonitorLog = append(dashboard.MonitorLog, time.Now().Format("15:04:05")+"  "+line)
		if len(dashboard.MonitorLog) > 100 {
			dashboard.MonitorLog = dashboard.MonitorLog[len(dashboard.MonitorLog)-100:]
		}
	}
	dashboard.rebuildViewport()
}

func (dashboard *monitorDashboard) finalSummary() string {
	event := dashboard.Current
	title := event.Title
	if title == "" && len(dashboard.Titles) > 0 {
		title = dashboard.Titles[min(max(event.QueueIndex-1, 0), len(dashboard.Titles)-1)]
	}
	script := filepath.Base(event.Script)
	if script == "." && len(dashboard.Scripts) > 0 {
		script = filepath.Base(dashboard.Scripts[min(max(event.QueueIndex-1, 0), len(dashboard.Scripts)-1)])
	}
	email := "not requested"
	if dashboard.emailSeen {
		email = dashboard.emailKind + " failed"
		if dashboard.emailSuccess {
			email = dashboard.emailKind + " sent"
		}
	}
	outcome := strings.ToUpper(emptyMonitorValue(event.Outcome))
	heading := "MONITOR RUN COMPLETE"
	if event.Outcome == "failed" || event.Outcome == "error" {
		heading = "MONITOR RUN FAILED"
	} else if event.Outcome == "cancelled" {
		heading = "MONITOR RUN CANCELLED"
	}
	severity := monitorEventSeverity(event)
	divider := strings.Repeat("─", len(heading))
	heading = colorMonitorMessage(heading, severity)
	outcome = colorMonitorMessage(outcome, severity)
	body := fmt.Sprintf("%s\n%s\n\n%-12s %s\n%-12s %s\n%-12s %s\n%-12s %d/%d\n%-12s %s\n%-12s %d\n%-12s crash %d • time %d • memory %d\n%-12s %s\n%-12s %s",
		heading, divider, "Title", emptyMonitorValue(title), "Outcome", outcome,
		"Script", emptyMonitorValue(script), "Queue", event.QueueIndex, max(event.QueueTotal, len(dashboard.Scripts)),
		"Elapsed", formatMonitorDuration(event.Elapsed), "Attempts", event.Attempt, "Restarts", event.CrashCount,
		event.ScheduledCount, event.MemoryCount, "Email", email, "Artifacts", emptyMonitorValue(event.RunDirectory))
	borderColor := "63"
	if severity != monitorNeutral {
		borderColor = monitorSeverityColor(severity)
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(borderColor)).Padding(0, 2).Render(body)
}

type monitorMessageSeverity int

const (
	monitorNeutral monitorMessageSeverity = iota
	monitorSuccess
	monitorFailure
	monitorWarning
)

func colorMonitorMessage(value string, severity monitorMessageSeverity) string {
	if severity == monitorNeutral {
		return value
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(monitorSeverityColor(severity))).Render(value)
}

func colorMonitorToggleValues(value string) string {
	value = strings.ReplaceAll(value, " on", " "+colorMonitorMessage("on", monitorSuccess))
	return strings.ReplaceAll(value, " off", " "+colorMonitorMessage("off", monitorFailure))
}

func colorMonitorResourceBody(value string) string {
	blue := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75"))
	active := lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
	inactive := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "CPU" || trimmed == "RAM" || strings.HasPrefix(trimmed, "GPU ·") || trimmed == "Utilization" || trimmed == "Memory" {
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			line = indent + blue.Render(trimmed)
		}
		line = strings.ReplaceAll(line, "━", active.Render("━"))
		lines[index] = strings.ReplaceAll(line, "─", inactive.Render("─"))
	}
	return strings.Join(lines, "\n")
}

func monitorSeverityColor(severity monitorMessageSeverity) string {
	switch severity {
	case monitorSuccess:
		return "42"
	case monitorFailure:
		return "196"
	case monitorWarning:
		return "208"
	default:
		return "250"
	}
}

func monitorEventSeverity(event monitorEvent) monitorMessageSeverity {
	switch event.Type {
	case "final_outcome":
		switch event.Outcome {
		case "success":
			return monitorSuccess
		case "failed", "error":
			return monitorFailure
		case "cancelled":
			return monitorWarning
		}
	case "email_result":
		if event.Success {
			return monitorSuccess
		}
		return monitorFailure
	case "restart_decision":
		return monitorWarning
	case "lifecycle":
		if strings.Contains(event.State, "warning") {
			return monitorWarning
		}
	}
	return monitorNeutral
}

func (dashboard *monitorDashboard) currentSeverity() monitorMessageSeverity {
	if dashboard.Stopping {
		return monitorWarning
	}
	if dashboard.Current.Outcome != "" {
		return monitorEventSeverity(monitorEvent{Type: "final_outcome", Outcome: dashboard.Current.Outcome})
	}
	return monitorNeutral
}

func (dashboard *monitorDashboard) render(output interface{ Write([]byte) (int, error) }) {
	event := dashboard.Current
	switch event.Type {
	case "lifecycle":
		fmt.Fprintf(output, "[%d/%d] %s — %s\n", event.QueueIndex, event.QueueTotal, filepath.Base(event.Script), event.State)
	case "attempt_started":
		fmt.Fprintf(output, "attempt %d · PID %d · %s (%s)\n", event.Attempt, event.PID, dashboard.Interpreter.Environment, dashboard.Interpreter.Path)
	case "resource_sample":
		fmt.Fprintf(output, "elapsed %.1fs · CPU %.1f%% · RAM %.1f MiB · GPU %v\n", event.Elapsed, event.CPUPercent, event.RAMMiB, event.GPUPercent)
	case "target_output":
		if len(dashboard.Output) > 0 {
			fmt.Fprintln(output, dashboard.Output[len(dashboard.Output)-1])
		}
	case "restart_decision":
		fmt.Fprintf(output, "restart: %s, delay %.1fs\n", event.Reason, event.DelaySeconds)
	case "email_result":
		fmt.Fprintf(output, "email delivery success: %t\n", event.Success)
	case "final_outcome":
		if event.Error != "" {
			fmt.Fprintf(output, "Monitor error: %s\n", event.Error)
		} else {
			fmt.Fprintf(output, "outcome: %s\n", event.Outcome)
		}
	}
}

func monitorActivityLine(event monitorEvent) string {
	switch event.Type {
	case "lifecycle":
		line := strings.ReplaceAll(event.State, "_", " ")
		if event.Message != "" {
			line += " • " + event.Message
		}
		return line
	case "attempt_started":
		return fmt.Sprintf("attempt %d started • PID %d", event.Attempt, event.PID)
	case "attempt_ended":
		return fmt.Sprintf("attempt %d ended", event.Attempt)
	case "restart_decision":
		return "restart • " + strings.ReplaceAll(event.Reason, "_", " ")
	case "email_result":
		return fmt.Sprintf("%s email • success %t", event.Kind, event.Success)
	}
	return ""
}

func monitorLogLine(event monitorEvent) string {
	switch event.Type {
	case "run_created":
		return "monitor started • logs " + event.RunDirectory
	case "attempt_ended":
		if event.ExitCode != 0 && !event.Scheduled && !event.MemoryRestart {
			return fmt.Sprintf("crash • attempt %d exited with exit code %d", event.Attempt, event.ExitCode)
		}
	case "restart_decision":
		if event.Reason == "crash" && event.Restart {
			return fmt.Sprintf("restart scheduled in %.1fs • crash notification follows stability classification", event.DelaySeconds)
		}
		if event.Reason == "crash" {
			return "crash retries exhausted • configured final notifications are being evaluated"
		}
		return "restart • " + strings.ReplaceAll(event.Reason, "_", " ")
	case "email_result":
		if event.Success {
			return strings.ReplaceAll(event.Kind, "_", " ") + " email sent"
		}
		return strings.ReplaceAll(event.Kind, "_", " ") + " email failed • " + emptyMonitorValue(event.Error)
	case "lifecycle":
		if event.Message != "" {
			return strings.ReplaceAll(event.State, "_", " ") + " • " + event.Message
		}
		return strings.ReplaceAll(event.State, "_", " ")
	case "final_outcome":
		if event.Error != "" {
			return "monitor error • " + event.Error
		}
		return "run finished • " + event.Outcome
	}
	return ""
}

func monitorMeter(percent float64) string {
	percent = max(0, min(100, percent))
	filled := int(percent / 10)
	active := lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Render(strings.Repeat("━", filled))
	inactive := lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(strings.Repeat("─", 10-filled))
	return active + inactive
}

func formatMonitorGB(bytes int64) string {
	if bytes <= 0 {
		return "unavailable"
	}
	return formatMonitorDecimal(float64(bytes)/1000000000) + " GB"
}

func formatMonitorMemoryPair(used float64, usedAvailable bool, total float64, totalAvailable bool) string {
	if !usedAvailable && !totalAvailable {
		return "unavailable"
	}
	usedText := "unavailable"
	if usedAvailable {
		usedText = fmt.Sprintf("%.3f", used)
	}
	totalText := "unavailable"
	if totalAvailable {
		totalText = fmt.Sprintf("%.3f", total)
	}
	return usedText + " / " + totalText + " GB"
}

func formatMonitorDecimal(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", value), "0"), ".")
}

func anyMonitorFloat(value any) float64 {
	switch value := value.(type) {
	case float64:
		return value
	case int:
		return float64(value)
	default:
		return 0
	}
}

func formatMonitorOptional(value any, suffix string) string {
	if value == nil {
		return "unavailable"
	}
	return fmt.Sprintf("%.1f%s", anyMonitorFloat(value), suffix)
}

func formatMonitorDuration(seconds float64) string {
	seconds = max(0, seconds)
	minutes := int(seconds) / 60
	remainder := int(seconds) % 60
	if minutes > 0 {
		return fmt.Sprintf("%dm%02ds", minutes, remainder)
	}
	return fmt.Sprintf("%ds", remainder)
}

func wrapMonitorText(value string, width int) string {
	value = strings.Join(sanitizeMonitorLines(value, -1), " ")
	return lipgloss.Wrap(value, max(1, width), "")
}

func wrapMonitorBlock(value string, width int) string {
	return lipgloss.Wrap(value, max(1, width), "")
}

func emptyMonitorValue(value string) string {
	if value == "" {
		return "—"
	}
	return value
}

func formatMonitorPID(pid int) string {
	if pid == 0 {
		return "—"
	}
	return fmt.Sprint(pid)
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}
