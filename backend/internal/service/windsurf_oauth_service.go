package service

import (
	"context"
	"net/http"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type WindsurfOAuthService struct {
	proxyRepo ProxyRepository
}

func NewWindsurfOAuthService(proxyRepo ProxyRepository) *WindsurfOAuthService {
	return &WindsurfOAuthService{proxyRepo: proxyRepo}
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
	_ = s
	_ = ctx
	_ = refreshToken
	_ = proxyID
	return nil, windsurfOAuthNotImplemented()
}

func (s *WindsurfOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*WindsurfTokenInfo, error) {
	_ = s
	_ = ctx
	if account == nil {
		return nil, infraerrors.BadRequest("WINDSURF_OAUTH_ACCOUNT_REQUIRED", "account is required")
	}
	if account.Platform != PlatformWindsurf || account.Type != AccountTypeOAuth {
		return nil, infraerrors.BadRequest("WINDSURF_OAUTH_ACCOUNT_INVALID", "account must be a windsurf oauth account")
	}
	return nil, windsurfOAuthNotImplemented()
}

func (s *WindsurfOAuthService) BuildAccountCredentials(tokenInfo *WindsurfTokenInfo) map[string]any {
	if tokenInfo == nil {
		return map[string]any{}
	}

	credentials := map[string]any{
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
	return credentials
}

func windsurfOAuthNotImplemented() error {
	return infraerrors.New(http.StatusNotImplemented, "WINDSURF_OAUTH_NOT_IMPLEMENTED", "windsurf oauth flow is not implemented yet")
}
