package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
)

var (
	ErrWindsurfNoSchedulableAccounts = errors.New("no schedulable windsurf accounts")
	ErrWindsurfModelNotSupported     = errors.New("windsurf model is not supported")
	ErrWindsurfChatBridgeUnavailable = errors.New("windsurf chat bridge is not configured")
)

type windsurfGatewayAccountRepository interface {
	ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error)
	ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error)
}

type WindsurfAccountSelection struct {
	Account *Account
	Model   WindsurfResolvedModel
}

type WindsurfBridgeResult struct {
	RequestID string
	Text      string
	Reasoning string
	Usage     WindsurfBridgeUsage
}

type WindsurfBridgeStreamChunk struct {
	Text      string
	Reasoning string
}

type WindsurfBridgeUsage struct {
	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int
	ImageOutputTokens   int
}

type windsurfChatBridge interface {
	Complete(ctx context.Context, account *Account, model WindsurfResolvedModel, req *apicompat.ChatCompletionsRequest) (*WindsurfBridgeResult, error)
	Stream(ctx context.Context, account *Account, model WindsurfResolvedModel, req *apicompat.ChatCompletionsRequest, emit func(WindsurfBridgeStreamChunk) error) (*WindsurfBridgeResult, error)
}

// WindsurfGatewayService owns the provider-facing Windsurf request flow.
// Phase 2 now includes dynamic model resolution and account selection so the
// later langserver bridge can consume a normalized gateway contract.
type WindsurfGatewayService struct {
	accountRepo     windsurfGatewayAccountRepository
	accountRepoFull AccountRepository
	accountProbe    *WindsurfAccountProbeService
	modelCatalog    *WindsurfModelCatalogService
	usageFetcher    *WindsurfUsageFetcher
	errorMapper     *WindsurfErrorMapper
	chatBridge      windsurfChatBridge

	usageLogRepo          UsageLogRepository
	usageBillingRepo      UsageBillingRepository
	userRepo              UserRepository
	userSubRepo           UserSubscriptionRepository
	cfg                   *config.Config
	billingService        *BillingService
	billingCacheService   *BillingCacheService
	deferredService       *DeferredService
	userGroupRateResolver *userGroupRateResolver
	resolver              *ModelPricingResolver
	channelService        *ChannelService
	balanceNotifyService  *BalanceNotifyService
}

func NewWindsurfGatewayService(
	accountRepo AccountRepository,
	accountProbe *WindsurfAccountProbeService,
	modelCatalog *WindsurfModelCatalogService,
	usageFetcher *WindsurfUsageFetcher,
	errorMapper *WindsurfErrorMapper,
	chatBridge windsurfChatBridge,
) *WindsurfGatewayService {
	return &WindsurfGatewayService{
		accountRepo:     accountRepo,
		accountRepoFull: accountRepo,
		accountProbe:    accountProbe,
		modelCatalog:    modelCatalog,
		usageFetcher:    usageFetcher,
		errorMapper:     errorMapper,
		chatBridge:      chatBridge,
	}
}

func (s *WindsurfGatewayService) ConfigureBilling(
	usageLogRepo UsageLogRepository,
	usageBillingRepo UsageBillingRepository,
	userRepo UserRepository,
	userSubRepo UserSubscriptionRepository,
	userGroupRateRepo UserGroupRateRepository,
	cfg *config.Config,
	billingService *BillingService,
	billingCacheService *BillingCacheService,
	deferredService *DeferredService,
	resolver *ModelPricingResolver,
	channelService *ChannelService,
	balanceNotifyService *BalanceNotifyService,
) {
	if s == nil {
		return
	}
	if billingCacheService == nil {
		billingCacheService = &BillingCacheService{}
	}
	if deferredService == nil {
		deferredService = &DeferredService{}
	}
	s.usageLogRepo = usageLogRepo
	s.usageBillingRepo = usageBillingRepo
	s.userRepo = userRepo
	s.userSubRepo = userSubRepo
	s.cfg = cfg
	s.billingService = billingService
	s.billingCacheService = billingCacheService
	s.deferredService = deferredService
	s.userGroupRateResolver = newUserGroupRateResolver(
		userGroupRateRepo,
		nil,
		resolveUserGroupRateCacheTTL(cfg),
		nil,
		"service.windsurf_gateway",
	)
	s.resolver = resolver
	s.channelService = channelService
	s.balanceNotifyService = balanceNotifyService
}

