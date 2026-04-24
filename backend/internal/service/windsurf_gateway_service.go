package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
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
	ToolCalls []apicompat.ChatToolCall
	Usage     WindsurfBridgeUsage

	conversation *windsurfConversationResult
}

type WindsurfBridgeStreamChunk struct {
	Text      string
	Reasoning string
	ToolCalls []apicompat.ChatToolCall
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
	selection, nextCtx, err := s.selectChatCompletionAccountForRequest(ctx, groupID, req)
	if err != nil {
		return nil, nil, err
	}
	return s.completeChatCompletionsWithSelection(nextCtx, groupID, req.Model, req, selection)
}

func (s *WindsurfGatewayService) completeChatCompletionsWithSelection(
	ctx context.Context,
	groupID *int64,
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
	cacheScope := windsurfCacheScope(groupID, selection)
	if cached, ok := defaultWindsurfResponseCache.Get(req, cacheScope); ok {
		restoreWindsurfConversationReuse(ctx)
		metadata := &WindsurfExecutionMetadata{
			RequestedModel: requestedModel,
			UpstreamModel:  selection.Model.UpstreamModel,
			Usage:          cached.Usage,
			Duration:       time.Since(startedAt),
			Stream:         false,
			Account:        selection.Account,
		}
		return buildWindsurfChatCompletionResponse(requestedModel, cached), metadata, nil
	}

	result, err := s.chatBridge.Complete(ctx, selection.Account, selection.Model, req)
	if err != nil {
		restoreWindsurfConversationReuse(ctx)
		return nil, nil, err
	}
	result = normalizeWindsurfBridgeResult(req, result)
	checkinWindsurfConversation(ctx, req, selection, result)
	defaultWindsurfResponseCache.Set(req, cacheScope, result)

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
	selection, nextCtx, err := s.selectChatCompletionAccountForRequest(ctx, groupID, req)
	if err != nil {
		return nil, err
	}
	return s.streamChatCompletionsWithSelection(nextCtx, groupID, req.Model, req, selection, emit)
}

