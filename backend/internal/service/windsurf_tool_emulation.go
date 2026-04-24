package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

const windsurfToolProtocolSystemHeader = `You have access to the following functions. To invoke a function, emit a block in this EXACT format:

<tool_call>{"name":"<function_name>","arguments":{...}}</tool_call>

Rules:
1. Each <tool_call>...</tool_call> block must fit on ONE line (no line breaks inside the JSON).
2. "arguments" must be a JSON object matching the function's parameter schema.
3. You MAY emit MULTIPLE <tool_call> blocks if the request requires calling several functions in parallel. Emit ALL needed calls consecutively, then STOP generating.
4. After emitting the last <tool_call> block, STOP. Do not write any explanation after it. The caller executes the functions and returns results wrapped in <tool_result tool_call_id="...">...</tool_result> tags in the next user turn.
5. NEVER say "I don't have access to tools" or "I cannot perform that action" - the functions listed below ARE your available tools.`

const windsurfToolProtocolUserHeader = `---
[Tool-calling context for this request]

For THIS request only, you additionally have access to the following caller-provided functions. These are real and callable. To invoke a function, emit a block in this EXACT format:

<tool_call>{"name":"<function_name>","arguments":{...}}</tool_call>

Rules:
1. Each <tool_call>...</tool_call> block must fit on ONE line.
2. "arguments" must be a JSON object matching the function's schema below.
3. Emit all needed calls consecutively, then STOP.
4. The caller executes all functions and returns results as <tool_result tool_call_id="...">...</tool_result>.
5. Only call a function if the request genuinely needs it.

Functions:`

const windsurfToolProtocolUserFooter = `
---
[End tool-calling context]

Now respond to the user request above. Use <tool_call> if appropriate, otherwise answer directly.`

func windsurfShouldEmulateTools(req *apicompat.ChatCompletionsRequest) bool {
	if req == nil {
		return false
	}
	if windsurfShouldOfferTools(req) {
		return true
	}
	return windsurfHasToolHistory(req)
}

func windsurfShouldOfferTools(req *apicompat.ChatCompletionsRequest) bool {
	if req == nil || windsurfToolChoiceIsNone(windsurfEffectiveToolChoice(req)) {
		return false
	}
	return len(req.Tools) > 0 || len(req.Functions) > 0
}

func windsurfHasToolHistory(req *apicompat.ChatCompletionsRequest) bool {
	if req == nil {
		return false
	}
	for _, msg := range req.Messages {
		if msg.Role == "tool" || msg.Role == "function" || len(msg.ToolCalls) > 0 || msg.FunctionCall != nil {
			return true
		}
	}
	return false
}

func normalizeWindsurfChatRequestForCascade(req *apicompat.ChatCompletionsRequest) *apicompat.ChatCompletionsRequest {
	if req == nil || !windsurfShouldEmulateTools(req) {
		return req
	}
	copyReq := *req
	copyReq.Messages = normalizeWindsurfMessagesForCascade(req.Messages, windsurfRequestTools(req), windsurfShouldOfferTools(req))
	return &copyReq
}

func windsurfRequestTools(req *apicompat.ChatCompletionsRequest) []apicompat.ChatTool {
	if req == nil {
		return nil
	}
	out := make([]apicompat.ChatTool, 0, len(req.Tools)+len(req.Functions))
	out = append(out, req.Tools...)
	for _, fn := range req.Functions {
		if strings.TrimSpace(fn.Name) == "" {
			continue
		}
		fnCopy := fn
		out = append(out, apicompat.ChatTool{
			Type:     "function",
			Function: &fnCopy,
		})
	}
	return out
}

func windsurfEffectiveToolChoice(req *apicompat.ChatCompletionsRequest) json.RawMessage {
	if req == nil {
		return nil
	}
	if len(req.ToolChoice) > 0 {
		return req.ToolChoice
	}
	if len(req.FunctionCall) == 0 {
		return nil
	}
	var mode string
	if json.Unmarshal(req.FunctionCall, &mode) == nil {
		return req.FunctionCall
	}
	var legacy struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(req.FunctionCall, &legacy) == nil && strings.TrimSpace(legacy.Name) != "" {
		converted, _ := json.Marshal(map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": strings.TrimSpace(legacy.Name),
			},
		})
		return converted
	}
	return req.FunctionCall
}

