package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Macro is one runnable script discovered from a registered macro driver.
type Macro struct {
	Subpackage   string
	Title        string
	Description  string
	RelativePath string
	SourcePath   string
	Content      string
	Arguments    []MacroArgument
}

// MacroArgument describes one ordered value supplied to a macro script.
type MacroArgument struct {
	ID      string `json:"id"`
	Prompt  string `json:"prompt"`
	Type    string `json:"type"`
	Default string `json:"default"`
}

// MacroAction identifies an action chosen in the macro browser.
type MacroAction string

const (
	MacroActionRun      MacroAction = "run"
	MacroActionDownload MacroAction = "download"
)

// MacroSelection is the macro and action selected by the user.
type MacroSelection struct {
	Macro  Macro
	Action MacroAction
}

type macroMetadata struct {
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Arguments   []MacroArgument `json:"arguments"`
}

type macroDriver struct {
	extension string
	goos      string
	label     string
}

var macroDrivers = map[string]macroDriver{
	"autohotkey": {".ahk", "windows", "Windows"},
	"ydotool":    {".sh", "linux", "Linux"},
	"cronjob":    {".sh", "linux", "Linux"},
}

func macroSupportsOS(macro Macro, goos string) bool {
	driver, ok := macroDrivers[macro.Subpackage]
	return ok && driver.goos == goos
}

func loadMacros(root string, warnings io.Writer) ([]Macro, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("macro library is missing: %s", root)
		}
		return nil, fmt.Errorf("inspect macro library %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("macro library is not a directory: %s", root)
	}
	if warnings == nil {
		warnings = io.Discard
	}
	macros := []Macro{}
	for subpackage, driver := range macroDrivers {
		driverRoot := filepath.Join(root, subpackage)
		if _, statErr := os.Stat(driverRoot); statErr != nil {
			continue
		}
		walkErr := filepath.WalkDir(driverRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				fmt.Fprintf(warnings, "warning: skip macro %s: %v\n", warningPath(root, path), walkErr)
				return nil
			}
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), driver.extension) {
				return nil
			}
			macro, macroErr := loadMacro(root, subpackage, path)
			if macroErr != nil {
				fmt.Fprintf(warnings, "warning: skip macro %s: %v\n", warningPath(root, path), macroErr)
				return nil
			}
			macros = append(macros, macro)
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("discover macro library %s: %w", root, walkErr)
		}
	}
	if len(macros) == 0 {
		return nil, fmt.Errorf("macro library contains no valid macros: %s", root)
	}
	sort.SliceStable(macros, func(i, j int) bool {
		return strings.ToLower(macros[i].Subpackage+"\x00"+macros[i].Title+"\x00"+macros[i].RelativePath) < strings.ToLower(macros[j].Subpackage+"\x00"+macros[j].Title+"\x00"+macros[j].RelativePath)
	})
	return macros, nil
}

func loadMacro(root, subpackage, path string) (Macro, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return Macro{}, fmt.Errorf("script is not a regular file")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return Macro{}, err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return Macro{}, err
	}
	macro := Macro{Subpackage: subpackage, Title: filenameTitle(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))), RelativePath: filepath.ToSlash(relative), SourcePath: path, Content: string(content)}
	metadataPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".json"
	metadataBytes, err := os.ReadFile(metadataPath)
	if os.IsNotExist(err) {
		return macro, nil
	}
	if err != nil {
		return Macro{}, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(metadataBytes)))
	decoder.DisallowUnknownFields()
	var metadata macroMetadata
	if err := decoder.Decode(&metadata); err != nil {
		return Macro{}, fmt.Errorf("invalid metadata: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Macro{}, fmt.Errorf("invalid metadata: trailing JSON value")
	}
	if strings.TrimSpace(metadata.Title) == "" {
		return Macro{}, fmt.Errorf("metadata title is required")
	}
	seen := map[string]bool{}
	for _, argument := range metadata.Arguments {
		if strings.TrimSpace(argument.ID) == "" || strings.TrimSpace(argument.Prompt) == "" {
			return Macro{}, fmt.Errorf("argument id and prompt are required")
		}
		if seen[argument.ID] {
			return Macro{}, fmt.Errorf("duplicate argument id %q", argument.ID)
		}
		seen[argument.ID] = true
		if argument.Type != "text" && argument.Type != "non_negative_integer" && argument.Type != "time_24h" {
			return Macro{}, fmt.Errorf("unsupported argument type %q", argument.Type)
		}
		if err := validateMacroArgument(argument, argument.Default); err != nil {
			return Macro{}, fmt.Errorf("invalid default for %q: %w", argument.ID, err)
		}
	}
	macro.Title, macro.Description, macro.Arguments = metadata.Title, metadata.Description, metadata.Arguments
	return macro, nil
}

func validateMacroArgument(argument MacroArgument, value string) error {
	if argument.Type == "time_24h" {
		parsed, err := time.Parse("15:04", value)
		if err != nil || parsed.Format("15:04") != value {
			return fmt.Errorf("must be a 24-hour time HH:MM")
		}
	}
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("value is required")
	}
	if argument.Type == "non_negative_integer" {
		number, err := strconv.ParseInt(value, 10, 64)
		if err != nil || number < 0 {
			return fmt.Errorf("must be a non-negative integer")
		}
	}
	return nil
}

// MacroWorkflow performs the selected macro action.
type MacroWorkflow interface{ Execute(MacroSelection) error }
