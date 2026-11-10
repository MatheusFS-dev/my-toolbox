//go:build !windows

package builtins

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type monitorInterpreter struct {
	Path        string
	Environment string
}

type monitorEmailSettings struct {
	Provider string
	Host     string
	Port     string
	Security string
	Sender   string
	Password string
}

func monitorEmailCredentials(provider, sender, password string, existing monitorEmailSettings) monitorEmailSettings {
	credentials := existing
	credentials.Provider = provider
	credentials.Sender = strings.TrimSpace(sender)
	if password != "" {
		credentials.Password = password
	}
	if provider == "gmail" {
		if password != "" {
			credentials.Password = strings.ReplaceAll(password, " ", "")
		}
		credentials.Host = "smtp.gmail.com"
		credentials.Port = "587"
		credentials.Security = "starttls"
	}
	return credentials
}

func monitorConfigurationSummary(credentials monitorEmailSettings, recipients []string, quick bool) string {
	provider := "Custom email server"
	if credentials.Provider == "gmail" {
		provider = "Gmail"
	}
	settings := "custom monitoring settings"
	if quick {
		settings = "default monitoring settings"
	}
	return fmt.Sprintf("Email provider: %s\nSender: %s\nNotifications go to: %s\nMonitoring: %s", provider, credentials.Sender, strings.Join(recipients, ", "), settings)
}

func monitorInitialRecipients(saved []string) string {
	return strings.Join(saved, ", ")
}

func monitorSystemRAMTotalBytes() int64 {
	content, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			kilobytes, parseErr := strconv.ParseInt(fields[1], 10, 64)
			if parseErr == nil && kilobytes > 0 {
				return kilobytes * 1024
			}
		}
	}
	return 0
}

func monitorMemoryRestartDescription(totalBytes int64) string {
	description := "Monitor restarts the target when total process-tree RAM reaches this decimal-gigabyte limit."
	if totalBytes > 0 {
		return fmt.Sprintf("%s This system has %.3f GB total system RAM.", description, float64(totalBytes)/1_000_000_000)
	}
	return description + " Total system RAM is unavailable."
}

func applyMonitorRestartMode(restart map[string]any, enabled bool, mode string, value float64) error {
	if !enabled {
		restart["memory_aware"] = false
		restart["scheduled_interval_minutes"] = 0
		return nil
	}
	if value <= 0 {
		return fmt.Errorf("automatic restart value must be greater than zero")
	}
	switch mode {
	case "memory":
		restart["memory_aware"] = true
		restart["memory_limit_gb"] = value
		delete(restart, "memory_limit_mib")
		restart["scheduled_interval_minutes"] = 0
	case "time":
		restart["memory_aware"] = false
		restart["scheduled_interval_minutes"] = int(value)
	default:
		return fmt.Errorf("unknown automatic restart mode: %s", mode)
	}
	return nil
}

func normalizeMonitorRestart(restart map[string]any) {
	if _, present := restart["rapid_crash_seconds"]; !present {
		restart["rapid_crash_seconds"] = 60
	}
	if _, present := restart["memory_limit_gb"]; !present {
		if legacy, legacyPresent := restart["memory_limit_mib"]; legacyPresent {
			restart["memory_limit_gb"] = numberValue(legacy, 1024) * 1048576 / 1000000000
		} else {
			restart["memory_limit_gb"] = 1.0
		}
	}
	delete(restart, "memory_limit_mib")
}

func applyMonitorLeakWarmup(leak map[string]any, enabled bool, minutes int) error {
	leak["enabled"] = enabled
	if !enabled {
		return nil
	}
	if minutes <= 0 {
		return fmt.Errorf("memory-leak warm-up must be greater than zero")
	}
	leak["warmup_seconds"] = minutes * 60
	return nil
}

