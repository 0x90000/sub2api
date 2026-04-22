package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
)

const (
	windsurfFirebaseAPIKey     = "AIzaSyDsOl-1XpT5err0Tcnx8FFod1H8gVGIycY"
	windsurfFirebaseRefreshURL = "https://securetoken.googleapis.com/v1/token?key=" + windsurfFirebaseAPIKey
	windsurfCodeiumRegisterURL = "https://api.codeium.com/register_user/"
	windsurfOAuthUserAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/130.0.0.0 Safari/537.36"
)

type WindsurfOAuthService struct {
	proxyRepo          ProxyRepository
	firebaseRefreshURL string
	codeiumRegisterURL string
	httpClientFactory  func(proxyURL string) (*http.Client, error)
	now                func() time.Time
}

func NewWindsurfOAuthService(proxyRepo ProxyRepository) *WindsurfOAuthService {
	return &WindsurfOAuthService{
		proxyRepo:          proxyRepo,
		firebaseRefreshURL: windsurfFirebaseRefreshURL,
		codeiumRegisterURL: windsurfCodeiumRegisterURL,
		httpClientFactory:  newWindsurfOAuthHTTPClient,
		now:                time.Now,
	}
}

func (s *WindsurfOAuthService) SetEndpointURLs(firebaseRefreshURL, codeiumRegisterURL string) {
	if s == nil {
		return
	}
	if strings.TrimSpace(firebaseRefreshURL) != "" {
		s.firebaseRefreshURL = strings.TrimSpace(firebaseRefreshURL)
	}
	if strings.TrimSpace(codeiumRegisterURL) != "" {
		s.codeiumRegisterURL = strings.TrimSpace(codeiumRegisterURL)
	}
}

func (s *WindsurfOAuthService) SetHTTPClientFactory(factory func(proxyURL string) (*http.Client, error)) {
	if s == nil || factory == nil {
		return
	}
	s.httpClientFactory = factory
}

func (s *WindsurfOAuthService) SetNowFunc(now func() time.Time) {
	if s == nil || now == nil {
		return
	}
	s.now = now
}

type WindsurfAuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
	State     string `json:"state,omitempty"`
}

type WindsurfExchangeCodeInput struct {
	SessionID   string
	Code        string
	State       string
	RedirectURI string
	ProxyID     *int64
}

