package service

import (
	"errors"
	"net/http"
)

// WindsurfErrorMapper will translate upstream Windsurf failures into the
// project's existing OpenAI/Anthropic-compatible error shapes.
type WindsurfErrorMapper struct{}

func NewWindsurfErrorMapper() *WindsurfErrorMapper {
	return &WindsurfErrorMapper{}
}

func (m *WindsurfErrorMapper) MapChatCompletionsError(err error) (status int, errType string, message string) {
	switch {
	case errors.Is(err, ErrWindsurfChatBridgeUnavailable):
		return http.StatusNotImplemented, "not_implemented", "Windsurf chat bridge is not implemented yet"
	case errors.Is(err, ErrWindsurfModelNotSupported):
		return http.StatusBadRequest, "invalid_request_error", "Requested model is not available for Windsurf"
	case errors.Is(err, ErrWindsurfNoSchedulableAccounts):
		return http.StatusServiceUnavailable, "api_error", "No available Windsurf accounts"
	default:
		return http.StatusBadGateway, "upstream_error", "Windsurf upstream request failed"
	}
}
