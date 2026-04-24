package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

func TestWindsurfToolChoiceNoneDisablesNewToolCallsButKeepsHistory(t *testing.T) {
	req := &apicompat.ChatCompletionsRequest{
		Model:      "gpt-4.1",
		ToolChoice: json.RawMessage(`"none"`),
		Tools: []apicompat.ChatTool{{
			Type:     "function",
			Function: &apicompat.ChatFunction{Name: "Bash"},
		}},
		Messages: []apicompat.ChatMessage{
			{
				Role: "assistant",
				ToolCalls: []apicompat.ChatToolCall{{
					ID:   "call_1",
					Type: "function",
					Function: apicompat.ChatFunctionCall{
						Name:      "Bash",
						Arguments: `{"command":"pwd"}`,
					},
				}},
			},
			{
				Role:       "tool",
				ToolCallID: "call_1",
				Content:    json.RawMessage(`"ok"`),
			},
			{
				Role:    "user",
				Content: json.RawMessage(`"answer without tools"`),
			},
		},
	}

	if windsurfShouldOfferTools(req) {
		t.Fatal("tool_choice none must disable new tool offering")
	}
	if !windsurfShouldEmulateTools(req) {
		t.Fatal("tool history should still be normalized")
	}
	if preamble := buildWindsurfToolPreambleForProto(req.Tools, req.ToolChoice); preamble != "" {
		t.Fatalf("proto preamble = %q, want empty", preamble)
	}

	normalized := normalizeWindsurfChatRequestForCascade(req)
	if normalized == req {
		t.Fatal("expected copied request with normalized tool history")
	}
	if len(normalized.Messages) != 3 {
		t.Fatalf("messages len = %d, want 3", len(normalized.Messages))
	}
	assistantText := windsurfChatContentToString(normalized.Messages[0].Content)
	if !strings.Contains(assistantText, `<tool_call>{"arguments":{"command":"pwd"},"name":"Bash"}</tool_call>`) {
		t.Fatalf("assistant history was not converted to tool markup: %q", assistantText)
	}
	userText := windsurfChatContentToString(normalized.Messages[2].Content)
	if strings.Contains(userText, "Tool-calling context") || strings.Contains(userText, "<tool_call>") {
		t.Fatalf("tool preamble should not be injected for tool_choice none: %q", userText)
	}

	result := normalizeWindsurfBridgeResult(req, &WindsurfBridgeResult{
		Text: `<tool_call>{"name":"Bash","arguments":{"command":"ls"}}</tool_call>`,
	})
	if len(result.ToolCalls) != 0 {
		t.Fatalf("result tool calls len = %d, want 0", len(result.ToolCalls))
	}
}

func TestWindsurfLegacyFunctionsAreInjectedAsTools(t *testing.T) {
	req := &apicompat.ChatCompletionsRequest{
		Model: "gpt-4.1",
		Messages: []apicompat.ChatMessage{{
			Role:    "user",
			Content: json.RawMessage(`"what is the weather"`),
		}},
		Functions: []apicompat.ChatFunction{{
			Name:        "get_weather",
			Description: "get weather",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
		}},
	}

	if !windsurfShouldOfferTools(req) {
		t.Fatal("legacy functions should offer tools")
	}
	normalized := normalizeWindsurfChatRequestForCascade(req)
	userText := windsurfChatContentToString(normalized.Messages[0].Content)
	if !strings.Contains(userText, "get_weather") || !strings.Contains(userText, "Tool-calling context") {
		t.Fatalf("legacy function preamble was not injected: %q", userText)
	}
}
