package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

func TestWindsurfResponsesBridge_NonStreamResponseUsesMessagesConversion(t *testing.T) {
	groupID := int64(9)
	bridge := &windsurfChatBridgeStub{
		completeResult: &WindsurfBridgeResult{
			RequestID: "resp_test_non_stream",
			Text:      "hello from responses",
			Reasoning: "reasoning trace",
			Usage: WindsurfBridgeUsage{
				InputTokens:     12,
				OutputTokens:    34,
				CacheReadTokens: 5,
			},
		},
	}
	svc := &WindsurfGatewayService{
		accountRepo: windsurfGatewayAccountRepoStub{
			accounts: []Account{
				{
					ID:       51,
					Platform: PlatformWindsurf,
					Type:     AccountTypeAPIKey,
					Extra: map[string]any{
						"allowed_models": []any{"gpt-4.1"},
						"model_configs": []any{
							map[string]any{
								"id":        "gpt-4.1",
								"model_uid": "MODEL_CHAT_GPT_4_1_2025_04_14",
								"provider":  "MODEL_PROVIDER_OPENAI",
							},
						},
					},
					Credentials: map[string]any{
						"model_mapping": map[string]any{
							"gpt-4o": "gpt-4.1",
						},
					},
				},
			},
		},
		chatBridge: bridge,
	}

	resp, metadata, err := svc.CompleteResponsesWithMetadata(context.Background(), &groupID, &apicompat.ResponsesRequest{
		Model: "gpt-4o",
		Input: []byte(`[{"role":"user","content":"hello"}]`),
	})
	if err != nil {
		t.Fatalf("CompleteResponsesWithMetadata() error = %v", err)
	}
	if bridge.completeModel.UpstreamModel != "gpt-4.1" {
		t.Fatalf("bridge upstream model = %q, want %q", bridge.completeModel.UpstreamModel, "gpt-4.1")
	}
	if resp.Model != "gpt-4o" {
		t.Fatalf("response model = %q, want %q", resp.Model, "gpt-4o")
	}
	if resp.Status != "completed" {
		t.Fatalf("response status = %q, want %q", resp.Status, "completed")
	}
	if len(resp.Output) != 2 {
		t.Fatalf("output len = %d, want 2", len(resp.Output))
	}
	if resp.Output[0].Type != "reasoning" || len(resp.Output[0].Summary) != 1 || resp.Output[0].Summary[0].Text != "reasoning trace" {
		t.Fatal("expected reasoning output item")
	}
	if resp.Output[1].Type != "message" || len(resp.Output[1].Content) != 1 || resp.Output[1].Content[0].Text != "hello from responses" {
		t.Fatal("expected message output item")
	}
	if resp.Usage == nil || resp.Usage.InputTokens != 17 || resp.Usage.OutputTokens != 34 || resp.Usage.TotalTokens != 51 {
		t.Fatal("expected responses usage to be mapped")
	}
	if resp.Usage.InputTokensDetails == nil || resp.Usage.InputTokensDetails.CachedTokens != 5 {
		t.Fatal("expected cached input tokens to be mapped")
	}
	if metadata == nil {
		t.Fatal("expected execution metadata")
	}
	if metadata.RequestID != "resp_test_non_stream" {
		t.Fatalf("metadata request id = %q, want %q", metadata.RequestID, "resp_test_non_stream")
	}
	if metadata.RequestedModel != "gpt-4o" {
		t.Fatalf("metadata requested model = %q, want %q", metadata.RequestedModel, "gpt-4o")
	}
	if metadata.UpstreamModel != "gpt-4.1" {
		t.Fatalf("metadata upstream model = %q, want %q", metadata.UpstreamModel, "gpt-4.1")
	}
	if metadata.Usage.OutputTokens != 34 {
		t.Fatalf("metadata output tokens = %d, want %d", metadata.Usage.OutputTokens, 34)
	}
}

