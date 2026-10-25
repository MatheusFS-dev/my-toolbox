package macros

import (
	"net/http"
)

type fakeUI struct {
	answers map[string]any
	err     error
	asked   []Question
}

func (ui *fakeUI) Ask(question Question) (any, error) {
	ui.asked = append(ui.asked, question)
	if ui.err != nil {
		return nil, ui.err
	}
	return ui.answers[question.ID], nil
}

type roundTripFunction func(*http.Request) (*http.Response, error)

func (function roundTripFunction) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
