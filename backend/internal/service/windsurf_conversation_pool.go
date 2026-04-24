package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

var defaultWindsurfConversationPool = newWindsurfConversationPool(30*time.Minute, 500)

type windsurfConversationPool struct {
	mu      sync.Mutex
	ttl     time.Duration
	maxSize int
	entries map[string]windsurfConversationPoolEntry
}

type windsurfConversationPoolEntry struct {
	CascadeID   string
	SessionID   string
	AccountID   int64
	EndpointKey string
	CreatedAt   time.Time
	LastAccess  time.Time
}

type windsurfConversationReuseContext struct {
	BeforeFingerprint string
	Entry             *windsurfConversationPoolEntry
}

type WindsurfConversationScope struct {
	GroupID         int64  `json:"group_id,omitempty"`
	APIKeyID        int64  `json:"api_key_id,omitempty"`
	UserID          int64  `json:"user_id,omitempty"`
	ClientSessionID string `json:"client_session_id,omitempty"`
}

type windsurfConversationScope = WindsurfConversationScope

type windsurfConversationResult struct {
	CascadeID   string
	SessionID   string
	EndpointKey string
}

type windsurfConversationReuseContextKey struct{}
type windsurfConversationScopeContextKey struct{}

func newWindsurfConversationPool(ttl time.Duration, maxSize int) *windsurfConversationPool {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	if maxSize <= 0 {
		maxSize = 500
	}
	return &windsurfConversationPool{
		ttl:     ttl,
		maxSize: maxSize,
		entries: make(map[string]windsurfConversationPoolEntry, maxSize),
	}
}

func (p *windsurfConversationPool) Checkout(fingerprint string) (*windsurfConversationPoolEntry, bool) {
	if p == nil || strings.TrimSpace(fingerprint) == "" {
		return nil, false
	}
	now := time.Now()
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, ok := p.entries[fingerprint]
	if !ok {
		return nil, false
	}
	delete(p.entries, fingerprint)
	if p.ttl > 0 && now.Sub(entry.LastAccess) > p.ttl {
		return nil, false
	}
	entry.LastAccess = now
	return &entry, true
}

func (p *windsurfConversationPool) Checkin(fingerprint string, entry *windsurfConversationPoolEntry) {
	if p == nil || strings.TrimSpace(fingerprint) == "" || entry == nil {
		return
	}
	now := time.Now()
	stored := *entry
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = now
	}
	stored.LastAccess = now

	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries[fingerprint] = stored
	p.pruneLocked(now)
}

func (p *windsurfConversationPool) pruneLocked(now time.Time) {
	if p == nil {
		return
	}
	if p.ttl > 0 {
		for fingerprint, entry := range p.entries {
			if now.Sub(entry.LastAccess) > p.ttl {
				delete(p.entries, fingerprint)
			}
		}
	}
	if len(p.entries) <= p.maxSize {
		return
	}

	type candidate struct {
		fingerprint string
		lastAccess  time.Time
	}
	ordered := make([]candidate, 0, len(p.entries))
	for fingerprint, entry := range p.entries {
		ordered = append(ordered, candidate{fingerprint: fingerprint, lastAccess: entry.LastAccess})
	}
	for len(ordered) > p.maxSize {
		oldestIdx := 0
		for i := 1; i < len(ordered); i++ {
			if ordered[i].lastAccess.Before(ordered[oldestIdx].lastAccess) {
				oldestIdx = i
			}
		}
		delete(p.entries, ordered[oldestIdx].fingerprint)
		ordered = append(ordered[:oldestIdx], ordered[oldestIdx+1:]...)
	}
}

func withWindsurfConversationReuse(ctx context.Context, reuse *windsurfConversationReuseContext) context.Context {
	if ctx == nil || reuse == nil {
		return ctx
	}
	return context.WithValue(ctx, windsurfConversationReuseContextKey{}, reuse)
}

func windsurfConversationReuseFromContext(ctx context.Context) *windsurfConversationReuseContext {
	if ctx == nil {
		return nil
	}
	reuse, _ := ctx.Value(windsurfConversationReuseContextKey{}).(*windsurfConversationReuseContext)
	return reuse
}

func WithWindsurfConversationScope(ctx context.Context, scope WindsurfConversationScope) context.Context {
	if ctx == nil {
		return ctx
	}
	scope = normalizeWindsurfConversationScope(scope)
	if scope.isZero() {
		return ctx
	}
	return context.WithValue(ctx, windsurfConversationScopeContextKey{}, scope)
}

