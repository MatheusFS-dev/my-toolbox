package main

import (
	"encoding/json"
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
	ID      string   `json:"id"`
	Type    string   `json:"type"`
	Title   string   `json:"title"`
	Options []Option `json:"options,omitempty"`
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
		switch response.Question.Type {
		case "text", "confirm", "single", "multiple":
		default:
			return ProtocolResponse{}, fmt.Errorf("unsupported question type %q", response.Question.Type)
		}
		if (response.Question.Type == "single" || response.Question.Type == "multiple") && len(response.Question.Options) == 0 {
			return ProtocolResponse{}, fmt.Errorf("%s question requires options", response.Question.Type)
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
