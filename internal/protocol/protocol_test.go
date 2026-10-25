package protocol

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
		{"question missing ID", `{"status":"question","question":{"type":"text","title":"Value","required":true}}`},
		{"unsupported question type", `{"status":"question","question":{"id":"x","type":"password","title":"Value","required":true}}`},
		{"optional question missing default", `{"status":"question","question":{"id":"x","type":"text","title":"Value","required":false}}`},
		{"required question with default", `{"status":"question","question":{"id":"x","type":"text","title":"Value","required":true,"default":"x"}}`},
		{"confirm with string default", `{"status":"question","question":{"id":"x","type":"confirm","title":"Continue?","required":false,"default":"yes"}}`},
		{"single default outside options", `{"status":"question","question":{"id":"x","type":"single","title":"Pick","required":false,"default":"z","options":[{"value":"a","label":"A"}]}}`},
		{"multiple default outside options", `{"status":"question","question":{"id":"x","type":"multiple","title":"Pick","required":false,"default":["z"],"options":[{"value":"a","label":"A"}]}}`},
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

func TestDecodeProtocolResponseAcceptsTypedDefaultsAndRequiredInputs(t *testing.T) {
	tests := []string{
		`{"status":"question","question":{"id":"name","type":"text","title":"Name","required":true}}`,
		`{"status":"question","question":{"id":"path","type":"text","title":"Path","required":false,"default":"/path"}}`,
		`{"status":"question","question":{"id":"yes","type":"confirm","title":"Continue?","required":false,"default":true}}`,
		`{"status":"question","question":{"id":"one","type":"single","title":"Pick","required":false,"default":"a","options":[{"value":"a","label":"A"}]}}`,
		`{"status":"question","question":{"id":"many","type":"multiple","title":"Pick","required":false,"default":["a"],"options":[{"value":"a","label":"A"}]}}`,
	}
	for _, input := range tests {
		if _, err := DecodeProtocolResponse(strings.NewReader(input)); err != nil {
			t.Fatalf("DecodeProtocolResponse(%s): %v", input, err)
		}
	}
}
