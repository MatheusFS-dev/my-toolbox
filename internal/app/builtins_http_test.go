package app

import (
	"io"
	"net/http"
	"strings"
)

type roundTripFunction func(*http.Request) (*http.Response, error)

func (function roundTripFunction) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
func responseWithBody(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(body))}
}
