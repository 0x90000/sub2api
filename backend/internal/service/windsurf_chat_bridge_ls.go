package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protowire"
)

const (
	windsurfLanguageServerServicePath = "/exa.language_server_pb.LanguageServerService"
	windsurfRawGetChatMessagePath     = windsurfLanguageServerServicePath + "/RawGetChatMessage"
	windsurfInitPanelStatePath        = windsurfLanguageServerServicePath + "/InitializeCascadePanelState"
	windsurfAddWorkspacePath          = windsurfLanguageServerServicePath + "/AddTrackedWorkspace"
	windsurfWorkspaceTrustPath        = windsurfLanguageServerServicePath + "/UpdateWorkspaceTrust"
	windsurfStartCascadePath          = windsurfLanguageServerServicePath + "/StartCascade"
	windsurfSendCascadeMessagePath    = windsurfLanguageServerServicePath + "/SendUserCascadeMessage"
	windsurfTrajectoryStepsPath       = windsurfLanguageServerServicePath + "/GetCascadeTrajectorySteps"
	windsurfTrajectoryStatusPath      = windsurfLanguageServerServicePath + "/GetCascadeTrajectory"
	windsurfTrajectoryMetadataPath    = windsurfLanguageServerServicePath + "/GetCascadeTrajectoryGeneratorMetadata"
)

type windsurfProtoField struct {
	Number protowire.Number
	Type   protowire.Type
	Bytes  []byte
	U64    uint64
}

type windsurfRawResponse struct {
	Text    string
	IsError bool
}

type windsurfTrajectoryStep struct {
	Type         uint64
	Status       uint64
	Text         string
	ResponseText string
	Thinking     string
	ErrorText    string
}

