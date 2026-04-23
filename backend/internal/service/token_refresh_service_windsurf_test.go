package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestProvideTokenRefreshServiceRegistersWindsurfRefresher(t *testing.T) {
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			Enabled: false,
		},
	}
	windsurfOAuthService := NewWindsurfOAuthService(nil)

	service := ProvideTokenRefreshService(
		nil,
		nil,
		nil,
		nil,
		nil,
		windsurfOAuthService,
		nil,
		nil,
		cfg,
		nil,
		nil,
		nil,
		nil,
	)

	require.Len(t, service.refreshers, 5)
	require.Len(t, service.executors, 5)

	refresher, ok := service.refreshers[4].(*WindsurfTokenRefresher)
	require.True(t, ok)
	require.Same(t, windsurfOAuthService, refresher.windsurfOAuthService)
}
