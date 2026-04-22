package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGroupHandlerCreate_AcceptsWindsurfPlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := newStubAdminService()
	router := gin.New()
	handler := NewGroupHandler(adminSvc, nil, nil)
	router.POST("/api/v1/admin/groups", handler.Create)

	body := map[string]any{
		"name":     "windsurf-default",
		"platform": "windsurf",
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/groups", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, adminSvc.createdGroups, 1)
	require.Equal(t, "windsurf", adminSvc.createdGroups[0].Platform)
}

func TestAccountHandlerCreate_AcceptsWindsurfAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := newStubAdminService()
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/accounts", handler.Create)

	body := map[string]any{
		"name":     "windsurf-account",
		"platform": "windsurf",
		"type":     "apikey",
		"credentials": map[string]any{
			"token": "ws-token",
		},
		"extra": map[string]any{
			"allowed_models": []string{"claude-3.5-sonnet", "gpt-4.1"},
			"plan_tier":      "pro",
			"credit_balance": 12.5,
		},
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, adminSvc.createdAccounts, 1)
	require.Equal(t, "windsurf", adminSvc.createdAccounts[0].Platform)
	require.Equal(t, "apikey", adminSvc.createdAccounts[0].Type)
	require.Equal(t, "ws-token", adminSvc.createdAccounts[0].Credentials["token"])
}