func (s *WindsurfGatewayService) streamChatCompletionsWithSelection(
	ctx context.Context,
	groupID *int64,
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
	textSanitizer := newWindsurfPathSanitizeStream()
	reasoningSanitizer := newWindsurfPathSanitizeStream()

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

	cacheScope := windsurfCacheScope(groupID, selection)
	if cached, ok := defaultWindsurfResponseCache.Get(req, cacheScope); ok {
		restoreWindsurfConversationReuse(ctx)
		if err := sendRole(); err != nil {
			return nil, err
		}
		if err := sendReasoning(cached.Reasoning); err != nil {
			return nil, err
		}
		if err := sendText(cached.Text); err != nil {
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
		if req.StreamOptions != nil && req.StreamOptions.IncludeUsage {
			if err := emit(apicompat.ChatCompletionsChunk{
				ID:      streamID,
				Object:  "chat.completion.chunk",
				Created: createdAt,
				Model:   requestedModel,
				Choices: []apicompat.ChatChunkChoice{},
				Usage:   buildWindsurfChatUsage(cached.Usage),
			}); err != nil {
				return nil, err
			}
		}
		return &WindsurfExecutionMetadata{
			RequestedModel: requestedModel,
			UpstreamModel:  selection.Model.UpstreamModel,
			Usage:          cached.Usage,
			Duration:       time.Since(startedAt),
			Stream:         true,
			Account:        selection.Account,
		}, nil
	}

	sendToolCall := func(call apicompat.ChatToolCall, index int) error {
		if err := sendRole(); err != nil {
			return err
		}
		call.Index = &index
		if call.Type == "" {
			call.Type = "function"
		}
		return emit(apicompat.ChatCompletionsChunk{
			ID:      streamID,
			Object:  "chat.completion.chunk",
			Created: createdAt,
			Model:   requestedModel,
			Choices: []apicompat.ChatChunkChoice{{
				Index: 0,
				Delta: apicompat.ChatDelta{
					ToolCalls: []apicompat.ChatToolCall{call},
				},
				FinishReason: nil,
			}},
		})
	}

	parser := &windsurfToolCallParser{}
	parseToolCalls := selection.Model.ModelUID != "" && windsurfShouldOfferTools(req)
	collectedToolCalls := make([]apicompat.ChatToolCall, 0)
	sawBridgeOutput := false

	processText := func(text string) error {
		if text == "" {
			return nil
		}
		text = textSanitizer.Feed(text)
		if text == "" {
			return nil
		}
		if !parseToolCalls {
			return sendText(text)
		}
		safeText, calls := parser.Feed(text)
		if err := sendText(safeText); err != nil {
			return err
		}
		for _, call := range calls {
			index := len(collectedToolCalls)
			collectedToolCalls = append(collectedToolCalls, call)
			if err := sendToolCall(call, index); err != nil {
				return err
			}
		}
		return nil
	}

	finalResult, err := s.chatBridge.Stream(ctx, selection.Account, selection.Model, req, func(chunk WindsurfBridgeStreamChunk) error {
		if chunk.Text != "" || chunk.Reasoning != "" || len(chunk.ToolCalls) > 0 {
			sawBridgeOutput = true
		}
		if err := sendReasoning(reasoningSanitizer.Feed(chunk.Reasoning)); err != nil {
			return err
		}
		if err := processText(chunk.Text); err != nil {
			return err
		}
		for _, call := range chunk.ToolCalls {
			call = sanitizeWindsurfToolCall(call)
			index := len(collectedToolCalls)
			collectedToolCalls = append(collectedToolCalls, call)
			if err := sendToolCall(call, index); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		restoreWindsurfConversationReuse(ctx)
		return nil, err
	}
	finalResult = normalizeWindsurfBridgeResult(req, finalResult)
	checkinWindsurfConversation(ctx, req, selection, finalResult)

	if finalResult != nil && !sawBridgeOutput {
		if err := sendReasoning(finalResult.Reasoning); err != nil {
			return nil, err
		}
		if err := processText(finalResult.Text); err != nil {
			return nil, err
		}
		for _, call := range finalResult.ToolCalls {
			index := len(collectedToolCalls)
			collectedToolCalls = append(collectedToolCalls, call)
			if err := sendToolCall(call, index); err != nil {
				return nil, err
			}
		}
	}
	if parseToolCalls {
		if err := sendReasoning(reasoningSanitizer.Flush()); err != nil {
			return nil, err
		}
		sanitizedTail := textSanitizer.Flush()
		safeText, calls := parser.Feed(sanitizedTail)
		if err := sendText(safeText); err != nil {
			return nil, err
		}
		for _, call := range calls {
			call = sanitizeWindsurfToolCall(call)
			index := len(collectedToolCalls)
			collectedToolCalls = append(collectedToolCalls, call)
			if err := sendToolCall(call, index); err != nil {
				return nil, err
			}
		}
		safeText, calls = parser.Flush()
		if err := sendText(safeText); err != nil {
			return nil, err
		}
		for _, call := range calls {
			call = sanitizeWindsurfToolCall(call)
			index := len(collectedToolCalls)
			collectedToolCalls = append(collectedToolCalls, call)
			if err := sendToolCall(call, index); err != nil {
				return nil, err
			}
		}
	} else {
		if err := sendReasoning(reasoningSanitizer.Flush()); err != nil {
			return nil, err
		}
		if err := sendText(textSanitizer.Flush()); err != nil {
			return nil, err
		}
	}
	if err := sendRole(); err != nil {
		return nil, err
	}

	finishReason := "stop"
	if len(collectedToolCalls) > 0 {
		finishReason = "tool_calls"
	}
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
	if req.StreamOptions != nil && req.StreamOptions.IncludeUsage && finalResult != nil {
		if err := emit(apicompat.ChatCompletionsChunk{
			ID:      streamID,
			Object:  "chat.completion.chunk",
			Created: createdAt,
			Model:   requestedModel,
			Choices: []apicompat.ChatChunkChoice{},
			Usage:   buildWindsurfChatUsage(finalResult.Usage),
		}); err != nil {
			return nil, err
		}
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
	if len(collectedToolCalls) == 0 {
		defaultWindsurfResponseCache.Set(req, cacheScope, finalResult)
	}
	return metadata, nil
}

func windsurfCacheScope(groupID *int64, selection *WindsurfAccountSelection) windsurfResponseCacheScope {
	var scope windsurfResponseCacheScope
	if groupID != nil {
		scope.GroupID = *groupID
	}
	if selection == nil {
		return scope
	}
	if selection.Account != nil {
		scope.AccountID = selection.Account.ID
	}
	scope.UpstreamModel = selection.Model.UpstreamModel
	scope.ModelUID = selection.Model.ModelUID
	return scope
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
	chatReq, err := convertWindsurfAnthropicToChatRequest(req)
	if err != nil {
		return nil, nil, err
	}
	selection, nextCtx, err := s.selectChatCompletionAccountForRequest(ctx, groupID, chatReq)
	if err != nil {
		return nil, nil, err
	}
	return s.completeMessagesWithChatRequest(nextCtx, groupID, req, chatReq, selection)
}

func (s *WindsurfGatewayService) completeMessagesWithSelection(
	ctx context.Context,
	groupID *int64,
	req *apicompat.AnthropicRequest,
	selection *WindsurfAccountSelection,
) (*apicompat.AnthropicResponse, *WindsurfExecutionMetadata, error) {
	chatReq, err := convertWindsurfAnthropicToChatRequest(req)
	if err != nil {
		return nil, nil, err
	}
	return s.completeMessagesWithChatRequest(ctx, groupID, req, chatReq, selection)
}

func (s *WindsurfGatewayService) completeMessagesWithChatRequest(
	ctx context.Context,
	groupID *int64,
	req *apicompat.AnthropicRequest,
	chatReq *apicompat.ChatCompletionsRequest,
	selection *WindsurfAccountSelection,
) (*apicompat.AnthropicResponse, *WindsurfExecutionMetadata, error) {
	chatResp, metadata, err := s.completeChatCompletionsWithSelection(ctx, groupID, req.Model, chatReq, selection)
	if err != nil {
		return nil, nil, err
	}
	if metadata != nil {
		metadata.RequestedModel = req.Model
	}
	resp := buildWindsurfAnthropicResponse(req.Model, chatResp)
	if metadata != nil {
		resp.Usage = *buildWindsurfAnthropicUsageFromMetadata(metadata)
	}
	return resp, metadata, nil
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
	chatReq, err := convertWindsurfAnthropicToChatRequest(req)
	if err != nil {
		return nil, err
	}
	selection, nextCtx, err := s.selectChatCompletionAccountForRequest(ctx, groupID, chatReq)
	if err != nil {
		return nil, err
	}
	return s.streamMessagesWithChatRequest(nextCtx, groupID, req, chatReq, selection, emit)
}

func (s *WindsurfGatewayService) streamMessagesWithSelection(
	ctx context.Context,
	groupID *int64,
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
	return s.streamMessagesWithChatRequest(ctx, groupID, req, chatReq, selection, emit)
}

func (s *WindsurfGatewayService) streamMessagesWithChatRequest(
	ctx context.Context,
	groupID *int64,
	req *apicompat.AnthropicRequest,
	chatReq *apicompat.ChatCompletionsRequest,
	selection *WindsurfAccountSelection,
	emit func(apicompat.AnthropicStreamEvent) error,
) (*WindsurfExecutionMetadata, error) {
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
		currentIndex int
		currentType  string
		currentTool  string
		nextIndex    int
		stopReason   string
	}
	state := &streamState{currentIndex: -1, stopReason: "end_turn"}

	closeCurrent := func() error {
		if state.currentIndex < 0 {
			return nil
		}
		idx := state.currentIndex
		state.currentIndex = -1
		state.currentType = ""
		state.currentTool = ""
		return emit(apicompat.AnthropicStreamEvent{
			Type:  "content_block_stop",
			Index: &idx,
		})
	}

	openBlock := func(blockType string, block apicompat.AnthropicContentBlock) error {
		if err := emitStart(); err != nil {
			return err
		}
		if state.currentType == blockType && state.currentIndex >= 0 {
			return nil
		}
		if err := closeCurrent(); err != nil {
			return err
		}
		index := state.nextIndex
		state.nextIndex++
		state.currentIndex = index
		state.currentType = blockType
		state.currentTool = ""
		return emit(apicompat.AnthropicStreamEvent{
			Type:         "content_block_start",
			Index:        &index,
			ContentBlock: &block,
		})
	}

	openReasoning := func() error {
		return openBlock("thinking", apicompat.AnthropicContentBlock{Type: "thinking"})
	}

	openText := func() error {
		return openBlock("text", apicompat.AnthropicContentBlock{Type: "text"})
	}

	openTool := func(call apicompat.ChatToolCall) error {
		key := call.ID
		if key == "" && call.Index != nil {
			key = strconv.Itoa(*call.Index)
		}
		if key != "" && state.currentType == "tool_use" && state.currentTool == key && state.currentIndex >= 0 {
			return nil
		}
		if state.currentType == "tool_use" && state.currentIndex >= 0 {
			if err := closeCurrent(); err != nil {
				return err
			}
		}
		if err := openBlock("tool_use", apicompat.AnthropicContentBlock{
			Type:  "tool_use",
			ID:    call.ID,
			Name:  call.Function.Name,
			Input: json.RawMessage(`{}`),
		}); err != nil {
			return err
		}
		state.currentTool = key
		return nil
	}

	metadata, err := s.streamChatCompletionsWithSelection(ctx, groupID, req.Model, chatReq, selection, func(chunk apicompat.ChatCompletionsChunk) error {
		if len(chunk.Choices) == 0 {
			return nil
		}
		choice := chunk.Choices[0]
		delta := choice.Delta
		if delta.ReasoningContent != nil && *delta.ReasoningContent != "" {
			if err := openReasoning(); err != nil {
				return err
			}
			return emit(apicompat.AnthropicStreamEvent{
				Type:  "content_block_delta",
				Index: &state.currentIndex,
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
				Index: &state.currentIndex,
				Delta: &apicompat.AnthropicDelta{
					Type: "text_delta",
					Text: *delta.Content,
				},
			})
		}
		for _, call := range delta.ToolCalls {
			if err := openTool(call); err != nil {
				return err
			}
			if call.Function.Arguments != "" {
				if err := emit(apicompat.AnthropicStreamEvent{
					Type:  "content_block_delta",
					Index: &state.currentIndex,
					Delta: &apicompat.AnthropicDelta{
						Type:        "input_json_delta",
						PartialJSON: call.Function.Arguments,
					},
				}); err != nil {
					return err
				}
			}
		}
		if choice.FinishReason != nil {
			state.stopReason = mapWindsurfChatFinishReasonToAnthropic(*choice.FinishReason)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := closeCurrent(); err != nil {
		return nil, err
	}

	if err := emitStart(); err != nil {
		return nil, err
	}
	if err := emit(apicompat.AnthropicStreamEvent{
		Type: "message_delta",
		Delta: &apicompat.AnthropicDelta{
			StopReason: state.stopReason,
		},
		Usage: buildWindsurfAnthropicUsageFromMetadata(metadata),
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

func (s *WindsurfGatewayService) selectChatCompletionAccountForRequest(
	ctx context.Context,
	groupID *int64,
	req *apicompat.ChatCompletionsRequest,
) (*WindsurfAccountSelection, context.Context, error) {
	if req == nil {
		return nil, ctx, ErrWindsurfModelNotSupported
	}
	candidates, err := s.listChatCompletionAccountSelections(ctx, groupID, req.Model)
	if err != nil {
		return nil, ctx, err
	}
	if len(candidates) == 0 {
		return nil, ctx, ErrWindsurfNoSchedulableAccounts
	}

	endpointKey := windsurfConversationEndpointKey(s.chatBridge)
	scope := windsurfConversationScopeForRequest(ctx, groupID)
	scopedCtx := WithWindsurfConversationScope(ctx, scope)
	seenFingerprints := make(map[string]struct{}, len(candidates))
	for i := range candidates {
		if strings.TrimSpace(candidates[i].Model.ModelUID) == "" {
			continue
		}
		modelKey := windsurfConversationModelKey(candidates[i].Model)
		fingerprint := windsurfConversationFingerprintBefore(req.Messages, modelKey, scope)
		if fingerprint == "" {
			continue
		}
		if _, ok := seenFingerprints[fingerprint]; ok {
			continue
		}
		seenFingerprints[fingerprint] = struct{}{}

		entry, ok := defaultWindsurfConversationPool.Checkout(fingerprint)
		if !ok || entry == nil {
			continue
		}

		for j := range candidates {
			if candidates[j].Account == nil || candidates[j].Account.ID != entry.AccountID {
				continue
			}
			if entry.EndpointKey != "" && endpointKey != "" && entry.EndpointKey != endpointKey {
				break
			}
			reuse := &windsurfConversationReuseContext{
				BeforeFingerprint: fingerprint,
				Entry:             entry,
			}
			return &candidates[j], withWindsurfConversationReuse(scopedCtx, reuse), nil
		}

		defaultWindsurfConversationPool.Checkin(fingerprint, entry)
	}

	return &candidates[0], scopedCtx, nil
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

func windsurfConversationEndpointKey(bridge windsurfChatBridge) string {
	if concrete, ok := bridge.(*WindsurfChatBridge); ok && concrete != nil {
		return strings.TrimSpace(concrete.baseURL)
	}
	return ""
}

func restoreWindsurfConversationReuse(ctx context.Context) {
	reuse := windsurfConversationReuseFromContext(ctx)
	if reuse == nil || reuse.Entry == nil || strings.TrimSpace(reuse.BeforeFingerprint) == "" {
		return
	}
	defaultWindsurfConversationPool.Checkin(reuse.BeforeFingerprint, reuse.Entry)
	reuse.Entry = nil
}

func checkinWindsurfConversation(ctx context.Context, req *apicompat.ChatCompletionsRequest, selection *WindsurfAccountSelection, result *WindsurfBridgeResult) {
	if req == nil || selection == nil || selection.Account == nil || result == nil || result.conversation == nil {
		return
	}
	scope := windsurfConversationScopeForRequest(ctx, nil)
	fingerprint := windsurfConversationFingerprintAfter(req.Messages, windsurfConversationModelKey(selection.Model), scope)
	if fingerprint == "" {
		return
	}
	defaultWindsurfConversationPool.Checkin(fingerprint, &windsurfConversationPoolEntry{
		CascadeID:   strings.TrimSpace(result.conversation.CascadeID),
		SessionID:   strings.TrimSpace(result.conversation.SessionID),
		AccountID:   selection.Account.ID,
		EndpointKey: strings.TrimSpace(result.conversation.EndpointKey),
	})
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
	if len(result.ToolCalls) > 0 {
		response.Choices[0].Message.ToolCalls = result.ToolCalls
		response.Choices[0].FinishReason = "tool_calls"
	}
	response.Usage = buildWindsurfChatUsage(result.Usage)
	return response
}

func normalizeWindsurfBridgeResult(req *apicompat.ChatCompletionsRequest, result *WindsurfBridgeResult) *WindsurfBridgeResult {
	if result == nil {
		return nil
	}
	result.Text = sanitizeWindsurfText(result.Text)
	result.Reasoning = sanitizeWindsurfText(result.Reasoning)
	for i := range result.ToolCalls {
		result.ToolCalls[i] = sanitizeWindsurfToolCall(result.ToolCalls[i])
	}
	if !windsurfShouldOfferTools(req) {
		return result
	}
	text, calls := parseWindsurfToolCallsFromText(result.Text)
	result.Text = text
	for _, call := range calls {
		result.ToolCalls = append(result.ToolCalls, sanitizeWindsurfToolCall(call))
	}
	return result
}

func buildWindsurfChatUsage(usage WindsurfBridgeUsage) *apicompat.ChatUsage {
	promptTokens := usage.InputTokens + usage.CacheReadTokens + usage.CacheCreationTokens
	completionTokens := usage.OutputTokens + usage.ImageOutputTokens
	out := &apicompat.ChatUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}
	if usage.CacheReadTokens > 0 {
		out.PromptTokensDetails = &apicompat.ChatTokenDetails{CachedTokens: usage.CacheReadTokens}
	}
	return out
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
	resp.StopReason = mapWindsurfChatFinishReasonToAnthropic(chatResp.Choices[0].FinishReason)
	if chatResp.Usage != nil {
		resp.Usage = apicompat.AnthropicUsage{
			InputTokens:              chatResp.Usage.PromptTokens,
			OutputTokens:             chatResp.Usage.CompletionTokens,
			CacheCreationInputTokens: 0,
		}
		if chatResp.Usage.PromptTokensDetails != nil {
			resp.Usage.CacheReadInputTokens = chatResp.Usage.PromptTokensDetails.CachedTokens
		}
	}
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
	for _, call := range message.ToolCalls {
		input := json.RawMessage(`{}`)
		if strings.TrimSpace(call.Function.Arguments) != "" {
			input = json.RawMessage(call.Function.Arguments)
		}
		resp.Content = append(resp.Content, apicompat.AnthropicContentBlock{
			Type:  "tool_use",
			ID:    call.ID,
			Name:  call.Function.Name,
			Input: input,
		})
	}
	if len(resp.Content) == 0 {
		resp.Content = append(resp.Content, apicompat.AnthropicContentBlock{Type: "text", Text: ""})
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
	if len(req.StopSeqs) > 0 {
		raw, _ := json.Marshal(req.StopSeqs)
		chatReq.Stop = raw
	}
	if len(req.Tools) > 0 {
		chatReq.Tools = convertWindsurfAnthropicToolsToChat(req.Tools)
	}
	if len(req.ToolChoice) > 0 {
		chatReq.ToolChoice = convertWindsurfAnthropicToolChoice(req.ToolChoice)
	}
	if systemText := parseAnthropicSystemPromptText(req.System); systemText != "" {
		raw, _ := json.Marshal(systemText)
		chatReq.Messages = append(chatReq.Messages, apicompat.ChatMessage{
			Role:    "system",
			Content: raw,
		})
	}
	for _, message := range req.Messages {
		chatReq.Messages = append(chatReq.Messages, convertWindsurfAnthropicMessageToChat(message)...)
	}
	return chatReq, nil
}

func mapWindsurfChatFinishReasonToAnthropic(reason string) string {
	switch reason {
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return "end_turn"
	}
}

func buildWindsurfAnthropicUsageFromMetadata(metadata *WindsurfExecutionMetadata) *apicompat.AnthropicUsage {
	if metadata == nil {
		return &apicompat.AnthropicUsage{}
	}
	return &apicompat.AnthropicUsage{
		InputTokens:              metadata.Usage.InputTokens,
		OutputTokens:             metadata.Usage.OutputTokens + metadata.Usage.ImageOutputTokens,
		CacheCreationInputTokens: metadata.Usage.CacheCreationTokens,
		CacheReadInputTokens:     metadata.Usage.CacheReadTokens,
	}
}

func convertWindsurfAnthropicToolsToChat(tools []apicompat.AnthropicTool) []apicompat.ChatTool {
	out := make([]apicompat.ChatTool, 0, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" {
			continue
		}
		out = append(out, apicompat.ChatTool{
			Type: "function",
			Function: &apicompat.ChatFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			},
		})
	}
	return out
}

func convertWindsurfAnthropicToolChoice(raw json.RawMessage) json.RawMessage {
	var obj struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &obj) != nil {
		return raw
	}
	switch obj.Type {
	case "auto":
		return json.RawMessage(`"auto"`)
	case "any":
		return json.RawMessage(`"required"`)
	case "none":
		return json.RawMessage(`"none"`)
	case "tool":
		converted, _ := json.Marshal(map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": obj.Name,
			},
		})
		return converted
	default:
		return raw
	}
}

func convertWindsurfAnthropicMessageToChat(message apicompat.AnthropicMessage) []apicompat.ChatMessage {
	role := "user"
	if message.Role == "assistant" {
		role = "assistant"
	}
	if len(message.Content) == 0 {
		return []apicompat.ChatMessage{{Role: role}}
	}
	var text string
	if json.Unmarshal(message.Content, &text) == nil {
		raw, _ := json.Marshal(text)
		return []apicompat.ChatMessage{{Role: role, Content: raw}}
	}
	var blocks []apicompat.AnthropicContentBlock
	if json.Unmarshal(message.Content, &blocks) != nil {
		raw, _ := json.Marshal(strings.TrimSpace(string(message.Content)))
		return []apicompat.ChatMessage{{Role: role, Content: raw}}
	}

	var out []apicompat.ChatMessage
	textParts := make([]string, 0)
	contentParts := make([]apicompat.ChatContentPart, 0)
	hasImage := false
	toolCalls := make([]apicompat.ChatToolCall, 0)

	flushPending := func() {
		if len(textParts) == 0 && len(contentParts) == 0 && len(toolCalls) == 0 {
			return
		}
		msg := apicompat.ChatMessage{Role: role}
		if role == "assistant" {
			raw, _ := json.Marshal(strings.Join(textParts, "\n"))
			msg.Content = raw
			msg.ToolCalls = append(msg.ToolCalls, toolCalls...)
		} else if hasImage {
			raw, _ := json.Marshal(contentParts)
			msg.Content = raw
		} else if len(textParts) > 0 {
			raw, _ := json.Marshal(strings.Join(textParts, "\n"))
			msg.Content = raw
		}
		out = append(out, msg)
		textParts = nil
		contentParts = nil
		hasImage = false
		toolCalls = nil
	}

	for _, block := range blocks {
		switch block.Type {
		case "text":
			if block.Text != "" {
				textParts = append(textParts, block.Text)
				if role == "user" {
					contentParts = append(contentParts, apicompat.ChatContentPart{Type: "text", Text: block.Text})
				}
			}
		case "image":
			if block.Source != nil && block.Source.Data != "" {
				hasImage = true
				contentParts = append(contentParts, apicompat.ChatContentPart{
					Type: "image_url",
					ImageURL: &apicompat.ChatImageURL{
						URL: "data:" + firstNonEmptyString(block.Source.MediaType, "image/png") + ";base64," + block.Source.Data,
					},
				})
			}
		case "thinking":
			continue
		case "tool_use":
			if role == "assistant" {
				args := "{}"
				if len(block.Input) > 0 {
					args = string(block.Input)
				}
				toolCalls = append(toolCalls, apicompat.ChatToolCall{
					ID:   firstNonEmptyString(block.ID, "call_"+uuid.NewString()),
					Type: "function",
					Function: apicompat.ChatFunctionCall{
						Name:      block.Name,
						Arguments: args,
					},
				})
			}
		case "tool_result":
			content := windsurfAnthropicToolResultMarkup(block.ToolUseID, anthropicToolResultContentText(block.Content))
			if role == "user" {
				textParts = append(textParts, content)
				contentParts = append(contentParts, apicompat.ChatContentPart{Type: "text", Text: content})
				continue
			}
			flushPending()
			raw, _ := json.Marshal(content)
			out = append(out, apicompat.ChatMessage{
				Role:       "tool",
				ToolCallID: block.ToolUseID,
				Content:    raw,
			})
		}
	}
	flushPending()
	return out
}

func windsurfAnthropicToolResultMarkup(toolCallID, content string) string {
	toolCallID = strings.TrimSpace(toolCallID)
	content = strings.TrimSpace(content)

	var b strings.Builder
	b.WriteString("<tool_result")
	if toolCallID != "" {
		b.WriteString(" tool_call_id=")
		b.WriteString(strconv.Quote(toolCallID))
	}
	b.WriteString(">")
	if content != "" {
		b.WriteString("\n")
		b.WriteString(content)
		b.WriteString("\n")
	}
	b.WriteString("</tool_result>")
	return b.String()
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

func anthropicToolResultContentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var blocks []apicompat.AnthropicContentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		lines := make([]string, 0, len(blocks))
		for _, block := range blocks {
			if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
				lines = append(lines, block.Text)
			}
		}
		return strings.Join(lines, "\n")
	}
	return strings.TrimSpace(string(raw))
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