func (s *WindsurfGatewayService) AccountProbe() *WindsurfAccountProbeService {
	if s == nil {
		return nil
	}
	return s.accountProbe
}

func (s *WindsurfGatewayService) ModelCatalog() *WindsurfModelCatalogService {
	if s == nil {
		return nil
	}
	return s.modelCatalog
}

func (s *WindsurfGatewayService) UsageFetcher() *WindsurfUsageFetcher {
	if s == nil {
		return nil
	}
	return s.usageFetcher
}

func (s *WindsurfGatewayService) ErrorMapper() *WindsurfErrorMapper {
	if s == nil {
		return nil
	}
	return s.errorMapper
}

func (s *WindsurfGatewayService) billingDeps() *billingDeps {
	billingCacheService := s.billingCacheService
	if billingCacheService == nil {
		billingCacheService = &BillingCacheService{}
	}
	deferredService := s.deferredService
	if deferredService == nil {
		deferredService = &DeferredService{}
	}
	return &billingDeps{
		accountRepo:          s.accountRepoFull,
		userRepo:             s.userRepo,
		userSubRepo:          s.userSubRepo,
		billingCacheService:  billingCacheService,
		deferredService:      deferredService,
		balanceNotifyService: s.balanceNotifyService,
	}
}

func (s *WindsurfGatewayService) getUserGroupRateMultiplier(ctx context.Context, userID, groupID int64, groupDefaultMultiplier float64) float64 {
	if s == nil {
		return groupDefaultMultiplier
	}
	resolver := s.userGroupRateResolver
	if resolver == nil {
		resolver = newUserGroupRateResolver(nil, nil, resolveUserGroupRateCacheTTL(s.cfg), nil, "service.windsurf_gateway")
	}
	return resolver.Resolve(ctx, userID, groupID, groupDefaultMultiplier)
}

type WindsurfRecordUsageInput struct {
	Result             *WindsurfRecordUsageResult
	APIKey             *APIKey
	User               *User
	Account            *Account
	Subscription       *UserSubscription
	InboundEndpoint    string
	UpstreamEndpoint   string
	UserAgent          string
	IPAddress          string
	RequestPayloadHash string
	APIKeyService      APIKeyQuotaUpdater
}

type WindsurfRecordUsageResult struct {
	RequestID     string
	Model         string
	UpstreamModel string
	Usage         WindsurfBridgeUsage
	Duration      time.Duration
	Stream        bool
}

type WindsurfExecutionMetadata struct {
	RequestID      string
	RequestedModel string
	UpstreamModel  string
	Usage          WindsurfBridgeUsage
	Duration       time.Duration
	Stream         bool
	Account        *Account
}

func (s *WindsurfGatewayService) CompleteChatCompletions(
	ctx context.Context,
	groupID *int64,
	req *apicompat.ChatCompletionsRequest,
) (*apicompat.ChatCompletionsResponse, error) {
	resp, _, err := s.CompleteChatCompletionsWithMetadata(ctx, groupID, req)
	return resp, err
}

func (s *WindsurfGatewayService) CompleteChatCompletionsWithMetadata(
	ctx context.Context,
	groupID *int64,
	req *apicompat.ChatCompletionsRequest,
) (*apicompat.ChatCompletionsResponse, *WindsurfExecutionMetadata, error) {
	if req == nil {
		return nil, nil, ErrWindsurfModelNotSupported
	}
	selection, err := s.SelectChatCompletionAccount(ctx, groupID, req.Model)
	if err != nil {
		return nil, nil, err
	}
	return s.completeChatCompletionsWithSelection(ctx, req.Model, req, selection)
}