type WindsurfTokenInfo struct {
	Token        string `json:"token,omitempty"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	Scope        string `json:"scope,omitempty"`
	ExpiresIn    int64  `json:"expires_in"`
	ExpiresAt    int64  `json:"expires_at"`
	Subject      string `json:"subject,omitempty"`
	Email        string `json:"email,omitempty"`
	DisplayName  string `json:"display_name,omitempty"`
	AvatarURL    string `json:"avatar_url,omitempty"`
	APIServerURL string `json:"api_server_url,omitempty"`
}

func (s *WindsurfOAuthService) GenerateAuthURL(ctx context.Context, proxyID *int64, redirectURI string) (*WindsurfAuthURLResult, error) {
	_ = s
	_ = ctx
	_ = proxyID
	_ = redirectURI
	return nil, windsurfOAuthNotImplemented()
}

func (s *WindsurfOAuthService) ExchangeCode(ctx context.Context, input *WindsurfExchangeCodeInput) (*WindsurfTokenInfo, error) {
	_ = s
	_ = ctx
	_ = input
	return nil, windsurfOAuthNotImplemented()
}

func (s *WindsurfOAuthService) RefreshToken(ctx context.Context, refreshToken string, proxyID *int64) (*WindsurfTokenInfo, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, infraerrors.BadRequest("WINDSURF_OAUTH_REFRESH_TOKEN_REQUIRED", "refresh token is required")
	}

	proxyURL, err := s.resolveProxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}

	idToken, nextRefreshToken, expiresIn, err := s.refreshFirebaseToken(ctx, refreshToken, proxyURL)
	if err != nil {
		return nil, err
	}

	runtimeToken, displayName, apiServerURL, err := s.registerRuntimeToken(ctx, idToken, proxyURL)
	if err != nil {
		return nil, err
	}

	now := s.timeNow().Unix()
	return &WindsurfTokenInfo{
		Token:        runtimeToken,
		AccessToken:  idToken,
		IDToken:      idToken,
		RefreshToken: nextRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		ExpiresAt:    now + expiresIn,
		DisplayName:  displayName,
		APIServerURL: apiServerURL,
	}, nil
}

func (s *WindsurfOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*WindsurfTokenInfo, error) {
	if account == nil {
		return nil, infraerrors.BadRequest("WINDSURF_OAUTH_ACCOUNT_REQUIRED", "account is required")
	}
	if account.Platform != PlatformWindsurf || account.Type != AccountTypeOAuth {
		return nil, infraerrors.BadRequest("WINDSURF_OAUTH_ACCOUNT_INVALID", "account must be a windsurf oauth account")
	}

	refreshToken := strings.TrimSpace(account.GetCredential("refresh_token"))
	if refreshToken == "" {
		return nil, infraerrors.BadRequest("WINDSURF_OAUTH_REFRESH_TOKEN_REQUIRED", "windsurf oauth account is missing refresh token")
	}

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	} else if account.ProxyID != nil && s.proxyRepo != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
		}
	}

	idToken, nextRefreshToken, expiresIn, err := s.refreshFirebaseToken(ctx, refreshToken, proxyURL)
	if err != nil {
		return nil, err
	}

	runtimeToken, displayName, apiServerURL, err := s.registerRuntimeToken(ctx, idToken, proxyURL)
	if err != nil {
		return nil, err
	}

	now := s.timeNow().Unix()
	tokenInfo := &WindsurfTokenInfo{
		Token:        runtimeToken,
		AccessToken:  idToken,
		IDToken:      idToken,
		RefreshToken: nextRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		ExpiresAt:    now + expiresIn,
		DisplayName:  displayName,
		APIServerURL: apiServerURL,
	}
	if tokenInfo.DisplayName == "" {
		tokenInfo.DisplayName = strings.TrimSpace(account.GetExtraString("oauth_display_name"))
	}
	return tokenInfo, nil
}

func (s *WindsurfOAuthService) BuildAccountCredentials(tokenInfo *WindsurfTokenInfo) map[string]any {
	if tokenInfo == nil {
		return map[string]any{}
	}

	credentials := map[string]any{
		"token":         tokenInfo.Token,
		"access_token":  tokenInfo.AccessToken,
		"refresh_token": tokenInfo.RefreshToken,
		"expires_in":    tokenInfo.ExpiresIn,
		"expires_at":    tokenInfo.ExpiresAt,
	}
	if strings.TrimSpace(tokenInfo.IDToken) != "" {
		credentials["id_token"] = tokenInfo.IDToken
	}
	if strings.TrimSpace(tokenInfo.TokenType) != "" {
		credentials["token_type"] = tokenInfo.TokenType
	}
	if strings.TrimSpace(tokenInfo.Scope) != "" {
		credentials["scope"] = tokenInfo.Scope
	}
	if strings.TrimSpace(tokenInfo.APIServerURL) != "" {
		credentials["api_server_url"] = tokenInfo.APIServerURL
	}
	return credentials
}

func (s *WindsurfOAuthService) BuildAccountExtra(tokenInfo *WindsurfTokenInfo, current map[string]any) map[string]any {
	extra := map[string]any{}
	for k, v := range current {
		extra[k] = v
	}
	extra["oauth_last_refresh_at"] = s.timeNow().UTC().Format(time.RFC3339)
	extra["oauth_last_refresh_error"] = ""
	if tokenInfo != nil {
		if displayName := strings.TrimSpace(tokenInfo.DisplayName); displayName != "" {
			extra["oauth_display_name"] = displayName
		}
		if email := strings.TrimSpace(tokenInfo.Email); email != "" {
			extra["oauth_email"] = email
		}
		if avatarURL := strings.TrimSpace(tokenInfo.AvatarURL); avatarURL != "" {
			extra["oauth_avatar_url"] = avatarURL
		}
	}
	return extra
}

func windsurfOAuthNotImplemented() error {
	return infraerrors.New(http.StatusNotImplemented, "WINDSURF_OAUTH_NOT_IMPLEMENTED", "windsurf oauth flow is not implemented yet")
}

type windsurfFirebaseRefreshResponse struct {
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    string `json:"expires_in"`
	Error        *struct {
		Message string `json:"message"`
		Code    any    `json:"code"`
	} `json:"error,omitempty"`
}

type windsurfCodeiumRegisterResponse struct {
	APIKey       string `json:"api_key"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	APIServerURL string `json:"api_server_url"`
}

