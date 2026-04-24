package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"strings"
	"testing"
	"time"
)

func TestWindsurfOAuthServiceRefreshToken(t *testing.T) {
	now := time.Unix(1_762_000_000, 0).UTC()
	var (
		firebaseGrantType string
		firebaseRefresh   string
		registerToken     string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/firebase":
			_ = r.ParseForm()
			firebaseGrantType = r.Form.Get("grant_type")
			firebaseRefresh = r.Form.Get("refresh_token")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id_token":      "firebase-id-token",
				"refresh_token": "firebase-refresh-token-new",
				"expires_in":    "3600",
			})
		case "/register":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			registerToken, _ = body["firebase_id_token"].(string)
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

	svc := NewWindsurfOAuthService(nil)
	svc.SetEndpointURLs(server.URL+"/firebase", server.URL+"/register")
	svc.SetHTTPClientFactory(func(string) (*http.Client, error) {
		return server.Client(), nil
	})
	svc.SetNowFunc(func() time.Time { return now })

	tokenInfo, err := svc.RefreshToken(context.Background(), "firebase-refresh-token-old", nil)
	if err != nil {
		t.Fatalf("RefreshToken() unexpected err: %v", err)
	}

	if firebaseGrantType != "refresh_token" {
		t.Fatalf("grant_type = %q, want %q", firebaseGrantType, "refresh_token")
	}
	if firebaseRefresh != "firebase-refresh-token-old" {
		t.Fatalf("refresh_token = %q, want %q", firebaseRefresh, "firebase-refresh-token-old")
	}
	if registerToken != "firebase-id-token" {
		t.Fatalf("register token = %q, want %q", registerToken, "firebase-id-token")
	}
	if tokenInfo.Token != "windsurf-runtime-token" {
		t.Fatalf("Token = %q, want %q", tokenInfo.Token, "windsurf-runtime-token")
	}
	if tokenInfo.AccessToken != "firebase-id-token" {
		t.Fatalf("AccessToken = %q, want %q", tokenInfo.AccessToken, "firebase-id-token")
	}
	if tokenInfo.RefreshToken != "firebase-refresh-token-new" {
		t.Fatalf("RefreshToken = %q, want %q", tokenInfo.RefreshToken, "firebase-refresh-token-new")
	}
	if tokenInfo.ExpiresIn != 3600 {
		t.Fatalf("ExpiresIn = %d, want %d", tokenInfo.ExpiresIn, 3600)
	}
	if tokenInfo.ExpiresAt != now.Unix()+3600 {
		t.Fatalf("ExpiresAt = %d, want %d", tokenInfo.ExpiresAt, now.Unix()+3600)
	}
	if tokenInfo.DisplayName != "Windsurf User" {
		t.Fatalf("DisplayName = %q, want %q", tokenInfo.DisplayName, "Windsurf User")
	}
	if tokenInfo.APIServerURL != "https://server.codeium.com" {
		t.Fatalf("APIServerURL = %q, want %q", tokenInfo.APIServerURL, "https://server.codeium.com")
	}
}

func TestWindsurfOAuthServiceRefreshAccountToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/firebase":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id_token":      "firebase-id-token",
				"refresh_token": "firebase-refresh-token-new",
				"expires_in":    "1800",
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

	svc := NewWindsurfOAuthService(nil)
	svc.SetEndpointURLs(server.URL+"/firebase", server.URL+"/register")
	svc.SetHTTPClientFactory(func(string) (*http.Client, error) {
		return server.Client(), nil
	})
	svc.SetNowFunc(func() time.Time { return time.Unix(1_762_000_100, 0).UTC() })

	account := &Account{
		ID:       42,
		Platform: PlatformWindsurf,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"refresh_token": "firebase-refresh-token-old",
		},
	}

	tokenInfo, err := svc.RefreshAccountToken(context.Background(), account)
	if err != nil {
		t.Fatalf("RefreshAccountToken() unexpected err: %v", err)
	}
	if tokenInfo.Token != "windsurf-runtime-token" {
		t.Fatalf("Token = %q, want %q", tokenInfo.Token, "windsurf-runtime-token")
	}
}

