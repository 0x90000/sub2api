package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestWindsurfOAuthHandlerRefreshTokenSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/firebase":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id_token":      "firebase-id-token",
				"refresh_token": "firebase-refresh-token-new",
				"expires_in":    "3600",
			})
		case "/register":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"api_key": "windsurf-runtime-token",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	oauthService := service.NewWindsurfOAuthService(nil)
	oauthService.SetEndpointURLs(server.URL+"/firebase", server.URL+"/register")
	oauthService.SetHTTPClientFactory(func(string) (*http.Client, error) {
		return server.Client(), nil
	})
	oauthService.SetNowFunc(func() time.Time { return time.Unix(1_762_000_300, 0).UTC() })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewWindsurfOAuthHandler(oauthService, newStubAdminService())
	router.POST("/api/v1/admin/windsurf/oauth/refresh-token", handler.RefreshToken)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/windsurf/oauth/refresh-token", strings.NewReader(`{"refresh_token":"firebase-refresh-token-old"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data struct {
			Token        string `json:"token"`
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "windsurf-runtime-token", resp.Data.Token)
	require.Equal(t, "firebase-id-token", resp.Data.AccessToken)
	require.Equal(t, "firebase-refresh-token-new", resp.Data.RefreshToken)
}
