package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type windsurfRecordUsageChannelRepoStub struct {
	ChannelRepository
	channels       []Channel
	groupPlatforms map[int64]string
}

func (s *windsurfRecordUsageChannelRepoStub) ListAll(context.Context) ([]Channel, error) {
	return append([]Channel(nil), s.channels...), nil
}

func (s *windsurfRecordUsageChannelRepoStub) GetGroupPlatforms(context.Context, []int64) (map[int64]string, error) {
	result := make(map[int64]string, len(s.groupPlatforms))
	for key, value := range s.groupPlatforms {
		result[key] = value
	}
	return result, nil
}

func newWindsurfRecordUsageServiceForTest(
	usageRepo UsageLogRepository,
	userRepo UserRepository,
	subRepo UserSubscriptionRepository,
	rateRepo UserGroupRateRepository,
	channelRepo ChannelRepository,
) *WindsurfGatewayService {
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1.0

	svc := NewWindsurfGatewayService(
		nil,
		nil,
		NewWindsurfModelCatalogService(nil),
		nil,
		nil,
		nil,
	)
	svc.ConfigureBilling(
		usageRepo,
		nil,
		userRepo,
		subRepo,
		rateRepo,
		cfg,
		NewBillingService(cfg, nil),
		&BillingCacheService{},
		&DeferredService{},
		NewModelPricingResolver(NewChannelService(channelRepo, nil), NewBillingService(cfg, nil)),
		NewChannelService(channelRepo, nil),
		nil,
	)
	return svc
}

func TestWindsurfGatewayServiceRecordUsage_UsesLocalPerRequestPricing(t *testing.T) {
	groupID := int64(11)
	perRequestPrice := 0.75

	channelRepo := &windsurfRecordUsageChannelRepoStub{
		channels: []Channel{
			{
				ID:       91,
				Status:   StatusActive,
				GroupIDs: []int64{groupID},
				ModelPricing: []ChannelModelPricing{
					{
						Platform:        PlatformWindsurf,
						Models:          []string{"gpt-4.1"},
						BillingMode:     BillingModePerRequest,
						PerRequestPrice: &perRequestPrice,
					},
				},
			},
		},
		groupPlatforms: map[int64]string{
			groupID: PlatformWindsurf,
		},
	}
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newWindsurfRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil, channelRepo)

	err := svc.RecordUsage(context.Background(), &WindsurfRecordUsageInput{
		Result: &WindsurfRecordUsageResult{
			RequestID: "windsurf_req_local_pricing",
			Model:     "gpt-4.1",
			Duration:  time.Second,
		},
		APIKey: &APIKey{
			ID:      2001,
			UserID:  3001,
			GroupID: i64p(groupID),
			Group: &Group{
				ID:             groupID,
				Platform:       PlatformWindsurf,
				RateMultiplier: 2.0,
			},
		},
		User: &User{ID: 3001},
		Account: &Account{
			ID:       4001,
			Platform: PlatformWindsurf,
		},
		InboundEndpoint:  "/v1/chat/completions",
		UpstreamEndpoint: "/windsurf/cascade",
	})
	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, int64(4001), usageRepo.lastLog.AccountID)
	require.Equal(t, "gpt-4.1", usageRepo.lastLog.Model)
	require.Equal(t, "gpt-4.1", usageRepo.lastLog.RequestedModel)
	require.NotNil(t, usageRepo.lastLog.BillingMode)
	require.Equal(t, string(BillingModePerRequest), *usageRepo.lastLog.BillingMode)
	require.InDelta(t, perRequestPrice, usageRepo.lastLog.TotalCost, 1e-12)
	require.InDelta(t, perRequestPrice*2.0, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, perRequestPrice*2.0, userRepo.lastAmount, 1e-12)
	require.Equal(t, 1, userRepo.deductCalls)
}

func TestWindsurfGatewayServiceRecordUsage_PreservesRequestedAndUpstreamModelMetadata(t *testing.T) {
	groupID := int64(12)
	perRequestPrice := 0.33

	channelRepo := &windsurfRecordUsageChannelRepoStub{
		channels: []Channel{
			{
				ID:       92,
				Status:   StatusActive,
				GroupIDs: []int64{groupID},
				ModelPricing: []ChannelModelPricing{
					{
						Platform:        PlatformWindsurf,
						Models:          []string{"gpt-4o"},
						BillingMode:     BillingModePerRequest,
						PerRequestPrice: &perRequestPrice,
					},
				},
			},
		},
		groupPlatforms: map[int64]string{
			groupID: PlatformWindsurf,
		},
	}
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newWindsurfRecordUsageServiceForTest(usageRepo, userRepo, subRepo, nil, channelRepo)

	err := svc.RecordUsage(context.Background(), &WindsurfRecordUsageInput{
		Result: &WindsurfRecordUsageResult{
			RequestID:     "windsurf_req_mapping_metadata",
			Model:         "gpt-4o",
			UpstreamModel: "gpt-4.1",
			Duration:      time.Second,
		},
		APIKey: &APIKey{
			ID:      2002,
			UserID:  3002,
			GroupID: i64p(groupID),
			Group: &Group{
				ID:             groupID,
				Platform:       PlatformWindsurf,
				RateMultiplier: 1.0,
			},
		},
		User: &User{ID: 3002},
		Account: &Account{
			ID:       4002,
			Platform: PlatformWindsurf,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, "gpt-4o", usageRepo.lastLog.Model)
	require.Equal(t, "gpt-4o", usageRepo.lastLog.RequestedModel)
	if assertUpstream := usageRepo.lastLog.UpstreamModel; assertUpstream == nil || *assertUpstream != "gpt-4.1" {
		t.Fatalf("UpstreamModel = %v, want gpt-4.1", usageRepo.lastLog.UpstreamModel)
	}
	require.InDelta(t, perRequestPrice, usageRepo.lastLog.TotalCost, 1e-12)
	require.InDelta(t, perRequestPrice, userRepo.lastAmount, 1e-12)
}