func (s *WindsurfGatewayService) completeChatCompletionsWithSelection(
	ctx context.Context,
	requestedModel string,
	req *apicompat.ChatCompletionsRequest,
	selection *WindsurfAccountSelection,
) (*apicompat.ChatCompletionsResponse, *WindsurfExecutionMetadata, error) {
	if s == nil || s.chatBridge == nil {
		return nil, nil, ErrWindsurfChatBridgeUnavailable
	}
	if req == nil || strings.TrimSpace(requestedModel) == "" {
		return nil, nil, ErrWindsurfModelNotSupported
	}
	if selection == nil || selection.Account == nil {
		return nil, nil, ErrWindsurfNoSchedulableAccounts
	}

	startedAt := time.Now()
	result, err := s.chatBridge.Complete(ctx, selection.Account, selection.Model, req)
	if err != nil {
		return nil, nil, err
	}

	metadata := &WindsurfExecutionMetadata{
		RequestedModel: requestedModel,
		UpstreamModel:  selection.Model.UpstreamModel,
		Duration:       time.Since(startedAt),
		Stream:         false,
		Account:        selection.Account,
	}
	if result != nil {
		metadata.RequestID = strings.TrimSpace(result.RequestID)
		metadata.Usage = result.Usage
	}
	return buildWindsurfChatCompletionResponse(requestedModel, result), metadata, nil
}

func (s *WindsurfGatewayService) StreamChatCompletions(
	ctx context.Context,
	groupID *int64,
	req *apicompat.ChatCompletionsRequest,
	emit func(apicompat.ChatCompletionsChunk) error,
) error {
	_, err := s.StreamChatCompletionsWithMetadata(ctx, groupID, req, emit)
	return err
}

func (s *WindsurfGatewayService) StreamChatCompletionsWithMetadata(
	ctx context.Context,
	groupID *int64,
	req *apicompat.ChatCompletionsRequest,
	emit func(apicompat.ChatCompletionsChunk) error,
) (*WindsurfExecutionMetadata, error) {
	if req == nil {
		return nil, ErrWindsurfModelNotSupported
	}
	selection, err := s.SelectChatCompletionAccount(ctx, groupID, req.Model)
	if err != nil {
		return nil, err
	}
	return s.streamChatCompletionsWithSelection(ctx, req.Model, req, selection, emit)
}

func (s *WindsurfGatewayService) streamChatCompletionsWithSelection(
	ctx context.Context,
	requestedModel string,
	req *apicompat.ChatCompletionsRequest,
	selection *WindsurfAccountSelection,
	emit func(apicompat.ChatCompletionsChunk) error,
) (*WindsurfExecutionMetadata, error) {
	if s == nil || s.chatBridge == nil {
		return nil, ErrWindsurfChatBridgeUnavailable
	}
	if req == nil || strings.TrimSpace(requestedModel) == "" {
		return nil, ErrWindsurfModelNotSupported
	}
	if selection == nil || selection.Account == nil {
		return nil, ErrWindsurfNoSchedulableAccounts
	}
	if emit == nil {
		return nil, errors.New("windsurf stream emit callback is required")
	}

	startedAt := time.Now()
	streamID := newWindsurfChatCompletionID()
	createdAt := time.Now().Unix()
	sentRole := false

	sendRole := func() error {
		if sentRole {
			return nil
		}
		sentRole = true
		return emit(apicompat.ChatCompletionsChunk{
			ID:      streamID,
			Object:  "chat.completion.chunk",
			Created: createdAt,
			Model:   requestedModel,
			Choices: []apicompat.ChatChunkChoice{{
				Index: 0,
				Delta: apicompat.ChatDelta{
					Role: "assistant",
				},
				FinishReason: nil,
			}},
		})
	}

	sendText := func(text string) error {
		if strings.TrimSpace(text) == "" {
			return nil
		}
		if err := sendRole(); err != nil {
			return err
		}
		return emit(apicompat.ChatCompletionsChunk{
			ID:      streamID,
			Object:  "chat.completion.chunk",
			Created: createdAt,
			Model:   requestedModel,
			Choices: []apicompat.ChatChunkChoice{{
				Index: 0,
				Delta: apicompat.ChatDelta{
					Content: &text,
				},
				FinishReason: nil,
			}},
		})
	}

	sendReasoning := func(text string) error {
		if strings.TrimSpace(text) == "" {
			return nil
		}
		if err := sendRole(); err != nil {
			return err
		}
		return emit(apicompat.ChatCompletionsChunk{
			ID:      streamID,
			Object:  "chat.completion.chunk",
			Created: createdAt,
			Model:   requestedModel,
			Choices: []apicompat.ChatChunkChoice{{
				Index: 0,
				Delta: apicompat.ChatDelta{
					ReasoningContent: &text,
				},
				FinishReason: nil,
			}},
		})
	}

	finalResult, err := s.chatBridge.Stream(ctx, selection.Account, selection.Model, req, func(chunk WindsurfBridgeStreamChunk) error {
		if err := sendReasoning(chunk.Reasoning); err != nil {
			return err
		}
		return sendText(chunk.Text)
	})
	if err != nil {
		return nil, err
	}

	if finalResult != nil {
		if err := sendReasoning(finalResult.Reasoning); err != nil {
			return nil, err
		}
		if err := sendText(finalResult.Text); err != nil {
			return nil, err
		}
	}
	if err := sendRole(); err != nil {
		return nil, err
	}

	finishReason := "stop"
	if err := emit(apicompat.ChatCompletionsChunk{
		ID:      streamID,
		Object:  "chat.completion.chunk",
		Created: createdAt,
		Model:   requestedModel,
		Choices: []apicompat.ChatChunkChoice{{
			Index:        0,
			Delta:        apicompat.ChatDelta{},
			FinishReason: &finishReason,
		}},
	}); err != nil {
		return nil, err
	}

	metadata := &WindsurfExecutionMetadata{
		RequestedModel: requestedModel,
		UpstreamModel:  selection.Model.UpstreamModel,
		Duration:       time.Since(startedAt),
		Stream:         true,
		Account:        selection.Account,
	}
	if finalResult != nil {
		metadata.RequestID = strings.TrimSpace(finalResult.RequestID)
		metadata.Usage = finalResult.Usage
	}
	return metadata, nil
}