func WindsurfConversationScopeFromAPIKey(apiKey *APIKey) WindsurfConversationScope {
	if apiKey == nil {
		return WindsurfConversationScope{}
	}
	scope := WindsurfConversationScope{
		APIKeyID: apiKey.ID,
		UserID:   apiKey.UserID,
	}
	if apiKey.GroupID != nil {
		scope.GroupID = *apiKey.GroupID
	} else if apiKey.Group != nil {
		scope.GroupID = apiKey.Group.ID
	}
	return normalizeWindsurfConversationScope(scope)
}

func windsurfConversationScopeForRequest(ctx context.Context, groupID *int64) windsurfConversationScope {
	scope := windsurfConversationScopeFromContext(ctx)
	if scope.GroupID == 0 && groupID != nil {
		scope.GroupID = *groupID
	}
	return normalizeWindsurfConversationScope(scope)
}

func windsurfConversationScopeFromContext(ctx context.Context) windsurfConversationScope {
	if ctx == nil {
		return windsurfConversationScope{}
	}
	scope, _ := ctx.Value(windsurfConversationScopeContextKey{}).(windsurfConversationScope)
	return normalizeWindsurfConversationScope(scope)
}

func normalizeWindsurfConversationScope(scope windsurfConversationScope) windsurfConversationScope {
	scope.ClientSessionID = strings.TrimSpace(scope.ClientSessionID)
	return scope
}

func (s windsurfConversationScope) isZero() bool {
	return s.GroupID == 0 && s.APIKeyID == 0 && s.UserID == 0 && strings.TrimSpace(s.ClientSessionID) == ""
}

func windsurfConversationModelKey(model WindsurfResolvedModel) string {
	if uid := strings.TrimSpace(model.ModelUID); uid != "" {
		return strings.ToLower(uid)
	}
	if upstream := strings.TrimSpace(model.UpstreamModel); upstream != "" {
		return strings.ToLower(upstream)
	}
	return strings.ToLower(strings.TrimSpace(model.RequestedModel))
}

func windsurfConversationFingerprintBefore(messages []apicompat.ChatMessage, modelKey string, scopes ...windsurfConversationScope) string {
	turns := windsurfStableConversationTurns(messages)
	if len(turns) < 2 {
		return ""
	}
	return windsurfConversationFingerprint(turns[:len(turns)-1], messages, modelKey, scopes...)
}

func windsurfConversationFingerprintAfter(messages []apicompat.ChatMessage, modelKey string, scopes ...windsurfConversationScope) string {
	turns := windsurfStableConversationTurns(messages)
	if len(turns) == 0 {
		return ""
	}
	return windsurfConversationFingerprint(turns, messages, modelKey, scopes...)
}

func windsurfConversationFingerprint(turns []windsurfConversationTurn, messages []apicompat.ChatMessage, modelKey string, scopes ...windsurfConversationScope) string {
	if len(turns) == 0 {
		return ""
	}
	payload := struct {
		Model  string                     `json:"model"`
		System string                     `json:"system,omitempty"`
		Scope  *windsurfConversationScope `json:"scope,omitempty"`
		Turns  []windsurfConversationTurn `json:"turns"`
	}{
		Model: strings.ToLower(strings.TrimSpace(modelKey)),
		Turns: turns,
	}
	if os.Getenv("CASCADE_REUSE_HASH_SYSTEM") != "0" {
		payload.System = windsurfConversationSystemPrefix(messages)
	}
	if len(scopes) > 0 {
		scope := normalizeWindsurfConversationScope(scopes[0])
		if !scope.isZero() {
			payload.Scope = &scope
		}
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type windsurfConversationTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func windsurfStableConversationTurns(messages []apicompat.ChatMessage) []windsurfConversationTurn {
	turns := make([]windsurfConversationTurn, 0, len(messages))
	for _, msg := range messages {
		role := strings.TrimSpace(strings.ToLower(msg.Role))
		switch role {
		case "user":
			turns = append(turns, windsurfConversationTurn{
				Role:    "user",
				Content: windsurfChatContentToString(msg.Content),
			})
		case "tool", "function":
			turns = append(turns, windsurfConversationTurn{
				Role:    "tool_result",
				Content: windsurfChatContentToString(msg.Content),
			})
		}
	}
	return turns
}

func windsurfConversationSystemPrefix(messages []apicompat.ChatMessage) string {
	if len(messages) == 0 {
		return ""
	}
	parts := make([]string, 0, len(messages))
	for _, msg := range messages {
		if strings.TrimSpace(strings.ToLower(msg.Role)) != "system" {
			continue
		}
		text := strings.TrimSpace(windsurfChatContentToString(msg.Content))
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\x00")
}
