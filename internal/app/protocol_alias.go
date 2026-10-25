package app

import (
	"io"

	"github.com/MatheusFS-dev/my-toolbox/internal/protocol"
)

type Option = protocol.Option
type Question = protocol.Question
type ProtocolResponse = protocol.ProtocolResponse
type ProtocolRequest = protocol.ProtocolRequest
type PackageContext = protocol.PackageContext

func questionRequired(value bool) *bool { return protocol.QuestionRequired(value) }
func DecodeProtocolResponse(reader io.Reader) (ProtocolResponse, error) {
	return protocol.DecodeProtocolResponse(reader)
}
func validateQuestionDefault(question Question) error {
	return protocol.ValidateQuestionDefault(question)
}
