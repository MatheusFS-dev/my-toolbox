//go:build !windows

package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"charm.land/huh/v2"
)

type monitorGPUSetting struct {
	Path  string `json:"path"`
	Line  int    `json:"line"`
	Value string `json:"value"`
}

// Read Python syntax without executing the target or importing its dependencies.
// This deliberately cannot resolve dynamic imports or computed device choices.
const monitorGPUScan = `
import ast
import json
import pathlib
import sys

pending = [pathlib.Path(p).resolve() for p in json.loads(sys.argv[1])]
roots = []
for target in pending:
    roots.append(target.parent)
    for parent in target.parents:
        if (parent / 'pyproject.toml').exists() or (parent / '.git').exists():
            roots.extend([parent, parent / 'src'])
            break
seen = set()
settings = []
def enqueue(module, bases):
    parts = module.split('.')
    for base in bases:
        for end in range(1, len(parts) + 1):
            package = base.joinpath(*parts[:end], '__init__.py')
            if package.is_file():
                pending.append(package)
        candidate = base.joinpath(*parts).with_suffix('.py')
        if candidate.is_file():
            pending.append(candidate)

def name(node):
    if isinstance(node, ast.Name):
        return node.id
    if isinstance(node, ast.Attribute):
        return name(node.value) + '.' + node.attr
    return ''

while pending:
    path = pending.pop().resolve()
    if path in seen:
        continue
    seen.add(path)
    tree = ast.parse(path.read_text(encoding='utf-8-sig'), filename=str(path))
    constants = {}
    for node in tree.body:
        if isinstance(node, ast.Assign) and isinstance(node.value, ast.Constant):
            for target in node.targets:
                if isinstance(target, ast.Name):
                    constants[target.id] = node.value.value
    def value(node):
        if isinstance(node, ast.Constant):
            return str(node.value)
        if isinstance(node, ast.Name) and node.id in constants:
            return str(constants[node.id])
        return '<dynamic>'
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for item in node.names:
                enqueue(item.name, roots)
        elif isinstance(node, ast.ImportFrom):
            bases = roots
            if node.level:
                base = path.parent
                for _ in range(node.level - 1):
                    base = base.parent
                bases = [base]
            module = node.module or ''
            if module:
                enqueue(module, bases)
            for item in node.names:
                if item.name != '*':
                    enqueue('.'.join(p for p in [module, item.name] if p), bases)
        targets = node.targets if isinstance(node, ast.Assign) else [node.target] if isinstance(node, ast.AnnAssign) else []
        for target in targets:
            if isinstance(target, ast.Subscript) and name(target.value) in ('os.environ', 'environ') and isinstance(target.slice, ast.Constant) and target.slice.value == 'CUDA_VISIBLE_DEVICES':
                settings.append(dict(path=str(path), line=node.lineno, value=value(node.value)))
        if isinstance(node, ast.Call):
            function = name(node.func)
            if function in ('os.putenv', 'putenv') and len(node.args) >= 2 and isinstance(node.args[0], ast.Constant) and node.args[0].value == 'CUDA_VISIBLE_DEVICES':
                settings.append(dict(path=str(path), line=node.lineno, value=value(node.args[1])))
            elif function in ('os.environ.update', 'environ.update'):
                entries = [(kw.arg, kw.value) for kw in node.keywords]
                for argument in node.args:
                    if isinstance(argument, ast.Dict):
                        entries.extend((key.value, val) for key, val in zip(argument.keys, argument.values) if isinstance(key, ast.Constant))
                for key, val in entries:
                    if key == 'CUDA_VISIBLE_DEVICES':
                        settings.append(dict(path=str(path), line=node.lineno, value=value(val)))
            elif function.endswith(('.set_visible_devices', '.set_device')):
                settings.append(dict(path=str(path), line=node.lineno, value='<dynamic>'))
print(json.dumps(settings))
`

func detectMonitorGPUSettings(targets []string, interpreter string) ([]monitorGPUSetting, error) {
	encoded, err := json.Marshal(targets)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	content, err := exec.CommandContext(ctx, interpreter, "-c", monitorGPUScan, string(encoded)).Output()
	if err != nil {
		return nil, fmt.Errorf("static GPU check unavailable: %w", err)
	}
	var settings []monitorGPUSetting
	err = json.Unmarshal(content, &settings)
	return settings, err
}

func warnMonitorGPUSettings(settings []monitorGPUSetting, messages *toolOutput) {
	for _, setting := range settings {
		_ = messages.Error(fmt.Sprintf("GPU WARNING: %s:%d sets its own GPU (%s). This code may override Monitor's GPU selection. Static detection cannot verify runtime behavior.", setting.Path, setting.Line, setting.Value))
	}
}

func confirmMonitorGPUSelection(settings []monitorGPUSetting, selected string, devices []monitorLaunchGPU, messages *toolOutput, prompt func(huh.Field) error) (bool, error) {
	if selected == "" {
		return true, nil
	}
	conflicts := []monitorGPUSetting{}
	for _, setting := range settings {
		matches := setting.Value == selected
		for _, device := range devices {
			if device.UUID == selected && setting.Value == strconv.Itoa(device.Index) {
				matches = true
			}
		}
		if !matches {
			conflicts = append(conflicts, setting)
		}
	}
	if len(conflicts) == 0 {
		return true, nil
	}
	warnMonitorGPUSettings(conflicts, messages)
	label := selected
	for _, device := range devices {
		if device.UUID == selected {
			label = device.Name + " (" + selected + ")"
		}
	}
	accepted := false
	details := []string{}
	for _, setting := range conflicts {
		details = append(details, fmt.Sprintf("%s:%d (%s)", setting.Path, setting.Line, setting.Value))
	}
	err := prompt(huh.NewConfirm().Key("gpu-conflict").Title("Are you sure you want to use the selected GPU?").Description("You selected " + label + ", but the code sets its own GPU at " + strings.Join(details, ", ") + ". It may override this selection and run on another GPU. Monitor does not enforce GPU isolation. Choose No (default) to return to GPU selection.").Affirmative("Yes, continue").Negative("No, go back (default)").Value(&accepted))
	return accepted, err
}
