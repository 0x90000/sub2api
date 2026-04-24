package service

import (
	"errors"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

func TestFormatWindsurfGRPCRequestError_LocalLSUnavailable(t *testing.T) {
	bridge := &WindsurfChatBridge{baseURL: "http://127.0.0.1:42100"}

	err := bridge.formatGRPCRequestError(
		windsurfStartCascadePath,
		errors.New(`Post "http://127.0.0.1:42100/exa.language_server_pb.LanguageServerService/StartCascade": dial tcp 127.0.0.1:42100: connect: connection refused`),
	)
	if err == nil {
		t.Fatal("formatGRPCRequestError() returned nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "Windsurf language server is not running") {
		t.Fatalf("error = %q, want helpful LS unavailable message", msg)
	}
	if !strings.Contains(msg, "WINDSURF_LS_ADDR") {
		t.Fatalf("error = %q, want setup guidance", msg)
	}
	if !strings.Contains(msg, "42100") {
		t.Fatalf("error = %q, want target address context", msg)
	}
}

func TestEstimateWindsurfUsageFallback_PopulatesMissingUsage(t *testing.T) {
	req := &apicompat.ChatCompletionsRequest{
		Model: "claude-sonnet-4-5-20250929",
		Messages: []apicompat.ChatMessage{
			{Role: "system", Content: json.RawMessage(`"你是一个乐于助人的助手"`)} ,
			{Role: "user", Content: json.RawMessage(`"写一段关于猫的短文"`)} ,
		},
	}

	result := &WindsurfBridgeResult{
		Text:      "猫喜欢晒太阳。",
		Reasoning: "先写一个简短回答。",
	}

	estimateWindsurfUsageFallback(req, result)

	if result.Usage.InputTokens <= 0 {
		t.Fatalf("InputTokens = %d, want > 0", result.Usage.InputTokens)
	}
	if result.Usage.OutputTokens <= 0 {
		t.Fatalf("OutputTokens = %d, want > 0", result.Usage.OutputTokens)
	}
}
