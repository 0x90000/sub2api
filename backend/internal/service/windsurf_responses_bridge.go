package service

import (
	"context"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

func (s *WindsurfGatewayService) CompleteResponses(
	ctx context.Context,
	groupID *int64,
	req *apicompat.ResponsesRequest,
) (*apicompat.ResponsesResponse, error) {
	resp, _, err := s.CompleteResponsesWithMetadata(ctx, groupID, req)
	return resp, err
}

func (s *WindsurfGatewayService) CompleteResponsesWithMetadata(
	ctx context.Context,
	groupID *int64,
	req *apicompat.ResponsesRequest,
) (*apicompat.ResponsesResponse, *WindsurfExecutionMetadata, error) {
	if req == nil || strings.TrimSpace(req.Model) == "" {
		return nil, nil, ErrWindsurfModelNotSupported
	}

	anthropicReq, err := apicompat.ResponsesToAnthropicRequest(req)
	if err != nil {
		return nil, nil, err
	}
	candidates, err := s.listChatCompletionAccountSelections(ctx, groupID, req.Model)
	if err != nil {
		return nil, nil, err
	}

	var lastErr error
	for i := range candidates {
		resp, metadata, err := s.completeResponsesWithSelection(ctx, req, anthropicReq, &candidates[i])
		if err == nil {
			return resp, metadata, nil
		}
		lastErr = err
	}
	return nil, nil, lastErr
}

func (s *WindsurfGatewayService) StreamResponses(
	ctx context.Context,
	groupID *int64,
	req *apicompat.ResponsesRequest,
	emit func(apicompat.ResponsesStreamEvent) error,
) error {
	_, err := s.StreamResponsesWithMetadata(ctx, groupID, req, emit)
	return err
}

func (s *WindsurfGatewayService) StreamResponsesWithMetadata(
	ctx context.Context,
	groupID *int64,
	req *apicompat.ResponsesRequest,
	emit func(apicompat.ResponsesStreamEvent) error,
) (*WindsurfExecutionMetadata, error) {
	if req == nil || strings.TrimSpace(req.Model) == "" {
		return nil, ErrWindsurfModelNotSupported
	}
	if emit == nil {
		return nil, errors.New("windsurf responses stream emit callback is required")
	}

	anthropicReq, err := apicompat.ResponsesToAnthropicRequest(req)
	if err != nil {
		return nil, err
	}
	candidates, err := s.listChatCompletionAccountSelections(ctx, groupID, req.Model)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for i := range candidates {
		metadata, err := s.streamResponsesWithSelection(ctx, req, anthropicReq, &candidates[i], emit)
		if err == nil {
			return metadata, nil
		}
		var startedErr *windsurfResponseStreamStartedError
		if errors.As(err, &startedErr) {
			return nil, startedErr.Unwrap()
		}
		lastErr = err
	}
	return nil, lastErr
}

func buildWindsurfResponsesUsage(usage WindsurfBridgeUsage) *apicompat.ResponsesUsage {
	return &apicompat.ResponsesUsage{
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		TotalTokens:  usage.InputTokens + usage.OutputTokens,
		InputTokensDetails: &apicompat.ResponsesInputTokensDetails{
			CachedTokens: usage.CacheReadTokens,
		},
	}
}

func (s *WindsurfGatewayService) completeResponsesWithSelection(
	ctx context.Context,
	req *apicompat.ResponsesRequest,
	anthropicReq *apicompat.AnthropicRequest,
	selection *WindsurfAccountSelection,
) (*apicompat.ResponsesResponse, *WindsurfExecutionMetadata, error) {
	resp, metadata, err := s.completeMessagesWithSelection(ctx, anthropicReq, selection)
	if err != nil {
		return nil, nil, err
	}
	if metadata != nil {
		metadata.RequestedModel = req.Model
	}

	responsesResp := apicompat.AnthropicToResponsesResponse(resp)
	if metadata != nil {
		responsesResp.Usage = buildWindsurfResponsesUsage(metadata.Usage)
	}
	return responsesResp, metadata, nil
}

func (s *WindsurfGatewayService) streamResponsesWithSelection(
	ctx context.Context,
	req *apicompat.ResponsesRequest,
	anthropicReq *apicompat.AnthropicRequest,
	selection *WindsurfAccountSelection,
	emit func(apicompat.ResponsesStreamEvent) error,
) (*WindsurfExecutionMetadata, error) {
	state := apicompat.NewAnthropicEventToResponsesState()
	streamStarted := false

	metadata, err := s.streamMessagesWithSelection(ctx, anthropicReq, selection, func(event apicompat.AnthropicStreamEvent) error {
		for _, responseEvent := range apicompat.AnthropicEventToResponsesEvents(&event, state) {
			if responseEvent.Type == "response.completed" {
				state.CompletedSent = false
				continue
			}
			streamStarted = true
			if err := emit(responseEvent); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if streamStarted {
			return nil, &windsurfResponseStreamStartedError{cause: err}
		}
		return nil, err
	}
	if metadata != nil {
		state.InputTokens = metadata.Usage.InputTokens
		state.OutputTokens = metadata.Usage.OutputTokens
		state.CacheReadInputTokens = metadata.Usage.CacheReadTokens
	}

	for _, responseEvent := range apicompat.FinalizeAnthropicResponsesStream(state) {
		streamStarted = true
		if err := emit(responseEvent); err != nil {
			return nil, err
		}
	}

	if metadata != nil {
		metadata.RequestedModel = req.Model
	}
	return metadata, nil
}

type windsurfResponseStreamStartedError struct {
	cause error
}

func (e *windsurfResponseStreamStartedError) Error() string {
	if e == nil || e.cause == nil {
		return "windsurf response stream already started"
	}
	return e.cause.Error()
}

func (e *windsurfResponseStreamStartedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}