func (s *WindsurfOAuthService) refreshFirebaseToken(ctx context.Context, refreshToken, proxyURL string) (string, string, int64, error) {
	form := "grant_type=refresh_token&refresh_token=" + neturl.QueryEscape(refreshToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.firebaseRefreshURL, strings.NewReader(form))
	if err != nil {
		return "", "", 0, fmt.Errorf("build windsurf firebase refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "https://windsurf.com/")
	req.Header.Set("Origin", "https://windsurf.com")
	req.Header.Set("User-Agent", windsurfOAuthUserAgent)

	var respBody windsurfFirebaseRefreshResponse
	if err := s.doJSON(req, proxyURL, &respBody); err != nil {
		return "", "", 0, err
	}
	if respBody.Error != nil {
		message := strings.TrimSpace(respBody.Error.Message)
		if message == "" {
			message = "unknown firebase refresh error"
		}
		return "", "", 0, fmt.Errorf("windsurf firebase token refresh failed: %s", message)
	}

	idToken := strings.TrimSpace(respBody.IDToken)
	if idToken == "" {
		return "", "", 0, fmt.Errorf("windsurf firebase token refresh returned empty id_token")
	}
	nextRefreshToken := strings.TrimSpace(respBody.RefreshToken)
	if nextRefreshToken == "" {
		nextRefreshToken = refreshToken
	}
	expiresIn, err := strconv.ParseInt(strings.TrimSpace(respBody.ExpiresIn), 10, 64)
	if err != nil || expiresIn <= 0 {
		expiresIn = 3600
	}
	return idToken, nextRefreshToken, expiresIn, nil
}

func (s *WindsurfOAuthService) registerRuntimeToken(ctx context.Context, idToken, proxyURL string) (string, string, string, error) {
	body, err := json.Marshal(map[string]any{
		"firebase_id_token": idToken,
	})
	if err != nil {
		return "", "", "", fmt.Errorf("marshal windsurf register payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.codeiumRegisterURL, bytes.NewReader(body))
	if err != nil {
		return "", "", "", fmt.Errorf("build windsurf register request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "https://windsurf.com/")
	req.Header.Set("Origin", "https://windsurf.com")
	req.Header.Set("User-Agent", windsurfOAuthUserAgent)

	var respBody windsurfCodeiumRegisterResponse
	if err := s.doJSON(req, proxyURL, &respBody); err != nil {
		return "", "", "", err
	}
	token := strings.TrimSpace(respBody.APIKey)
	if token == "" {
		return "", "", "", fmt.Errorf("windsurf runtime token registration returned empty api_key")
	}
	return token, strings.TrimSpace(respBody.Name), strings.TrimSpace(respBody.APIServerURL), nil
}

func (s *WindsurfOAuthService) doJSON(req *http.Request, proxyURL string, out any) error {
	client, err := s.httpClient(proxyURL)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read windsurf oauth response: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		preview := strings.TrimSpace(string(body))
		if len(preview) > 200 {
			preview = preview[:200]
		}
		return fmt.Errorf("windsurf oauth upstream returned status %d: %s", resp.StatusCode, preview)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode windsurf oauth response: %w", err)
	}
	return nil
}

func (s *WindsurfOAuthService) resolveProxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil {
		return "", nil
	}
	if s.proxyRepo == nil {
		return "", infraerrors.BadRequest("WINDSURF_OAUTH_PROXY_NOT_FOUND", "proxy repository is not configured")
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		return "", infraerrors.Newf(http.StatusBadRequest, "WINDSURF_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
	}
	if proxy == nil {
		return "", nil
	}
	return proxy.URL(), nil
}

func (s *WindsurfOAuthService) httpClient(proxyURL string) (*http.Client, error) {
	if s.httpClientFactory == nil {
		return newWindsurfOAuthHTTPClient(proxyURL)
	}
	return s.httpClientFactory(proxyURL)
}

func (s *WindsurfOAuthService) timeNow() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func newWindsurfOAuthHTTPClient(proxyURL string) (*http.Client, error) {
	transport := &http.Transport{}
	if proxyURL != "" {
		_, parsed, err := proxyurl.Parse(proxyURL)
		if err != nil {
			return nil, err
		}
		if parsed != nil {
			transport.Proxy = http.ProxyURL(parsed)
		}
	}
	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}, nil
}
