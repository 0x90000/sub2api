package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type windsurfOAuthAdminService struct {
	*stubAdminService
	accounts map[int64]*service.Account
}

func (s *windsurfOAuthAdminService) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	if account, ok := s.accounts[id]; ok {
		copyAccount := *account
		return &copyAccount, nil
	}
	return s.stubAdminService.GetAccount(context.Background(), id)
}

func setupWindsurfOAuthRouter(adminSvc service.AdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewWindsurfOAuthHandler(service.NewWindsurfOAuthService(nil), adminSvc)

	windsurf := router.Group("/api/v1/admin/windsurf")
	{
		windsurf.POST("/oauth/auth-url", handler.GenerateAuthURL)
		windsurf.POST("/oauth/exchange-code", handler.ExchangeCode)
		windsurf.POST("/oauth/refresh-token", handler.RefreshToken)
		windsurf.POST("/oauth/accounts/:id/refresh", handler.RefreshAccountToken)
	}

	return router
}

func TestWindsurfOAuthHandlerRoutesExist(t *testing.T) {
	adminSvc := &windsurfOAuthAdminService{
		stubAdminService: newStubAdminService(),
		accounts: map[int64]*service.Account{
			9: {
				ID:       9,
				Platform: service.PlatformWindsurf,
				Type:     service.AccountTypeOAuth,
				Status:   service.StatusActive,
				Credentials: map[string]any{
					"refresh_token": "ws-refresh-token",
				},
			},
		},
	}
	router := setupWindsurfOAuthRouter(adminSvc)

	testCases := []struct {
		name string
		path string
		body string
	}{
		{
			name: "generate auth url route",
			path: "/api/v1/admin/windsurf/oauth/auth-url",
			body: `{}`,
		},
		{
			name: "exchange code route",
			path: "/api/v1/admin/windsurf/oauth/exchange-code",
			body: `{"session_id":"session","code":"code","state":"state"}`,
		},
		{
			name: "refresh token route",
			path: "/api/v1/admin/windsurf/oauth/refresh-token",
			body: `{"refresh_token":"refresh-token"}`,
		},
		{
			name: "refresh account route",
			path: "/api/v1/admin/windsurf/oauth/accounts/9/refresh",
			body: `{}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusNotImplemented, rec.Code)
		})
	}
}

func TestWindsurfOAuthHandlerRefreshAccountRejectsOtherPlatforms(t *testing.T) {
	adminSvc := &windsurfOAuthAdminService{
		stubAdminService: newStubAdminService(),
		accounts: map[int64]*service.Account{
			11: {
				ID:       11,
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeOAuth,
				Status:   service.StatusActive,
			},
		},
	}
	router := setupWindsurfOAuthRouter(adminSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/windsurf/oauth/accounts/11/refresh", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Account platform does not match OAuth endpoint")
}
