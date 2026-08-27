package main

import (
	"errors"
	"fmt"

	"charm.land/huh/v2"
)

// HuhUI renders Arrow/Space selection and typed configuration forms.
type HuhUI struct{}

// Select renders the catalog as a checkbox list.
//
// Args:
//   - commands: Catalog-ordered tools with package and description metadata.
//
// Returns:
//   - []string: Selected command names.
//   - error: ErrCancelled for Ctrl+C or Escape, or a terminal rendering error.
func (HuhUI) Select(commands []Command) ([]string, error) {
	selected := []string{}
	options := make([]huh.Option[string], 0, len(commands))
	for _, command := range commands {
		options = append(options, huh.NewOption(selectorLabel(command), command.Name))
	}
	field := huh.NewMultiSelect[string]().
		Title("Select tools (Arrow keys move, Space toggles, Enter continues)").
		Options(options...).
		Value(&selected)
	if err := field.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, ErrCancelled
		}
		return nil, err
	}
	return selected, nil
}

// Ask renders exactly one typed configuration question in a visible form.
//
// Args:
//   - question: Validated text, confirmation, single, or multiple question.
//
// Returns:
//   - any: String, bool, or string slice matching the question type.
//   - error: ErrCancelled for Ctrl+C or Escape, validation, or terminal error.
func (HuhUI) Ask(question Question) (any, error) {
	var field huh.Field
	var value any
	switch question.Type {
	case "text":
		answer := ""
		field = huh.NewInput().Title(question.Title).Value(&answer)
		value = &answer
	case "confirm":
		answer := false
		field = huh.NewConfirm().Title(question.Title).Value(&answer)
		value = &answer
	case "single":
		answer := ""
		field = huh.NewSelect[string]().Title(question.Title).Options(huhOptions(question.Options)...).Value(&answer)
		value = &answer
	case "multiple":
		answer := []string{}
		field = huh.NewMultiSelect[string]().Title(question.Title).Options(huhOptions(question.Options)...).Value(&answer)
		value = &answer
	default:
		return nil, fmt.Errorf("unsupported question type %q", question.Type)
	}
	if err := huh.NewForm(huh.NewGroup(field)).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, ErrCancelled
		}
		return nil, err
	}
	switch answer := value.(type) {
	case *string:
		return *answer, nil
	case *bool:
		return *answer, nil
	case *[]string:
		return *answer, nil
	default:
		return nil, fmt.Errorf("question %q produced an invalid answer", question.ID)
	}
}

// selectorLabel formats one catalog tool for the interactive selector.
//
// Args:
//   - command: Tool metadata whose name, package, and description are shown.
//
// Returns:
//   - string: Two-line label with an indented ANSI gray description and a
//     trailing blank line separating it from the next tool.
//
// Raises:
//   - None.
func selectorLabel(command Command) string {
	return fmt.Sprintf("%s  [%s]\n    \x1b[38;5;8m%s\x1b[0m\n\n", command.Name, command.Package, command.Description)
}

func huhOptions(options []Option) []huh.Option[string] {
	values := make([]huh.Option[string], 0, len(options))
	for _, option := range options {
		values = append(values, huh.NewOption(option.Label, option.Value))
	}
	return values
}
