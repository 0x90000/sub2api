package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

func TestWindsurfModelCatalogAggregatesSchedulableAccounts(t *testing.T) {
	groupID := int64(7)
	repo := &windsurfModelCatalogRepoStub{
		accounts: []Account{
			{
				ID:       1,
				Platform: PlatformWindsurf,
				Type:     AccountTypeAPIKey,
				Status:   StatusActive,
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"gpt-4o":        "gpt-4.1",
						"claude-custom": "claude-4-sonnet",
						"gpt-5":         "gpt-5.2",
					},
				},
				Extra: map[string]any{
					"allowed_models": []string{"gpt-4.1", "claude-4-sonnet"},
				},
			},
			{
				ID:       2,
				Platform: PlatformWindsurf,
				Type:     AccountTypeAPIKey,
				Status:   StatusActive,
				Extra: map[string]any{
					"allowed_models": []string{"gemini-2.5-flash"},
				},
			},
		},
	}

	service := NewWindsurfModelCatalogService(repo)
	models, err := service.ListModels(context.Background(), &groupID)
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}

	want := []openai.Model{
		{ID: "claude-custom", Object: "model", Type: "model", OwnedBy: "anthropic", DisplayName: "claude-custom"},
		{ID: "gemini-2.5-flash", Object: "model", Type: "model", OwnedBy: "google", DisplayName: "gemini-2.5-flash"},
		{ID: "gpt-4o", Object: "model", Type: "model", OwnedBy: "openai", DisplayName: "gpt-4o"},
	}
	if len(models) != len(want) {
		t.Fatalf("models len = %d, want %d (%v)", len(models), len(want), models)
	}
	for i := range want {
		if models[i].ID != want[i].ID || models[i].OwnedBy != want[i].OwnedBy {
			t.Fatalf("models[%d] = %+v, want %+v", i, models[i], want[i])
		}
	}

	if !repo.listByGroupAndPlatformCalled {
		t.Fatal("expected ListSchedulableByGroupIDAndPlatform to be used")
	}
}

type windsurfModelCatalogRepoStub struct {
	accounts                     []Account
	listByGroupAndPlatformCalled bool
}

func (s *windsurfModelCatalogRepoStub) Create(context.Context, *Account) error {
	panic("unexpected Create")
}

func (s *windsurfModelCatalogRepoStub) GetByID(context.Context, int64) (*Account, error) {
	panic("unexpected GetByID")
}

func (s *windsurfModelCatalogRepoStub) GetByIDs(context.Context, []int64) ([]*Account, error) {
	panic("unexpected GetByIDs")
}

func (s *windsurfModelCatalogRepoStub) ExistsByID(context.Context, int64) (bool, error) {
	panic("unexpected ExistsByID")
}

func (s *windsurfModelCatalogRepoStub) GetByCRSAccountID(context.Context, string) (*Account, error) {
	panic("unexpected GetByCRSAccountID")
}

func (s *windsurfModelCatalogRepoStub) FindByExtraField(context.Context, string, any) ([]Account, error) {
	panic("unexpected FindByExtraField")
}

func (s *windsurfModelCatalogRepoStub) ListCRSAccountIDs(context.Context) (map[string]int64, error) {
	panic("unexpected ListCRSAccountIDs")
}

func (s *windsurfModelCatalogRepoStub) Update(context.Context, *Account) error {
	panic("unexpected Update")
}

func (s *windsurfModelCatalogRepoStub) Delete(context.Context, int64) error {
	panic("unexpected Delete")
}

func (s *windsurfModelCatalogRepoStub) List(context.Context, pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	panic("unexpected List")
}

func (s *windsurfModelCatalogRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, string, int64, string) ([]Account, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters")
}

func (s *windsurfModelCatalogRepoStub) ListByGroup(context.Context, int64) ([]Account, error) {
	panic("unexpected ListByGroup")
}

func (s *windsurfModelCatalogRepoStub) ListActive(context.Context) ([]Account, error) {
	panic("unexpected ListActive")
}

func (s *windsurfModelCatalogRepoStub) ListByPlatform(context.Context, string) ([]Account, error) {
	panic("unexpected ListByPlatform")
}

func (s *windsurfModelCatalogRepoStub) UpdateLastUsed(context.Context, int64) error {
	panic("unexpected UpdateLastUsed")
}

func (s *windsurfModelCatalogRepoStub) BatchUpdateLastUsed(context.Context, map[int64]time.Time) error {
	panic("unexpected BatchUpdateLastUsed")
}

