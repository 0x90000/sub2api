package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogRepoListWithFiltersPlatform(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	filters := usagestats.UsageLogFilters{
		Platform:   service.PlatformWindsurf,
		ExactTotal: true,
	}

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM usage_logs WHERE EXISTS \\(SELECT 1 FROM groups g WHERE g.id = usage_logs\\.group_id AND g.platform = \\$1\\)").
		WithArgs(service.PlatformWindsurf).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery("SELECT .* FROM usage_logs WHERE EXISTS \\(SELECT 1 FROM groups g WHERE g.id = usage_logs\\.group_id AND g.platform = \\$1\\) ORDER BY id DESC LIMIT \\$2 OFFSET \\$3").
		WithArgs(service.PlatformWindsurf, 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	logs, page, err := repo.ListWithFilters(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20}, filters)
	require.NoError(t, err)
	require.Empty(t, logs)
	require.NotNil(t, page)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepoGetStatsWithFiltersPlatform(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	filters := usagestats.UsageLogFilters{
		Platform: service.PlatformOpenAI,
	}

	mock.ExpectQuery("FROM usage_logs\\s+WHERE EXISTS \\(SELECT 1 FROM groups g WHERE g.id = usage_logs\\.group_id AND g.platform = \\$1\\)").
		WithArgs(service.PlatformOpenAI).
		WillReturnRows(sqlmock.NewRows([]string{
			"total_requests",
			"total_input_tokens",
			"total_output_tokens",
			"total_cache_tokens",
			"total_cost",
			"total_actual_cost",
			"total_account_cost",
			"avg_duration_ms",
		}).AddRow(int64(1), int64(2), int64(3), int64(4), 1.2, 1.0, 1.2, 20.0))
	mock.ExpectQuery("SELECT COALESCE\\(NULLIF\\(TRIM\\(inbound_endpoint\\), ''\\), 'unknown'\\) AS endpoint").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), service.PlatformOpenAI).
		WillReturnRows(sqlmock.NewRows([]string{"endpoint", "requests", "total_tokens", "cost", "actual_cost"}))
	mock.ExpectQuery("SELECT COALESCE\\(NULLIF\\(TRIM\\(upstream_endpoint\\), ''\\), 'unknown'\\) AS endpoint").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), service.PlatformOpenAI).
		WillReturnRows(sqlmock.NewRows([]string{"endpoint", "requests", "total_tokens", "cost", "actual_cost"}))
	mock.ExpectQuery("SELECT CONCAT\\(").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), service.PlatformOpenAI).
		WillReturnRows(sqlmock.NewRows([]string{"endpoint", "requests", "total_tokens", "cost", "actual_cost"}))

	stats, err := repo.GetStatsWithFilters(context.Background(), filters)
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.TotalRequests)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepoGetUserBreakdownStatsPlatform(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery("FROM usage_logs ul\\s+LEFT JOIN users u ON u.id = ul.user_id\\s+WHERE ul.created_at >= \\$1 AND ul.created_at < \\$2 AND EXISTS \\(SELECT 1 FROM groups g WHERE g.id = ul.group_id AND g.platform = \\$3\\)").
		WithArgs(start, end, service.PlatformWindsurf).
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id",
			"email",
			"requests",
			"total_tokens",
			"cost",
			"actual_cost",
			"account_cost",
		}))

	stats, err := repo.GetUserBreakdownStats(context.Background(), start, end, usagestats.UserBreakdownDimension{
		Platform: service.PlatformWindsurf,
	}, 50)
	require.NoError(t, err)
	require.Empty(t, stats)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepoGetUsageTrendWithFiltersPlatform(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery("FROM usage_logs\\s+WHERE created_at >= \\$1 AND created_at < \\$2 AND EXISTS \\(SELECT 1 FROM groups g WHERE g.id = group_id AND g.platform = \\$3\\) GROUP BY date ORDER BY date ASC").
		WithArgs(start, end, service.PlatformWindsurf).
		WillReturnRows(sqlmock.NewRows([]string{
			"date",
			"requests",
			"input_tokens",
			"output_tokens",
			"cache_creation_tokens",
			"cache_read_tokens",
			"total_tokens",
			"cost",
			"actual_cost",
		}))

	stats, err := repo.GetUsageTrendWithFilters(context.Background(), start, end, "day", 0, 0, 0, 0, "", service.PlatformWindsurf, nil, nil, nil)
	require.NoError(t, err)
	require.Empty(t, stats)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepoGetModelStatsWithFiltersPlatform(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery("FROM usage_logs\\s+WHERE created_at >= \\$1 AND created_at < \\$2 AND EXISTS \\(SELECT 1 FROM groups g WHERE g.id = group_id AND g.platform = \\$3\\) GROUP BY").
		WithArgs(start, end, service.PlatformOpenAI).
		WillReturnRows(sqlmock.NewRows([]string{
			"model",
			"requests",
			"input_tokens",
			"output_tokens",
			"cache_creation_tokens",
			"cache_read_tokens",
			"total_tokens",
			"cost",
			"actual_cost",
			"account_cost",
		}))

	stats, err := repo.GetModelStatsWithFilters(context.Background(), start, end, 0, 0, 0, 0, service.PlatformOpenAI, nil, nil, nil)
	require.NoError(t, err)
	require.Empty(t, stats)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepoGetGroupStatsWithFiltersPlatform(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery("FROM usage_logs ul\\s+LEFT JOIN groups g ON g.id = ul.group_id\\s+WHERE ul.created_at >= \\$1 AND ul.created_at < \\$2 AND EXISTS \\(SELECT 1 FROM groups g WHERE g.id = ul.group_id AND g.platform = \\$3\\) GROUP BY ul.group_id, g.name ORDER BY total_tokens DESC").
		WithArgs(start, end, service.PlatformWindsurf).
		WillReturnRows(sqlmock.NewRows([]string{
			"group_id",
			"group_name",
			"requests",
			"total_tokens",
			"cost",
			"actual_cost",
			"account_cost",
		}))

	stats, err := repo.GetGroupStatsWithFilters(context.Background(), start, end, 0, 0, 0, 0, service.PlatformWindsurf, nil, nil, nil)
	require.NoError(t, err)
	require.Empty(t, stats)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepoGetEndpointStatsWithFiltersPlatform(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT COALESCE\\(NULLIF\\(TRIM\\(inbound_endpoint\\), ''\\), 'unknown'\\) AS endpoint").
		WithArgs(start, end, service.PlatformWindsurf).
		WillReturnRows(sqlmock.NewRows([]string{"endpoint", "requests", "total_tokens", "cost", "actual_cost"}))

	stats, err := repo.GetEndpointStatsWithFilters(context.Background(), start, end, 0, 0, 0, 0, "", service.PlatformWindsurf, nil, nil, nil)
	require.NoError(t, err)
	require.Empty(t, stats)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepoGetUpstreamEndpointStatsWithFiltersPlatform(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT COALESCE\\(NULLIF\\(TRIM\\(upstream_endpoint\\), ''\\), 'unknown'\\) AS endpoint").
		WithArgs(start, end, service.PlatformWindsurf).
		WillReturnRows(sqlmock.NewRows([]string{"endpoint", "requests", "total_tokens", "cost", "actual_cost"}))

	stats, err := repo.GetUpstreamEndpointStatsWithFilters(context.Background(), start, end, 0, 0, 0, 0, "", service.PlatformWindsurf, nil, nil, nil)
	require.NoError(t, err)
	require.Empty(t, stats)
	require.NoError(t, mock.ExpectationsWereMet())
}