func (b *WindsurfChatBridge) completeLegacy(
	ctx context.Context,
	account *Account,
	model WindsurfResolvedModel,
	req *apicompat.ChatCompletionsRequest,
) (*WindsurfBridgeResult, error) {
	var result WindsurfBridgeResult
	_, err := b.streamLegacy(ctx, account, model, req, func(chunk WindsurfBridgeStreamChunk) error {
		result.Text += chunk.Text
		result.Reasoning += chunk.Reasoning
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (b *WindsurfChatBridge) streamLegacy(
	ctx context.Context,
	account *Account,
	model WindsurfResolvedModel,
	req *apicompat.ChatCompletionsRequest,
	emit func(WindsurfBridgeStreamChunk) error,
) (*WindsurfBridgeResult, error) {
	if account == nil || req == nil {
		return nil, ErrWindsurfChatBridgeUnavailable
	}
	token := strings.TrimSpace(account.GetWindsurfToken())
	if token == "" {
		return nil, fmt.Errorf("windsurf token is empty")
	}

	payload := buildWindsurfRawGetChatMessageRequest(token, req.Messages, model.EnumValue, model.UpstreamModel, b.extensionVersion)
	var result WindsurfBridgeResult
	textSanitizer := newWindsurfPathSanitizeStream()
	err := b.grpcStream(ctx, windsurfRawGetChatMessagePath, payload, func(message []byte) error {
		parsed, err := parseWindsurfRawResponse(message)
		if err != nil {
			return err
		}
		if parsed.IsError {
			if parsed.Text == "" {
				return fmt.Errorf("windsurf raw chat returned error")
			}
			return errors.New(strings.TrimSpace(sanitizeWindsurfText(parsed.Text)))
		}
		if parsed.Text == "" {
			return nil
		}
		clean := textSanitizer.Feed(parsed.Text)
		if clean == "" {
			return nil
		}
		result.Text += clean
		if emit != nil {
			return emit(WindsurfBridgeStreamChunk{Text: clean})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if tail := textSanitizer.Flush(); tail != "" {
		result.Text += tail
		if emit != nil {
			if err := emit(WindsurfBridgeStreamChunk{Text: tail}); err != nil {
				return nil, err
			}
		}
	}
	estimateWindsurfUsageFallback(req, &result)
	return &result, nil
}

func (b *WindsurfChatBridge) completeCascade(
	ctx context.Context,
	account *Account,
	model WindsurfResolvedModel,
	req *apicompat.ChatCompletionsRequest,
) (*WindsurfBridgeResult, error) {
	return b.runCascade(ctx, account, model, req, nil)
}

func (b *WindsurfChatBridge) streamCascade(
	ctx context.Context,
	account *Account,
	model WindsurfResolvedModel,
	req *apicompat.ChatCompletionsRequest,
	emit func(WindsurfBridgeStreamChunk) error,
) (*WindsurfBridgeResult, error) {
	return b.runCascade(ctx, account, model, req, emit)
}

func (b *WindsurfChatBridge) runCascade(
	ctx context.Context,
	account *Account,
	model WindsurfResolvedModel,
	req *apicompat.ChatCompletionsRequest,
	emit func(WindsurfBridgeStreamChunk) error,
) (*WindsurfBridgeResult, error) {
	if account == nil || req == nil {
		return nil, ErrWindsurfChatBridgeUnavailable
	}
	token := strings.TrimSpace(account.GetWindsurfToken())
	if token == "" {
		return nil, fmt.Errorf("windsurf token is empty")
	}
	cascadeReq := normalizeWindsurfChatRequestForCascade(req)
	toolPreamble := buildWindsurfToolPreambleForProto(windsurfRequestTools(req), windsurfEffectiveToolChoice(req))
	sessionID, _, err := b.ensureWindsurfCascadeSession(ctx, account, token, false)
	if err != nil {
		return nil, err
	}
	reuse := windsurfConversationReuseFromContext(ctx)
	cascadeID := ""
	if reuse != nil && reuse.Entry != nil && reuse.Entry.AccountID == account.ID {
		if reuse.Entry.EndpointKey == "" || reuse.Entry.EndpointKey == strings.TrimSpace(b.baseURL) {
			sessionID = firstNonEmptyString(strings.TrimSpace(reuse.Entry.SessionID), sessionID)
			cascadeID = strings.TrimSpace(reuse.Entry.CascadeID)
		}
	}
	if cascadeID == "" {
		cascadeIDResp, err := b.grpcUnary(ctx, windsurfStartCascadePath, buildWindsurfStartCascadeRequest(token, sessionID, b.extensionVersion))
		if err != nil {
			return nil, err
		}
		cascadeID, err = parseWindsurfStartCascadeResponse(cascadeIDResp)
		if err != nil {
			return nil, err
		}
		if cascadeID == "" {
			return nil, fmt.Errorf("windsurf start cascade returned empty cascade id")
		}
	}

	inputText, inputImages := buildWindsurfCascadeInput(cascadeReq.Messages, cascadeID != "" && reuse != nil && reuse.Entry != nil)
	sendMessage := func() error {
		_, err := b.grpcUnary(
			ctx,
			windsurfSendCascadeMessagePath,
			buildWindsurfSendCascadeMessageRequest(token, cascadeID, inputText, model.EnumValue, model.ModelUID, sessionID, b.extensionVersion, toolPreamble, inputImages),
		)
		return err
	}
	if err := sendMessage(); err != nil {
		if !windsurfPanelStateMissing(err) {
			return nil, err
		}
		sessionID, _, err = b.ensureWindsurfCascadeSession(ctx, account, token, true)
		if err != nil {
			return nil, err
		}
		cascadeID = ""
		inputText, inputImages = buildWindsurfCascadeInput(cascadeReq.Messages, false)
		cascadeIDResp, startErr := b.grpcUnary(ctx, windsurfStartCascadePath, buildWindsurfStartCascadeRequest(token, sessionID, b.extensionVersion))
		if startErr != nil {
			return nil, startErr
		}
		cascadeID, err = parseWindsurfStartCascadeResponse(cascadeIDResp)
		if err != nil {
			return nil, err
		}
		if cascadeID == "" {
			return nil, fmt.Errorf("windsurf start cascade returned empty cascade id")
		}
		if err := sendMessage(); err != nil {
			return nil, err
		}
	}

	var (
		result            WindsurfBridgeResult
		textCursorByStep  = make(map[int]int)
		thinkCursorByStep = make(map[int]int)
		sawActive         bool
		idleCount         int
	)
	textSanitizer := newWindsurfPathSanitizeStream()
	thinkingSanitizer := newWindsurfPathSanitizeStream()
	flushSanitized := func() error {
		if tail := thinkingSanitizer.Flush(); tail != "" {
			result.Reasoning += tail
			if emit != nil {
				if err := emit(WindsurfBridgeStreamChunk{Reasoning: tail}); err != nil {
					return err
				}
			}
		}
		if tail := textSanitizer.Flush(); tail != "" {
			result.Text += tail
			if emit != nil {
				if err := emit(WindsurfBridgeStreamChunk{Text: tail}); err != nil {
					return err
				}
			}
		}
		return nil
	}

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}

		stepBuf, err := b.grpcUnary(ctx, windsurfTrajectoryStepsPath, buildWindsurfTrajectoryStepsRequest(cascadeID, 0))
		if err != nil {
			return nil, err
		}
		steps, err := parseWindsurfTrajectorySteps(stepBuf)
		if err != nil {
			return nil, err
		}

		for idx, step := range steps {
			if strings.TrimSpace(step.ErrorText) != "" {
				return nil, errors.New(strings.TrimSpace(sanitizeWindsurfText(step.ErrorText)))
			}
			liveThinking := step.Thinking
			if liveThinking != "" {
				prev := thinkCursorByStep[idx]
				if len(liveThinking) > prev {
					delta := liveThinking[prev:]
					thinkCursorByStep[idx] = len(liveThinking)
					clean := thinkingSanitizer.Feed(delta)
					if clean != "" {
						result.Reasoning += clean
						if emit != nil {
							if err := emit(WindsurfBridgeStreamChunk{Reasoning: clean}); err != nil {
								return nil, err
							}
						}
					}
				}
			}

			liveText := step.ResponseText
			if liveText == "" {
				liveText = step.Text
			}
			if liveText == "" {
				continue
			}
			prev := textCursorByStep[idx]
			if len(liveText) > prev {
				delta := liveText[prev:]
				textCursorByStep[idx] = len(liveText)
				clean := textSanitizer.Feed(delta)
				if clean != "" {
					result.Text += clean
					if emit != nil {
						if err := emit(WindsurfBridgeStreamChunk{Text: clean}); err != nil {
							return nil, err
						}
					}
				}
			}
		}

		statusBuf, err := b.grpcUnary(ctx, windsurfTrajectoryStatusPath, buildWindsurfTrajectoryStatusRequest(cascadeID))
		if err != nil {
			return nil, err
		}
		status, err := parseWindsurfTrajectoryStatus(statusBuf)
		if err != nil {
			return nil, err
		}
		if status != 1 {
			sawActive = true
			idleCount = 0
			continue
		}
		if sawActive {
			idleCount++
			if idleCount >= 2 {
				if err := flushSanitized(); err != nil {
					return nil, err
				}
				estimateWindsurfUsageFallback(req, &result)
				if usage, ok := b.fetchWindsurfGeneratorMetadata(ctx, cascadeID); ok {
					result.Usage = usage
				}
				result.conversation = &windsurfConversationResult{
					CascadeID:   cascadeID,
					SessionID:   sessionID,
					EndpointKey: strings.TrimSpace(b.baseURL),
				}
				return &result, nil
			}
		}
	}

	if err := flushSanitized(); err != nil {
		return nil, err
	}
	if result.Text == "" && result.Reasoning == "" {
		return nil, fmt.Errorf("windsurf cascade request timed out")
	}
	estimateWindsurfUsageFallback(req, &result)
	if usage, ok := b.fetchWindsurfGeneratorMetadata(ctx, cascadeID); ok {
		result.Usage = usage
	}
	result.conversation = &windsurfConversationResult{
		CascadeID:   cascadeID,
		SessionID:   sessionID,
		EndpointKey: strings.TrimSpace(b.baseURL),
	}
	return &result, nil
}

type windsurfCascadeSessionState struct {
	mu          sync.Mutex
	sessionID   string
	workspace   string
	initialized bool
}

func (b *WindsurfChatBridge) ensureWindsurfCascadeSession(ctx context.Context, account *Account, apiKey string, force bool) (string, string, error) {
	if b == nil {
		return "", "", ErrWindsurfChatBridgeUnavailable
	}
	state := b.loadWindsurfCascadeSessionState(account, apiKey)
	state.mu.Lock()
	defer state.mu.Unlock()

	if force {
		state.initialized = false
		state.sessionID = ""
	}
	if strings.TrimSpace(state.sessionID) == "" {
		state.sessionID = uuid.NewString()
	}
	if strings.TrimSpace(state.workspace) == "" {
		state.workspace = resolveWindsurfCascadeWorkspacePath(account, apiKey, b.workspaceDir)
	}
	if state.initialized {
		return state.sessionID, state.workspace, nil
	}

	_, _ = b.grpcUnary(ctx, windsurfInitPanelStatePath, buildWindsurfInitializePanelStateRequest(apiKey, state.sessionID, b.extensionVersion))
	_, _ = b.grpcUnary(ctx, windsurfAddWorkspacePath, buildWindsurfAddTrackedWorkspaceRequest(state.workspace))
	_, _ = b.grpcUnary(ctx, windsurfWorkspaceTrustPath, buildWindsurfUpdateWorkspaceTrustRequest(apiKey, state.sessionID, true, b.extensionVersion))

	state.initialized = true
	return state.sessionID, state.workspace, nil
}

func (b *WindsurfChatBridge) loadWindsurfCascadeSessionState(account *Account, apiKey string) *windsurfCascadeSessionState {
	key := windsurfCascadeSessionKey(account, apiKey)
	if existing, ok := b.cascadeSessions.Load(key); ok {
		if state, ok := existing.(*windsurfCascadeSessionState); ok && state != nil {
			return state
		}
	}
	state := &windsurfCascadeSessionState{}
	actual, _ := b.cascadeSessions.LoadOrStore(key, state)
	if loaded, ok := actual.(*windsurfCascadeSessionState); ok && loaded != nil {
		return loaded
	}
	return state
}

func windsurfCascadeSessionKey(account *Account, apiKey string) string {
	if account != nil && account.ID > 0 {
		return fmt.Sprintf("acct:%d", account.ID)
	}
	token := strings.TrimSpace(apiKey)
	if token == "" {
		return "acct:anonymous"
	}
	if len(token) > 12 {
		token = token[:12]
	}
	return "token:" + token
}

func resolveWindsurfCascadeWorkspacePath(account *Account, apiKey string, configured string) string {
	suffix := windsurfCascadeWorkspaceSuffix(account, apiKey)
	base := strings.TrimSpace(configured)
	if base == "" {
		return "/home/user/projects/workspace-" + suffix
	}
	base = filepath.ToSlash(base)
	base = strings.TrimRight(base, "/")
	lowerBase := strings.ToLower(filepath.Base(base))
	if strings.HasPrefix(lowerBase, "workspace-") && lowerBase != "windsurf-workspace" {
		return base
	}
	return base + "/workspace-" + suffix
}

func windsurfCascadeWorkspaceSuffix(account *Account, apiKey string) string {
	if account != nil && account.ID > 0 {
		return fmt.Sprintf("account-%d", account.ID)
	}
	return sanitizeWindsurfWorkspaceSuffix(apiKey)
}

func sanitizeWindsurfWorkspaceSuffix(apiKey string) string {
	token := strings.TrimSpace(apiKey)
	if len(token) > 8 {
		token = token[:8]
	}
	if token == "" {
		token = "default"
	}
	var builder strings.Builder
	for _, ch := range token {
		switch {
		case ch == '-' || ch == '_' || ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z':
			builder.WriteRune(ch)
		default:
			builder.WriteByte('x')
		}
	}
	if builder.Len() == 0 {
		return "default"
	}
	return strings.ToLower(builder.String())
}

func windsurfPanelStateMissing(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "panel state not found") || (strings.Contains(msg, "not_found") && strings.Contains(msg, "panel"))
}

func buildWindsurfCascadeInput(messages []apicompat.ChatMessage, resume bool) (string, []string) {
	systemMessages := make([]string, 0, len(messages))
	conversation := make([]apicompat.ChatMessage, 0, len(messages))
	for _, msg := range messages {
		switch strings.TrimSpace(strings.ToLower(msg.Role)) {
		case "system":
			text := strings.TrimSpace(windsurfChatContentToString(msg.Content))
			if text != "" {
				systemMessages = append(systemMessages, text)
			}
		case "user", "assistant":
			conversation = append(conversation, msg)
		}
	}

	systemPrefix := strings.Join(systemMessages, "\n\n")
	if resume && len(conversation) > 0 {
		last := conversation[len(conversation)-1]
		text, images := extractWindsurfCascadeMessage(last.Content)
		if systemPrefix != "" {
			text = systemPrefix + "\n\n" + text
		}
		return text, images
	}

	text := buildWindsurfCascadeInputText(messages)
	return text, nil
}

func extractWindsurfCascadeMessage(raw json.RawMessage) (string, []string) {
	if len(raw) == 0 {
		return "", nil
	}
	var plain string
	if err := json.Unmarshal(raw, &plain); err == nil {
		return plain, nil
	}
	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		lines := make([]string, 0, len(parts))
		images := make([]string, 0, len(parts))
		for _, part := range parts {
			switch strings.TrimSpace(strings.ToLower(part.Type)) {
			case "text":
				if strings.TrimSpace(part.Text) != "" {
					lines = append(lines, part.Text)
				}
			case "image_url":
				if part.ImageURL != nil && strings.TrimSpace(part.ImageURL.URL) != "" {
					images = append(images, strings.TrimSpace(part.ImageURL.URL))
				}
			}
		}
		return strings.Join(lines, "\n"), images
	}
	return strings.TrimSpace(string(raw)), nil
}

func (b *WindsurfChatBridge) fetchWindsurfGeneratorMetadata(ctx context.Context, cascadeID string) (WindsurfBridgeUsage, bool) {
	cascadeID = strings.TrimSpace(cascadeID)
	if cascadeID == "" {
		return WindsurfBridgeUsage{}, false
	}
	buf, err := b.grpcUnary(ctx, windsurfTrajectoryMetadataPath, buildWindsurfGeneratorMetadataRequest(cascadeID, 0))
	if err != nil {
		return WindsurfBridgeUsage{}, false
	}
	usage, err := parseWindsurfGeneratorMetadata(buf)
	if err != nil {
		return WindsurfBridgeUsage{}, false
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.CacheReadTokens == 0 && usage.CacheCreationTokens == 0 {
		return WindsurfBridgeUsage{}, false
	}
	return usage, true
}

func estimateWindsurfUsageFallback(req *apicompat.ChatCompletionsRequest, result *WindsurfBridgeResult) {
	if req == nil || result == nil {
		return
	}
	if result.Usage.InputTokens > 0 ||
		result.Usage.OutputTokens > 0 ||
		result.Usage.CacheCreationTokens > 0 ||
		result.Usage.CacheReadTokens > 0 ||
		result.Usage.ImageOutputTokens > 0 {
		return
	}

	inputBuilder := strings.Builder{}
	for _, msg := range req.Messages {
		text := strings.TrimSpace(windsurfChatContentToString(msg.Content))
		if text != "" {
			if inputBuilder.Len() > 0 {
				inputBuilder.WriteString("\n")
			}
			inputBuilder.WriteString(text)
		}
		for _, call := range msg.ToolCalls {
			name := strings.TrimSpace(call.Function.Name)
			args := strings.TrimSpace(call.Function.Arguments)
			if name == "" && args == "" {
				continue
			}
			if inputBuilder.Len() > 0 {
				inputBuilder.WriteString("\n")
			}
			if name != "" {
				inputBuilder.WriteString(name)
			}
			if args != "" {
				if name != "" {
					inputBuilder.WriteString(" ")
				}
				inputBuilder.WriteString(args)
			}
		}
	}

	result.Usage.InputTokens = estimateTokensForText(inputBuilder.String())
	result.Usage.OutputTokens = estimateTokensForText(result.Text) + estimateTokensForText(result.Reasoning)
}

func (b *WindsurfChatBridge) grpcUnary(ctx context.Context, rpcPath string, payload []byte) ([]byte, error) {
	resp, err := b.doGRPCRequest(ctx, rpcPath, payload)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read windsurf grpc response: %w", err)
	}
	if err := windsurfCheckGRPCStatus(resp.Trailer); err != nil {
		return nil, err
	}
	frames, err := extractWindsurfGRPCFrames(body)
	if err != nil {
		return nil, err
	}
	if len(frames) == 0 {
		return nil, nil
	}
	return bytes.Join(frames, nil), nil
}

func (b *WindsurfChatBridge) grpcStream(ctx context.Context, rpcPath string, payload []byte, handle func([]byte) error) error {
	resp, err := b.doGRPCRequest(ctx, rpcPath, payload)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	buffer := make([]byte, 0, 16*1024)
	temp := make([]byte, 8*1024)
	for {
		n, readErr := resp.Body.Read(temp)
		if n > 0 {
			buffer = append(buffer, temp[:n]...)
			for {
				frame, rest, ok, err := consumeWindsurfGRPCFrame(buffer)
				if err != nil {
					return err
				}
				if !ok {
					break
				}
				buffer = rest
				if handle != nil {
					if err := handle(frame); err != nil {
						return err
					}
				}
			}
		}
		if readErr == io.EOF {
			if len(buffer) > 0 {
				frame, _, ok, err := consumeWindsurfGRPCFrame(buffer)
				if err != nil {
					return err
				}
				if ok && handle != nil {
					if err := handle(frame); err != nil {
						return err
					}
				}
			}
			break
		}
		if readErr != nil {
			return fmt.Errorf("read windsurf grpc stream: %w", readErr)
		}
	}
	return windsurfCheckGRPCStatus(resp.Trailer)
}

func (b *WindsurfChatBridge) doGRPCRequest(ctx context.Context, rpcPath string, payload []byte) (*http.Response, error) {
	if b == nil || b.httpClient == nil {
		return nil, ErrWindsurfChatBridgeUnavailable
	}
	body := frameWindsurfGRPCMessage(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+rpcPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build windsurf grpc request: %w", err)
	}
	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("TE", "trailers")
	req.Header.Set("X-Codeium-Csrf-Token", b.csrfToken)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, b.formatGRPCRequestError(rpcPath, err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer func() { _ = resp.Body.Close() }()
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("windsurf grpc %s returned %d: %s", rpcPath, resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	return resp, nil
}

func (b *WindsurfChatBridge) formatGRPCRequestError(rpcPath string, err error) error {
	if err == nil {
		return nil
	}
	baseURL := ""
	if b != nil {
		baseURL = strings.TrimSpace(b.baseURL)
	}
	if isWindsurfLocalLSUnavailable(baseURL, err) {
		return fmt.Errorf(
			"call windsurf grpc %s: Windsurf language server is not running at %s. Install or mount the LS binary, make sure it is started, or set WINDSURF_LS_ADDR to a reachable server: %w",
			rpcPath,
			firstNonEmptyString(baseURL, "http://127.0.0.1:42100"),
			err,
		)
	}
	return fmt.Errorf("call windsurf grpc %s: %w", rpcPath, err)
}

func isWindsurfLocalLSUnavailable(baseURL string, err error) bool {
	if err == nil {
		return false
	}
	if !windsurfTargetsLoopback(baseURL) {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "actively refused") ||
		strings.Contains(msg, "connectex") ||
		strings.Contains(msg, "cannot assign requested address")
}

func windsurfTargetsLoopback(baseURL string) bool {
	if strings.TrimSpace(baseURL) == "" {
		return true
	}
	parsed, err := neturl.Parse(baseURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

func buildWindsurfRawGetChatMessageRequest(apiKey string, messages []apicompat.ChatMessage, modelEnum int, modelName string, version string) []byte {
	conversationID := uuid.NewString()
	out := make([]byte, 0, 1024)
	out = appendProtoMessageField(out, 1, buildWindsurfMetadata(apiKey, uuid.NewString(), version))

	var systemPrompt strings.Builder
	for _, msg := range messages {
		if msg.Role == "system" {
			if systemPrompt.Len() > 0 {
				systemPrompt.WriteString("\n")
			}
			systemPrompt.WriteString(windsurfChatContentToString(msg.Content))
			continue
		}
		source, text := windsurfNormalizeLegacyMessage(msg)
		out = appendProtoMessageField(out, 2, buildWindsurfLegacyChatMessage(text, source, conversationID))
	}

	if systemPrompt.Len() > 0 {
		out = appendProtoStringField(out, 3, systemPrompt.String())
	}
	out = appendProtoVarintField(out, 4, uint64(modelEnum))
	if strings.TrimSpace(modelName) != "" {
		out = appendProtoStringField(out, 5, modelName)
	}
	return out
}

func buildWindsurfLegacyChatMessage(text string, source uint64, conversationID string) []byte {
	out := make([]byte, 0, 256)
	out = appendProtoStringField(out, 1, uuid.NewString())
	out = appendProtoVarintField(out, 2, source)
	out = appendProtoMessageField(out, 3, buildWindsurfTimestamp())
	out = appendProtoStringField(out, 4, conversationID)

	generic := appendProtoStringField(nil, 1, text)
	if source == 3 {
		action := appendProtoMessageField(nil, 1, generic)
		out = appendProtoMessageField(out, 6, action)
		return out
	}
	intent := appendProtoMessageField(nil, 1, generic)
	out = appendProtoMessageField(out, 5, intent)
	return out
}

func buildWindsurfInitializePanelStateRequest(apiKey, sessionID, version string) []byte {
	out := appendProtoMessageField(nil, 1, buildWindsurfMetadata(apiKey, sessionID, version))
	out = appendProtoBoolField(out, 3, true)
	return out
}

func buildWindsurfAddTrackedWorkspaceRequest(workspacePath string) []byte {
	return appendProtoStringField(nil, 1, workspacePath)
}

func buildWindsurfUpdateWorkspaceTrustRequest(apiKey, sessionID string, trusted bool, version string) []byte {
	out := appendProtoMessageField(nil, 1, buildWindsurfMetadata(apiKey, sessionID, version))
	out = appendProtoBoolField(out, 2, trusted)
	return out
}

func buildWindsurfStartCascadeRequest(apiKey, sessionID, version string) []byte {
	out := appendProtoMessageField(nil, 1, buildWindsurfMetadata(apiKey, sessionID, version))
	out = appendProtoVarintField(out, 4, 1)
	out = appendProtoVarintField(out, 5, 1)
	return out
}

func buildWindsurfSendCascadeMessageRequest(apiKey, cascadeID, text string, modelEnum int, modelUID, sessionID, version string, toolPreamble string, images []string) []byte {
	out := appendProtoStringField(nil, 1, cascadeID)
	out = appendProtoMessageField(out, 2, appendProtoStringField(nil, 1, text))
	out = appendProtoMessageField(out, 3, buildWindsurfMetadata(apiKey, sessionID, version))
	for _, image := range images {
		if strings.TrimSpace(image) == "" {
			continue
		}
		out = appendProtoStringField(out, 6, strings.TrimSpace(image))
	}
	out = appendProtoMessageField(out, 5, buildWindsurfCascadeConfig(modelEnum, modelUID, toolPreamble))
	return out
}

func buildWindsurfCascadeConfig(modelEnum int, modelUID string, toolPreamble string) []byte {
	conversation := appendProtoVarintField(nil, 4, 3)
	if strings.TrimSpace(toolPreamble) != "" {
		additional := appendProtoVarintField(nil, 1, 1)
		additional = appendProtoStringField(additional, 2, toolPreamble+"\n\nThe functions listed above are available and callable. When a function is relevant, emit the exact <tool_call> JSON block.")
		conversation = appendProtoMessageField(conversation, 12, additional)

		toolSection := appendProtoVarintField(nil, 1, 1)
		toolSection = appendProtoStringField(toolSection, 2, toolPreamble)
		conversation = appendProtoMessageField(conversation, 10, toolSection)

		communication := appendProtoVarintField(nil, 1, 1)
		communication = appendProtoStringField(communication, 2, "You are accessed via API. Respond in the same language as the user. Use the functions above when relevant.")
		conversation = appendProtoMessageField(conversation, 13, communication)
	} else {
		noToolSection := appendProtoVarintField(nil, 1, 1)
		noToolSection = appendProtoStringField(noToolSection, 2, "No tools are available.")
		conversation = appendProtoMessageField(conversation, 10, noToolSection)

		additional := appendProtoVarintField(nil, 1, 1)
		additional = appendProtoStringField(additional, 2, "You have no tools, no file access, and no command execution. Answer all questions directly using your knowledge.")
		conversation = appendProtoMessageField(conversation, 12, additional)

		communication := appendProtoVarintField(nil, 1, 1)
		communication = appendProtoStringField(communication, 2, "You are accessed via API, not inside an IDE. Answer directly and never reveal server infrastructure details.")
		conversation = appendProtoMessageField(conversation, 13, communication)
	}

	planner := appendProtoMessageField(nil, 2, conversation)
	if strings.TrimSpace(modelUID) != "" {
		planner = appendProtoStringField(planner, 35, modelUID)
		planner = appendProtoStringField(planner, 34, modelUID)
	}
	if modelEnum > 0 {
		planner = appendProtoMessageField(planner, 15, appendProtoVarintField(nil, 1, uint64(modelEnum)))
		planner = appendProtoVarintField(planner, 1, uint64(modelEnum))
	}
	planner = appendProtoVarintField(planner, 6, 32768)

	brain := appendProtoBoolField(nil, 1, true)
	brain = appendProtoMessageField(brain, 6, appendProtoMessageField(nil, 6, nil))
	memory := appendProtoBoolField(nil, 1, false)

	out := appendProtoMessageField(nil, 1, planner)
	out = appendProtoMessageField(out, 5, memory)
	out = appendProtoMessageField(out, 7, brain)
	return out
}

func buildWindsurfTrajectoryStepsRequest(cascadeID string, offset int) []byte {
	out := appendProtoStringField(nil, 1, cascadeID)
	if offset > 0 {
		out = appendProtoVarintField(out, 2, uint64(offset))
	}
	return out
}

func buildWindsurfTrajectoryStatusRequest(cascadeID string) []byte {
	return appendProtoStringField(nil, 1, cascadeID)
}

func buildWindsurfGeneratorMetadataRequest(cascadeID string, offset int) []byte {
	out := appendProtoStringField(nil, 1, cascadeID)
	if offset > 0 {
		out = appendProtoVarintField(out, 2, uint64(offset))
	}
	return out
}

func buildWindsurfMetadata(apiKey, sessionID, version string) []byte {
	out := make([]byte, 0, 256)
	out = appendProtoStringField(out, 1, "windsurf")
	out = appendProtoStringField(out, 2, version)
	out = appendProtoStringField(out, 3, apiKey)
	out = appendProtoStringField(out, 4, "en")
	out = appendProtoStringField(out, 5, runtime.GOOS)
	out = appendProtoStringField(out, 7, version)
	out = appendProtoStringField(out, 8, runtime.GOARCH)
	out = appendProtoVarintField(out, 9, uint64(time.Now().UnixMilli()))
	out = appendProtoStringField(out, 10, sessionID)
	out = appendProtoStringField(out, 12, "windsurf")
	return out
}

func buildWindsurfTimestamp() []byte {
	now := time.Now()
	out := appendProtoVarintField(nil, 1, uint64(now.Unix()))
	nanos := uint64(now.Nanosecond())
	if nanos > 0 {
		out = appendProtoVarintField(out, 2, nanos)
	}
	return out
}

func buildWindsurfCascadeInputText(messages []apicompat.ChatMessage) string {
	var (
		systemLines []string
		convoLines  []string
	)
	for _, msg := range messages {
		text := windsurfChatContentToString(msg.Content)
		switch msg.Role {
		case "system":
			if strings.TrimSpace(text) != "" {
				systemLines = append(systemLines, text)
			}
		case "user":
			convoLines = append(convoLines, "<human>\n"+text+"\n</human>")
		case "assistant":
			convoLines = append(convoLines, "<assistant>\n"+text+"\n</assistant>")
		}
	}

	var text string
	switch len(convoLines) {
	case 0:
		text = strings.Join(systemLines, "\n\n")
	case 1:
		text = convoLines[0]
	default:
		text = "The following is a multi-turn conversation. You MUST remember and use all information from prior turns.\n\n" + strings.Join(convoLines, "\n\n")
	}
	if len(systemLines) == 0 {
		return text
	}
	if strings.TrimSpace(text) == "" {
		return strings.Join(systemLines, "\n\n")
	}
	return strings.Join(systemLines, "\n\n") + "\n\n" + text
}

func windsurfNormalizeLegacyMessage(msg apicompat.ChatMessage) (source uint64, text string) {
	baseText := windsurfChatContentToString(msg.Content)
	switch msg.Role {
	case "assistant":
		source = 3
		text = baseText
		if len(msg.ToolCalls) > 0 {
			lines := make([]string, 0, len(msg.ToolCalls))
			for _, call := range msg.ToolCalls {
				lines = append(lines, fmt.Sprintf("[called tool %s with %s]", firstNonEmptyString(call.Function.Name, "unknown"), firstNonEmptyString(call.Function.Arguments, "{}")))
			}
			if text != "" {
				text += "\n"
			}
			text += strings.Join(lines, "\n")
		}
	case "tool":
		source = 1
		text = "[tool result"
		if msg.ToolCallID != "" {
			text += " for " + msg.ToolCallID
		}
		text += "]: " + baseText
	case "system":
		source = 2
		text = baseText
	default:
		source = 1
		text = baseText
	}
	return source, text
}

func windsurfChatContentToString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var plain string
	if err := json.Unmarshal(raw, &plain); err == nil {
		return plain
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		lines := make([]string, 0, len(parts))
		for _, part := range parts {
			if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
				lines = append(lines, part.Text)
			}
		}
		if len(lines) > 0 {
			return strings.Join(lines, "\n")
		}
	}
	return strings.TrimSpace(string(raw))
}

func parseWindsurfRawResponse(buf []byte) (*windsurfRawResponse, error) {
	fields, err := parseWindsurfProtoFields(buf)
	if err != nil {
		return nil, err
	}
	outer := findWindsurfProtoField(fields, 1, protowire.BytesType)
	if outer == nil {
		return &windsurfRawResponse{}, nil
	}
	innerFields, err := parseWindsurfProtoFields(outer.Bytes)
	if err != nil {
		return nil, err
	}
	text := ""
	if field := findWindsurfProtoField(innerFields, 5, protowire.BytesType); field != nil {
		text = string(field.Bytes)
	}
	isError := false
	if field := findWindsurfProtoField(innerFields, 7, protowire.VarintType); field != nil {
		isError = field.U64 == 1
	}
	return &windsurfRawResponse{Text: text, IsError: isError}, nil
}

func parseWindsurfStartCascadeResponse(buf []byte) (string, error) {
	fields, err := parseWindsurfProtoFields(buf)
	if err != nil {
		return "", err
	}
	if field := findWindsurfProtoField(fields, 1, protowire.BytesType); field != nil {
		return string(field.Bytes), nil
	}
	return "", nil
}

func parseWindsurfTrajectoryStatus(buf []byte) (uint64, error) {
	fields, err := parseWindsurfProtoFields(buf)
	if err != nil {
		return 0, err
	}
	if field := findWindsurfProtoField(fields, 2, protowire.VarintType); field != nil {
		return field.U64, nil
	}
	return 0, nil
}

func parseWindsurfGeneratorMetadata(buf []byte) (WindsurfBridgeUsage, error) {
	fields, err := parseWindsurfProtoFields(buf)
	if err != nil {
		return WindsurfBridgeUsage{}, err
	}
	entries := findAllWindsurfProtoFields(fields, 1, protowire.BytesType)
	var usage WindsurfBridgeUsage
	for _, entry := range entries {
		entryFields, err := parseWindsurfProtoFields(entry.Bytes)
		if err != nil {
			return WindsurfBridgeUsage{}, err
		}
		chatModel := findWindsurfProtoField(entryFields, 1, protowire.BytesType)
		if chatModel == nil {
			continue
		}
		chatModelFields, err := parseWindsurfProtoFields(chatModel.Bytes)
		if err != nil {
			return WindsurfBridgeUsage{}, err
		}
		usageField := findWindsurfProtoField(chatModelFields, 4, protowire.BytesType)
		if usageField == nil {
			continue
		}
		usageFields, err := parseWindsurfProtoFields(usageField.Bytes)
		if err != nil {
			return WindsurfBridgeUsage{}, err
		}
		if field := findWindsurfProtoField(usageFields, 2, protowire.VarintType); field != nil {
			usage.InputTokens += int(field.U64)
		}
		if field := findWindsurfProtoField(usageFields, 3, protowire.VarintType); field != nil {
			usage.OutputTokens += int(field.U64)
		}
		if field := findWindsurfProtoField(usageFields, 4, protowire.VarintType); field != nil {
			usage.CacheCreationTokens += int(field.U64)
		}
		if field := findWindsurfProtoField(usageFields, 5, protowire.VarintType); field != nil {
			usage.CacheReadTokens += int(field.U64)
		}
	}
	return usage, nil
}

func parseWindsurfTrajectorySteps(buf []byte) ([]windsurfTrajectoryStep, error) {
	fields, err := parseWindsurfProtoFields(buf)
	if err != nil {
		return nil, err
	}
	stepFields := findAllWindsurfProtoFields(fields, 1, protowire.BytesType)
	result := make([]windsurfTrajectoryStep, 0, len(stepFields))
	for _, stepField := range stepFields {
		stepFields, err := parseWindsurfProtoFields(stepField.Bytes)
		if err != nil {
			return nil, err
		}
		step := windsurfTrajectoryStep{}
		if field := findWindsurfProtoField(stepFields, 1, protowire.VarintType); field != nil {
			step.Type = field.U64
		}
		if field := findWindsurfProtoField(stepFields, 4, protowire.VarintType); field != nil {
			step.Status = field.U64
		}
		if field := findWindsurfProtoField(stepFields, 20, protowire.BytesType); field != nil {
			plannerFields, err := parseWindsurfProtoFields(field.Bytes)
			if err != nil {
				return nil, err
			}
			if textField := findWindsurfProtoField(plannerFields, 1, protowire.BytesType); textField != nil {
				step.ResponseText = string(textField.Bytes)
			}
			if modField := findWindsurfProtoField(plannerFields, 8, protowire.BytesType); modField != nil {
				step.Text = string(modField.Bytes)
			}
			if step.Text == "" {
				step.Text = step.ResponseText
			}
			if thinkField := findWindsurfProtoField(plannerFields, 3, protowire.BytesType); thinkField != nil {
				step.Thinking = string(thinkField.Bytes)
			}
		}
		if step.ErrorText == "" {
			if field := findWindsurfProtoField(stepFields, 24, protowire.BytesType); field != nil {
				step.ErrorText = extractWindsurfErrorText(field.Bytes, true)
			}
		}
		if step.ErrorText == "" {
			if field := findWindsurfProtoField(stepFields, 31, protowire.BytesType); field != nil {
				step.ErrorText = extractWindsurfErrorText(field.Bytes, false)
			}
		}
		result = append(result, step)
	}
	return result, nil
}

func extractWindsurfErrorText(buf []byte, nested bool) string {
	fields, err := parseWindsurfProtoFields(buf)
	if err != nil {
		return ""
	}
	if nested {
		if detailField := findWindsurfProtoField(fields, 3, protowire.BytesType); detailField != nil {
			return extractWindsurfErrorDetails(detailField.Bytes)
		}
	}
	return extractWindsurfErrorDetails(buf)
}

func extractWindsurfErrorDetails(buf []byte) string {
	fields, err := parseWindsurfProtoFields(buf)
	if err != nil {
		return ""
	}
	for _, number := range []protowire.Number{1, 2, 3} {
		if field := findWindsurfProtoField(fields, number, protowire.BytesType); field != nil {
			text := strings.TrimSpace(string(field.Bytes))
			if text != "" {
				lines := strings.Split(text, "\n")
				return strings.TrimSpace(lines[0])
			}
		}
	}
	return ""
}

func frameWindsurfGRPCMessage(payload []byte) []byte {
	frame := make([]byte, 5+len(payload))
	frame[0] = 0
	size := uint32(len(payload))
	frame[1] = byte(size >> 24)
	frame[2] = byte(size >> 16)
	frame[3] = byte(size >> 8)
	frame[4] = byte(size)
	copy(frame[5:], payload)
	return frame
}

func consumeWindsurfGRPCFrame(buf []byte) ([]byte, []byte, bool, error) {
	if len(buf) < 5 {
		return nil, buf, false, nil
	}
	if buf[0] != 0 {
		return nil, nil, false, fmt.Errorf("windsurf grpc compression is not supported")
	}
	size := int(buf[1])<<24 | int(buf[2])<<16 | int(buf[3])<<8 | int(buf[4])
	if len(buf) < 5+size {
		return nil, buf, false, nil
	}
	return buf[5 : 5+size], buf[5+size:], true, nil
}

func extractWindsurfGRPCFrames(buf []byte) ([][]byte, error) {
	var frames [][]byte
	for len(buf) > 0 {
		frame, rest, ok, err := consumeWindsurfGRPCFrame(buf)
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		frames = append(frames, append([]byte(nil), frame...))
		buf = rest
	}
	return frames, nil
}

func windsurfCheckGRPCStatus(trailer http.Header) error {
	if trailer == nil {
		return nil
	}
	status := strings.TrimSpace(trailer.Get("grpc-status"))
	if status == "" || status == "0" {
		return nil
	}
	message := strings.TrimSpace(trailer.Get("grpc-message"))
	if message == "" {
		return fmt.Errorf("windsurf grpc status %s", status)
	}
	decoded, err := neturl.QueryUnescape(message)
	if err == nil {
		message = decoded
	}
	return errors.New(message)
}

func parseWindsurfProtoFields(buf []byte) ([]windsurfProtoField, error) {
	fields := make([]windsurfProtoField, 0, 8)
	for len(buf) > 0 {
		number, fieldType, n := protowire.ConsumeTag(buf)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		buf = buf[n:]
		field := windsurfProtoField{Number: number, Type: fieldType}

		switch fieldType {
		case protowire.VarintType:
			value, m := protowire.ConsumeVarint(buf)
			if m < 0 {
				return nil, protowire.ParseError(m)
			}
			field.U64 = value
			buf = buf[m:]
		case protowire.BytesType:
			value, m := protowire.ConsumeBytes(buf)
			if m < 0 {
				return nil, protowire.ParseError(m)
			}
			field.Bytes = append([]byte(nil), value...)
			buf = buf[m:]
		case protowire.Fixed32Type:
			value, m := protowire.ConsumeFixed32(buf)
			if m < 0 {
				return nil, protowire.ParseError(m)
			}
			field.U64 = uint64(value)
			buf = buf[m:]
		case protowire.Fixed64Type:
			value, m := protowire.ConsumeFixed64(buf)
			if m < 0 {
				return nil, protowire.ParseError(m)
			}
			field.U64 = value
			buf = buf[m:]
		default:
			return nil, fmt.Errorf("unsupported windsurf proto wire type %d", fieldType)
		}

		fields = append(fields, field)
	}
	return fields, nil
}

func findWindsurfProtoField(fields []windsurfProtoField, number protowire.Number, fieldType protowire.Type) *windsurfProtoField {
	for i := range fields {
		if fields[i].Number == number && fields[i].Type == fieldType {
			return &fields[i]
		}
	}
	return nil
}

func findAllWindsurfProtoFields(fields []windsurfProtoField, number protowire.Number, fieldType protowire.Type) []windsurfProtoField {
	var out []windsurfProtoField
	for _, field := range fields {
		if field.Number == number && field.Type == fieldType {
			out = append(out, field)
		}
	}
	return out
}

func appendProtoVarintField(buf []byte, number protowire.Number, value uint64) []byte {
	buf = protowire.AppendTag(buf, number, protowire.VarintType)
	return protowire.AppendVarint(buf, value)
}

func appendProtoStringField(buf []byte, number protowire.Number, value string) []byte {
	if value == "" {
		return buf
	}
	buf = protowire.AppendTag(buf, number, protowire.BytesType)
	return protowire.AppendString(buf, value)
}

func appendProtoMessageField(buf []byte, number protowire.Number, payload []byte) []byte {
	if len(payload) == 0 {
		return buf
	}
	buf = protowire.AppendTag(buf, number, protowire.BytesType)
	return protowire.AppendBytes(buf, payload)
}

func appendProtoBoolField(buf []byte, number protowire.Number, value bool) []byte {
	if !value {
		return buf
	}
	return appendProtoVarintField(buf, number, 1)
}
