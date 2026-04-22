package handler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
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
	h.notImplemented(c)
}

func (h *WindsurfGatewayHandler) Responses(c *gin.Context) {
	h.notImplemented(c)
}

func (h *WindsurfGatewayHandler) ResponsesWebSocket(c *gin.Context) {
	h.notImplemented(c)
}

func (h *WindsurfGatewayHandler) ChatCompletions(c *gin.Context) {
	h.notImplemented(c)
}

func (h *WindsurfGatewayHandler) Models(c *gin.Context) {
	h.notImplemented(c)
}

func (h *WindsurfGatewayHandler) notImplemented(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": gin.H{
			"type":    "not_implemented",
			"message": "Windsurf gateway is not implemented yet",
		},
	})
}
