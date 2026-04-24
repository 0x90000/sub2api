package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

func TestWindsurfResponseCacheKey_IgnoresStreamFlag(t *testing.T) {
	reqA := &apicompat.ChatCompletionsRequest{
		Model: "claude-sonnet-4-5",
		Messages: []apicompat.ChatMessage{{
			Role:    "user",
			Content: json.RawMessage(`"hello"`),
		}},
		Stream: false,
	}
	reqB := &apicompat.ChatCompletionsRequest{
		Model:    reqA.Model,
		Messages: reqA.Messages,
		Stream:   true,
	}

	keyA := windsurfResponseCacheKey(reqA, windsurfResponseCacheScope{AccountID: 1})
	keyB := windsurfResponseCacheKey(reqB, windsurfResponseCacheScope{AccountID: 1})
	if keyA == "" || keyA != keyB {
		t.Fatalf("cache key should ignore stream flag: %q vs %q", keyA, keyB)
	}
}

func TestWindsurfResponseCacheHitConvertsPromptSideUsageToCacheRead(t *testing.T) {
	cache := newWindsurfResponseCache(time.Minute, time.Minute)
	req := &apicompat.ChatCompletionsRequest{
		Model: "claude-sonnet-4-5",
		Messages: []apicompat.ChatMessage{{
			Role:    "user",
			Content: json.RawMessage(`"hello"`),
		}},
	}
	cache.Set(req, windsurfResponseCacheScope{AccountID: 1}, &WindsurfBridgeResult{
		Text: "cached",
		Usage: WindsurfBridgeUsage{
			InputTokens:         10,
			OutputTokens:        3,
			CacheCreationTokens: 4,
			CacheReadTokens:     2,
		},
	})

	got, ok := cache.Get(req, windsurfResponseCacheScope{AccountID: 1})
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Usage.InputTokens != 0 || got.Usage.CacheCreationTokens != 0 || got.Usage.CacheReadTokens != 16 || got.Usage.OutputTokens != 3 {
		t.Fatalf("usage = %+v, want prompt side converted to cache read", got.Usage)
	}
}
