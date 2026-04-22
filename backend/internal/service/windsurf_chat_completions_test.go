package service

import (
	"context"
	"encoding/json"
	"testing"

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

type windsurfChatBridgeStub struct {
	completeResult *WindsurfBridgeResult
	streamChunks   []WindsurfBridgeStreamChunk
	completeModel  WindsurfResolvedModel
	streamModel    WindsurfResolvedModel
}

func (s *windsurfChatBridgeStub) Complete(
	_ context.Context,
	_ *Account,
	model WindsurfResolvedModel,
	_ *apicompat.ChatCompletionsRequest,
) (*WindsurfBridgeResult, error) {
	s.completeModel = model
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
	return nil, nil
}
