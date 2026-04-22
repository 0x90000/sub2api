package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

type windsurfProbeAccountRepository interface {
	GetByID(ctx context.Context, id int64) (*Account, error)
	UpdateExtra(ctx context.Context, id int64, updates map[string]any) error
	SetError(ctx context.Context, id int64, errorMsg string) error
	ClearError(ctx context.Context, id int64) error
	SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error
	ClearRateLimit(ctx context.Context, id int64) error
}

type windsurfProbeProxyRepository interface {
	GetByID(ctx context.Context, id int64) (*Proxy, error)
}

type WindsurfAccountProbeService struct {
	accountRepo  windsurfProbeAccountRepository
	proxyRepo    windsurfProbeProxyRepository
	usageFetcher *WindsurfUsageFetcher
}

func NewWindsurfAccountProbeService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	usageFetcher *WindsurfUsageFetcher,
) *WindsurfAccountProbeService {
	return &WindsurfAccountProbeService{
		accountRepo:  accountRepo,
		proxyRepo:    proxyRepo,
		usageFetcher: usageFetcher,
	}
}

func (s *WindsurfAccountProbeService) ProbeAndPersist(ctx context.Context, accountID int64) (*Account, error) {
	if s == nil || s.accountRepo == nil {
		return nil, fmt.Errorf("windsurf account probe service not configured")
	}

	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, fmt.Errorf("account not found")
	}
	if !account.IsWindsurf() {
		return nil, fmt.Errorf("account %d is not a windsurf account", account.ID)
	}

	updates, resetAt, err := s.probe(ctx, account)
	if err != nil {
		_ = s.accountRepo.SetError(ctx, account.ID, err.Error())
		return nil, err
	}

	if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		return nil, fmt.Errorf("persist windsurf probe snapshot: %w", err)
	}
	mergeAccountExtra(account, updates)

	if resetAt != nil && account.GetExtraString("rate_limited_at") != "" {
		_ = s.accountRepo.SetRateLimited(ctx, account.ID, *resetAt)
	} else {
		_ = s.accountRepo.ClearRateLimit(ctx, account.ID)
	}
	_ = s.accountRepo.ClearError(ctx, account.ID)

	return account, nil
}

func (s *WindsurfAccountProbeService) probe(ctx context.Context, account *Account) (map[string]any, *time.Time, error) {
	if s.usageFetcher == nil {
		return nil, nil, fmt.Errorf("windsurf usage fetcher not configured")
	}

	token := account.GetWindsurfToken()
	if token == "" {
		return nil, nil, fmt.Errorf("no windsurf token available")
	}

	req := WindsurfFetchRequest{
		APIToken:           token,
		ProxyURL:           s.resolveProxyURL(ctx, account),
		AccountID:          account.ID,
		AccountConcurrency: account.Concurrency,
	}

	status, err := s.usageFetcher.GetUserStatus(ctx, req)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch windsurf user status: %w", err)
	}

	modelConfigs, modelErr := s.usageFetcher.GetCascadeModelConfigs(ctx, req)
	rateLimit, rateErr := s.usageFetcher.CheckMessageRateLimit(ctx, req)

	now := time.Now().UTC()
	planTier := resolveWindsurfPlanTier(status)
	allowedModels := resolveWindsurfAllowedModels(planTier, modelConfigs, account.GetWindsurfAllowedModels())

	updates := map[string]any{
		"plan_tier":        planTier,
		"plan_name":        status.PlanName,
		"credit_balance":   status.CreditBalance,
		"usage_updated_at": now.Format(time.RFC3339),
		"allowed_models":   allowedModels,
		"probe_error":      buildWindsurfProbeWarning(modelErr, rateErr),
	}

	if status.DailyRemainingPercent > 0 || status.DailyResetAt != nil {
		updates["daily_remaining_percent"] = status.DailyRemainingPercent
	}
	if status.WeeklyRemainingPercent > 0 || status.WeeklyResetAt != nil {
		updates["weekly_remaining_percent"] = status.WeeklyRemainingPercent
	}
	if status.DailyResetAt != nil {
		updates["daily_reset_at"] = status.DailyResetAt.UTC().Format(time.RFC3339)
	}
	if status.WeeklyResetAt != nil {
		updates["weekly_reset_at"] = status.WeeklyResetAt.UTC().Format(time.RFC3339)
	}

	var resetAt *time.Time
	if rateLimit != nil {
		updates["messages_remaining"] = rateLimit.MessagesRemaining
		updates["max_messages"] = rateLimit.MaxMessages
		if !rateLimit.HasCapacity {
			updates["rate_limited_at"] = now.Format(time.RFC3339)
			resetAt = preferredWindsurfRateLimitReset(status)
			if resetAt != nil {
				updates["rate_limit_reset_at"] = resetAt.UTC().Format(time.RFC3339)
			}
		} else {
			updates["rate_limited_at"] = ""
			updates["rate_limit_reset_at"] = ""
		}
	}

	return updates, resetAt, nil
}

func (s *WindsurfAccountProbeService) resolveProxyURL(ctx context.Context, account *Account) string {
	if account == nil {
		return ""
	}
	if account.Proxy != nil {
		return account.Proxy.URL()
	}
	if account.ProxyID == nil || s.proxyRepo == nil {
		return ""
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
	if err != nil || proxy == nil {
		return ""
	}
	return proxy.URL()
}

func resolveWindsurfPlanTier(status *WindsurfUserStatusSnapshot) string {
	if status == nil {
		return "free"
	}
	if status.HasPaidFeatures || status.CreditBalance > 0 {
		return "pro"
	}
	name := strings.ToLower(strings.TrimSpace(status.PlanName))
	switch {
	case name == "":
		return "free"
	case strings.Contains(name, "free"):
		return "free"
	case strings.Contains(name, "pro"),
		strings.Contains(name, "trial"),
		strings.Contains(name, "team"),
		strings.Contains(name, "enterprise"),
		strings.Contains(name, "ultimate"),
		strings.Contains(name, "max"):
		return "pro"
	default:
		return "free"
	}
}

func resolveWindsurfAllowedModels(planTier string, configs []WindsurfModelConfig, existing []string) []string {
	unique := make(map[string]struct{}, len(configs))
	for _, item := range configs {
		if item.ModelID == "" {
			continue
		}
		unique[item.ModelID] = struct{}{}
	}

	if planTier != "pro" {
		freeModels := []string{"gemini-2.5-flash", "gpt-4o-mini"}
		out := make([]string, 0, len(freeModels))
		for _, model := range freeModels {
			if len(unique) == 0 {
				out = append(out, model)
				continue
			}
			if _, ok := unique[model]; ok {
				out = append(out, model)
			}
		}
		sort.Strings(out)
		return out
	}

	if len(unique) == 0 {
		out := append([]string(nil), existing...)
		sort.Strings(out)
		return out
	}

	out := make([]string, 0, len(unique))
	for model := range unique {
		out = append(out, model)
	}
	sort.Strings(out)
	return out
}

func preferredWindsurfRateLimitReset(status *WindsurfUserStatusSnapshot) *time.Time {
	if status == nil {
		return nil
	}
	if status.DailyResetAt != nil {
		return status.DailyResetAt
	}
	return status.WeeklyResetAt
}

func buildWindsurfProbeWarning(modelErr, rateErr error) string {
	var parts []string
	if modelErr != nil {
		parts = append(parts, "model_catalog: "+modelErr.Error())
	}
	if rateErr != nil {
		parts = append(parts, "rate_limit: "+rateErr.Error())
	}
	return strings.Join(parts, "; ")
}
