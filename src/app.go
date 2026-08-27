package main

import (
	"errors"
	"fmt"
	"io"
)

// ErrCancelled identifies selection or configuration cancellation.
var ErrCancelled = errors.New("toolbox batch cancelled")

// UI owns interactive selection and typed question rendering.
type UI interface {
	Select(commands []Command) ([]string, error)
	Ask(question Question) (any, error)
}

// Executor discovers questions and runs fully configured commands.
type Executor interface {
	Questions(command Command, answers map[string]any, arguments []string) (ProtocolResponse, error)
	Run(command Command, answers map[string]any, arguments []string) error
}

type configuredCommand struct {
	command   Command
	answers   map[string]any
	arguments []string
	skipped   string
}

// App coordinates command parsing, preflight configuration, and execution.
type App struct {
	Catalog  Catalog
	UI       UI
	Executor Executor
	Output   io.Writer
	Version  string
}

// Execute handles one public tb invocation.
//
// Args:
//   - arguments: Command-line arguments after the executable name. Bare input
//     is invalid; list is the only interactive multi-tool selector.
//
// Returns:
//   - error: Usage, cancellation, configuration, or execution failure.
func (app App) Execute(arguments []string) error {
	if len(arguments) == 0 {
		return fmt.Errorf("missing command; run 'tb list' to select tools or 'tb help' for usage")
	}
	output := app.Output
	if output == nil {
		output = io.Discard
	}
	switch arguments[0] {
	case "help":
		_, err := fmt.Fprintln(output, "Usage: tb list | tb <tool> [arguments...] | tb update | tb uninstall | tb version | tb help")
		return err
	case "version":
		_, err := fmt.Fprintln(output, app.Version)
		return err
	case "list":
		if len(arguments) != 1 {
			return fmt.Errorf("tb list does not accept arguments")
		}
		selected, err := app.UI.Select(app.Catalog.Commands)
		if err != nil {
			return err
		}
		if len(selected) == 0 {
			_, err := fmt.Fprintln(output, "No tools selected.")
			return err
		}
		selectedSet := map[string]bool{}
		for _, name := range selected {
			if _, exists := app.Catalog.Find(name); !exists {
				return fmt.Errorf("selector returned unknown tool %q", name)
			}
			selectedSet[name] = true
		}
		commands := make([]Command, 0, len(selectedSet))
		for _, command := range app.Catalog.Commands {
			if selectedSet[command.Name] {
				commands = append(commands, command)
			}
		}
		return app.executeBatch(commands, nil, output)
	case "update":
		if len(arguments) != 1 {
			return fmt.Errorf("tb update does not accept arguments")
		}
		command := Command{Name: "update", Description: "Update toolbox", Package: "toolbox", Protocol: "builtin"}
		return app.Executor.Run(command, map[string]any{}, nil)
	case "uninstall":
		if len(arguments) != 1 {
			return fmt.Errorf("tb uninstall does not accept arguments")
		}
		answer, err := app.UI.Ask(Question{
			ID:    "confirm-uninstall",
			Type:  "confirm",
			Title: "Uninstall my-toolbox and remove all installed versions?",
		})
		if err != nil {
			return err
		}
		confirmed, valid := answer.(bool)
		if !valid {
			return fmt.Errorf("uninstall confirmation returned an invalid answer")
		}
		if !confirmed {
			_, err := fmt.Fprintln(output, "Uninstall cancelled.")
			return err
		}
		command := Command{Name: "uninstall", Description: "Uninstall toolbox", Package: "toolbox", Protocol: "builtin"}
		return app.Executor.Run(command, map[string]any{}, nil)
	default:
		command, exists := app.Catalog.Find(arguments[0])
		if !exists {
			return fmt.Errorf("unknown command %q; run 'tb help' for usage", arguments[0])
		}
		return app.executeBatch([]Command{command}, arguments[1:], output)
	}
}

func (app App) executeBatch(commands []Command, directArguments []string, output io.Writer) error {
	// Complete every questionnaire first so cancellation cannot leave a partial
	// multi-tool batch on disk.
	configured := make([]configuredCommand, 0, len(commands))
	for _, command := range commands {
		arguments := []string(nil)
		if len(commands) == 1 {
			arguments = directArguments
		}
		answers, skipped, err := app.configure(command, arguments)
		if err != nil {
			return fmt.Errorf("configure %s: %w", command.Name, err)
		}
		configured = append(configured, configuredCommand{
			command:   command,
			answers:   answers,
			arguments: arguments,
			skipped:   skipped,
		})
	}

	executed := []string{}
	skipped := []string{}
	for index, configuredTool := range configured {
		if configuredTool.skipped != "" {
			skipped = append(skipped, fmt.Sprintf("%s (%s)", configuredTool.command.Name, configuredTool.skipped))
			continue
		}
		if err := app.Executor.Run(configuredTool.command, configuredTool.answers, configuredTool.arguments); err != nil {
			notRun := make([]string, 0, len(configured)-index-1)
			allSkipped := append([]string(nil), skipped...)
			for _, remaining := range configured[index+1:] {
				if remaining.skipped != "" {
					allSkipped = append(allSkipped, fmt.Sprintf("%s (%s)", remaining.command.Name, remaining.skipped))
				} else {
					notRun = append(notRun, remaining.command.Name)
				}
			}
			printSummary(output, executed, configuredTool.command.Name, allSkipped, notRun)
			return fmt.Errorf("execute %s: %w", configuredTool.command.Name, err)
		}
		executed = append(executed, configuredTool.command.Name)
	}
	printSummary(output, executed, "", skipped, nil)
	return nil
}

func (app App) configure(command Command, arguments []string) (map[string]any, string, error) {
	answers := map[string]any{}
	seenQuestions := map[string]bool{}
	for {
		response, err := app.Executor.Questions(command, answers, arguments)
		if err != nil {
			return nil, "", err
		}
		switch response.Status {
		case "ready":
			return answers, "", nil
		case "skipped":
			if response.Reason == "" {
				return nil, "", fmt.Errorf("skipped response is missing a reason")
			}
			return answers, response.Reason, nil
		case "question":
			if response.Question == nil || response.Question.ID == "" {
				return nil, "", fmt.Errorf("question response is malformed")
			}
			if seenQuestions[response.Question.ID] {
				return nil, "", fmt.Errorf("repeated question ID %q", response.Question.ID)
			}
			seenQuestions[response.Question.ID] = true
			answer, err := app.UI.Ask(*response.Question)
			if err != nil {
				return nil, "", err
			}
			if answer == nil {
				return nil, "", fmt.Errorf("question %q returned no answer", response.Question.ID)
			}
			answers[response.Question.ID] = answer
		default:
			return nil, "", fmt.Errorf("unexpected adapter status %q", response.Status)
		}
	}
}

func printSummary(output io.Writer, executed []string, failed string, skipped []string, notRun []string) {
	if len(executed) > 0 {
		fmt.Fprintf(output, "executed: %s\n", joinNames(executed))
	}
	if failed != "" {
		fmt.Fprintf(output, "failed: %s\n", failed)
	}
	if len(skipped) > 0 {
		fmt.Fprintf(output, "skipped: %s\n", joinNames(skipped))
	}
	if len(notRun) > 0 {
		fmt.Fprintf(output, "not run: %s\n", joinNames(notRun))
	}
}

func joinNames(names []string) string {
	joined := ""
	for index, name := range names {
		if index > 0 {
			joined += ", "
		}
		joined += name
	}
	return joined
}