func normalizeWindsurfMessagesForCascade(messages []apicompat.ChatMessage, tools []apicompat.ChatTool, injectToolPreamble bool) []apicompat.ChatMessage {
	out := make([]apicompat.ChatMessage, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "tool", "function":
			id := firstNonEmptyString(msg.ToolCallID, msg.Name, "unknown")
			content := windsurfChatContentToString(msg.Content)
			text := fmt.Sprintf("<tool_result tool_call_id=%q>\n%s\n</tool_result>", id, content)
			raw, _ := json.Marshal(text)
			out = append(out, apicompat.ChatMessage{Role: "user", Content: raw})
		case "assistant":
			if len(msg.ToolCalls) == 0 && msg.FunctionCall == nil {
				out = append(out, msg)
				continue
			}
			parts := make([]string, 0, len(msg.ToolCalls)+2)
			if text := strings.TrimSpace(windsurfChatContentToString(msg.Content)); text != "" {
				parts = append(parts, text)
			}
			for _, call := range msg.ToolCalls {
				parts = append(parts, windsurfFormatToolCallMarkup(call.Function.Name, call.Function.Arguments))
			}
			if msg.FunctionCall != nil {
				parts = append(parts, windsurfFormatToolCallMarkup(msg.FunctionCall.Name, msg.FunctionCall.Arguments))
			}
			raw, _ := json.Marshal(strings.Join(parts, "\n"))
			msg.Content = raw
			msg.ToolCalls = nil
			msg.FunctionCall = nil
			out = append(out, msg)
		default:
			out = append(out, msg)
		}
	}
	if preamble := buildWindsurfToolPreambleForUser(tools, injectToolPreamble); preamble != "" {
		for i := len(out) - 1; i >= 0; i-- {
			if out[i].Role != "user" {
				continue
			}
			current := windsurfChatContentToString(out[i].Content)
			raw, _ := json.Marshal(preamble + "\n\n" + current)
			out[i].Content = raw
			break
		}
	}
	return out
}

func windsurfToolChoiceIsNone(raw json.RawMessage) bool {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	var mode string
	if json.Unmarshal(raw, &mode) == nil {
		return mode == "none"
	}
	var obj struct {
		Type string `json:"type"`
	}
	return json.Unmarshal(raw, &obj) == nil && obj.Type == "none"
}

func windsurfFormatToolCallMarkup(name, args string) string {
	name = firstNonEmptyString(strings.TrimSpace(name), "unknown")
	var parsed any
	if strings.TrimSpace(args) == "" || json.Unmarshal([]byte(args), &parsed) != nil {
		parsed = map[string]any{}
	}
	body, _ := json.Marshal(map[string]any{
		"name":      name,
		"arguments": parsed,
	})
	return "<tool_call>" + string(body) + "</tool_call>"
}

func buildWindsurfToolPreambleForUser(tools []apicompat.ChatTool, enabled bool) string {
	if !enabled || len(tools) == 0 {
		return ""
	}
	lines := []string{windsurfToolProtocolUserHeader}
	for _, tool := range tools {
		if tool.Type != "function" || tool.Function == nil || strings.TrimSpace(tool.Function.Name) == "" {
			continue
		}
		lines = append(lines, "", "### "+tool.Function.Name)
		if tool.Function.Description != "" {
			lines = append(lines, tool.Function.Description)
		}
		if len(tool.Function.Parameters) > 0 {
			lines = append(lines, "parameters schema:", "```json", string(tool.Function.Parameters), "```")
		}
	}
	lines = append(lines, windsurfToolProtocolUserFooter)
	return strings.Join(lines, "\n")
}

func buildWindsurfToolPreambleForProto(tools []apicompat.ChatTool, toolChoice json.RawMessage) string {
	if len(tools) == 0 || windsurfToolChoiceIsNone(toolChoice) {
		return ""
	}
	lines := []string{windsurfToolProtocolSystemHeader, windsurfToolChoiceInstruction(toolChoice), "", "Available functions:"}
	for _, tool := range tools {
		if tool.Type != "function" || tool.Function == nil || strings.TrimSpace(tool.Function.Name) == "" {
			continue
		}
		lines = append(lines, "", "### "+tool.Function.Name)
		if tool.Function.Description != "" {
			lines = append(lines, tool.Function.Description)
		}
		if len(tool.Function.Parameters) > 0 {
			lines = append(lines, "Parameters:", "```json", string(tool.Function.Parameters), "```")
		}
	}
	return strings.Join(lines, "\n")
}

