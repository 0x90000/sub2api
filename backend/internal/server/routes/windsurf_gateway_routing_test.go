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
			WindsurfGateway: handler.NewWindsurfGatewayHandler(service.NewWindsurfGatewayService()),
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

	for _, path := range []string{"/v1/messages", "/v1/chat/completions", "/v1/responses"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"windsurf-test"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotImplemented, w.Code, "path=%s should hit windsurf placeholder handler", path)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotImplemented, w.Code, "path=%s should hit windsurf placeholder handler", "/v1/models")
}
