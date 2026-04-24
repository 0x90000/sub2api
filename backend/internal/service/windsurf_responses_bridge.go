package service

import (
	"context"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

type windsurfChatRequestAttempt struct {
	selection *WindsurfAccountSelection
	ctx       context.Context
}

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
	chatReq, err := convertWindsurfAnthropicToChatRequest(anthropicReq)
	if err != nil {
		return nil, nil, err
	}
	attempts, err := s.responseChatRequestAttempts(ctx, groupID, chatReq)
	if err != nil {
		return nil, nil, err
	}

	var lastErr error
	for _, attempt := range attempts {
		resp, metadata, err := s.completeResponsesWithChatRequest(attempt.ctx, groupID, req, anthropicReq, chatReq, attempt.selection)
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
	chatReq, err := convertWindsurfAnthropicToChatRequest(anthropicReq)
	if err != nil {
		return nil, err
	}
	attempts, err := s.responseChatRequestAttempts(ctx, groupID, chatReq)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for _, attempt := range attempts {
		attemptStarted := false
		metadata, err := s.streamResponsesWithChatRequest(attempt.ctx, groupID, req, anthropicReq, chatReq, attempt.selection, func(event apicompat.ResponsesStreamEvent) error {
			attemptStarted = true
			return emit(event)
		})
		if err == nil {
			return metadata, nil
		}
		if attemptStarted {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

func (s *WindsurfGatewayService) responseChatRequestAttempts(
	ctx context.Context,
	groupID *int64,
	chatReq *apicompat.ChatCompletionsRequest,
) ([]windsurfChatRequestAttempt, error) {
	primary, primaryCtx, err := s.selectChatCompletionAccountForRequest(ctx, groupID, chatReq)
	if err != nil {
		return nil, err
	}

	candidates, err := s.listChatCompletionAccountSelections(ctx, groupID, chatReq.Model)
	if err != nil {
		return nil, err
	}

	scope := windsurfConversationScopeForRequest(ctx, groupID)
	baseCtx := WithWindsurfConversationScope(ctx, scope)
	attempts := make([]windsurfChatRequestAttempt, 0, len(candidates))
	seenAccounts := make(map[int64]struct{}, len(candidates))

	appendAttempt := func(selection *WindsurfAccountSelection, attemptCtx context.Context) {
		if selection == nil || selection.Account == nil {
			return
		}
		if _, ok := seenAccounts[selection.Account.ID]; ok {
			return
		}
		seenAccounts[selection.Account.ID] = struct{}{}
		attempts = append(attempts, windsurfChatRequestAttempt{
			selection: selection,
			ctx:       attemptCtx,
		})
	}

	appendAttempt(primary, primaryCtx)
	for i := range candidates {
		candidate := candidates[i]
		appendAttempt(&candidate, baseCtx)
	}
	return attempts, nil
}

func buildWindsurfResponsesUsage(usage WindsurfBridgeUsage) *apicompat.ResponsesUsage {
	inputTokens := usage.InputTokens + usage.CacheReadTokens + usage.CacheCreationTokens
	outputTokens := usage.OutputTokens + usage.ImageOutputTokens
	return &apicompat.ResponsesUsage{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  inputTokens + outputTokens,
		InputTokensDetails: &apicompat.ResponsesInputTokensDetails{
			CachedTokens: usage.CacheReadTokens,
		},
	}
}

func (s *WindsurfGatewayService) completeResponsesWithSelection(
	ctx context.Context,
	groupID *int64,
	req *apicompat.ResponsesRequest,
	anthropicReq *apicompat.AnthropicRequest,
	selection *WindsurfAccountSelection,
) (*apicompat.ResponsesResponse, *WindsurfExecutionMetadata, error) {
	chatReq, err := convertWindsurfAnthropicToChatRequest(anthropicReq)
	if err != nil {
		return nil, nil, err
	}
	return s.completeResponsesWithChatRequest(ctx, groupID, req, anthropicReq, chatReq, selection)
}

func (s *WindsurfGatewayService) completeResponsesWithChatRequest(
	ctx context.Context,
	groupID *int64,
	req *apicompat.ResponsesRequest,
	anthropicReq *apicompat.AnthropicRequest,
	chatReq *apicompat.ChatCompletionsRequest,
	selection *WindsurfAccountSelection,
) (*apicompat.ResponsesResponse, *WindsurfExecutionMetadata, error) {
	resp, metadata, err := s.completeMessagesWithChatRequest(ctx, groupID, anthropicReq, chatReq, selection)
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
	groupID *int64,
	req *apicompat.ResponsesRequest,
	anthropicReq *apicompat.AnthropicRequest,
	selection *WindsurfAccountSelection,
	emit func(apicompat.ResponsesStreamEvent) error,
) (*WindsurfExecutionMetadata, error) {
	chatReq, err := convertWindsurfAnthropicToChatRequest(anthropicReq)
	if err != nil {
		return nil, err
	}
	return s.streamResponsesWithChatRequest(ctx, groupID, req, anthropicReq, chatReq, selection, emit)
}

func (s *WindsurfGatewayService) streamResponsesWithChatRequest(
	ctx context.Context,
	groupID *int64,
	req *apicompat.ResponsesRequest,
	anthropicReq *apicompat.AnthropicRequest,
	chatReq *apicompat.ChatCompletionsRequest,
	selection *WindsurfAccountSelection,
	emit func(apicompat.ResponsesStreamEvent) error,
) (*WindsurfExecutionMetadata, error) {
	state := apicompat.NewAnthropicEventToResponsesState()
	streamStarted := false

	metadata, err := s.streamMessagesWithChatRequest(ctx, groupID, anthropicReq, chatReq, selection, func(event apicompat.AnthropicStreamEvent) error {
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
		state.InputTokens = metadata.Usage.InputTokens + metadata.Usage.CacheReadTokens + metadata.Usage.CacheCreationTokens
		state.OutputTokens = metadata.Usage.OutputTokens + metadata.Usage.ImageOutputTokens
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
