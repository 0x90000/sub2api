package handler

import (
	"encoding/json"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// WindsurfGatewayHandler is a Phase 1 placeholder. The real request forwarding
// chain is added in later phases after platform plumbing is in place.
type WindsurfGatewayHandler struct {
	service *service.WindsurfGatewayService
}

func NewWindsurfGatewayHandler(service *service.WindsurfGatewayService) *WindsurfGatewayHandler {
	return &WindsurfGatewayHandler{service: service}
}

func (h *WindsurfGatewayHandler) Messages(c *gin.Context) {
	if h == nil || h.service == nil {
		h.writeAnthropicError(c, service.ErrWindsurfChatBridgeUnavailable)
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		h.writeAnthropicShapeError(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	if len(body) == 0 {
		h.writeAnthropicShapeError(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}
	if !gjson.ValidBytes(body) {
		h.writeAnthropicShapeError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}
	if model := gjson.GetBytes(body, "model"); !model.Exists() || model.Type != gjson.String || model.String() == "" {
		h.writeAnthropicShapeError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}

	var req apicompat.AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.writeAnthropicShapeError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}

	apiKey, _ := middleware2.GetAPIKeyFromContext(c)
	var groupID *int64
	if apiKey != nil && apiKey.Group != nil {
		groupID = &apiKey.Group.ID
	}

	if req.Stream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)

		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			h.writeAnthropicShapeError(c, http.StatusInternalServerError, "api_error", "Streaming is not supported by this server")
			return
		}

		metadata, err := h.service.StreamMessagesWithMetadata(c.Request.Context(), groupID, &req, func(event apicompat.AnthropicStreamEvent) error {
			sse, err := apicompat.ResponsesAnthropicEventToSSE(event)
			if err != nil {
				return err
			}
			if _, err := c.Writer.WriteString(sse); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		})
		if err != nil {
			if !c.Writer.Written() {
				h.writeAnthropicError(c, err)
			}
			return
		}
		_ = h.recordUsage(c, metadata, "/v1/messages", "/windsurf/cascade")
		return
	}

	resp, metadata, err := h.service.CompleteMessagesWithMetadata(c.Request.Context(), groupID, &req)
	if err != nil {
		h.writeAnthropicError(c, err)
		return
	}
	if err := h.recordUsage(c, metadata, "/v1/messages", "/windsurf/cascade"); err != nil {
		h.writeAnthropicError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *WindsurfGatewayHandler) Responses(c *gin.Context) {
	if h == nil || h.service == nil {
		h.writeResponsesShapeError(c, http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable")
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		h.writeResponsesShapeError(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	if len(body) == 0 {
		h.writeResponsesShapeError(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}
	if !gjson.ValidBytes(body) {
		h.writeResponsesShapeError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}
	if model := gjson.GetBytes(body, "model"); !model.Exists() || model.Type != gjson.String || model.String() == "" {
		h.writeResponsesShapeError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}

	var req apicompat.ResponsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.writeResponsesShapeError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}

	apiKey, _ := middleware2.GetAPIKeyFromContext(c)
	var groupID *int64
	if apiKey != nil && apiKey.Group != nil {
		groupID = &apiKey.Group.ID
	}

	if req.Stream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)

		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			h.writeResponsesShapeError(c, http.StatusInternalServerError, "api_error", "Streaming is not supported by this server")
			return
		}

		metadata, err := h.service.StreamResponsesWithMetadata(c.Request.Context(), groupID, &req, func(event apicompat.ResponsesStreamEvent) error {
			sse, err := apicompat.ResponsesEventToSSE(event)
			if err != nil {
				return err
			}
			if _, err := c.Writer.WriteString(sse); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		})
		if err != nil {
			if !c.Writer.Written() {
				h.writeResponsesError(c, err)
			}
			return
		}
		_ = h.recordUsage(c, metadata, "/v1/responses", "/windsurf/cascade")
		return
	}

	resp, metadata, err := h.service.CompleteResponsesWithMetadata(c.Request.Context(), groupID, &req)
	if err != nil {
		h.writeResponsesError(c, err)
		return
	}
	if err := h.recordUsage(c, metadata, "/v1/responses", "/windsurf/cascade"); err != nil {
		h.writeResponsesError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *WindsurfGatewayHandler) ResponsesWebSocket(c *gin.Context) {
	h.notImplemented(c)
}

func (h *WindsurfGatewayHandler) ChatCompletions(c *gin.Context) {
	if h == nil || h.service == nil {
		h.writeChatError(c, service.ErrWindsurfChatBridgeUnavailable)
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		h.writeOpenAIShapeError(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	if len(body) == 0 {
		h.writeOpenAIShapeError(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}
	if !gjson.ValidBytes(body) {
		h.writeOpenAIShapeError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}
	if model := gjson.GetBytes(body, "model"); !model.Exists() || model.Type != gjson.String || model.String() == "" {
		h.writeOpenAIShapeError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}

	var req apicompat.ChatCompletionsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.writeOpenAIShapeError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}

	apiKey, _ := middleware2.GetAPIKeyFromContext(c)
	var groupID *int64
	if apiKey != nil && apiKey.Group != nil {
		groupID = &apiKey.Group.ID
	}

	if req.Stream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)

		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			h.writeOpenAIShapeError(c, http.StatusInternalServerError, "api_error", "Streaming is not supported by this server")
			return
		}

		metadata, err := h.service.StreamChatCompletionsWithMetadata(c.Request.Context(), groupID, &req, func(chunk apicompat.ChatCompletionsChunk) error {
			sse, err := apicompat.ChatChunkToSSE(chunk)
			if err != nil {
				return err
			}
			if _, err := c.Writer.WriteString(sse); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		})
		if err != nil {
			if !c.Writer.Written() {
				h.writeChatError(c, err)
				return
			}
			return
		}
		if _, err := c.Writer.WriteString("data: [DONE]\n\n"); err == nil {
			flusher.Flush()
		}
		_ = h.recordUsage(c, metadata, "/v1/chat/completions", "/windsurf/cascade")
		return
	}

	resp, metadata, err := h.service.CompleteChatCompletionsWithMetadata(c.Request.Context(), groupID, &req)
	if err != nil {
		h.writeChatError(c, err)
		return
	}
	if err := h.recordUsage(c, metadata, "/v1/chat/completions", "/windsurf/cascade"); err != nil {
		h.writeChatError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *WindsurfGatewayHandler) Models(c *gin.Context) {
	apiKey, _ := middleware2.GetAPIKeyFromContext(c)

	var groupID *int64
	if apiKey != nil && apiKey.Group != nil {
		groupID = &apiKey.Group.ID
	}

	var models any = []any{}
	if h != nil && h.service != nil && h.service.ModelCatalog() != nil {
		result, err := h.service.ModelCatalog().ListModels(c.Request.Context(), groupID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"type":    "internal_error",
					"message": err.Error(),
				},
			})
			return
		}
		models = result
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   models,
	})
}

