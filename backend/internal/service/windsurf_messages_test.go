package service

import (
	"context"
	"encoding/json"
	"testing"

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
