package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

func TestWindsurfResponsesFailover_NonStreamSwitchesAccount(t *testing.T) {
	bridge := &windsurfResponsesFailoverBridgeStub{
		completeErrByAccount: map[int64]error{
			61: errors.New("temporary upstream failure"),
		},
		completeResultByAccount: map[int64]*WindsurfBridgeResult{
			62: {
				RequestID: "resp_failover_non_stream",
				Text:      "recovered",
			},
		},
	}
	svc := &WindsurfGatewayService{
		accountRepo: windsurfGatewayAccountRepoStub{
			accounts: []Account{
				newWindsurfResponsesFailoverAccount(61),
				newWindsurfResponsesFailoverAccount(62),
			},
		},
		chatBridge: bridge,
	}

	resp, metadata, err := svc.CompleteResponsesWithMetadata(context.Background(), nil, &apicompat.ResponsesRequest{
		Model: "gpt-4.1",
		Input: []byte(`[{"role":"user","content":"hello"}]`),
	})
	if err != nil {
		t.Fatalf("CompleteResponsesWithMetadata() error = %v", err)
	}
	if len(bridge.completeAttempts) != 2 || bridge.completeAttempts[0] != 61 || bridge.completeAttempts[1] != 62 {
		t.Fatalf("complete attempts = %v, want [61 62]", bridge.completeAttempts)
	}
	if metadata == nil || metadata.Account == nil || metadata.Account.ID != 62 {
		t.Fatal("expected second account metadata after failover")
	}
	if len(resp.Output) == 0 || resp.Output[len(resp.Output)-1].Type != "message" || resp.Output[len(resp.Output)-1].Content[0].Text != "recovered" {
		t.Fatal("expected recovered response output")
	}
}

func TestWindsurfResponsesFailover_StreamSwitchesAccountBeforeFirstEvent(t *testing.T) {
	bridge := &windsurfResponsesFailoverBridgeStub{
		streamErrByAccount: map[int64]error{
			71: errors.New("connect failed"),
		},
		streamChunksByAccount: map[int64][]WindsurfBridgeStreamChunk{
			72: {
				{Text: "hello"},
			},
		},
		streamResultByAccount: map[int64]*WindsurfBridgeResult{
			72: {
				RequestID: "resp_failover_stream",
				Usage: WindsurfBridgeUsage{
					InputTokens:  3,
					OutputTokens: 4,
				},
			},
		},
	}
	svc := &WindsurfGatewayService{
		accountRepo: windsurfGatewayAccountRepoStub{
			accounts: []Account{
				newWindsurfResponsesFailoverAccount(71),
				newWindsurfResponsesFailoverAccount(72),
			},
		},
		chatBridge: bridge,
	}

	var events []apicompat.ResponsesStreamEvent
	metadata, err := svc.StreamResponsesWithMetadata(context.Background(), nil, &apicompat.ResponsesRequest{
		Model:  "gpt-4.1",
		Stream: true,
		Input:  []byte(`[{"role":"user","content":"hello"}]`),
	}, func(event apicompat.ResponsesStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamResponsesWithMetadata() error = %v", err)
	}
	if len(bridge.streamAttempts) != 2 || bridge.streamAttempts[0] != 71 || bridge.streamAttempts[1] != 72 {
		t.Fatalf("stream attempts = %v, want [71 72]", bridge.streamAttempts)
	}
	if metadata == nil || metadata.Account == nil || metadata.Account.ID != 72 {
		t.Fatal("expected second account metadata after stream failover")
	}
	if len(events) == 0 || events[len(events)-1].Type != "response.completed" {
		t.Fatal("expected completed response stream after failover")
	}
}

func newWindsurfResponsesFailoverAccount(id int64) Account {
	return Account{
		ID:       id,
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
}

type windsurfResponsesFailoverBridgeStub struct {
	completeErrByAccount    map[int64]error
	completeResultByAccount map[int64]*WindsurfBridgeResult
	completeAttempts        []int64
	streamErrByAccount      map[int64]error
	streamChunksByAccount   map[int64][]WindsurfBridgeStreamChunk
	streamResultByAccount   map[int64]*WindsurfBridgeResult
	streamAttempts          []int64
}

func (s *windsurfResponsesFailoverBridgeStub) Complete(
	_ context.Context,
	account *Account,
	_ WindsurfResolvedModel,
	_ *apicompat.ChatCompletionsRequest,
) (*WindsurfBridgeResult, error) {
	s.completeAttempts = append(s.completeAttempts, account.ID)
	if err := s.completeErrByAccount[account.ID]; err != nil {
		return nil, err
	}
	return s.completeResultByAccount[account.ID], nil
}

func (s *windsurfResponsesFailoverBridgeStub) Stream(
	_ context.Context,
	account *Account,
	_ WindsurfResolvedModel,
	_ *apicompat.ChatCompletionsRequest,
	emit func(WindsurfBridgeStreamChunk) error,
) (*WindsurfBridgeResult, error) {
	s.streamAttempts = append(s.streamAttempts, account.ID)
	for _, chunk := range s.streamChunksByAccount[account.ID] {
		if err := emit(chunk); err != nil {
			return nil, err
		}
	}
	if err := s.streamErrByAccount[account.ID]; err != nil {
		return nil, err
	}
	return s.streamResultByAccount[account.ID], nil
}