func (h *WindsurfGatewayHandler) notImplemented(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": gin.H{
			"type":    "not_implemented",
			"message": "Windsurf gateway is not implemented yet",
		},
	})
}

func (h *WindsurfGatewayHandler) writeChatError(c *gin.Context, err error) {
	if h != nil && h.service != nil && h.service.ErrorMapper() != nil {
		status, errType, message := h.service.ErrorMapper().MapChatCompletionsError(err)
		h.writeOpenAIShapeError(c, status, errType, message)
		return
	}
	h.writeOpenAIShapeError(c, http.StatusBadGateway, "upstream_error", "Windsurf upstream request failed")
}

func (h *WindsurfGatewayHandler) writeOpenAIShapeError(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

func (h *WindsurfGatewayHandler) writeResponsesError(c *gin.Context, err error) {
	if h != nil && h.service != nil && h.service.ErrorMapper() != nil {
		status, code, message := h.service.ErrorMapper().MapResponsesError(err)
		h.writeResponsesShapeError(c, status, code, message)
		return
	}
	h.writeResponsesShapeError(c, http.StatusBadGateway, "upstream_error", "Windsurf upstream request failed")
}

func (h *WindsurfGatewayHandler) writeResponsesShapeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}

func (h *WindsurfGatewayHandler) writeAnthropicError(c *gin.Context, err error) {
	if h != nil && h.service != nil && h.service.ErrorMapper() != nil {
		status, errType, message := h.service.ErrorMapper().MapChatCompletionsError(err)
		h.writeAnthropicShapeError(c, status, errType, message)
		return
	}
	h.writeAnthropicShapeError(c, http.StatusBadGateway, "upstream_error", "Windsurf upstream request failed")
}

func (h *WindsurfGatewayHandler) writeAnthropicShapeError(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

func (h *WindsurfGatewayHandler) recordUsage(c *gin.Context, metadata *service.WindsurfExecutionMetadata, inboundEndpoint, upstreamEndpoint string) error {
	if h == nil || h.service == nil || metadata == nil || metadata.Account == nil {
		return nil
	}

	apiKey, _ := middleware2.GetAPIKeyFromContext(c)
	if apiKey == nil || apiKey.UserID == 0 {
		return nil
	}

	user := apiKey.User
	if user == nil {
		user = &service.User{ID: apiKey.UserID}
	}
	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	return h.service.RecordUsage(c.Request.Context(), &service.WindsurfRecordUsageInput{
		Result: &service.WindsurfRecordUsageResult{
			RequestID:     metadata.RequestID,
			Model:         metadata.RequestedModel,
			UpstreamModel: metadata.UpstreamModel,
			Usage:         metadata.Usage,
			Duration:      metadata.Duration,
			Stream:        metadata.Stream,
		},
		APIKey:           apiKey,
		User:             user,
		Account:          metadata.Account,
		Subscription:     subscription,
		InboundEndpoint:  inboundEndpoint,
		UpstreamEndpoint: upstreamEndpoint,
		UserAgent:        c.Request.UserAgent(),
		IPAddress:        c.ClientIP(),
	})
}