func (s *WindsurfGatewayService) CompleteMessages(
	ctx context.Context,
	groupID *int64,
	req *apicompat.AnthropicRequest,
) (*apicompat.AnthropicResponse, error) {
	resp, _, err := s.CompleteMessagesWithMetadata(ctx, groupID, req)
	return resp, err
}

func (s *WindsurfGatewayService) CompleteMessagesWithMetadata(
	ctx context.Context,
	groupID *int64,
	req *apicompat.AnthropicRequest,
) (*apicompat.AnthropicResponse, *WindsurfExecutionMetadata, error) {
	if req == nil {
		return nil, nil, ErrWindsurfModelNotSupported
	}
	selection, err := s.SelectChatCompletionAccount(ctx, groupID, req.Model)
	if err != nil {
		return nil, nil, err
	}
	return s.completeMessagesWithSelection(ctx, req, selection)
}

func (s *WindsurfGatewayService) completeMessagesWithSelection(
	ctx context.Context,
	req *apicompat.AnthropicRequest,
	selection *WindsurfAccountSelection,
) (*apicompat.AnthropicResponse, *WindsurfExecutionMetadata, error) {
	chatReq, err := convertWindsurfAnthropicToChatRequest(req)
	if err != nil {
		return nil, nil, err
	}
	chatResp, metadata, err := s.completeChatCompletionsWithSelection(ctx, req.Model, chatReq, selection)
	if err != nil {
		return nil, nil, err
	}
	if metadata != nil {
		metadata.RequestedModel = req.Model
	}
	return buildWindsurfAnthropicResponse(req.Model, chatResp), metadata, nil
}

func (s *WindsurfGatewayService) StreamMessages(
	ctx context.Context,
	groupID *int64,
	req *apicompat.AnthropicRequest,
	emit func(apicompat.AnthropicStreamEvent) error,
) error {
	_, err := s.StreamMessagesWithMetadata(ctx, groupID, req, emit)
	return err
}

func (s *WindsurfGatewayService) StreamMessagesWithMetadata(
	ctx context.Context,
	groupID *int64,
	req *apicompat.AnthropicRequest,
	emit func(apicompat.AnthropicStreamEvent) error,
) (*WindsurfExecutionMetadata, error) {
	if req == nil {
		return nil, ErrWindsurfModelNotSupported
	}
	selection, err := s.SelectChatCompletionAccount(ctx, groupID, req.Model)
	if err != nil {
		return nil, err
	}
	return s.streamMessagesWithSelection(ctx, req, selection, emit)
}

