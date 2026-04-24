package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

func TestWindsurfMessages_NonStreamResponseUsesChatBridge(t *testing.T) {
	bridge := &windsurfChatBridgeStub{
		completeResult: &WindsurfBridgeResult{
			Text:      "anthropic text",
			Reasoning: "anthropic thinking",
		},
	}
	svc := &WindsurfGatewayService{
		accountRepo: windsurfGatewayAccountRepoStub{
			accounts: []Account{
				{
					ID:       41,
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

	resp, err := svc.CompleteMessages(context.Background(), nil, &apicompat.AnthropicRequest{
		Model:  "claude-sonnet-4-20250514",
		System: json.RawMessage(`"system prompt"`),
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"hello"`)}},
	})
	if err != nil {
		t.Fatalf("CompleteMessages() error = %v", err)
	}
	if bridge.completeModel.UpstreamModel != "claude-4-sonnet" {
		t.Fatalf("bridge upstream model = %q, want %q", bridge.completeModel.UpstreamModel, "claude-4-sonnet")
	}
	if len(resp.Content) != 2 {
		t.Fatalf("content len = %d, want 2", len(resp.Content))
	}
	if resp.Content[0].Type != "thinking" || resp.Content[0].Thinking != "anthropic thinking" {
		t.Fatal("expected thinking content block")
	}
	if resp.Content[1].Type != "text" || resp.Content[1].Text != "anthropic text" {
		t.Fatal("expected text content block")
	}
}

func TestWindsurfMessages_NonStreamConvertsToolUse(t *testing.T) {
	bridge := &windsurfChatBridgeStub{
		completeResult: &WindsurfBridgeResult{
			Text: `<tool_call>{"name":"Bash","arguments":{"command":"pwd"}}</tool_call>`,
		},
	}
	svc := &WindsurfGatewayService{
		accountRepo: windsurfGatewayAccountRepoStub{
			accounts: []Account{{
				ID:       43,
				Platform: PlatformWindsurf,
				Type:     AccountTypeAPIKey,
				Extra: map[string]any{
					"allowed_models": []any{"claude-4-sonnet"},
				},
			}},
		},
		chatBridge: bridge,
	}

	resp, err := svc.CompleteMessages(context.Background(), nil, &apicompat.AnthropicRequest{
		Model: "claude-sonnet-4-20250514",
		Tools: []apicompat.AnthropicTool{{
			Name:        "Bash",
			Description: "run shell",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		Messages: []apicompat.AnthropicMessage{{
			Role:    "user",
			Content: json.RawMessage(`"run pwd"`),
		}},
	})
	if err != nil {
		t.Fatalf("CompleteMessages() error = %v", err)
	}
	if len(bridge.completeModel.ModelUID) == 0 {
		t.Fatal("expected cascade model uid")
	}
	if resp.StopReason != "tool_use" {
		t.Fatalf("stop reason = %q, want tool_use", resp.StopReason)
	}
	if len(resp.Content) != 1 || resp.Content[0].Type != "tool_use" {
		t.Fatalf("expected tool_use content, got %+v", resp.Content)
	}
	if resp.Content[0].Name != "Bash" || string(resp.Content[0].Input) != `{"command":"pwd"}` {
		t.Fatalf("unexpected tool_use block: %+v", resp.Content[0])
	}
}

func TestWindsurfMessages_NonStreamUsageUsesGeneratorMetadataFields(t *testing.T) {
	bridge := &windsurfChatBridgeStub{
		completeResult: &WindsurfBridgeResult{
			Text: "usage response",
			Usage: WindsurfBridgeUsage{
				InputTokens:         10,
				OutputTokens:        3,
				CacheCreationTokens: 4,
				CacheReadTokens:     5,
			},
		},
	}
	svc := &WindsurfGatewayService{
		accountRepo: windsurfGatewayAccountRepoStub{
			accounts: []Account{{
				ID:       46,
				Platform: PlatformWindsurf,
				Type:     AccountTypeAPIKey,
				Extra: map[string]any{
					"allowed_models": []any{"claude-4-sonnet"},
				},
			}},
		},
		chatBridge: bridge,
	}

	resp, err := svc.CompleteMessages(context.Background(), nil, &apicompat.AnthropicRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []apicompat.AnthropicMessage{{
			Role:    "user",
			Content: json.RawMessage(`"report usage"`),
		}},
	})
	if err != nil {
		t.Fatalf("CompleteMessages() error = %v", err)
	}
	if resp.Usage.InputTokens != 10 ||
		resp.Usage.OutputTokens != 3 ||
		resp.Usage.CacheCreationInputTokens != 4 ||
		resp.Usage.CacheReadInputTokens != 5 {
		t.Fatalf("usage = %+v, want input/output/cache_creation/cache_read = 10/3/4/5", resp.Usage)
	}
}

func TestWindsurfMessages_NonStreamReusesCascadeAfterToolResult(t *testing.T) {
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
				CascadeID:   "cascade-1",
				SessionID:   "session-1",
				EndpointKey: "",
			},
		},
	}
	svc := &WindsurfGatewayService{
		accountRepo: windsurfGatewayAccountRepoStub{
			accounts: []Account{{
				ID:       45,
				Platform: PlatformWindsurf,
				Type:     AccountTypeAPIKey,
				Extra: map[string]any{
					"allowed_models": []any{"claude-4-sonnet"},
				},
			}},
		},
		chatBridge: bridge,
	}

	firstReq := &apicompat.AnthropicRequest{
		Model: "claude-sonnet-4-20250514",
		Tools: []apicompat.AnthropicTool{{
			Name:        "Bash",
			Description: "run shell",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		Messages: []apicompat.AnthropicMessage{{
			Role:    "user",
			Content: json.RawMessage(`"run pwd"`),
		}},
	}
	if _, err := svc.CompleteMessages(context.Background(), nil, firstReq); err != nil {
		t.Fatalf("first CompleteMessages() error = %v", err)
	}

	secondReq := &apicompat.AnthropicRequest{
		Model: firstReq.Model,
		Tools: firstReq.Tools,
		Messages: []apicompat.AnthropicMessage{
			{
				Role:    "user",
				Content: json.RawMessage(`"run pwd"`),
			},
			{
				Role:    "assistant",
				Content: json.RawMessage(`[{"type":"tool_use","id":"call_1","name":"Bash","input":{"command":"pwd"}}]`),
			},
			{
				Role:    "user",
				Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_1","content":"ok"}]`),
			},
		},
	}
	if _, err := svc.CompleteMessages(context.Background(), nil, secondReq); err != nil {
		t.Fatalf("second CompleteMessages() error = %v", err)
	}

	if len(bridge.completeReuse) != 2 {
		t.Fatalf("bridge complete calls = %d, want 2", len(bridge.completeReuse))
	}
	reuse := bridge.completeReuse[1]
	if reuse == nil || reuse.Entry == nil {
		t.Fatalf("second call reuse context = %#v, want checked-out cascade", reuse)
	}
	if reuse.Entry.CascadeID != "cascade-1" || reuse.Entry.SessionID != "session-1" || reuse.Entry.AccountID != 45 {
		t.Fatalf("reuse entry = %+v, want original cascade/session/account", reuse.Entry)
	}
}

func TestWindsurfMessages_StreamResponseUsesAnthropicEvents(t *testing.T) {
	bridge := &windsurfChatBridgeStub{
		streamChunks: []WindsurfBridgeStreamChunk{
			{Reasoning: "step1"},
			{Text: "hello"},
		},
	}
	svc := &WindsurfGatewayService{
		accountRepo: windsurfGatewayAccountRepoStub{
			accounts: []Account{
				{
					ID:       42,
					Platform: PlatformWindsurf,
					Type:     AccountTypeAPIKey,
					Extra: map[string]any{
						"allowed_models": []any{"gpt-4.1"},
					},
				},
			},
		},
		chatBridge: bridge,
	}

	var events []apicompat.AnthropicStreamEvent
	err := svc.StreamMessages(context.Background(), nil, &apicompat.AnthropicRequest{
		Model:  "gpt-4.1",
		Stream: true,
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"hello"`)}},
	}, func(event apicompat.AnthropicStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamMessages() error = %v", err)
	}
	if len(events) < 7 {
		t.Fatalf("events len = %d, want at least 7", len(events))
	}
	if events[0].Type != "message_start" {
		t.Fatalf("first event = %q, want message_start", events[0].Type)
	}
	if events[len(events)-1].Type != "message_stop" {
		t.Fatalf("last event = %q, want message_stop", events[len(events)-1].Type)
	}
}

func TestWindsurfMessages_StreamToolCallsUseSeparateContentBlocks(t *testing.T) {
	bridge := &windsurfChatBridgeStub{
		streamChunks: []WindsurfBridgeStreamChunk{
			{
				ToolCalls: []apicompat.ChatToolCall{
					{
						ID:   "call_1",
						Type: "function",
						Function: apicompat.ChatFunctionCall{
							Name:      "Bash",
							Arguments: `{"command":"pwd"}`,
						},
					},
					{
						ID:   "call_2",
						Type: "function",
						Function: apicompat.ChatFunctionCall{
							Name:      "Read",
							Arguments: `{"file_path":"README.md"}`,
						},
					},
				},
			},
		},
	}
	svc := &WindsurfGatewayService{
		accountRepo: windsurfGatewayAccountRepoStub{
			accounts: []Account{{
				ID:       44,
				Platform: PlatformWindsurf,
				Type:     AccountTypeAPIKey,
				Extra: map[string]any{
					"allowed_models": []any{"claude-4-sonnet"},
				},
			}},
		},
		chatBridge: bridge,
	}

	var events []apicompat.AnthropicStreamEvent
	err := svc.StreamMessages(context.Background(), nil, &apicompat.AnthropicRequest{
		Model:  "claude-sonnet-4-20250514",
		Stream: true,
		Tools: []apicompat.AnthropicTool{
			{Name: "Bash", InputSchema: json.RawMessage(`{"type":"object"}`)},
			{Name: "Read", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
		Messages: []apicompat.AnthropicMessage{{
			Role:    "user",
			Content: json.RawMessage(`"inspect workspace"`),
		}},
	}, func(event apicompat.AnthropicStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamMessages() error = %v", err)
	}

	var toolStarts []apicompat.AnthropicContentBlock
	var toolDeltas int
	for _, event := range events {
		if event.Type == "content_block_start" && event.ContentBlock != nil && event.ContentBlock.Type == "tool_use" {
			toolStarts = append(toolStarts, *event.ContentBlock)
		}
		if event.Type == "content_block_delta" && event.Delta != nil && event.Delta.Type == "input_json_delta" {
			toolDeltas++
		}
	}
	if len(toolStarts) != 2 {
		t.Fatalf("tool starts len = %d, want 2; events=%+v", len(toolStarts), events)
	}
	if toolStarts[0].Name != "Bash" || toolStarts[1].Name != "Read" {
		t.Fatalf("tool start names = [%q %q], want [Bash Read]", toolStarts[0].Name, toolStarts[1].Name)
	}
	if toolDeltas != 2 {
		t.Fatalf("tool deltas = %d, want 2", toolDeltas)
	}
}

func TestWindsurfAnthropicMessageToChatKeepsToolResultAndFollowUpInSameUserTurn(t *testing.T) {
	messages := convertWindsurfAnthropicMessageToChat(apicompat.AnthropicMessage{
		Role: "user",
		Content: json.RawMessage(`[
			{"type":"tool_result","tool_use_id":"toolu_1","content":"tool output"},
			{"type":"text","text":"continue after tool"}
		]`),
	})
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1: %+v", len(messages), messages)
	}
	if messages[0].Role != "user" {
		t.Fatalf("role = %q, want user", messages[0].Role)
	}
	text := windsurfChatContentToString(messages[0].Content)
	if !strings.Contains(text, `<tool_result tool_call_id="toolu_1">`) || !strings.Contains(text, "tool output") || !strings.Contains(text, "continue after tool") {
		t.Fatalf("content = %q, want combined tool_result markup and follow-up text", text)
	}
}

func TestWindsurfAnthropicMessageToChatCombinesToolResultAndTextInOneUserTurn(t *testing.T) {
	messages := convertWindsurfAnthropicMessageToChat(apicompat.AnthropicMessage{
		Role: "user",
		Content: json.RawMessage(`[
			{"type":"tool_result","tool_use_id":"toolu_1","content":"tool output"},
			{"type":"text","text":"continue after tool"}
		]`),
	})
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1 combined user turn: %+v", len(messages), messages)
	}
	if messages[0].Role != "user" {
		t.Fatalf("message role = %q, want user", messages[0].Role)
	}
	text := windsurfChatContentToString(messages[0].Content)
	if !strings.Contains(text, `<tool_result tool_call_id="toolu_1">`) || !strings.Contains(text, "tool output") || !strings.Contains(text, "continue after tool") {
		t.Fatalf("combined content = %q, want tool_result markup and follow-up text", text)
	}
}
