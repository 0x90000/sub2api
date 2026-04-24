package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	gocache "github.com/patrickmn/go-cache"
)

var defaultWindsurfResponseCache = newWindsurfResponseCache(10*time.Minute, 20*time.Minute)

type windsurfResponseCache struct {
	cache *gocache.Cache
}

type windsurfCachedResponse struct {
	Text      string
	Reasoning string
	Usage     WindsurfBridgeUsage
}

type windsurfResponseCacheScope struct {
	GroupID       int64  `json:"group_id,omitempty"`
	AccountID     int64  `json:"account_id,omitempty"`
	UpstreamModel string `json:"upstream_model,omitempty"`
	ModelUID      string `json:"model_uid,omitempty"`
}

func newWindsurfResponseCache(defaultExpiration, cleanupInterval time.Duration) *windsurfResponseCache {
	return &windsurfResponseCache{cache: gocache.New(defaultExpiration, cleanupInterval)}
}

func (c *windsurfResponseCache) Get(req *apicompat.ChatCompletionsRequest, scope windsurfResponseCacheScope) (*WindsurfBridgeResult, bool) {
	if c == nil || c.cache == nil || !windsurfCanUseResponseCache(req) {
		return nil, false
	}
	value, ok := c.cache.Get(windsurfResponseCacheKey(req, scope))
	if !ok {
		return nil, false
	}
	cached, ok := value.(windsurfCachedResponse)
	if !ok {
		return nil, false
	}
	usage := cached.Usage
	promptSideTokens := usage.InputTokens + usage.CacheCreationTokens + usage.CacheReadTokens
	if promptSideTokens > 0 {
		usage.InputTokens = 0
		usage.CacheCreationTokens = 0
		usage.CacheReadTokens = promptSideTokens
	}
	return &WindsurfBridgeResult{
		Text:      cached.Text,
		Reasoning: cached.Reasoning,
		Usage:     usage,
	}, true
}

func (c *windsurfResponseCache) Set(req *apicompat.ChatCompletionsRequest, scope windsurfResponseCacheScope, result *WindsurfBridgeResult) {
	if c == nil || c.cache == nil || result == nil || !windsurfCanUseResponseCache(req) {
		return
	}
	if len(result.ToolCalls) > 0 || (result.Text == "" && result.Reasoning == "") {
		return
	}
	c.cache.Set(windsurfResponseCacheKey(req, scope), windsurfCachedResponse{
		Text:      result.Text,
		Reasoning: result.Reasoning,
		Usage:     result.Usage,
	}, gocache.DefaultExpiration)
}

func windsurfCanUseResponseCache(req *apicompat.ChatCompletionsRequest) bool {
	return req != nil
}

func windsurfResponseCacheKey(req *apicompat.ChatCompletionsRequest, scope windsurfResponseCacheScope) string {
	if req == nil {
		return ""
	}
	keyPayload := struct {
		Scope       windsurfResponseCacheScope `json:"scope"`
		Model       string                     `json:"model"`
		Messages    []apicompat.ChatMessage    `json:"messages"`
		Tools       []apicompat.ChatTool       `json:"tools,omitempty"`
		ToolChoice  json.RawMessage            `json:"tool_choice,omitempty"`
		Temperature *float64                   `json:"temperature,omitempty"`
		TopP        *float64                   `json:"top_p,omitempty"`
		MaxTokens   *int                       `json:"max_tokens,omitempty"`
	}{
		Scope:       scope,
		Model:       req.Model,
		Messages:    req.Messages,
		Tools:       req.Tools,
		ToolChoice:  req.ToolChoice,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxTokens,
	}
	raw, _ := json.Marshal(keyPayload)
	sum := sha256.Sum256(raw)
	return "windsurf:chat:" + hex.EncodeToString(sum[:])
}
