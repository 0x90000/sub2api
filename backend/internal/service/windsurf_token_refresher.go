package service

import (
	"context"
	"strconv"
	"time"
)

type WindsurfTokenRefresher struct {
	windsurfOAuthService *WindsurfOAuthService
}

func NewWindsurfTokenRefresher(windsurfOAuthService *WindsurfOAuthService) *WindsurfTokenRefresher {
	return &WindsurfTokenRefresher{windsurfOAuthService: windsurfOAuthService}
}

func (r *WindsurfTokenRefresher) CacheKey(account *Account) string {
	return "windsurf:account:" + strconv.FormatInt(account.ID, 10)
}

func (r *WindsurfTokenRefresher) CanRefresh(account *Account) bool {
	return account != nil && account.Platform == PlatformWindsurf && account.Type == AccountTypeOAuth
}

func (r *WindsurfTokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	if !r.CanRefresh(account) {
		return false
	}
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return false
	}
	return time.Until(*expiresAt) < refreshWindow
}

func (r *WindsurfTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	tokenInfo, err := r.windsurfOAuthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
	}

	newCredentials := r.windsurfOAuthService.BuildAccountCredentials(tokenInfo)
	newCredentials = MergeCredentials(account.Credentials, newCredentials)

	return newCredentials, nil
}