func (s *WindsurfGatewayService) streamMessagesWithSelection(
	ctx context.Context,
	req *apicompat.AnthropicRequest,
	selection *WindsurfAccountSelection,
	emit func(apicompat.AnthropicStreamEvent) error,
) (*WindsurfExecutionMetadata, error) {
	if emit == nil {
		return nil, errors.New("windsurf anthropic stream emit callback is required")
	}
	chatReq, err := convertWindsurfAnthropicToChatRequest(req)
	if err != nil {
		return nil, err
	}

	streamID := newWindsurfAnthropicMessageID()
	started := false
	emitStart := func() error {
		if started {
			return nil
		}
		started = true
		start := &apicompat.AnthropicResponse{
			ID:         streamID,
			Type:       "message",
			Role:       "assistant",
			Model:      req.Model,
			StopReason: "",
			Usage: apicompat.AnthropicUsage{
				InputTokens:  0,
				OutputTokens: 0,
			},
		}
		return emit(apicompat.AnthropicStreamEvent{
			Type:    "message_start",
			Message: start,
		})
	}

	type streamState struct {
		reasoningIndex *int
		textIndex      *int
	}
	state := &streamState{}

	openReasoning := func() error {
		if err := emitStart(); err != nil {
			return err
		}
		if state.reasoningIndex != nil {
			return nil
		}
		index := 0
		state.reasoningIndex = &index
		return emit(apicompat.AnthropicStreamEvent{
			Type:  "content_block_start",
			Index: &index,
			ContentBlock: &apicompat.AnthropicContentBlock{
				Type: "thinking",
			},
		})
	}

	openText := func() error {
		if err := emitStart(); err != nil {
			return err
		}
		if state.textIndex != nil {
			return nil
		}
		index := 0
		if state.reasoningIndex != nil {
			index = 1
		}
		state.textIndex = &index
		return emit(apicompat.AnthropicStreamEvent{
			Type:  "content_block_start",
			Index: &index,
			ContentBlock: &apicompat.AnthropicContentBlock{
				Type: "text",
			},
		})
	}

	metadata, err := s.streamChatCompletionsWithSelection(ctx, req.Model, chatReq, selection, func(chunk apicompat.ChatCompletionsChunk) error {
		if len(chunk.Choices) == 0 {
			return nil
		}
		delta := chunk.Choices[0].Delta
		if delta.ReasoningContent != nil && *delta.ReasoningContent != "" {
			if err := openReasoning(); err != nil {
				return err
			}
			return emit(apicompat.AnthropicStreamEvent{
				Type:  "content_block_delta",
				Index: state.reasoningIndex,
				Delta: &apicompat.AnthropicDelta{
					Type:     "thinking_delta",
					Thinking: *delta.ReasoningContent,
				},
			})
		}
		if delta.Content != nil && *delta.Content != "" {
			if err := openText(); err != nil {
				return err
			}
			return emit(apicompat.AnthropicStreamEvent{
				Type:  "content_block_delta",
				Index: state.textIndex,
				Delta: &apicompat.AnthropicDelta{
					Type: "text_delta",
					Text: *delta.Content,
				},
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if state.reasoningIndex != nil {
		if err := emit(apicompat.AnthropicStreamEvent{
			Type:  "content_block_stop",
			Index: state.reasoningIndex,
		}); err != nil {
			return nil, err
		}
	}
	if state.textIndex != nil {
		if err := emit(apicompat.AnthropicStreamEvent{
			Type:  "content_block_stop",
			Index: state.textIndex,
		}); err != nil {
			return nil, err
		}
	}

	if err := emitStart(); err != nil {
		return nil, err
	}
	if err := emit(apicompat.AnthropicStreamEvent{
		Type: "message_delta",
		Delta: &apicompat.AnthropicDelta{
			StopReason: "end_turn",
		},
		Usage: &apicompat.AnthropicUsage{
			InputTokens:  0,
			OutputTokens: 0,
		},
	}); err != nil {
		return nil, err
	}
	if err := emit(apicompat.AnthropicStreamEvent{Type: "message_stop"}); err != nil {
		return nil, err
	}
	if metadata != nil {
		metadata.RequestedModel = req.Model
	}
	return metadata, nil
}

func (s *WindsurfGatewayService) RecordUsage(ctx context.Context, input *WindsurfRecordUsageInput) error {
	if s == nil || input == nil || input.Result == nil || input.APIKey == nil || input.User == nil || input.Account == nil {
		return nil
	}

	result := input.Result
	apiKey := input.APIKey
	user := input.User
	account := input.Account
	subscription := input.Subscription

	multiplier := 1.0
	if s.cfg != nil {
		multiplier = s.cfg.Default.RateMultiplier
	}
	if apiKey.GroupID != nil && apiKey.Group != nil {
		multiplier = s.getUserGroupRateMultiplier(ctx, user.ID, *apiKey.GroupID, apiKey.Group.RateMultiplier)
	}

	billingModel := forwardResultBillingModel(result.Model, result.UpstreamModel)
	tokens := UsageTokens{
		InputTokens:         result.Usage.InputTokens,
		OutputTokens:        result.Usage.OutputTokens,
		CacheCreationTokens: result.Usage.CacheCreationTokens,
		CacheReadTokens:     result.Usage.CacheReadTokens,
		ImageOutputTokens:   result.Usage.ImageOutputTokens,
	}

	cost := &CostBreakdown{ActualCost: 0}
	if s.billingService != nil {
		var err error
		if s.resolver != nil && apiKey.GroupID != nil {
			gid := *apiKey.GroupID
			cost, err = s.billingService.CalculateCostUnified(CostInput{
				Ctx:            ctx,
				Model:          billingModel,
				GroupID:        &gid,
				Tokens:         tokens,
				RequestCount:   1,
				RateMultiplier: multiplier,
				Resolver:       s.resolver,
			})
		} else {
			cost, err = s.billingService.CalculateCost(billingModel, tokens, multiplier)
		}
		if err != nil {
			cost = &CostBreakdown{ActualCost: 0}
		}
	}

	isSubscriptionBilling := subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
	billingType := BillingTypeBalance
	if isSubscriptionBilling {
		billingType = BillingTypeSubscription
	}

	requestID := resolveUsageBillingRequestID(ctx, result.RequestID)
	durationMs := int(result.Duration.Milliseconds())
	accountRateMultiplier := account.BillingRateMultiplier()
	usageLog := &UsageLog{
		UserID:                user.ID,
		APIKeyID:              apiKey.ID,
		AccountID:             account.ID,
		RequestID:             requestID,
		Model:                 strings.TrimSpace(result.Model),
		RequestedModel:        strings.TrimSpace(result.Model),
		UpstreamModel:         optionalNonEqualStringPtr(strings.TrimSpace(result.UpstreamModel), strings.TrimSpace(result.Model)),
		InboundEndpoint:       optionalTrimmedStringPtr(input.InboundEndpoint),
		UpstreamEndpoint:      optionalTrimmedStringPtr(input.UpstreamEndpoint),
		InputTokens:           result.Usage.InputTokens,
		OutputTokens:          result.Usage.OutputTokens,
		CacheCreationTokens:   result.Usage.CacheCreationTokens,
		CacheReadTokens:       result.Usage.CacheReadTokens,
		ImageOutputTokens:     result.Usage.ImageOutputTokens,
		RateMultiplier:        multiplier,
		AccountRateMultiplier: &accountRateMultiplier,
		BillingType:           billingType,
		Stream:                result.Stream,
		DurationMs:            &durationMs,
		CreatedAt:             time.Now(),
	}
	if cost != nil {
		usageLog.InputCost = cost.InputCost
		usageLog.OutputCost = cost.OutputCost
		usageLog.ImageOutputCost = cost.ImageOutputCost
		usageLog.CacheCreationCost = cost.CacheCreationCost
		usageLog.CacheReadCost = cost.CacheReadCost
		usageLog.TotalCost = cost.TotalCost
		usageLog.ActualCost = cost.ActualCost
	}
	if cost != nil && cost.BillingMode != "" {
		billingMode := cost.BillingMode
		usageLog.BillingMode = &billingMode
	} else {
		billingMode := string(BillingModeToken)
		usageLog.BillingMode = &billingMode
	}
	if input.UserAgent != "" {
		usageLog.UserAgent = &input.UserAgent
	}
	if input.IPAddress != "" {
		usageLog.IPAddress = &input.IPAddress
	}
	if apiKey.GroupID != nil {
		usageLog.GroupID = apiKey.GroupID
	}
	if subscription != nil {
		usageLog.SubscriptionID = &subscription.ID
	}

	if apiKey.GroupID != nil && s.channelService != nil && s.billingService != nil {
		applyAccountStatsCost(ctx, usageLog, s.channelService, s.billingService,
			account.ID, *apiKey.GroupID, result.UpstreamModel, result.Model, tokens, cost.TotalCost)
	}

	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.windsurf_gateway")
		logger.LegacyPrintf("service.windsurf_gateway", "[SIMPLE MODE] Usage recorded (not billed): user=%d, tokens=%d", usageLog.UserID, usageLog.TotalTokens())
		if s.deferredService != nil {
			s.deferredService.ScheduleLastUsedUpdate(account.ID)
		}
		return nil
	}

	_, billingErr := applyUsageBilling(ctx, requestID, usageLog, &postUsageBillingParams{
		Cost:                  cost,
		User:                  user,
		APIKey:                apiKey,
		Account:               account,
		Subscription:          subscription,
		RequestPayloadHash:    resolveUsageBillingPayloadFingerprint(ctx, input.RequestPayloadHash),
		IsSubscriptionBill:    isSubscriptionBilling,
		AccountRateMultiplier: accountRateMultiplier,
		APIKeyService:         input.APIKeyService,
	}, s.billingDeps(), s.usageBillingRepo)
	if billingErr != nil {
		return billingErr
	}

	writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.windsurf_gateway")
	return nil
}

func (s *WindsurfGatewayService) SelectChatCompletionAccount(
	ctx context.Context,
	groupID *int64,
	requestedModel string,
) (*WindsurfAccountSelection, error) {
	candidates, err := s.listChatCompletionAccountSelections(ctx, groupID, requestedModel)
	if err != nil {
		return nil, err
	}
	return &candidates[0], nil
}

func (s *WindsurfGatewayService) listChatCompletionAccountSelections(
	ctx context.Context,
	groupID *int64,
	requestedModel string,
) ([]WindsurfAccountSelection, error) {
	if s == nil || s.accountRepo == nil {
		return nil, ErrWindsurfNoSchedulableAccounts
	}

	var (
		accounts []Account
		err      error
	)
	if groupID != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, PlatformWindsurf)
	} else {
		accounts, err = s.accountRepo.ListSchedulableByPlatform(ctx, PlatformWindsurf)
	}
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, ErrWindsurfNoSchedulableAccounts
	}

	var lastModelErr error
	candidates := make([]WindsurfAccountSelection, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		resolved, err := resolveWindsurfAccountModel(account, requestedModel)
		if err != nil {
			lastModelErr = err
			continue
		}
		candidates = append(candidates, WindsurfAccountSelection{
			Account: account,
			Model:   resolved,
		})
	}

	if len(candidates) > 0 {
		return candidates, nil
	}

	if lastModelErr != nil {
		return nil, lastModelErr
	}
	return nil, ErrWindsurfNoSchedulableAccounts
}

func resolveWindsurfAccountModel(account *Account, requestedModel string) (WindsurfResolvedModel, error) {
	if account == nil {
		return WindsurfResolvedModel{}, ErrWindsurfModelNotSupported
	}

	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return WindsurfResolvedModel{}, ErrWindsurfModelNotSupported
	}

	canonicalRequest := resolveWindsurfCanonicalModelID(requestedModel)
	mappedModel, matched := account.ResolveMappedModel(requestedModel)
	if !matched && canonicalRequest != "" && canonicalRequest != requestedModel {
		mappedModel, matched = account.ResolveMappedModel(canonicalRequest)
	}
	if !matched {
		mappedModel = canonicalRequest
	}

	if mappedModel == "" {
		return WindsurfResolvedModel{}, ErrWindsurfModelNotSupported
	}
	if !windsurfAccountAllowsModel(account, mappedModel) {
		return WindsurfResolvedModel{}, ErrWindsurfModelNotSupported
	}

	var cfg *WindsurfModelConfig
	if foundCfg, ok := account.GetWindsurfModelConfigByID(mappedModel); ok {
		cfg = &foundCfg
	}

	resolved := buildWindsurfResolvedModel(requestedModel, mappedModel, cfg)
	if resolved.ModelUID == "" && resolved.EnumValue == 0 {
		return WindsurfResolvedModel{}, ErrWindsurfModelNotSupported
	}
	return resolved, nil
}

