package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type refreshWindsurfAdminService struct {
	*stubAdminService
	account       *service.Account
	lastUpdateID  int64
	lastUpdateReq *service.UpdateAccountInput
}

func (s *refreshWindsurfAdminService) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	if s.account != nil && s.account.ID == id {
		copyAccount := *s.account
		copyAccount.Credentials = map[string]any{}
		for k, v := range s.account.Credentials {
			copyAccount.Credentials[k] = v
		}
		return &copyAccount, nil
	}
	return s.stubAdminService.GetAccount(context.Background(), id)
}

func (s *refreshWindsurfAdminService) UpdateAccount(ctx context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
	s.lastUpdateID = id
	s.lastUpdateReq = input
	return s.stubAdminService.UpdateAccount(ctx, id, input)
}

func TestAccountHandlerRefresh_WindsurfOAuth(t *testing.T) {
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

	windsurfOAuthService := service.NewWindsurfOAuthService(nil)
	windsurfOAuthService.SetEndpointURLs(server.URL+"/firebase", server.URL+"/register")
	windsurfOAuthService.SetHTTPClientFactory(func(string) (*http.Client, error) {
		return server.Client(), nil
	})
	windsurfOAuthService.SetNowFunc(func() time.Time { return time.Unix(1_762_000_200, 0).UTC() })

	adminSvc := &refreshWindsurfAdminService{
		stubAdminService: newStubAdminService(),
		account: &service.Account{
			ID:       21,
			Platform: service.PlatformWindsurf,
			Type:     service.AccountTypeOAuth,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"refresh_token": "firebase-refresh-token-old",
			},
		},
	}

	handler := NewAccountHandler(adminSvc, nil, nil, windsurfOAuthService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/api/v1/admin/accounts/:id/refresh", handler.Refresh)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/21/refresh", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(21), adminSvc.lastUpdateID)
	require.NotNil(t, adminSvc.lastUpdateReq)
	require.Equal(t, "windsurf-runtime-token", adminSvc.lastUpdateReq.Credentials["token"])
	require.Equal(t, "firebase-id-token", adminSvc.lastUpdateReq.Credentials["access_token"])
	require.Equal(t, "firebase-refresh-token-new", adminSvc.lastUpdateReq.Credentials["refresh_token"])
	require.NotNil(t, adminSvc.lastUpdateReq.Extra)
	require.NotEmpty(t, adminSvc.lastUpdateReq.Extra["oauth_last_refresh_at"])
	require.Equal(t, "", adminSvc.lastUpdateReq.Extra["oauth_last_refresh_error"])
}