func TestWindsurfResponsesBridge_NonStreamReusesCascadeAfterFunctionOutput(t *testing.T) {
	oldPool := defaultWindsurfConversationPool
	defaultWindsurfConversationPool = newWindsurfConversationPool(time.Minute, 8)
	defer func() { defaultWindsurfConversationPool = oldPool }()

	bridge := &windsurfChatBridgeStub{
		completeResult: &WindsurfBridgeResult{
			ToolCalls: []apicompat.ChatToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: apicompat.ChatFunctionCall{
					Name:      "Bash",
					Arguments: `{"command":"pwd"}`,
				},
			}},
			conversation: &windsurfConversationResult{
				CascadeID:   "cascade-responses-1",
				SessionID:   "session-responses-1",
				EndpointKey: "",
			},
		},
	}
	svc := &WindsurfGatewayService{
		accountRepo: windsurfGatewayAccountRepoStub{
			accounts: []Account{{
				ID:       53,
				Platform: PlatformWindsurf,
				Type:     AccountTypeAPIKey,
				Extra: map[string]any{
					"allowed_models": []any{"claude-4-sonnet"},
				},
			}},
		},
		chatBridge: bridge,
	}

	tools := []apicompat.ResponsesTool{{
		Type:       "function",
		Name:       "Bash",
		Parameters: json.RawMessage(`{"type":"object"}`),
	}}
	firstReq := &apicompat.ResponsesRequest{
		Model: "claude-sonnet-4-20250514",
		Tools: tools,
		Input: json.RawMessage(`[{"role":"user","content":"run pwd"}]`),
	}
	if _, _, err := svc.CompleteResponsesWithMetadata(context.Background(), nil, firstReq); err != nil {
		t.Fatalf("first CompleteResponsesWithMetadata() error = %v", err)
	}

	secondReq := &apicompat.ResponsesRequest{
		Model: firstReq.Model,
		Tools: tools,
		Input: json.RawMessage(`[
			{"role":"user","content":"run pwd"},
			{"type":"function_call","call_id":"call_1","name":"Bash","arguments":"{\"command\":\"pwd\"}"},
			{"type":"function_call_output","call_id":"call_1","output":"ok"}
		]`),
	}
	if _, _, err := svc.CompleteResponsesWithMetadata(context.Background(), nil, secondReq); err != nil {
		t.Fatalf("second CompleteResponsesWithMetadata() error = %v", err)
	}

	if len(bridge.completeReuse) != 2 {
		t.Fatalf("bridge complete calls = %d, want 2", len(bridge.completeReuse))
	}
	reuse := bridge.completeReuse[1]
	if reuse == nil || reuse.Entry == nil {
		t.Fatalf("second call reuse context = %#v, want checked-out cascade", reuse)
	}
	if reuse.Entry.CascadeID != "cascade-responses-1" || reuse.Entry.SessionID != "session-responses-1" || reuse.Entry.AccountID != 53 {
		t.Fatalf("reuse entry = %+v, want original responses cascade/session/account", reuse.Entry)
	}
}

func TestWindsurfResponsesBridge_StreamResponseEmitsResponsesEvents(t *testing.T) {
	bridge := &windsurfChatBridgeStub{
		streamChunks: []WindsurfBridgeStreamChunk{
			{Reasoning: "step1"},
			{Text: "hello"},
			{Text: " world"},
		},
		streamResult: &WindsurfBridgeResult{
			RequestID: "resp_test_stream",
			Usage: WindsurfBridgeUsage{
				InputTokens:       7,
				OutputTokens:      9,
				CacheReadTokens:   5,
				ImageOutputTokens: 2,
			},
		},
	}
	svc := &WindsurfGatewayService{
		accountRepo: windsurfGatewayAccountRepoStub{
			accounts: []Account{
				{
					ID:       52,
					Platform: PlatformWindsurf,
					Type:     AccountTypeAPIKey,
					Extra: map[string]any{
						"allowed_models": []any{"claude-4-sonnet"},
					},
				},
			},
		},
		chatBridge: bridge,
	}

	var events []apicompat.ResponsesStreamEvent
	metadata, err := svc.StreamResponsesWithMetadata(context.Background(), nil, &apicompat.ResponsesRequest{
		Model:  "claude-sonnet-4-20250514",
		Stream: true,
		Input:  []byte(`[{"role":"user","content":"hello"}]`),
	}, func(event apicompat.ResponsesStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamResponsesWithMetadata() error = %v", err)
	}
	if bridge.streamModel.UpstreamModel != "claude-4-sonnet" {
		t.Fatalf("bridge upstream model = %q, want %q", bridge.streamModel.UpstreamModel, "claude-4-sonnet")
	}
	if metadata == nil {
		t.Fatal("expected execution metadata")
	}
	if metadata.RequestID != "resp_test_stream" {
		t.Fatalf("metadata request id = %q, want %q", metadata.RequestID, "resp_test_stream")
	}
	if metadata.Usage.InputTokens != 7 || metadata.Usage.OutputTokens != 9 {
		t.Fatal("expected stream metadata usage to be mapped")
	}
	if len(events) < 6 {
		t.Fatalf("events len = %d, want at least 6", len(events))
	}
	if events[0].Type != "response.created" {
		t.Fatalf("first event = %q, want response.created", events[0].Type)
	}
	if events[len(events)-1].Type != "response.completed" {
		t.Fatalf("last event = %q, want response.completed", events[len(events)-1].Type)
	}

	foundReasoningDelta := false
	foundTextDelta := false
	for _, event := range events {
		if event.Type == "response.reasoning_summary_text.delta" && event.Delta == "step1" {
			foundReasoningDelta = true
		}
		if event.Type == "response.output_text.delta" && event.Delta == "hello" {
			foundTextDelta = true
		}
	}
	if !foundReasoningDelta {
		t.Fatal("expected reasoning delta event")
	}
	if !foundTextDelta {
		t.Fatal("expected text delta event")
	}

	completed := events[len(events)-1].Response
	if completed == nil || completed.Usage == nil || completed.Usage.InputTokens != 12 || completed.Usage.OutputTokens != 11 || completed.Usage.TotalTokens != 23 {
		t.Fatal("expected completed event usage to be populated")
	}
	if completed.Usage.InputTokensDetails == nil || completed.Usage.InputTokensDetails.CachedTokens != 5 {
		t.Fatal("expected completed event cached token usage to be populated")
	}
}
