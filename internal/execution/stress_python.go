package execution

import (
	"fmt"
	"os/exec"
)

const stressGPUVersionCondition = "(3, 10) <= sys.version_info[:2] < (3, 15)"

// stressGPUPython selects a supported system Python without installing it.
// Args: platform (string): Toolbox platform identifier.
// Returns: []string: Executable and optional launcher prefix. error: Unsupported Python.
// Raises: None.
func StressGPUPython(platform string) ([]string, error) {
	candidates := [][]string{{"python3"}}
	if platform == "windows-amd64" {
		candidates = [][]string{{"py", "-3"}, {"python"}, {"python3"}}
	}
	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate[0]); err == nil && supportsPythonVersion(path, candidate[1:], stressGPUVersionCondition) {
			return append([]string{path}, candidate[1:]...), nil
		}
	}
	return nil, fmt.Errorf("stress-gpu requires Python 3.10 through 3.14 with venv support")
}
