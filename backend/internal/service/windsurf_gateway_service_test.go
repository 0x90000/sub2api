package service

import (
	"context"
	"testing"
)

func TestWindsurfGatewayServiceSelectChatCompletionAccount_UsesMappedModelConfig(t *testing.T) {
	groupID := int64(3)
	svc := &WindsurfGatewayService{
		accountRepo: windsurfGatewayAccountRepoStub{
			accounts: []Account{
				{
					ID:       11,
					Platform: PlatformWindsurf,
					Type:     AccountTypeAPIKey,
					Credentials: map[string]any{
						"model_mapping": map[string]any{
							"gpt-4o": "gpt-4.1",
						},
					},
					Extra: map[string]any{
						"allowed_models": []any{"gpt-4.1"},
						"model_configs": []any{
							map[string]any{
								"id":        "gpt-4.1",
								"model_uid": "MODEL_CHAT_GPT_4_1_2025_04_14",
								"provider":  "MODEL_PROVIDER_OPENAI",
							},
						},
					},
				},
			},
		},
	}

	selection, err := svc.SelectChatCompletionAccount(context.Background(), &groupID, "gpt-4o")
	if err != nil {
		t.Fatalf("SelectChatCompletionAccount() error = %v", err)
	}
	if selection == nil || selection.Account == nil {
		t.Fatal("expected account selection")
	}
	if selection.Account.ID != 11 {
		t.Fatalf("selected account = %d, want %d", selection.Account.ID, 11)
	}
	if selection.Model.UpstreamModel != "gpt-4.1" {
		t.Fatalf("UpstreamModel = %q, want %q", selection.Model.UpstreamModel, "gpt-4.1")
	}
	if selection.Model.ModelUID != "MODEL_CHAT_GPT_4_1_2025_04_14" {
		t.Fatalf("ModelUID = %q, want %q", selection.Model.ModelUID, "MODEL_CHAT_GPT_4_1_2025_04_14")
	}
}

func TestWindsurfGatewayServiceSelectChatCompletionAccount_ResolvesCanonicalAlias(t *testing.T) {
	svc := &WindsurfGatewayService{
		accountRepo: windsurfGatewayAccountRepoStub{
			accounts: []Account{
				{
					ID:       12,
					Platform: PlatformWindsurf,
					Type:     AccountTypeAPIKey,
					Extra: map[string]any{
						"allowed_models": []any{"claude-4-sonnet"},
					},
				},
			},
		},
	}

	selection, err := svc.SelectChatCompletionAccount(context.Background(), nil, "claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("SelectChatCompletionAccount() error = %v", err)
	}
	if selection.Model.UpstreamModel != "claude-4-sonnet" {
		t.Fatalf("UpstreamModel = %q, want %q", selection.Model.UpstreamModel, "claude-4-sonnet")
	}
	if selection.Model.ModelUID != "MODEL_CLAUDE_4_SONNET" {
		t.Fatalf("ModelUID = %q, want %q", selection.Model.ModelUID, "MODEL_CLAUDE_4_SONNET")
	}
	if selection.Model.EnumValue != 281 {
		t.Fatalf("EnumValue = %d, want %d", selection.Model.EnumValue, 281)
	}
}

type windsurfGatewayAccountRepoStub struct {
	accounts []Account
}

func (s windsurfGatewayAccountRepoStub) ListSchedulableByPlatform(context.Context, string) ([]Account, error) {
	return append([]Account(nil), s.accounts...), nil
}

func (s windsurfGatewayAccountRepoStub) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]Account, error) {
	return append([]Account(nil), s.accounts...), nil
}