func windsurfAccountAllowsModel(account *Account, model string) bool {
	if account == nil {
		return false
	}
	allowedModels := account.GetWindsurfAllowedModels()
	if len(allowedModels) == 0 {
		return true
	}
	target := resolveWindsurfCanonicalModelID(model)
	for _, allowed := range allowedModels {
		if resolveWindsurfCanonicalModelID(allowed) == target {
			return true
		}
	}
	return false
}

func buildWindsurfChatCompletionResponse(model string, result *WindsurfBridgeResult) *apicompat.ChatCompletionsResponse {
	response := &apicompat.ChatCompletionsResponse{
		ID:      newWindsurfChatCompletionID(),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   strings.TrimSpace(model),
		Choices: []apicompat.ChatChoice{{
			Index: 0,
			Message: apicompat.ChatMessage{
				Role: "assistant",
			},
			FinishReason: "stop",
		}},
	}
	if result == nil {
		return response
	}
	if result.Text != "" {
		content, _ := json.Marshal(result.Text)
		response.Choices[0].Message.Content = content
	}
	if result.Reasoning != "" {
		response.Choices[0].Message.ReasoningContent = result.Reasoning
	}
	return response
}

func newWindsurfChatCompletionID() string {
	return "chatcmpl-" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func newWindsurfAnthropicMessageID() string {
	return "msg_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func buildWindsurfAnthropicResponse(model string, chatResp *apicompat.ChatCompletionsResponse) *apicompat.AnthropicResponse {
	resp := &apicompat.AnthropicResponse{
		ID:         newWindsurfAnthropicMessageID(),
		Type:       "message",
		Role:       "assistant",
		Model:      strings.TrimSpace(model),
		StopReason: "end_turn",
		Usage: apicompat.AnthropicUsage{
			InputTokens:  0,
			OutputTokens: 0,
		},
	}
	if chatResp == nil || len(chatResp.Choices) == 0 {
		return resp
	}

	message := chatResp.Choices[0].Message
	if message.ReasoningContent != "" {
		resp.Content = append(resp.Content, apicompat.AnthropicContentBlock{
			Type:     "thinking",
			Thinking: message.ReasoningContent,
		})
	}
	if text := parseChatMessageContentString(message.Content); text != "" {
		resp.Content = append(resp.Content, apicompat.AnthropicContentBlock{
			Type: "text",
			Text: text,
		})
	}
	return resp
}

func convertWindsurfAnthropicToChatRequest(req *apicompat.AnthropicRequest) (*apicompat.ChatCompletionsRequest, error) {
	if req == nil {
		return nil, ErrWindsurfModelNotSupported
	}
	chatReq := &apicompat.ChatCompletionsRequest{
		Model:  req.Model,
		Stream: req.Stream,
	}
	if req.Temperature != nil {
		chatReq.Temperature = req.Temperature
	}
	if req.TopP != nil {
		chatReq.TopP = req.TopP
	}
	if req.MaxTokens > 0 {
		maxTokens := req.MaxTokens
		chatReq.MaxTokens = &maxTokens
	}
	if systemText := parseAnthropicSystemPromptText(req.System); systemText != "" {
		raw, _ := json.Marshal(systemText)
		chatReq.Messages = append(chatReq.Messages, apicompat.ChatMessage{
			Role:    "system",
			Content: raw,
		})
	}
	for _, message := range req.Messages {
		text := anthropicMessageContentText(message.Content)
		raw, _ := json.Marshal(text)
		chatReq.Messages = append(chatReq.Messages, apicompat.ChatMessage{
			Role:    message.Role,
			Content: raw,
		})
	}
	return chatReq, nil
}

func parseAnthropicSystemPromptText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var blocks []apicompat.AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var lines []string
	for _, block := range blocks {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			lines = append(lines, block.Text)
		}
	}
	return strings.Join(lines, "\n\n")
}

func anthropicMessageContentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var blocks []apicompat.AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return strings.TrimSpace(string(raw))
	}
	var lines []string
	for _, block := range blocks {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				lines = append(lines, block.Text)
			}
		case "tool_result":
			if block.Content != nil {
				lines = append(lines, anthropicMessageContentText(block.Content))
			}
		}
	}
	return strings.Join(lines, "\n\n")
}

func parseChatMessageContentString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	return strings.TrimSpace(string(raw))
}
