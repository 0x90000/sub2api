package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newWindsurfGatewayRoutesTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	RegisterGatewayRoutes(
		router,
		&handler.Handlers{
			Gateway:         &handler.GatewayHandler{},
			OpenAIGateway:   &handler.OpenAIGatewayHandler{},
			WindsurfGateway: handler.NewWindsurfGatewayHandler(service.NewWindsurfGatewayService(nil, nil, service.NewWindsurfModelCatalogService(nil), nil, nil, nil)),
		},
		servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
			groupID := int64(1)
			c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
				GroupID: &groupID,
				Group:   &service.Group{Platform: service.PlatformWindsurf},
			})
			c.Next()
		}),
		nil,
		nil,
		nil,
		nil,
		&config.Config{},
	)

	return router
}

func TestWindsurfGatewayRouting(t *testing.T) {
	router := newWindsurfGatewayRoutesTestRouter()

	messagesReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"windsurf-test"}`))
	messagesReq.Header.Set("Content-Type", "application/json")
	messagesResp := httptest.NewRecorder()
	router.ServeHTTP(messagesResp, messagesReq)
	require.NotEqual(t, http.StatusNotFound, messagesResp.Code, "path=%s should be routed to windsurf handler", "/v1/messages")

	responsesReq := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"windsurf-test"}`))
	responsesReq.Header.Set("Content-Type", "application/json")
	responsesResp := httptest.NewRecorder()
	router.ServeHTTP(responsesResp, responsesReq)
	require.Equal(t, http.StatusNotImplemented, responsesResp.Code, "path=%s should hit windsurf placeholder handler", "/v1/responses")

	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"windsurf-test"}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatResp := httptest.NewRecorder()
	router.ServeHTTP(chatResp, chatReq)
	require.NotEqual(t, http.StatusNotFound, chatResp.Code, "path=%s should be routed to windsurf handler", "/v1/chat/completions")

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "path=%s should hit windsurf models handler", "/v1/models")
}
