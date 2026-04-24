//go:build unit

package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

type windsurfAccountTestBridgeStub struct {
	completeAccount *Account
	completeModel   WindsurfResolvedModel
	completeReq     *apicompat.ChatCompletionsRequest
	completeResult  *WindsurfBridgeResult
}

func (s *windsurfAccountTestBridgeStub) Complete(
	_ context.Context,
	account *Account,
	model WindsurfResolvedModel,
	req *apicompat.ChatCompletionsRequest,
) (*WindsurfBridgeResult, error) {
	s.completeAccount = account
	s.completeModel = model
	s.completeReq = req
	return s.completeResult, nil
}

func (s *windsurfAccountTestBridgeStub) Stream(
	_ context.Context,
	_ *Account,
	_ WindsurfResolvedModel,
	_ *apicompat.ChatCompletionsRequest,
	_ func(WindsurfBridgeStreamChunk) error,
) (*WindsurfBridgeResult, error) {
	return nil, nil
}

func TestAccountTestService_WindsurfUsesChatBridgeForTestConnection(t *testing.T) {
	ctx, recorder := newTestContext()
	bridge := &windsurfAccountTestBridgeStub{
		completeResult: &WindsurfBridgeResult{Text: "hello from windsurf"},
	}
	svc := &AccountTestService{windsurfChatBridge: bridge}
	account := &Account{
		ID:       42,
		Platform: PlatformWindsurf,
		Type:     AccountTypeAPIKey,
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
	}

	err := svc.testWindsurfAccountConnection(ctx, account, "gpt-4.1")
	require.NoError(t, err)
	require.NotNil(t, bridge.completeReq)
	require.Equal(t, "gpt-4.1", bridge.completeModel.UpstreamModel)
	require.Equal(t, "MODEL_CHAT_GPT_4_1_2025_04_14", bridge.completeModel.ModelUID)
	require.Equal(t, account.ID, bridge.completeAccount.ID)
	require.Equal(t, "gpt-4.1", bridge.completeReq.Model)
	require.Len(t, bridge.completeReq.Messages, 1)
	require.Equal(t, "user", bridge.completeReq.Messages[0].Role)
	require.JSONEq(t, `"hi"`, string(bridge.completeReq.Messages[0].Content))

	body := recorder.Body.String()
	require.Contains(t, body, `"type":"content","text":"hello from windsurf"`)
	require.Contains(t, body, `"type":"test_complete","success":true`)
	require.NotContains(t, strings.ToLower(body), "plan=pro")
}
