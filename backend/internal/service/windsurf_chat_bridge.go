package service

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"golang.org/x/net/http2"
)

// WindsurfChatBridge is the phase-2 bridge seam for forwarding normalized chat
// requests to the actual Windsurf upstream protocol adapter. The concrete LS /
// gRPC implementation is added behind this contract without changing handler or
// gateway code.
type WindsurfChatBridge struct {
	baseURL          string
	csrfToken        string
	workspaceDir     string
	extensionVersion string
	httpClient       *http.Client
}

func NewWindsurfChatBridge() *WindsurfChatBridge {
	return &WindsurfChatBridge{
		baseURL:          resolveWindsurfLSBaseURL(),
		csrfToken:        firstNonEmptyString(os.Getenv("WINDSURF_LS_CSRF_TOKEN"), "windsurf-api-csrf-fixed-token"),
		workspaceDir:     firstNonEmptyString(os.Getenv("WINDSURF_LS_WORKSPACE_DIR"), filepath.Join(os.TempDir(), "windsurf-workspace")),
		extensionVersion: firstNonEmptyString(os.Getenv("WINDSURF_LS_VERSION"), "1.9600.41"),
		httpClient: &http.Client{
			Transport: &http2.Transport{
				AllowHTTP: true,
				DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
					var dialer net.Dialer
					return dialer.DialContext(ctx, network, addr)
				},
			},
		},
	}
}

func (b *WindsurfChatBridge) Complete(
	ctx context.Context,
	account *Account,
	model WindsurfResolvedModel,
	req *apicompat.ChatCompletionsRequest,
) (*WindsurfBridgeResult, error) {
	if b == nil || b.httpClient == nil || strings.TrimSpace(b.baseURL) == "" {
		return nil, ErrWindsurfChatBridgeUnavailable
	}
	if model.ModelUID != "" {
		return b.completeCascade(ctx, account, model, req)
	}
	if model.EnumValue > 0 {
		return b.completeLegacy(ctx, account, model, req)
	}
	return nil, ErrWindsurfModelNotSupported
}

func (b *WindsurfChatBridge) Stream(
	ctx context.Context,
	account *Account,
	model WindsurfResolvedModel,
	req *apicompat.ChatCompletionsRequest,
	emit func(WindsurfBridgeStreamChunk) error,
) (*WindsurfBridgeResult, error) {
	if b == nil || b.httpClient == nil || strings.TrimSpace(b.baseURL) == "" {
		return nil, ErrWindsurfChatBridgeUnavailable
	}
	if model.ModelUID != "" {
		return b.streamCascade(ctx, account, model, req, emit)
	}
	if model.EnumValue > 0 {
		return b.streamLegacy(ctx, account, model, req, emit)
	}
	return nil, ErrWindsurfModelNotSupported
}

func resolveWindsurfLSBaseURL() string {
	baseURL := strings.TrimSpace(os.Getenv("WINDSURF_LS_ADDR"))
	if baseURL == "" {
		baseURL = "http://127.0.0.1:42100"
	}
	if strings.HasPrefix(baseURL, "http://") || strings.HasPrefix(baseURL, "https://") {
		return strings.TrimRight(baseURL, "/")
	}
	return "http://" + strings.TrimRight(baseURL, "/")
}
