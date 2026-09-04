package main

import (
	"strings"
	"testing"
)

func TestDecodeProtocolResponseRejectsMalformedStates(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{"unknown status", `{"status":"later"}`},
		{"question missing body", `{"status":"question"}`},
		{"question missing ID", `{"status":"question","question":{"type":"text","title":"Value"}}`},
		{"unsupported question type", `{"status":"question","question":{"id":"x","type":"password","title":"Value"}}`},
		{"skipped missing reason", `{"status":"skipped"}`},
		{"ready with question", `{"status":"ready","question":{"id":"x","type":"text","title":"Value"}}`},
		{"trailing response", `{"status":"ready"}{"status":"ready"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeProtocolResponse(strings.NewReader(test.json)); err == nil {
				t.Fatal("DecodeProtocolResponse() succeeded, want error")
			}
		})
	}
}
