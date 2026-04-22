package service

import (
	"reflect"
	"testing"
)

func TestWindsurfAccountFields(t *testing.T) {
	account := &Account{
		Platform: PlatformWindsurf,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"token": "ws-token",
		},
		Extra: map[string]any{
			"allowed_models": []any{" claude-3.5-sonnet ", "", "gpt-4.1"},
			"model_configs": []any{
				map[string]any{
					"id":                "gpt-4.1",
					"model_uid":         "MODEL_CHAT_GPT_4_1_2025_04_14",
					"provider":          "MODEL_PROVIDER_OPENAI",
					"credit_multiplier": 1.5,
				},
			},
			"plan_tier":      "pro",
			"credit_balance": "12.5",
		},
	}

	if !account.IsWindsurf() {
		t.Fatal("expected windsurf account")
	}
	if got := account.GetWindsurfToken(); got != "ws-token" {
		t.Fatalf("GetWindsurfToken() = %q, want %q", got, "ws-token")
	}
	if got := account.GetWindsurfPlanTier(); got != "pro" {
		t.Fatalf("GetWindsurfPlanTier() = %q, want %q", got, "pro")
	}
	if got := account.GetWindsurfCreditBalance(); got != 12.5 {
		t.Fatalf("GetWindsurfCreditBalance() = %v, want %v", got, 12.5)
	}

	wantModels := []string{"claude-3.5-sonnet", "gpt-4.1"}
	if got := account.GetWindsurfAllowedModels(); !reflect.DeepEqual(got, wantModels) {
		t.Fatalf("GetWindsurfAllowedModels() = %v, want %v", got, wantModels)
	}
	gotConfigs := account.GetWindsurfModelConfigs()
	if len(gotConfigs) != 1 {
		t.Fatalf("GetWindsurfModelConfigs() len = %d, want %d", len(gotConfigs), 1)
	}
	if gotConfigs[0].ModelID != "gpt-4.1" || gotConfigs[0].ModelUID != "MODEL_CHAT_GPT_4_1_2025_04_14" {
		t.Fatalf("GetWindsurfModelConfigs()[0] = %+v", gotConfigs[0])
	}
	cfg, ok := account.GetWindsurfModelConfigByID("gpt-4.1")
	if !ok {
		t.Fatal("expected GetWindsurfModelConfigByID to resolve stored config")
	}
	if cfg.CreditMultiplier != 1.5 {
		t.Fatalf("GetWindsurfModelConfigByID().CreditMultiplier = %v, want %v", cfg.CreditMultiplier, 1.5)
	}
}

func TestValidatePlatformAccountType(t *testing.T) {
	if err := validatePlatformAccountType(PlatformWindsurf, AccountTypeAPIKey); err != nil {
		t.Fatalf("validatePlatformAccountType() unexpected err: %v", err)
	}
	if err := validatePlatformAccountType(PlatformWindsurf, AccountTypeOAuth); err == nil {
		t.Fatal("expected windsurf oauth validation error")
	}
	if err := validatePlatformAccountType(PlatformOpenAI, AccountTypeOAuth); err != nil {
		t.Fatalf("openai oauth should remain valid: %v", err)
	}
}
