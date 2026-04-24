package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

func TestWindsurfChatCompletions_NonStreamResponseUsesBridge(t *testing.T) {
	groupID := int64(9)
	bridge := &windsurfChatBridgeStub{
		completeResult: &WindsurfBridgeResult{
			Text:      "hello from windsurf",
			Reasoning: "trace",
		},
	}
	svc := &WindsurfGatewayService{
		accountRepo: windsurfGatewayAccountRepoStub{
			accounts: []Account{
				{
					ID:       31,
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

	resp, err := svc.CompleteChatCompletions(context.Background(), &groupID, &apicompat.ChatCompletionsRequest{
		Model: "gpt-4o",
		Messages: []apicompat.ChatMessage{
			{Role: "user"},
		},
	})
	if err != nil {
		t.Fatalf("CompleteChatCompletions() error = %v", err)
	}
	if bridge.completeModel.UpstreamModel != "gpt-4.1" {
		t.Fatalf("bridge upstream model = %q, want %q", bridge.completeModel.UpstreamModel, "gpt-4.1")
	}
	if bridge.completeModel.ModelUID != "MODEL_CHAT_GPT_4_1_2025_04_14" {
		t.Fatalf("bridge model uid = %q, want %q", bridge.completeModel.ModelUID, "MODEL_CHAT_GPT_4_1_2025_04_14")
	}
	if resp.Model != "gpt-4o" {
		t.Fatalf("response model = %q, want %q", resp.Model, "gpt-4o")
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("choices len = %d, want 1", len(resp.Choices))
	}
	var content string
	if err := json.Unmarshal(resp.Choices[0].Message.Content, &content); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if content != "hello from windsurf" {
		t.Fatalf("content = %q, want %q", content, "hello from windsurf")
	}
	if resp.Choices[0].Message.ReasoningContent != "trace" {
		t.Fatalf("reasoning = %q, want %q", resp.Choices[0].Message.ReasoningContent, "trace")
	}
}

func TestWindsurfChatCompletions_StreamResponseUsesBridgeChunks(t *testing.T) {
	bridge := &windsurfChatBridgeStub{
		streamChunks: []WindsurfBridgeStreamChunk{
			{Reasoning: "thinking"},
			{Text: "hello"},
			{Text: " world"},
		},
	}
	svc := &WindsurfGatewayService{
		accountRepo: windsurfGatewayAccountRepoStub{
			accounts: []Account{
				{
					ID:       32,
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

	var chunks []apicompat.ChatCompletionsChunk
	err := svc.StreamChatCompletions(context.Background(), nil, &apicompat.ChatCompletionsRequest{
		Model:  "claude-sonnet-4-20250514",
		Stream: true,
	}, func(chunk apicompat.ChatCompletionsChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamChatCompletions() error = %v", err)
	}
	if bridge.streamModel.UpstreamModel != "claude-4-sonnet" {
		t.Fatalf("bridge upstream model = %q, want %q", bridge.streamModel.UpstreamModel, "claude-4-sonnet")
	}
	if len(chunks) != 5 {
		t.Fatalf("chunks len = %d, want 5", len(chunks))
	}
	if chunks[0].Choices[0].Delta.Role != "assistant" {
		t.Fatalf("first chunk role = %q, want assistant", chunks[0].Choices[0].Delta.Role)
	}
	if chunks[1].Choices[0].Delta.ReasoningContent == nil || *chunks[1].Choices[0].Delta.ReasoningContent != "thinking" {
		t.Fatal("expected reasoning delta chunk")
	}
	if chunks[2].Choices[0].Delta.Content == nil || *chunks[2].Choices[0].Delta.Content != "hello" {
		t.Fatal("expected first text delta chunk")
	}
	if chunks[3].Choices[0].Delta.Content == nil || *chunks[3].Choices[0].Delta.Content != " world" {
		t.Fatal("expected second text delta chunk")
	}
	if chunks[4].Choices[0].FinishReason == nil || *chunks[4].Choices[0].FinishReason != "stop" {
		t.Fatal("expected final stop chunk")
	}
}

func TestWindsurfChatCompletions_NonStreamParsesToolCall(t *testing.T) {
	bridge := &windsurfChatBridgeStub{
		completeResult: &WindsurfBridgeResult{
			Text: `<tool_call>{"name":"Bash","arguments":{"command":"pwd"}}</tool_call>`,
		},
	}
	svc := &WindsurfGatewayService{
		accountRepo: windsurfGatewayAccountRepoStub{
			accounts: []Account{{
				ID:       33,
				Platform: PlatformWindsurf,
				Type:     AccountTypeAPIKey,
				Extra: map[string]any{
					"allowed_models": []any{"claude-4-sonnet"},
				},
			}},
		},
		chatBridge: bridge,
	}

	resp, err := svc.CompleteChatCompletions(context.Background(), nil, &apicompat.ChatCompletionsRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []apicompat.ChatMessage{{
			Role:    "user",
			Content: json.RawMessage(`"run pwd"`),
		}},
		Tools: []apicompat.ChatTool{{
			Type: "function",
			Function: &apicompat.ChatFunction{
				Name:       "Bash",
				Parameters: json.RawMessage(`{"type":"object"}`),
			},
		}},
	})
	if err != nil {
		t.Fatalf("CompleteChatCompletions() error = %v", err)
	}
	if resp.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("finish reason = %q, want tool_calls", resp.Choices[0].FinishReason)
	}
	calls := resp.Choices[0].Message.ToolCalls
	if len(calls) != 1 {
		t.Fatalf("tool calls len = %d, want 1", len(calls))
	}
	if calls[0].Function.Name != "Bash" || calls[0].Function.Arguments != `{"command":"pwd"}` {
		t.Fatalf("unexpected tool call: %+v", calls[0].Function)
	}
}

func TestWindsurfChatCompletions_StreamEmitsToolCallDelta(t *testing.T) {
	bridge := &windsurfChatBridgeStub{
		streamChunks: []WindsurfBridgeStreamChunk{
			{Text: `<tool_call>{"name":"Write","arguments":{"file_path":"/tmp/wind`},
			{Text: `surf-workspace/output.txt","content":"ok"}}</tool_call>`},
		},
		streamResult: &WindsurfBridgeResult{},
	}
	svc := &WindsurfGatewayService{
		accountRepo: windsurfGatewayAccountRepoStub{
			accounts: []Account{{
				ID:       34,
				Platform: PlatformWindsurf,
				Type:     AccountTypeAPIKey,
				Extra: map[string]any{
					"allowed_models": []any{"claude-4-sonnet"},
				},
			}},
		},
		chatBridge: bridge,
	}

	var chunks []apicompat.ChatCompletionsChunk
	err := svc.StreamChatCompletions(context.Background(), nil, &apicompat.ChatCompletionsRequest{
		Model:  "claude-sonnet-4-20250514",
		Stream: true,
		Messages: []apicompat.ChatMessage{{
			Role:    "user",
			Content: json.RawMessage(`"list files"`),
		}},
		Tools: []apicompat.ChatTool{{
			Type:     "function",
			Function: &apicompat.ChatFunction{Name: "Write"},
		}},
	}, func(chunk apicompat.ChatCompletionsChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamChatCompletions() error = %v", err)
	}
	var sawTool bool
	var finish string
	for _, chunk := range chunks {
		if len(chunk.Choices) == 0 {
			continue
		}
		if len(chunk.Choices[0].Delta.ToolCalls) > 0 {
			sawTool = true
		}
		if chunk.Choices[0].FinishReason != nil {
			finish = *chunk.Choices[0].FinishReason
		}
	}
	if !sawTool {
		t.Fatal("expected tool call delta")
	}
	if finish != "tool_calls" {
		t.Fatalf("finish = %q, want tool_calls", finish)
	}
	for _, chunk := range chunks {
		if len(chunk.Choices) == 0 || len(chunk.Choices[0].Delta.ToolCalls) == 0 {
			continue
		}
		args := chunk.Choices[0].Delta.ToolCalls[0].Function.Arguments
		if containsWindsurfInternalPath(args) {
			t.Fatalf("tool call args leaked internal path: %q", args)
		}
		if args != `{"content":"ok","file_path":"./output.txt"}` && args != `{"file_path":"./output.txt","content":"ok"}` {
			t.Fatalf("unexpected sanitized args: %q", args)
		}
	}
}

func TestWindsurfChatCompletions_NonStreamCacheHitSkipsBridge(t *testing.T) {
	cache := newWindsurfResponseCache(time.Minute, time.Minute)
	oldCache := defaultWindsurfResponseCache
	defaultWindsurfResponseCache = cache
	defer func() { defaultWindsurfResponseCache = oldCache }()

	bridge := &windsurfChatBridgeStub{
		completeResult: &WindsurfBridgeResult{
			Text: "cached text",
			Usage: WindsurfBridgeUsage{
				InputTokens:  4,
				OutputTokens: 2,
			},
		},
	}
	svc := &WindsurfGatewayService{
		accountRepo: windsurfGatewayAccountRepoStub{
			accounts: []Account{{
				ID:       35,
				Platform: PlatformWindsurf,
				Type:     AccountTypeAPIKey,
				Extra: map[string]any{
					"allowed_models": []any{"gpt-4.1"},
				},
			}},
		},
		chatBridge: bridge,
	}
	req := &apicompat.ChatCompletionsRequest{
		Model: "gpt-4.1",
		Messages: []apicompat.ChatMessage{{
			Role:    "user",
			Content: json.RawMessage(`"hello cache"`),
		}},
	}
	if _, err := svc.CompleteChatCompletions(context.Background(), nil, req); err != nil {
		t.Fatalf("first CompleteChatCompletions() error = %v", err)
	}
	if bridge.completeCalls != 1 {
		t.Fatalf("bridge calls after first request = %d, want 1", bridge.completeCalls)
	}
	bridge.completeResult = &WindsurfBridgeResult{Text: "should not be used"}
	resp, err := svc.CompleteChatCompletions(context.Background(), nil, req)
	if err != nil {
		t.Fatalf("second CompleteChatCompletions() error = %v", err)
	}
	if bridge.completeCalls != 1 {
		t.Fatalf("bridge calls after cache hit = %d, want 1", bridge.completeCalls)
	}
	var content string
	if err := json.Unmarshal(resp.Choices[0].Message.Content, &content); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if content != "cached text" {
		t.Fatalf("content = %q, want cached text", content)
	}
	if resp.Usage == nil || resp.Usage.PromptTokensDetails == nil || resp.Usage.PromptTokensDetails.CachedTokens != 4 {
		t.Fatalf("expected cached token usage, got %+v", resp.Usage)
	}
}

func TestWindsurfChatCompletions_CacheIsScopedByAccount(t *testing.T) {
	cache := newWindsurfResponseCache(time.Minute, time.Minute)
	oldCache := defaultWindsurfResponseCache
	defaultWindsurfResponseCache = cache
	defer func() { defaultWindsurfResponseCache = oldCache }()

	bridge := &windsurfChatBridgeStub{
		completeResult: &WindsurfBridgeResult{Text: "account one"},
	}
	svc := &WindsurfGatewayService{chatBridge: bridge}
	req := &apicompat.ChatCompletionsRequest{
		Model: "gpt-4.1",
		Messages: []apicompat.ChatMessage{{
			Role:    "user",
			Content: json.RawMessage(`"same prompt"`),
		}},
	}

	_, _, err := svc.completeChatCompletionsWithSelection(context.Background(), nil, req.Model, req, &WindsurfAccountSelection{
		Account: &Account{ID: 101, Platform: PlatformWindsurf, Type: AccountTypeAPIKey},
		Model:   WindsurfResolvedModel{UpstreamModel: "gpt-4.1", ModelUID: "MODEL_A"},
	})
	if err != nil {
		t.Fatalf("first completeChatCompletionsWithSelection() error = %v", err)
	}

	bridge.completeResult = &WindsurfBridgeResult{Text: "account two"}
	resp, _, err := svc.completeChatCompletionsWithSelection(context.Background(), nil, req.Model, req, &WindsurfAccountSelection{
		Account: &Account{ID: 202, Platform: PlatformWindsurf, Type: AccountTypeAPIKey},
		Model:   WindsurfResolvedModel{UpstreamModel: "gpt-4.1", ModelUID: "MODEL_B"},
	})
	if err != nil {
		t.Fatalf("second completeChatCompletionsWithSelection() error = %v", err)
	}
	if bridge.completeCalls != 2 {
		t.Fatalf("bridge calls = %d, want 2", bridge.completeCalls)
	}
	var content string
	if err := json.Unmarshal(resp.Choices[0].Message.Content, &content); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if content != "account two" {
		t.Fatalf("content = %q, want account two", content)
	}
}

type windsurfChatBridgeStub struct {
	completeResult *WindsurfBridgeResult
	streamChunks   []WindsurfBridgeStreamChunk
	streamResult   *WindsurfBridgeResult
	completeModel  WindsurfResolvedModel
	streamModel    WindsurfResolvedModel
	completeCalls  int
	completeReuse  []*windsurfConversationReuseContext
}

func (s *windsurfChatBridgeStub) Complete(
	ctx context.Context,
	_ *Account,
	model WindsurfResolvedModel,
	_ *apicompat.ChatCompletionsRequest,
) (*WindsurfBridgeResult, error) {
	s.completeModel = model
	s.completeCalls++
	s.completeReuse = append(s.completeReuse, windsurfConversationReuseFromContext(ctx))
	return s.completeResult, nil
}

func (s *windsurfChatBridgeStub) Stream(
	_ context.Context,
	_ *Account,
	model WindsurfResolvedModel,
	_ *apicompat.ChatCompletionsRequest,
	emit func(WindsurfBridgeStreamChunk) error,
) (*WindsurfBridgeResult, error) {
	s.streamModel = model
	for _, chunk := range s.streamChunks {
		if err := emit(chunk); err != nil {
			return nil, err
		}
	}
	return s.streamResult, nil
}
