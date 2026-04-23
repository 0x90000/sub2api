package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWindsurfTokenRefresherCanRefreshAndNeedsRefresh(t *testing.T) {
	refresher := &WindsurfTokenRefresher{}
	refreshWindow := 30 * time.Minute

	now := time.Now()
	account := &Account{
		Platform: PlatformWindsurf,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"expires_at": now.Add(10 * time.Minute).Unix(),
		},
	}

	require.True(t, refresher.CanRefresh(account))
	require.True(t, refresher.NeedsRefresh(account, refreshWindow))

	account.Type = AccountTypeAPIKey
	require.False(t, refresher.CanRefresh(account))
	require.False(t, refresher.NeedsRefresh(account, refreshWindow))
}

func TestWindsurfTokenRefresherRefresh(t *testing.T) {
	now := time.Unix(1_762_000_000, 0).UTC()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/firebase":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id_token":      "firebase-id-token-new",
				"refresh_token": "firebase-refresh-token-new",
				"expires_in":    "1800",
			})
		case "/register":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"api_key":        "windsurf-runtime-token-new",
				"name":           "Windsurf User",
				"api_server_url": "https://server.codeium.com",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	oauthService := NewWindsurfOAuthService(nil)
	oauthService.SetEndpointURLs(server.URL+"/firebase", server.URL+"/register")
	oauthService.SetHTTPClientFactory(func(string) (*http.Client, error) {
		return server.Client(), nil
	})
	oauthService.SetNowFunc(func() time.Time { return now })

	refresher := NewWindsurfTokenRefresher(oauthService)
	account := &Account{
		ID:       42,
		Platform: PlatformWindsurf,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"refresh_token": "firebase-refresh-token-old",
			"model_mapping": map[string]any{
				"claude-3-7-sonnet": "gpt-4.1",
			},
		},
	}

	credentials, err := refresher.Refresh(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "windsurf-runtime-token-new", credentials["token"])
	require.Equal(t, "firebase-id-token-new", credentials["access_token"])
	require.Equal(t, "firebase-refresh-token-new", credentials["refresh_token"])
	require.Equal(t, "https://server.codeium.com", credentials["api_server_url"])
	require.Equal(t, map[string]any{
		"claude-3-7-sonnet": "gpt-4.1",
	}, credentials["model_mapping"])
}
