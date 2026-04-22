package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	windsurfUserStatusPath   = "/exa.seat_management_pb.SeatManagementService/GetUserStatus"
	windsurfModelConfigsPath = "/exa.api_server_pb.ApiServerService/GetCascadeModelConfigs"
	windsurfRateLimitPath    = "/exa.api_server_pb.ApiServerService/CheckUserMessageRateLimit"
	windsurfUserAgent        = "windsurf/1.108.2"
)

var windsurfServerHosts = []string{
	"server.codeium.com",
	"server.self-serve.windsurf.com",
}

type WindsurfFetchRequest struct {
	APIToken           string
	ProxyURL           string
	AccountID          int64
	AccountConcurrency int
}

type WindsurfUserStatusSnapshot struct {
	PlanName               string
	HasPaidFeatures        bool
	DailyRemainingPercent  float64
	WeeklyRemainingPercent float64
	DailyResetAt           *time.Time
	WeeklyResetAt          *time.Time
	CreditBalance          float64
}

type WindsurfModelConfig struct {
	ModelID          string
	ModelUID         string
	DisplayName      string
	Provider         string
	CreditMultiplier float64
}

type WindsurfRateLimitSnapshot struct {
	HasCapacity       bool
	MessagesRemaining int
	MaxMessages       int
}

type windsurfJSONMetadata struct {
	APIKey           string `json:"apiKey"`
	IDEName          string `json:"ideName"`
	IDEVersion       string `json:"ideVersion"`
	ExtensionName    string `json:"extensionName"`
	ExtensionVersion string `json:"extensionVersion"`
	Locale           string `json:"locale"`
}

type windsurfJSONRequest struct {
	Metadata windsurfJSONMetadata `json:"metadata"`
}

type windsurfUserStatusResponse struct {
	UserStatus struct {
		PlanStatus struct {
			DailyQuotaRemainingPercent  float64 `json:"dailyQuotaRemainingPercent"`
			WeeklyQuotaRemainingPercent float64 `json:"weeklyQuotaRemainingPercent"`
			DailyQuotaResetAtUnix       int64   `json:"dailyQuotaResetAtUnix"`
			WeeklyQuotaResetAtUnix      int64   `json:"weeklyQuotaResetAtUnix"`
			OverageBalanceMicros        float64 `json:"overageBalanceMicros"`
			AvailablePromptCredits      float64 `json:"availablePromptCredits"`
			AvailableFlexCredits        float64 `json:"availableFlexCredits"`
			PlanInfo                    struct {
				PlanName        string `json:"planName"`
				HasPaidFeatures bool   `json:"hasPaidFeatures"`
			} `json:"planInfo"`
		} `json:"planStatus"`
	} `json:"userStatus"`
	PlanInfo struct {
		PlanName        string `json:"planName"`
		HasPaidFeatures bool   `json:"hasPaidFeatures"`
	} `json:"planInfo"`
}

type windsurfModelConfigsResponse struct {
	ClientModelConfigs []struct {
		ModelUID         string  `json:"modelUid"`
		DisplayName      string  `json:"displayName"`
		Name             string  `json:"name"`
		Provider         string  `json:"provider"`
		CreditMultiplier float64 `json:"creditMultiplier"`
	} `json:"clientModelConfigs"`
}

type windsurfRateLimitResponse struct {
	HasCapacity       *bool `json:"hasCapacity"`
	MessagesRemaining int   `json:"messagesRemaining"`
	MaxMessages       int   `json:"maxMessages"`
}

// WindsurfUsageFetcher queries public Windsurf Connect-RPC JSON endpoints.
type WindsurfUsageFetcher struct {
	httpUpstream HTTPUpstream
}

func NewWindsurfUsageFetcher(httpUpstream HTTPUpstream) *WindsurfUsageFetcher {
	return &WindsurfUsageFetcher{httpUpstream: httpUpstream}
}

