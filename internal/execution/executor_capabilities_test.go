package execution

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestSupportsUbuntuGPUIsolationVersions(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"Ubuntu 24.04", "ID=ubuntu\nVERSION_ID=\"24.04\"\n", true},
		{"Ubuntu 26.04", "ID=ubuntu\nVERSION_ID=26.04\n", true},
		{"Ubuntu 25.04", "ID=ubuntu\nVERSION_ID=25.04\n", false},
		{"Debian 24.04", "ID=debian\nVERSION_ID=24.04\n", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := supportsUbuntuGPUIsolation([]byte(test.content)); got != test.want {
				t.Fatalf("supportsUbuntuGPUIsolation() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestSupportedPowerShellPathPrefersPowerShell7(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell host selection is Windows-specific")
	}
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("PowerShell 7 is unavailable")
	}
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skip("Windows PowerShell is unavailable")
	}

	got, exists := SupportedPowerShellPath()
	if !exists || !strings.EqualFold(got, pwsh) {
		t.Fatalf("SupportedPowerShellPath() = %q, %t, want PowerShell 7 at %q", got, exists, pwsh)
	}
}