func resolveMonitorInterpreter(requested string) (monitorInterpreter, error) {
	type candidate struct {
		path        string
		environment string
	}
	candidates := []candidate{}
	if requested != "" {
		candidates = append(candidates, candidate{path: requested, environment: filepath.Base(filepath.Dir(filepath.Dir(requested)))})
	} else {
		for _, variable := range []string{"VIRTUAL_ENV", "CONDA_PREFIX"} {
			if prefix := os.Getenv(variable); prefix != "" {
				candidates = append(candidates, candidate{path: filepath.Join(prefix, "bin", "python3"), environment: filepath.Base(prefix)})
			}
		}
		if path, err := exec.LookPath("python3"); err == nil {
			candidates = append(candidates, candidate{path: path, environment: "system"})
		}
	}
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate.path)
		if err != nil || !regularExecutable(absolute) || !supportsPythonVersion(absolute, nil, "sys.version_info[0] == 3") {
			continue
		}
		environment := candidate.environment
		if environment == "." || environment == string(filepath.Separator) {
			environment = "system"
		}
		return monitorInterpreter{Path: absolute, Environment: environment}, nil
	}
	return monitorInterpreter{}, fmt.Errorf("no Python 3 interpreter is available")
}

func regularExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

func validateMonitorTarget(argument string) (string, error) {
	absolute, err := filepath.Abs(argument)
	if err != nil {
		return "", fmt.Errorf("resolve target %s: %w", argument, err)
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o444 == 0 || strings.ToLower(filepath.Ext(absolute)) != ".py" {
		return "", fmt.Errorf("target must be a readable regular .py file: %s", argument)
	}
	return absolute, nil
}

func validateMonitorTargets(arguments []string) ([]string, error) {
	if len(arguments) == 0 {
		return nil, fmt.Errorf("at least one Python script is required")
	}
	targets := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		absolute, err := validateMonitorTarget(argument)
		if err != nil {
			return nil, err
		}
		targets = append(targets, absolute)
	}
	return targets, nil
}

func parseMonitorTargets(groups []string) ([]string, [][]string, error) {
	if len(groups) == 0 {
		return nil, nil, fmt.Errorf("at least one Python script is required")
	}
	targets := make([]string, 0, len(groups))
	scriptArgs := make([][]string, 0, len(groups))
	for _, group := range groups {
		if strings.ContainsRune(group, '\x00') {
			return nil, nil, monitorArgumentSyntaxError()
		}
		if target, err := validateMonitorTarget(group); err == nil {
			targets = append(targets, target)
			scriptArgs = append(scriptArgs, nil)
			continue
		}
		parts, err := splitMonitorCommand(group)
		if err != nil || len(parts) == 0 {
			return nil, nil, monitorArgumentSyntaxError()
		}
		target, err := validateMonitorTarget(parts[0])
		if err != nil {
			return nil, nil, monitorArgumentSyntaxError()
		}
		targets = append(targets, target)
		scriptArgs = append(scriptArgs, append([]string(nil), parts[1:]...))
	}
	return targets, scriptArgs, nil
}

func monitorArgumentSyntaxError() error {
	return fmt.Errorf("invalid script arguments. Use monitor \"script.py --argument value\" \"other.py\", or monitor script.py other.py when no arguments are needed.")
}

func splitMonitorCommand(value string) ([]string, error) {
	var parts []string
	var word strings.Builder
	var quote rune
	escaped := false
	inWord := false
	flush := func() {
		if inWord {
			parts = append(parts, word.String())
			word.Reset()
			inWord = false
		}
	}
	for _, char := range value {
		if escaped {
			word.WriteRune(char)
			escaped = false
			inWord = true
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped = true
			inWord = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				word.WriteRune(char)
			}
			inWord = true
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			inWord = true
			continue
		}
		if char == ' ' || char == '\t' || char == '\n' || char == '\r' {
			flush()
			continue
		}
		word.WriteRune(char)
		inWord = true
	}
	if escaped || quote != 0 {
		return nil, fmt.Errorf("unterminated command quote or escape")
	}
	flush()
	return parts, nil
}

var monitorControlSequence = regexp.MustCompile(`(?:\x1b\[[0-?]*[ -/]*[@-~])|[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
var regexpMonitorGPU = regexp.MustCompile(`^GPU-[0-9a-fA-F-]+$`)

func sanitizeMonitorLines(value string, limit int) []string {
	clean := monitorControlSequence.ReplaceAllString(value, "")
	lines := strings.Split(strings.ReplaceAll(clean, "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if limit >= 0 && len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return lines
}
