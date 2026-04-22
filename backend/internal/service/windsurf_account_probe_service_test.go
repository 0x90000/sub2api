package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

func TestWindsurfAccountProbePersistsAccountSnapshot(t *testing.T) {
	repo := &windsurfProbeAccountRepoStub{
		account: &Account{
			ID:          42,
			Platform:    PlatformWindsurf,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{"token": "ws-token"},
			Extra:       map[string]any{},
			Concurrency: 2,
		},
	}
	upstream := &windsurfProbeHTTPUpstreamStub{
		responses: map[string]*http.Response{
			"https://server.codeium.com/exa.seat_management_pb.SeatManagementService/GetUserStatus": {
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"userStatus": {
						"planStatus": {
							"overageBalanceMicros": 42500000,
							"dailyQuotaRemainingPercent": 81,
							"weeklyQuotaRemainingPercent": 54,
							"dailyQuotaResetAtUnix": 1767225600,
							"weeklyQuotaResetAtUnix": 1767484800
						}
					}
				}`)),
			},
			"https://server.codeium.com/exa.api_server_pb.ApiServerService/GetCascadeModelConfigs": {
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"clientModelConfigs": [
						{"modelUid": "gpt-4.1", "provider": "MODEL_PROVIDER_OPENAI", "creditMultiplier": 1},
						{"modelUid": "claude-4-sonnet", "provider": "MODEL_PROVIDER_ANTHROPIC", "creditMultiplier": 2}
					]
				}`)),
			},
			"https://server.codeium.com/exa.api_server_pb.ApiServerService/CheckUserMessageRateLimit": {
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"hasCapacity": false,
					"messagesRemaining": 0,
					"maxMessages": 20
				}`)),
			},
		},
	}

	usageFetcher := NewWindsurfUsageFetcher(upstream)
	service := NewWindsurfAccountProbeService(repo, &windsurfProbeProxyRepoStub{}, usageFetcher)

	account, err := service.ProbeAndPersist(context.Background(), 42)
	if err != nil {
		t.Fatalf("ProbeAndPersist() error = %v", err)
	}

	if got := account.GetWindsurfPlanTier(); got != "pro" {
		t.Fatalf("plan_tier = %q, want %q", got, "pro")
	}
	if got := account.GetWindsurfCreditBalance(); got != 42.5 {
		t.Fatalf("credit_balance = %v, want %v", got, 42.5)
	}

	wantModels := []string{"claude-4-sonnet", "gpt-4.1"}
	gotModels := account.GetWindsurfAllowedModels()
	if len(gotModels) != len(wantModels) {
		t.Fatalf("allowed_models len = %d, want %d (%v)", len(gotModels), len(wantModels), gotModels)
	}
	for i := range wantModels {
		if gotModels[i] != wantModels[i] {
			t.Fatalf("allowed_models[%d] = %q, want %q", i, gotModels[i], wantModels[i])
		}
	}

	if repo.rateLimitedAt == nil {
		t.Fatal("expected rate limit snapshot to be persisted")
	}
	if got := ParseExtraInt(account.Extra["messages_remaining"]); got != 0 {
		t.Fatalf("messages_remaining = %d, want %d", got, 0)
	}
	if got := ParseExtraInt(account.Extra["max_messages"]); got != 20 {
		t.Fatalf("max_messages = %d, want %d", got, 20)
	}
	if got := account.GetExtraString("rate_limited_at"); got == "" {
		t.Fatal("expected rate_limited_at to be stored in extra")
	}
}

type windsurfProbeAccountRepoStub struct {
	account       *Account
	rateLimitedAt *time.Time
}

func (s *windsurfProbeAccountRepoStub) Create(context.Context, *Account) error {
	panic("unexpected Create")
}

func (s *windsurfProbeAccountRepoStub) GetByID(context.Context, int64) (*Account, error) {
	return s.account, nil
}

func (s *windsurfProbeAccountRepoStub) GetByIDs(context.Context, []int64) ([]*Account, error) {
	panic("unexpected GetByIDs")
}

func (s *windsurfProbeAccountRepoStub) ExistsByID(context.Context, int64) (bool, error) {
	panic("unexpected ExistsByID")
}

func (s *windsurfProbeAccountRepoStub) GetByCRSAccountID(context.Context, string) (*Account, error) {
	panic("unexpected GetByCRSAccountID")
}

func (s *windsurfProbeAccountRepoStub) FindByExtraField(context.Context, string, any) ([]Account, error) {
	panic("unexpected FindByExtraField")
}

func (s *windsurfProbeAccountRepoStub) ListCRSAccountIDs(context.Context) (map[string]int64, error) {
	panic("unexpected ListCRSAccountIDs")
}

func (s *windsurfProbeAccountRepoStub) Update(context.Context, *Account) error {
	panic("unexpected Update")
}

func (s *windsurfProbeAccountRepoStub) Delete(context.Context, int64) error {
	panic("unexpected Delete")
}

func (s *windsurfProbeAccountRepoStub) List(context.Context, pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	panic("unexpected List")
}

func (s *windsurfProbeAccountRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, string, int64, string) ([]Account, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters")
}

func (s *windsurfProbeAccountRepoStub) ListByGroup(context.Context, int64) ([]Account, error) {
	panic("unexpected ListByGroup")
}

func (s *windsurfProbeAccountRepoStub) ListActive(context.Context) ([]Account, error) {
	panic("unexpected ListActive")
}

func (s *windsurfProbeAccountRepoStub) ListByPlatform(context.Context, string) ([]Account, error) {
	panic("unexpected ListByPlatform")
}

func (s *windsurfProbeAccountRepoStub) UpdateLastUsed(context.Context, int64) error {
	panic("unexpected UpdateLastUsed")
}

func (s *windsurfProbeAccountRepoStub) BatchUpdateLastUsed(context.Context, map[int64]time.Time) error {
	panic("unexpected BatchUpdateLastUsed")
}

func (s *windsurfProbeAccountRepoStub) SetError(context.Context, int64, string) error {
	return nil
}

func (s *windsurfProbeAccountRepoStub) ClearError(context.Context, int64) error {
	return nil
}

func (s *windsurfProbeAccountRepoStub) SetSchedulable(context.Context, int64, bool) error {
	panic("unexpected SetSchedulable")
}

func (s *windsurfProbeAccountRepoStub) AutoPauseExpiredAccounts(context.Context, time.Time) (int64, error) {
	panic("unexpected AutoPauseExpiredAccounts")
}

func (s *windsurfProbeAccountRepoStub) BindGroups(context.Context, int64, []int64) error {
	panic("unexpected BindGroups")
}

func (s *windsurfProbeAccountRepoStub) ListSchedulable(context.Context) ([]Account, error) {
	panic("unexpected ListSchedulable")
}

func (s *windsurfProbeAccountRepoStub) ListSchedulableByGroupID(context.Context, int64) ([]Account, error) {
	panic("unexpected ListSchedulableByGroupID")
}

func (s *windsurfProbeAccountRepoStub) ListSchedulableByPlatform(context.Context, string) ([]Account, error) {
	panic("unexpected ListSchedulableByPlatform")
}

func (s *windsurfProbeAccountRepoStub) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]Account, error) {
	panic("unexpected ListSchedulableByGroupIDAndPlatform")
}

func (s *windsurfProbeAccountRepoStub) ListSchedulableByPlatforms(context.Context, []string) ([]Account, error) {
	panic("unexpected ListSchedulableByPlatforms")
}

func (s *windsurfProbeAccountRepoStub) ListSchedulableByGroupIDAndPlatforms(context.Context, int64, []string) ([]Account, error) {
	panic("unexpected ListSchedulableByGroupIDAndPlatforms")
}

func (s *windsurfProbeAccountRepoStub) ListSchedulableUngroupedByPlatform(context.Context, string) ([]Account, error) {
	panic("unexpected ListSchedulableUngroupedByPlatform")
}

func (s *windsurfProbeAccountRepoStub) ListSchedulableUngroupedByPlatforms(context.Context, []string) ([]Account, error) {
	panic("unexpected ListSchedulableUngroupedByPlatforms")
}

func (s *windsurfProbeAccountRepoStub) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	s.rateLimitedAt = &resetAt
	s.account.RateLimitResetAt = &resetAt
	return nil
}

func (s *windsurfProbeAccountRepoStub) SetModelRateLimit(context.Context, int64, string, time.Time) error {
	panic("unexpected SetModelRateLimit")
}

func (s *windsurfProbeAccountRepoStub) SetOverloaded(context.Context, int64, time.Time) error {
	panic("unexpected SetOverloaded")
}

func (s *windsurfProbeAccountRepoStub) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	panic("unexpected SetTempUnschedulable")
}

func (s *windsurfProbeAccountRepoStub) ClearTempUnschedulable(context.Context, int64) error {
	panic("unexpected ClearTempUnschedulable")
}

func (s *windsurfProbeAccountRepoStub) ClearRateLimit(context.Context, int64) error {
	s.rateLimitedAt = nil
	s.account.RateLimitResetAt = nil
	return nil
}

func (s *windsurfProbeAccountRepoStub) ClearAntigravityQuotaScopes(context.Context, int64) error {
	panic("unexpected ClearAntigravityQuotaScopes")
}

func (s *windsurfProbeAccountRepoStub) ClearModelRateLimits(context.Context, int64) error {
	panic("unexpected ClearModelRateLimits")
}

func (s *windsurfProbeAccountRepoStub) UpdateSessionWindow(context.Context, int64, *time.Time, *time.Time, string) error {
	panic("unexpected UpdateSessionWindow")
}

func (s *windsurfProbeAccountRepoStub) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	if s.account.Extra == nil {
		s.account.Extra = make(map[string]any, len(updates))
	}
	for key, value := range updates {
		s.account.Extra[key] = value
	}
	return nil
}

func (s *windsurfProbeAccountRepoStub) BulkUpdate(context.Context, []int64, AccountBulkUpdate) (int64, error) {
	panic("unexpected BulkUpdate")
}

func (s *windsurfProbeAccountRepoStub) IncrementQuotaUsed(context.Context, int64, float64) error {
	panic("unexpected IncrementQuotaUsed")
}

func (s *windsurfProbeAccountRepoStub) ResetQuotaUsed(context.Context, int64) error {
	panic("unexpected ResetQuotaUsed")
}

type windsurfProbeProxyRepoStub struct{}

func (s *windsurfProbeProxyRepoStub) Create(context.Context, *Proxy) error {
	panic("unexpected Create")
}

func (s *windsurfProbeProxyRepoStub) GetByID(context.Context, int64) (*Proxy, error) {
	return nil, nil
}

func (s *windsurfProbeProxyRepoStub) ListByIDs(context.Context, []int64) ([]Proxy, error) {
	panic("unexpected ListByIDs")
}

func (s *windsurfProbeProxyRepoStub) Update(context.Context, *Proxy) error {
	panic("unexpected Update")
}

func (s *windsurfProbeProxyRepoStub) Delete(context.Context, int64) error {
	panic("unexpected Delete")
}

func (s *windsurfProbeProxyRepoStub) List(context.Context, pagination.PaginationParams) ([]Proxy, *pagination.PaginationResult, error) {
	panic("unexpected List")
}

func (s *windsurfProbeProxyRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string) ([]Proxy, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters")
}

func (s *windsurfProbeProxyRepoStub) ListWithFiltersAndAccountCount(context.Context, pagination.PaginationParams, string, string, string) ([]ProxyWithAccountCount, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFiltersAndAccountCount")
}

func (s *windsurfProbeProxyRepoStub) ListActive(context.Context) ([]Proxy, error) {
	panic("unexpected ListActive")
}

func (s *windsurfProbeProxyRepoStub) ListActiveWithAccountCount(context.Context) ([]ProxyWithAccountCount, error) {
	panic("unexpected ListActiveWithAccountCount")
}

func (s *windsurfProbeProxyRepoStub) ExistsByHostPortAuth(context.Context, string, int, string, string) (bool, error) {
	panic("unexpected ExistsByHostPortAuth")
}

func (s *windsurfProbeProxyRepoStub) CountAccountsByProxyID(context.Context, int64) (int64, error) {
	panic("unexpected CountAccountsByProxyID")
}

func (s *windsurfProbeProxyRepoStub) ListAccountSummariesByProxyID(context.Context, int64) ([]ProxyAccountSummary, error) {
	panic("unexpected ListAccountSummariesByProxyID")
}

type windsurfProbeHTTPUpstreamStub struct {
	responses map[string]*http.Response
}

func (s *windsurfProbeHTTPUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return s.lookup(req)
}

func (s *windsurfProbeHTTPUpstreamStub) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.lookup(req)
}

func (s *windsurfProbeHTTPUpstreamStub) lookup(req *http.Request) (*http.Response, error) {
	if s.responses == nil {
		return nil, nil
	}
	if resp, ok := s.responses[req.URL.String()]; ok {
		return resp, nil
	}
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader(`{"error":"not found"}`)),
	}, nil
}
