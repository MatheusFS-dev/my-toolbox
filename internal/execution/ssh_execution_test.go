//go:build linux

package execution

import (
	"bytes"
	"strings"
	"testing"
)

func TestSSHDirectCommandRejectsArguments(t *testing.T) {
	var output bytes.Buffer
	executor := ProcessExecutor{Root: t.TempDir(), Environment: "linux-native", Output: &output}
	err := executor.Run(Command{Name: "ssh-tools", Protocol: "builtin"}, nil, []string{"unexpected"})
	if err == nil || !strings.Contains(err.Error(), "does not accept arguments") {
		t.Fatalf("error = %v", err)
	}
}
