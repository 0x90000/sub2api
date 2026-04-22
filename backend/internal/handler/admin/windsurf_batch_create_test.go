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

func TestAccountHandlerBatchCreate_AcceptsWindsurfTokenPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := newStubAdminService()
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/accounts/batch", handler.BatchCreate)

	body := map[string]any{
		"name":        "windsurf-batch",
		"platform":    "windsurf",
		"tokens":      []string{"ws-token-1", "ws-token-2"},
		"load_factor": 42,
		"credentials": map[string]any{
			"model_mapping": map[string]any{
				"claude-3.5-sonnet": "gpt-4.1",
			},
		},
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/batch", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, adminSvc.createdAccounts, 2)
	require.Equal(t, "windsurf", adminSvc.createdAccounts[0].Platform)
	require.Equal(t, "apikey", adminSvc.createdAccounts[0].Type)
	require.Equal(t, "ws-token-1", adminSvc.createdAccounts[0].Credentials["token"])
	require.Equal(t, "gpt-4.1", adminSvc.createdAccounts[0].Credentials["model_mapping"].(map[string]any)["claude-3.5-sonnet"])
	require.Equal(t, "windsurf-batch #1", adminSvc.createdAccounts[0].Name)
	require.NotNil(t, adminSvc.createdAccounts[0].LoadFactor)
	require.Equal(t, 42, *adminSvc.createdAccounts[0].LoadFactor)
	require.Equal(t, "ws-token-2", adminSvc.createdAccounts[1].Credentials["token"])
	require.Equal(t, "windsurf-batch #2", adminSvc.createdAccounts[1].Name)
}

func TestAccountHandlerBatchCreate_WindsurfTokenPayloadRejectsOtherPlatforms(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := newStubAdminService()
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/accounts/batch", handler.BatchCreate)

	body := map[string]any{
		"name":     "not-windsurf",
		"platform": "openai",
		"tokens":   []string{"should-fail"},
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/batch", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Empty(t, adminSvc.createdAccounts)
}
