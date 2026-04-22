package service

import (
	"context"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// WindsurfModelCatalogService aggregates requestable model IDs from
// schedulable Windsurf accounts in the target group.
type WindsurfModelCatalogService struct {
	accountRepo AccountRepository
}

func NewWindsurfModelCatalogService(accountRepo AccountRepository) *WindsurfModelCatalogService {
	return &WindsurfModelCatalogService{accountRepo: accountRepo}
}

func (s *WindsurfModelCatalogService) ListModels(ctx context.Context, groupID *int64) ([]openai.Model, error) {
	if s == nil || s.accountRepo == nil {
		return nil, nil
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

	modelsByID := make(map[string]openai.Model)
	for _, account := range accounts {
		for _, model := range windsurfModelsForAccount(&account) {
			if _, exists := modelsByID[model.ID]; exists {
				continue
			}
			modelsByID[model.ID] = model
		}
	}

	models := make([]openai.Model, 0, len(modelsByID))
	for _, model := range modelsByID {
		models = append(models, model)
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})
	return models, nil
}

func windsurfModelsForAccount(account *Account) []openai.Model {
	if account == nil {
		return nil
	}

	allowedModels := account.GetWindsurfAllowedModels()
	allowedSet := make(map[string]struct{}, len(allowedModels))
	for _, model := range allowedModels {
		if trimmed := strings.TrimSpace(model); trimmed != "" {
			allowedSet[trimmed] = struct{}{}
		}
	}

	mapping := account.GetModelMapping()
	if len(mapping) > 0 {
		models := make([]openai.Model, 0, len(mapping))
		for requestedModel, upstreamModel := range mapping {
			if strings.Contains(requestedModel, "*") {
				continue
			}
			if len(allowedSet) > 0 {
				if _, ok := allowedSet[upstreamModel]; !ok {
					continue
				}
			}
			models = append(models, buildWindsurfCatalogModel(requestedModel, upstreamModel))
		}
		return models
	}

	models := make([]openai.Model, 0, len(allowedSet))
	for _, modelID := range allowedModels {
		if strings.TrimSpace(modelID) == "" {
			continue
		}
		models = append(models, buildWindsurfCatalogModel(modelID, modelID))
	}
	return models
}

func buildWindsurfCatalogModel(modelID, providerHint string) openai.Model {
	return openai.Model{
		ID:          modelID,
		Object:      "model",
		Type:        "model",
		OwnedBy:     inferWindsurfModelProvider(providerHint),
		DisplayName: modelID,
	}
}

func inferWindsurfModelProvider(modelID string) string {
	normalized := strings.ToLower(strings.TrimSpace(modelID))
	switch {
	case strings.HasPrefix(normalized, "gpt"), strings.HasPrefix(normalized, "o1"), strings.HasPrefix(normalized, "o3"), strings.HasPrefix(normalized, "o4"):
		return "openai"
	case strings.HasPrefix(normalized, "claude"):
		return "anthropic"
	case strings.HasPrefix(normalized, "gemini"):
		return "google"
	case strings.HasPrefix(normalized, "grok"):
		return "xai"
	case strings.HasPrefix(normalized, "deepseek"):
		return "deepseek"
	case strings.HasPrefix(normalized, "swe"), strings.HasPrefix(normalized, "arena"), strings.HasPrefix(normalized, "windsurf"):
		return "windsurf"
	default:
		return "unknown"
	}
}