func TestWindsurfOAuthServiceRefreshAccountTokenRequiresRefreshToken(t *testing.T) {
	svc := NewWindsurfOAuthService(nil)
	_, err := svc.RefreshAccountToken(context.Background(), &Account{
		ID:          7,
		Platform:    PlatformWindsurf,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error when refresh token is missing")
	}
	if !strings.Contains(err.Error(), "refresh token") {
		t.Fatalf("error = %q, want to contain %q", err.Error(), "refresh token")
	}
}

func TestWindsurfOAuthServiceResolveRuntimeTokenKeepsExistingAPIKey(t *testing.T) {
	var (
		probeCalls    int
		registerCalls int
	)

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case windsurfUserStatusPath:
			probeCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"userStatus": map[string]any{
					"planStatus": map[string]any{},
				},
			})
		case "/register":
			registerCalls++
			t.Fatalf("register endpoint should not be called for an already valid runtime key")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := NewWindsurfOAuthService(nil)
	svc.SetEndpointURLs(server.URL+"/firebase", server.URL+"/register")
	svc.SetHTTPClientFactory(func(string) (*http.Client, error) {
		return newWindsurfOAuthRewriteClient(server), nil
	})

	tokenInfo, err := svc.ResolveRuntimeToken(context.Background(), "windsurf-runtime-token", nil)
	if err != nil {
		t.Fatalf("ResolveRuntimeToken() unexpected err: %v", err)
	}
	if tokenInfo.Token != "windsurf-runtime-token" {
		t.Fatalf("Token = %q, want %q", tokenInfo.Token, "windsurf-runtime-token")
	}
	if probeCalls == 0 {
		t.Fatal("expected runtime token probe to be called")
	}
	if registerCalls != 0 {
		t.Fatalf("registerCalls = %d, want %d", registerCalls, 0)
	}
}

func TestWindsurfOAuthServiceResolveRuntimeTokenExchangesAuthToken(t *testing.T) {
	var (
		probeCalls     int
		registerCalls  int
		registerSource string
	)

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case windsurfUserStatusPath:
			probeCalls++
			w.WriteHeader(http.StatusUnauthorized)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":"unauthenticated","message":"invalid api key"}`))
		case "/register":
			registerCalls++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			registerSource, _ = body["firebase_id_token"].(string)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"api_key":        "windsurf-runtime-token",
				"name":           "Windsurf Token User",
				"api_server_url": "https://server.self-serve.windsurf.com",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := NewWindsurfOAuthService(nil)
	svc.SetEndpointURLs(server.URL+"/firebase", server.URL+"/register")
	svc.SetHTTPClientFactory(func(string) (*http.Client, error) {
		return newWindsurfOAuthRewriteClient(server), nil
	})

	tokenInfo, err := svc.ResolveRuntimeToken(context.Background(), "windsurf-auth-token", nil)
	if err != nil {
		t.Fatalf("ResolveRuntimeToken() unexpected err: %v", err)
	}
	if tokenInfo.Token != "windsurf-runtime-token" {
		t.Fatalf("Token = %q, want %q", tokenInfo.Token, "windsurf-runtime-token")
	}
	if tokenInfo.DisplayName != "Windsurf Token User" {
		t.Fatalf("DisplayName = %q, want %q", tokenInfo.DisplayName, "Windsurf Token User")
	}
	if tokenInfo.APIServerURL != "https://server.self-serve.windsurf.com" {
		t.Fatalf("APIServerURL = %q, want %q", tokenInfo.APIServerURL, "https://server.self-serve.windsurf.com")
	}
	if probeCalls == 0 {
		t.Fatal("expected runtime token probe to be attempted before exchange")
	}
	if registerCalls != 1 {
		t.Fatalf("registerCalls = %d, want %d", registerCalls, 1)
	}
	if registerSource != "windsurf-auth-token" {
		t.Fatalf("register source = %q, want %q", registerSource, "windsurf-auth-token")
	}
}

func newWindsurfOAuthRewriteClient(server *httptest.Server) *http.Client {
	target, err := neturl.Parse(server.URL)
	if err != nil {
		panic(err)
	}
	baseClient := server.Client()
	baseTransport := baseClient.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}

	return &http.Client{
		Timeout: baseClient.Timeout,
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			cloned := req.Clone(req.Context())
			cloned.URL.Scheme = target.Scheme
			cloned.URL.Host = target.Host
			cloned.Host = target.Host
			return baseTransport.RoundTrip(cloned)
		}),
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
