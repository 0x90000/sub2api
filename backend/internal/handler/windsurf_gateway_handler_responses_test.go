package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newWindsurfResponsesTestHandler() *WindsurfGatewayHandler {
	return NewWindsurfGatewayHandler(service.NewWindsurfGatewayService(nil, nil, nil, nil, service.NewWindsurfErrorMapper(), nil))
}

func TestWindsurfResponses_EmptyBodyReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	newWindsurfResponsesTestHandler().Responses(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "Request body is empty")
}

func TestWindsurfResponses_InvalidJSONReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("{"))
	c.Request.Header.Set("Content-Type", "application/json")

	newWindsurfResponsesTestHandler().Responses(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "Failed to parse request body")
}

func TestWindsurfResponses_MissingModelReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"input":"hello"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	newWindsurfResponsesTestHandler().Responses(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "model is required")
}

func TestWindsurfResponses_ServiceUnavailableIsStructured(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"windsurf-sonnet-4","input":"hello"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	newWindsurfResponsesTestHandler().Responses(c)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)

	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "api_error", errorObj["code"])
	require.Equal(t, "No available Windsurf accounts", errorObj["message"])
}