func (s *windsurfModelCatalogRepoStub) SetError(context.Context, int64, string) error {
	panic("unexpected SetError")
}

func (s *windsurfModelCatalogRepoStub) ClearError(context.Context, int64) error {
	panic("unexpected ClearError")
}

func (s *windsurfModelCatalogRepoStub) SetSchedulable(context.Context, int64, bool) error {
	panic("unexpected SetSchedulable")
}

func (s *windsurfModelCatalogRepoStub) AutoPauseExpiredAccounts(context.Context, time.Time) (int64, error) {
	panic("unexpected AutoPauseExpiredAccounts")
}

func (s *windsurfModelCatalogRepoStub) BindGroups(context.Context, int64, []int64) error {
	panic("unexpected BindGroups")
}

func (s *windsurfModelCatalogRepoStub) ListSchedulable(context.Context) ([]Account, error) {
	panic("unexpected ListSchedulable")
}

func (s *windsurfModelCatalogRepoStub) ListSchedulableByGroupID(context.Context, int64) ([]Account, error) {
	panic("unexpected ListSchedulableByGroupID")
}

func (s *windsurfModelCatalogRepoStub) ListSchedulableByPlatform(context.Context, string) ([]Account, error) {
	panic("unexpected ListSchedulableByPlatform")
}

func (s *windsurfModelCatalogRepoStub) ListSchedulableByGroupIDAndPlatform(_ context.Context, _ int64, platform string) ([]Account, error) {
	s.listByGroupAndPlatformCalled = true
	if platform != PlatformWindsurf {
		panic("unexpected platform")
	}
	return s.accounts, nil
}

func (s *windsurfModelCatalogRepoStub) ListSchedulableByPlatforms(context.Context, []string) ([]Account, error) {
	panic("unexpected ListSchedulableByPlatforms")
}

func (s *windsurfModelCatalogRepoStub) ListSchedulableByGroupIDAndPlatforms(context.Context, int64, []string) ([]Account, error) {
	panic("unexpected ListSchedulableByGroupIDAndPlatforms")
}

func (s *windsurfModelCatalogRepoStub) ListSchedulableUngroupedByPlatform(context.Context, string) ([]Account, error) {
	panic("unexpected ListSchedulableUngroupedByPlatform")
}

func (s *windsurfModelCatalogRepoStub) ListSchedulableUngroupedByPlatforms(context.Context, []string) ([]Account, error) {
	panic("unexpected ListSchedulableUngroupedByPlatforms")
}

func (s *windsurfModelCatalogRepoStub) SetRateLimited(context.Context, int64, time.Time) error {
	panic("unexpected SetRateLimited")
}

func (s *windsurfModelCatalogRepoStub) SetModelRateLimit(context.Context, int64, string, time.Time) error {
	panic("unexpected SetModelRateLimit")
}

func (s *windsurfModelCatalogRepoStub) SetOverloaded(context.Context, int64, time.Time) error {
	panic("unexpected SetOverloaded")
}

func (s *windsurfModelCatalogRepoStub) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	panic("unexpected SetTempUnschedulable")
}

func (s *windsurfModelCatalogRepoStub) ClearTempUnschedulable(context.Context, int64) error {
	panic("unexpected ClearTempUnschedulable")
}

func (s *windsurfModelCatalogRepoStub) ClearRateLimit(context.Context, int64) error {
	panic("unexpected ClearRateLimit")
}

func (s *windsurfModelCatalogRepoStub) ClearAntigravityQuotaScopes(context.Context, int64) error {
	panic("unexpected ClearAntigravityQuotaScopes")
}

func (s *windsurfModelCatalogRepoStub) ClearModelRateLimits(context.Context, int64) error {
	panic("unexpected ClearModelRateLimits")
}

func (s *windsurfModelCatalogRepoStub) UpdateSessionWindow(context.Context, int64, *time.Time, *time.Time, string) error {
	panic("unexpected UpdateSessionWindow")
}

func (s *windsurfModelCatalogRepoStub) UpdateExtra(context.Context, int64, map[string]any) error {
	panic("unexpected UpdateExtra")
}

func (s *windsurfModelCatalogRepoStub) BulkUpdate(context.Context, []int64, AccountBulkUpdate) (int64, error) {
	panic("unexpected BulkUpdate")
}

func (s *windsurfModelCatalogRepoStub) IncrementQuotaUsed(context.Context, int64, float64) error {
	panic("unexpected IncrementQuotaUsed")
}

func (s *windsurfModelCatalogRepoStub) ResetQuotaUsed(context.Context, int64) error {
	panic("unexpected ResetQuotaUsed")
}
