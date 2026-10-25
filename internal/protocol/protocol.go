package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Option is one stable value and label rendered by a selection question.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Question describes one typed form field returned by an adapter.
type Question struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Title    string   `json:"title"`
	Options  []Option `json:"options,omitempty"`
	Default  any      `json:"default,omitempty"`
	Required *bool    `json:"required"`
}

func QuestionRequired(value bool) *bool {
	return &value
}

// ProtocolResponse is the only response envelope accepted from adapters.
type ProtocolResponse struct {
	Status   string    `json:"status"`
	Question *Question `json:"question,omitempty"`
	Reason   string    `json:"reason,omitempty"`
}

// ProtocolRequest sends operation context, accumulated answers, and arguments.
type ProtocolRequest struct {
	Operation string         `json:"operation"`
	Package   PackageContext `json:"package"`
	Answers   map[string]any `json:"answers"`
	Arguments []string       `json:"arguments"`
}

// PackageContext identifies the package and command invoking an adapter.
type PackageContext struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

// DecodeProtocolResponse validates an adapter response without silent defaults.
//
// Args:
//   - reader: JSON response stream from one adapter invocation.
//
// Returns:
//   - ProtocolResponse: Validated question, ready, or skipped state.
//   - error: Malformed JSON or invalid state.
func DecodeProtocolResponse(reader io.Reader) (ProtocolResponse, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var response ProtocolResponse
	if err := decoder.Decode(&response); err != nil {
		return ProtocolResponse{}, fmt.Errorf("decode adapter response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return ProtocolResponse{}, fmt.Errorf("adapter response contains multiple JSON values")
		}
		return ProtocolResponse{}, fmt.Errorf("decode trailing adapter response: %w", err)
	}
	switch response.Status {
	case "question":
		if response.Question == nil {
			return ProtocolResponse{}, fmt.Errorf("question response is missing question")
		}
		if response.Question.ID == "" || response.Question.Title == "" {
			return ProtocolResponse{}, fmt.Errorf("question response requires ID and title")
		}
		if response.Question.Required == nil {
			return ProtocolResponse{}, fmt.Errorf("question response requires explicit required metadata")
		}
		switch response.Question.Type {
		case "text", "confirm", "single", "multiple":
		default:
			return ProtocolResponse{}, fmt.Errorf("unsupported question type %q", response.Question.Type)
		}
		if (response.Question.Type == "single" || response.Question.Type == "multiple") && len(response.Question.Options) == 0 {
			return ProtocolResponse{}, fmt.Errorf("%s question requires options", response.Question.Type)
		}
		if err := ValidateQuestionDefault(*response.Question); err != nil {
			return ProtocolResponse{}, err
		}
		if response.Reason != "" {
			return ProtocolResponse{}, fmt.Errorf("question response cannot contain a reason")
		}
	case "ready":
		if response.Question != nil || response.Reason != "" {
			return ProtocolResponse{}, fmt.Errorf("ready response cannot contain question or reason")
		}
	case "skipped":
		if response.Reason == "" {
			return ProtocolResponse{}, fmt.Errorf("skipped response requires a reason")
		}
		if response.Question != nil {
			return ProtocolResponse{}, fmt.Errorf("skipped response cannot contain a question")
		}
	default:
		return ProtocolResponse{}, fmt.Errorf("unsupported adapter status %q", response.Status)
	}
	return response, nil
}

func ValidateQuestionDefault(question Question) error {
	if *question.Required {
		if question.Default != nil {
			return fmt.Errorf("required question %q cannot define a default", question.ID)
		}
		return nil
	}
	if question.Default == nil {
		return fmt.Errorf("optional question %q requires a default", question.ID)
	}
	options := map[string]bool{}
	for _, option := range question.Options {
		options[option.Value] = true
	}
	switch question.Type {
	case "text":
		if _, valid := question.Default.(string); !valid {
			return fmt.Errorf("text question %q requires a string default", question.ID)
		}
	case "confirm":
		if _, valid := question.Default.(bool); !valid {
			return fmt.Errorf("confirm question %q requires a boolean default", question.ID)
		}
	case "single":
		value, valid := question.Default.(string)
		if !valid || !options[value] {
			return fmt.Errorf("single question %q default must match an option", question.ID)
		}
	case "multiple":
		values, valid := question.Default.([]any)
		if !valid {
			return fmt.Errorf("multiple question %q requires an array default", question.ID)
		}
		for _, raw := range values {
			value, valid := raw.(string)
			if !valid || !options[value] {
				return fmt.Errorf("multiple question %q default must contain only options", question.ID)
			}
		}
	}
	return nil
}

// ErrCancelled identifies a user-cancelled interactive workflow.
var ErrCancelled = errors.New("toolbox batch cancelled")
