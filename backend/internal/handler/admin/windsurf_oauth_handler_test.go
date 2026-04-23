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
	accounts       map[int64]*service.Account
	lastUpdateID   int64
	lastUpdateReq  *service.UpdateAccountInput
	lastClearedID  int64
}

func (s *windsurfOAuthAdminService) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	if account, ok := s.accounts[id]; ok {
		copyAccount := *account
		if account.Credentials != nil {
			copyAccount.Credentials = map[string]any{}
			for k, v := range account.Credentials {
				copyAccount.Credentials[k] = v
			}
		}
		if account.Extra != nil {
			copyAccount.Extra = map[string]any{}
			for k, v := range account.Extra {
				copyAccount.Extra[k] = v
			}
		}
		return &copyAccount, nil
	}
	return s.stubAdminService.GetAccount(context.Background(), id)
}

func (s *windsurfOAuthAdminService) UpdateAccount(ctx context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
	s.lastUpdateID = id
	s.lastUpdateReq = input
	if account, ok := s.accounts[id]; ok {
		if input.Credentials != nil {
			account.Credentials = map[string]any{}
			for k, v := range input.Credentials {
				account.Credentials[k] = v
			}
		}
		if input.Extra != nil {
			account.Extra = map[string]any{}
			for k, v := range input.Extra {
				account.Extra[k] = v
			}
		}
		return s.GetAccount(ctx, id)
	}
	return s.stubAdminService.UpdateAccount(ctx, id, input)
}

func (s *windsurfOAuthAdminService) ClearAccountError(ctx context.Context, id int64) (*service.Account, error) {
	s.lastClearedID = id
	return s.GetAccount(ctx, id)
}

func setupWindsurfOAuthRouter(
	adminSvc service.AdminService,
	oauthService *service.WindsurfOAuthService,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if oauthService == nil {
		oauthService = service.NewWindsurfOAuthService(nil)
	}
	handler := NewWindsurfOAuthHandler(oauthService, adminSvc)

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
	oauthService.SetNowFunc(func() time.Time { return time.Unix(1_762_000_250, 0).UTC() })

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
	router := setupWindsurfOAuthRouter(adminSvc, oauthService)

	testCases := []struct {
		name       string
		path       string
		body       string
		wantStatus int
	}{
		{
			name:       "generate auth url route",
			path:       "/api/v1/admin/windsurf/oauth/auth-url",
			body:       `{}`,
			wantStatus: http.StatusNotImplemented,
		},
		{
			name:       "exchange code route",
			path:       "/api/v1/admin/windsurf/oauth/exchange-code",
			body:       `{"session_id":"session","code":"code","state":"state"}`,
			wantStatus: http.StatusNotImplemented,
		},
		{
			name:       "refresh token route",
			path:       "/api/v1/admin/windsurf/oauth/refresh-token",
			body:       `{"refresh_token":"refresh-token"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "refresh account route",
			path:       "/api/v1/admin/windsurf/oauth/accounts/9/refresh",
			body:       `{}`,
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(rec, req)

			require.Equal(t, tc.wantStatus, rec.Code)
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
	router := setupWindsurfOAuthRouter(adminSvc, nil)

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

func TestWindsurfOAuthHandlerRefreshAccountSuccessClearsRecoverableState(t *testing.T) {
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
				"api_key":        "windsurf-runtime-token",
				"name":           "Windsurf User",
				"api_server_url": "https://server.codeium.com",
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
	oauthService.SetNowFunc(func() time.Time { return time.Unix(1_762_000_400, 0).UTC() })

	adminSvc := &windsurfOAuthAdminService{
		stubAdminService: newStubAdminService(),
		accounts: map[int64]*service.Account{
			17: {
				ID:       17,
				Platform: service.PlatformWindsurf,
				Type:     service.AccountTypeOAuth,
				Status:   service.StatusError,
				Credentials: map[string]any{
					"refresh_token": "firebase-refresh-token-old",
					"model_mapping": map[string]any{
						"claude-3-7-sonnet": "gpt-4.1",
					},
				},
				Extra: map[string]any{
					"oauth_last_refresh_error": "stale token",
				},
			},
		},
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewWindsurfOAuthHandler(oauthService, adminSvc)
	router.POST("/api/v1/admin/windsurf/oauth/accounts/:id/refresh", handler.RefreshAccountToken)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/windsurf/oauth/accounts/17/refresh", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(17), adminSvc.lastUpdateID)
	require.Equal(t, int64(17), adminSvc.lastClearedID)
	require.NotNil(t, adminSvc.lastUpdateReq)
	require.Equal(t, "windsurf-runtime-token", adminSvc.lastUpdateReq.Credentials["token"])
	require.Equal(t, "firebase-id-token", adminSvc.lastUpdateReq.Credentials["access_token"])
	require.Equal(t, "firebase-refresh-token-new", adminSvc.lastUpdateReq.Credentials["refresh_token"])
	require.Equal(t, "https://server.codeium.com", adminSvc.lastUpdateReq.Credentials["api_server_url"])
	require.Equal(t, map[string]any{
		"claude-3-7-sonnet": "gpt-4.1",
	}, adminSvc.lastUpdateReq.Credentials["model_mapping"])
	require.NotNil(t, adminSvc.lastUpdateReq.Extra)
	require.Equal(t, "", adminSvc.lastUpdateReq.Extra["oauth_last_refresh_error"])
}