func (f *WindsurfUsageFetcher) GetUserStatus(ctx context.Context, req WindsurfFetchRequest) (*WindsurfUserStatusSnapshot, error) {
	var resp windsurfUserStatusResponse
	if err := f.postJSON(ctx, req, windsurfUserStatusPath, &resp); err != nil {
		return nil, err
	}

	planStatus := resp.UserStatus.PlanStatus
	planName := strings.TrimSpace(planStatus.PlanInfo.PlanName)
	if planName == "" {
		planName = strings.TrimSpace(resp.PlanInfo.PlanName)
	}
	hasPaidFeatures := planStatus.PlanInfo.HasPaidFeatures || resp.PlanInfo.HasPaidFeatures

	creditBalance := 0.0
	switch {
	case planStatus.OverageBalanceMicros > 0:
		creditBalance = planStatus.OverageBalanceMicros / 1_000_000
	case planStatus.AvailableFlexCredits > 0:
		creditBalance = planStatus.AvailableFlexCredits / 100
	case planStatus.AvailablePromptCredits > 0:
		creditBalance = planStatus.AvailablePromptCredits / 100
	}

	return &WindsurfUserStatusSnapshot{
		PlanName:               planName,
		HasPaidFeatures:        hasPaidFeatures,
		DailyRemainingPercent:  planStatus.DailyQuotaRemainingPercent,
		WeeklyRemainingPercent: planStatus.WeeklyQuotaRemainingPercent,
		DailyResetAt:           unixTimePtr(planStatus.DailyQuotaResetAtUnix),
		WeeklyResetAt:          unixTimePtr(planStatus.WeeklyQuotaResetAtUnix),
		CreditBalance:          creditBalance,
	}, nil
}

func (f *WindsurfUsageFetcher) GetCascadeModelConfigs(ctx context.Context, req WindsurfFetchRequest) ([]WindsurfModelConfig, error) {
	var resp windsurfModelConfigsResponse
	if err := f.postJSON(ctx, req, windsurfModelConfigsPath, &resp); err != nil {
		return nil, err
	}

	configs := make([]WindsurfModelConfig, 0, len(resp.ClientModelConfigs))
	for _, item := range resp.ClientModelConfigs {
		modelID := normalizeWindsurfModelIdentifier(item.Name)
		if modelID == "" {
			modelID = normalizeWindsurfModelIdentifier(item.DisplayName)
		}
		if modelID == "" {
			modelID = normalizeWindsurfModelIdentifier(item.ModelUID)
		}
		if modelID == "" {
			continue
		}
		configs = append(configs, WindsurfModelConfig{
			ModelID:          modelID,
			ModelUID:         strings.TrimSpace(item.ModelUID),
			DisplayName:      strings.TrimSpace(firstNonEmptyString(item.DisplayName, item.Name, item.ModelUID)),
			Provider:         strings.TrimSpace(item.Provider),
			CreditMultiplier: item.CreditMultiplier,
		})
	}
	return configs, nil
}

func (f *WindsurfUsageFetcher) CheckMessageRateLimit(ctx context.Context, req WindsurfFetchRequest) (*WindsurfRateLimitSnapshot, error) {
	var resp windsurfRateLimitResponse
	if err := f.postJSON(ctx, req, windsurfRateLimitPath, &resp); err != nil {
		return nil, err
	}

	hasCapacity := true
	if resp.HasCapacity != nil {
		hasCapacity = *resp.HasCapacity
	}

	return &WindsurfRateLimitSnapshot{
		HasCapacity:       hasCapacity,
		MessagesRemaining: resp.MessagesRemaining,
		MaxMessages:       resp.MaxMessages,
	}, nil
}

func (f *WindsurfUsageFetcher) postJSON(ctx context.Context, req WindsurfFetchRequest, path string, out any) error {
	if f == nil || f.httpUpstream == nil {
		return fmt.Errorf("windsurf usage fetcher not configured")
	}

	body := windsurfJSONRequest{
		Metadata: windsurfJSONMetadata{
			APIKey:           req.APIToken,
			IDEName:          "windsurf",
			IDEVersion:       "1.108.2",
			ExtensionName:    "windsurf",
			ExtensionVersion: "1.108.2",
			Locale:           "en",
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal windsurf request: %w", err)
	}

	var lastErr error
	for _, host := range windsurfServerHosts {
		endpoint := "https://" + host + path
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("build windsurf request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json")
		httpReq.Header.Set("Connect-Protocol-Version", "1")
		httpReq.Header.Set("User-Agent", windsurfUserAgent)

		resp, err := f.httpUpstream.Do(httpReq, req.ProxyURL, req.AccountID, req.AccountConcurrency)
		if err != nil {
			lastErr = fmt.Errorf("%s request failed: %w", host, err)
			continue
		}

		readErr := func() error {
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode >= http.StatusBadRequest {
				bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
				return fmt.Errorf("%s returned %d: %s", host, resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
			}
			if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
				return fmt.Errorf("%s decode failed: %w", host, err)
			}
			return nil
		}()
		if readErr == nil {
			return nil
		}
		lastErr = readErr
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("all windsurf upstream hosts failed")
	}
	return lastErr
}

func normalizeWindsurfModelIdentifier(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return strings.ToLower(strings.ReplaceAll(trimmed, "_", "-"))
}

func unixTimePtr(value int64) *time.Time {
	if value <= 0 {
		return nil
	}
	ts := time.Unix(value, 0).UTC()
	return &ts
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
