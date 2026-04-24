package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

func TestWindsurfConversationPool_ReusesUserAndToolTurnsOnly(t *testing.T) {
	messages := []apicompat.ChatMessage{
		{Role: "system", Content: json.RawMessage(`"system-a"`)},
		{Role: "user", Content: json.RawMessage(`"hello"`)},
		{Role: "assistant", Content: json.RawMessage(`"draft one"`)},
		{Role: "assistant", Content: json.RawMessage(`"draft two"`)},
		{Role: "user", Content: json.RawMessage(`"next turn"`)},
	}

	beforeA := windsurfConversationFingerprintBefore(messages, "claude-sonnet")
	messages[2].Content = json.RawMessage(`"changed assistant"`)
	beforeB := windsurfConversationFingerprintBefore(messages, "claude-sonnet")
	if beforeA == "" || beforeA != beforeB {
		t.Fatalf("assistant-only changes should not change before fingerprint: %q vs %q", beforeA, beforeB)
	}
}

func TestWindsurfConversationPool_CheckoutRemovesEntryUntilCheckin(t *testing.T) {
	pool := newWindsurfConversationPool(time.Minute, 8)
	entry := &windsurfConversationPoolEntry{
		CascadeID: "cascade-1",
		SessionID: "session-1",
		AccountID: 42,
	}
	pool.Checkin("fp-1", entry)

	first, ok := pool.Checkout("fp-1")
	if !ok || first == nil || first.CascadeID != "cascade-1" {
		t.Fatalf("first checkout = %#v, %v", first, ok)
	}
	if second, ok := pool.Checkout("fp-1"); ok || second != nil {
		t.Fatalf("second checkout should miss, got %#v, %v", second, ok)
	}
}

func TestWindsurfConversationFingerprint_IncludesSystemByDefault(t *testing.T) {
	messagesA := []apicompat.ChatMessage{
		{Role: "system", Content: json.RawMessage(`"system-a"`)},
		{Role: "user", Content: json.RawMessage(`"same user"`)},
	}
	messagesB := []apicompat.ChatMessage{
		{Role: "system", Content: json.RawMessage(`"system-b"`)},
		{Role: "user", Content: json.RawMessage(`"same user"`)},
	}

	fpA := windsurfConversationFingerprintAfter(messagesA, "claude-sonnet")
	fpB := windsurfConversationFingerprintAfter(messagesB, "claude-sonnet")
	if fpA == "" || fpB == "" || fpA == fpB {
		t.Fatalf("system prompts should partition conversation reuse: %q vs %q", fpA, fpB)
	}
}

func TestWindsurfConversationFingerprint_IncludesCallerScope(t *testing.T) {
	messages := []apicompat.ChatMessage{{
		Role:    "user",
		Content: json.RawMessage(`"same user"`),
	}}

	fpA := windsurfConversationFingerprintAfter(messages, "claude-sonnet", windsurfConversationScope{
		GroupID:  7,
		APIKeyID: 101,
		UserID:   201,
	})
	fpB := windsurfConversationFingerprintAfter(messages, "claude-sonnet", windsurfConversationScope{
		GroupID:  7,
		APIKeyID: 102,
		UserID:   201,
	})
	if fpA == "" || fpB == "" || fpA == fpB {
		t.Fatalf("caller scope should partition conversation reuse: %q vs %q", fpA, fpB)
	}
}