func windsurfToolChoiceInstruction(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == `"auto"` {
		return "6. When a function is relevant to the user's request, you SHOULD call it rather than answering from memory. Prefer using a tool over guessing."
	}
	var mode string
	if json.Unmarshal(raw, &mode) == nil {
		switch mode {
		case "required", "any":
			return "6. You MUST call at least one function for every request. Do NOT answer directly in plain text - always use a <tool_call>."
		case "none":
			return "6. Do NOT call any functions. Answer the user's question directly in plain text."
		}
	}
	var obj struct {
		Type     string `json:"type"`
		Function *struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if json.Unmarshal(raw, &obj) == nil && obj.Function != nil && obj.Function.Name != "" {
		return fmt.Sprintf("6. You MUST call the function %q. No other function and no direct answer.", obj.Function.Name)
	}
	return "6. When a function is relevant to the user's request, you SHOULD call it rather than answering from memory. Prefer using a tool over guessing."
}

type windsurfToolCallParser struct {
	buffer       string
	inToolCall   bool
	inToolResult bool
	inToolCode   bool
	inBareCall   bool
	totalSeen    int
}

func (p *windsurfToolCallParser) Feed(delta string) (string, []apicompat.ChatToolCall) {
	if delta == "" {
		return "", nil
	}
	p.buffer += delta
	return p.consume(false)
}

func (p *windsurfToolCallParser) Flush() (string, []apicompat.ChatToolCall) {
	return p.consume(true)
}

func (p *windsurfToolCallParser) consume(flush bool) (string, []apicompat.ChatToolCall) {
	var safeParts []string
	var calls []apicompat.ChatToolCall
	for {
		if p.inToolResult {
			closeIdx := strings.Index(p.buffer, "</tool_result>")
			if closeIdx < 0 {
				if flush {
					p.buffer = ""
					p.inToolResult = false
				}
				break
			}
			p.buffer = p.buffer[closeIdx+len("</tool_result>"):]
			p.inToolResult = false
			continue
		}
		if p.inToolCall {
			closeIdx := strings.Index(p.buffer, "</tool_call>")
			if closeIdx < 0 {
				if flush {
					safeParts = append(safeParts, "<tool_call>"+p.buffer)
					p.buffer = ""
					p.inToolCall = false
				}
				break
			}
			body := strings.TrimSpace(p.buffer[:closeIdx])
			p.buffer = p.buffer[closeIdx+len("</tool_call>"):]
			p.inToolCall = false
			if call, ok := p.parseNamedToolCall(body); ok {
				calls = append(calls, call)
			} else {
				safeParts = append(safeParts, "<tool_call>"+body+"</tool_call>")
			}
			continue
		}
		if p.inToolCode || p.inBareCall {
			endIdx := findWindsurfToolJSONEnd(p.buffer)
			if endIdx < 0 {
				if flush {
					safeParts = append(safeParts, p.buffer)
					p.buffer = ""
					p.inToolCode = false
					p.inBareCall = false
				}
				break
			}
			body := p.buffer[:endIdx+1]
			p.buffer = p.buffer[endIdx+1:]
			if p.inToolCode {
				if call, ok := p.parseToolCodeCall(body); ok {
					calls = append(calls, call)
				} else {
					safeParts = append(safeParts, body)
				}
				p.inToolCode = false
			} else {
				if call, ok := p.parseNamedToolCall(body); ok {
					calls = append(calls, call)
				} else {
					safeParts = append(safeParts, body)
				}
				p.inBareCall = false
			}
			continue
		}

		nextIdx, tag := windsurfNextToolTag(p.buffer)
		if nextIdx < 0 {
			if flush {
				safeParts = append(safeParts, p.buffer)
				p.buffer = ""
				break
			}
			hold := windsurfToolParserHoldLen(p.buffer)
			emitUpto := len(p.buffer) - hold
			if emitUpto > 0 {
				safeParts = append(safeParts, p.buffer[:emitUpto])
				p.buffer = p.buffer[emitUpto:]
			}
			break
		}
		if nextIdx > 0 {
			safeParts = append(safeParts, p.buffer[:nextIdx])
		}
		switch tag {
		case "tool_call":
			p.buffer = p.buffer[nextIdx+len("<tool_call>"):]
			p.inToolCall = true
		case "tool_result":
			closeAngle := strings.Index(p.buffer[nextIdx:], ">")
			if closeAngle < 0 {
				p.buffer = p.buffer[nextIdx:]
				return strings.Join(safeParts, ""), calls
			}
			p.buffer = p.buffer[nextIdx+closeAngle+1:]
			p.inToolResult = true
		case "tool_code":
			p.buffer = p.buffer[nextIdx:]
			p.inToolCode = true
		case "bare":
			p.buffer = p.buffer[nextIdx:]
			p.inBareCall = true
		}
	}
	return strings.Join(safeParts, ""), calls
}

func (p *windsurfToolCallParser) parseNamedToolCall(raw string) (apicompat.ChatToolCall, bool) {
	var body struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal([]byte(raw), &body) != nil || strings.TrimSpace(body.Name) == "" {
		return apicompat.ChatToolCall{}, false
	}
	args := "{}"
	if len(body.Arguments) > 0 {
		args = string(body.Arguments)
	}
	return p.newToolCall(body.Name, args), true
}

func (p *windsurfToolCallParser) parseToolCodeCall(raw string) (apicompat.ChatToolCall, bool) {
	var body struct {
		ToolCode string `json:"tool_code"`
	}
	if json.Unmarshal([]byte(raw), &body) != nil || strings.TrimSpace(body.ToolCode) == "" {
		return apicompat.ChatToolCall{}, false
	}
	open := strings.Index(body.ToolCode, "(")
	close := strings.LastIndex(body.ToolCode, ")")
	if open <= 0 || close <= open {
		return apicompat.ChatToolCall{}, false
	}
	name := strings.TrimSpace(body.ToolCode[:open])
	args := strings.TrimSpace(body.ToolCode[open+1 : close])
	if !strings.HasPrefix(args, "{") {
		encoded, _ := json.Marshal(map[string]any{"input": args})
		args = string(encoded)
	}
	return p.newToolCall(name, args), true
}

func (p *windsurfToolCallParser) newToolCall(name, args string) apicompat.ChatToolCall {
	id := fmt.Sprintf("call_%d_%x", p.totalSeen, time.Now().UnixNano())
	p.totalSeen++
	return apicompat.ChatToolCall{
		ID:   id,
		Type: "function",
		Function: apicompat.ChatFunctionCall{
			Name:      strings.TrimSpace(name),
			Arguments: firstNonEmptyString(strings.TrimSpace(args), "{}"),
		},
	}
}

func windsurfNextToolTag(s string) (int, string) {
	type candidate struct {
		idx int
		tag string
	}
	var candidates []candidate
	if idx := strings.Index(s, "<tool_call>"); idx >= 0 {
		candidates = append(candidates, candidate{idx: idx, tag: "tool_call"})
	}
	if idx := strings.Index(s, "<tool_result"); idx >= 0 {
		candidates = append(candidates, candidate{idx: idx, tag: "tool_result"})
	}
	if idx := strings.Index(s, `{"tool_code"`); idx >= 0 {
		candidates = append(candidates, candidate{idx: idx, tag: "tool_code"})
	}
	if idx := strings.Index(s, `{"name"`); idx >= 0 {
		candidates = append(candidates, candidate{idx: idx, tag: "bare"})
	}
	if len(candidates) == 0 {
		return -1, ""
	}
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.idx < best.idx {
			best = c
		}
	}
	return best.idx, best.tag
}

func windsurfToolParserHoldLen(s string) int {
	prefixes := []string{"<tool_call>", "<tool_result", `{"tool_code"`, `{"name"`}
	hold := 0
	for _, prefix := range prefixes {
		max := len(prefix) - 1
		if len(s) < max {
			max = len(s)
		}
		for i := max; i > 0; i-- {
			if strings.HasSuffix(s, prefix[:i]) && i > hold {
				hold = i
			}
		}
	}
	return hold
}

func findWindsurfToolJSONEnd(s string) int {
	depth := 0
	inString := false
	escaped := false
	for i, ch := range s {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func parseWindsurfToolCallsFromText(text string) (string, []apicompat.ChatToolCall) {
	parser := &windsurfToolCallParser{}
	aText, aCalls := parser.Feed(text)
	bText, bCalls := parser.Flush()
	return aText + bText, append(aCalls, bCalls...)
}
