package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dashboardPlatformRepoCapture struct {
	service.UsageLogRepository
	trendPlatform string
	modelPlatform string
	groupPlatform string
}

func (r *dashboardPlatformRepoCapture) GetUsageTrendWithFilters(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	userID, apiKeyID, accountID, groupID int64,
	model, platform string,
	requestType *int16,
	stream *bool,
	billingType *int8,
) ([]usagestats.TrendDataPoint, error) {
	r.trendPlatform = platform
	return []usagestats.TrendDataPoint{}, nil
}

func (r *dashboardPlatformRepoCapture) GetModelStatsWithFilters(
	ctx context.Context,
	startTime, endTime time.Time,
	userID, apiKeyID, accountID, groupID int64,
	platform string,
	requestType *int16,
	stream *bool,
	billingType *int8,
) ([]usagestats.ModelStat, error) {
	r.modelPlatform = platform
	return []usagestats.ModelStat{}, nil
}

func (r *dashboardPlatformRepoCapture) GetGroupStatsWithFilters(
	ctx context.Context,
	startTime, endTime time.Time,
	userID, apiKeyID, accountID, groupID int64,
	platform string,
	requestType *int16,
	stream *bool,
	billingType *int8,
) ([]usagestats.GroupStat, error) {
	r.groupPlatform = platform
	return []usagestats.GroupStat{}, nil
}

func newDashboardPlatformRouter(repo *dashboardPlatformRepoCapture) *gin.Engine {
	gin.SetMode(gin.TestMode)
	resetDashboardReadCachesForTest()
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	handler := NewDashboardHandler(dashboardSvc, nil)
	router := gin.New()
	router.GET("/admin/dashboard/trend", handler.GetUsageTrend)
	router.GET("/admin/dashboard/models", handler.GetModelStats)
	router.GET("/admin/dashboard/groups", handler.GetGroupStats)
	router.GET("/admin/dashboard/snapshot-v2", handler.GetSnapshotV2)
	return router
}

func TestPlatformDashboardUserBreakdownFilter(t *testing.T) {
	repo := &userBreakdownRepoCapture{}
	router := newUserBreakdownRouter(repo)

	req := httptest.NewRequest(http.MethodGet,
		"/admin/dashboard/user-breakdown?start_date=2026-03-01&end_date=2026-03-16&platform=windsurf", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "windsurf", repo.capturedDim.Platform)
}

func TestPlatformDashboardUserBreakdownInvalidPlatform(t *testing.T) {
	repo := &userBreakdownRepoCapture{}
	router := newUserBreakdownRouter(repo)

	req := httptest.NewRequest(http.MethodGet,
		"/admin/dashboard/user-breakdown?start_date=2026-03-01&end_date=2026-03-16&platform=unknown", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPlatformDashboardFiltersPropagateToTrendModelsGroupsAndSnapshot(t *testing.T) {
	repo := &dashboardPlatformRepoCapture{}
	router := newDashboardPlatformRouter(repo)

	testCases := []struct {
		name     string
		path     string
		assertFn func(t *testing.T)
	}{
		{
			name: "trend",
			path: "/admin/dashboard/trend?start_date=2026-03-01&end_date=2026-03-16&platform=windsurf",
			assertFn: func(t *testing.T) {
				require.Equal(t, "windsurf", repo.trendPlatform)
			},
		},
		{
			name: "models",
			path: "/admin/dashboard/models?start_date=2026-03-01&end_date=2026-03-16&platform=windsurf",
			assertFn: func(t *testing.T) {
				require.Equal(t, "windsurf", repo.modelPlatform)
			},
		},
		{
			name: "groups",
			path: "/admin/dashboard/groups?start_date=2026-03-01&end_date=2026-03-16&platform=windsurf",
			assertFn: func(t *testing.T) {
				require.Equal(t, "windsurf", repo.groupPlatform)
			},
		},
		{
			name: "snapshot-v2",
			path: "/admin/dashboard/snapshot-v2?start_date=2026-03-01&end_date=2026-03-16&platform=windsurf&include_stats=false&include_trend=true&include_model_stats=true&include_group_stats=true",
			assertFn: func(t *testing.T) {
				require.Equal(t, "windsurf", repo.trendPlatform)
				require.Equal(t, "windsurf", repo.modelPlatform)
				require.Equal(t, "windsurf", repo.groupPlatform)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			tc.assertFn(t)
		})
	}
}

func TestPlatformDashboardSnapshotInvalidPlatform(t *testing.T) {
	repo := &dashboardPlatformRepoCapture{}
	router := newDashboardPlatformRouter(repo)

	req := httptest.NewRequest(http.MethodGet,
		"/admin/dashboard/snapshot-v2?start_date=2026-03-01&end_date=2026-03-16&platform=invalid", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
