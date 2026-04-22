package service

import "strings"

type WindsurfResolvedModel struct {
	RequestedModel string
	CanonicalModel string
	UpstreamModel  string
	Provider       string
	ModelUID       string
	EnumValue      int
}

type windsurfStaticModelInfo struct {
	Provider  string
	ModelUID  string
	EnumValue int
}

var windsurfStaticModels = map[string]windsurfStaticModelInfo{
	"claude-3.5-sonnet":          {Provider: "anthropic", EnumValue: 166},
	"claude-3.7-sonnet":          {Provider: "anthropic", EnumValue: 226},
	"claude-4-sonnet":            {Provider: "anthropic", ModelUID: "MODEL_CLAUDE_4_SONNET", EnumValue: 281},
	"claude-4-opus":              {Provider: "anthropic", ModelUID: "MODEL_CLAUDE_4_OPUS", EnumValue: 290},
	"claude-4.1-opus":            {Provider: "anthropic", ModelUID: "MODEL_CLAUDE_4_1_OPUS", EnumValue: 328},
	"claude-4.5-haiku":           {Provider: "anthropic", ModelUID: "MODEL_PRIVATE_11"},
	"claude-4.5-sonnet":          {Provider: "anthropic", ModelUID: "MODEL_PRIVATE_2", EnumValue: 353},
	"claude-4.5-opus":            {Provider: "anthropic", ModelUID: "MODEL_CLAUDE_4_5_OPUS", EnumValue: 391},
	"claude-sonnet-4.6":          {Provider: "anthropic", ModelUID: "claude-sonnet-4-6"},
	"claude-sonnet-4.6-thinking": {Provider: "anthropic", ModelUID: "claude-sonnet-4-6-thinking"},
	"claude-opus-4.6":            {Provider: "anthropic", ModelUID: "claude-opus-4-6"},
	"claude-opus-4.6-thinking":   {Provider: "anthropic", ModelUID: "claude-opus-4-6-thinking"},
	"claude-opus-4-7-medium":     {Provider: "anthropic", ModelUID: "claude-opus-4-7-medium"},
	"gpt-4o":                     {Provider: "openai", ModelUID: "MODEL_CHAT_GPT_4O_2024_08_06", EnumValue: 109},
	"gpt-4o-mini":                {Provider: "openai", EnumValue: 113},
	"gpt-4.1":                    {Provider: "openai", ModelUID: "MODEL_CHAT_GPT_4_1_2025_04_14", EnumValue: 259},
	"gpt-4.1-mini":               {Provider: "openai", EnumValue: 260},
	"gpt-4.1-nano":               {Provider: "openai", EnumValue: 261},
	"gpt-5":                      {Provider: "openai", ModelUID: "MODEL_PRIVATE_6", EnumValue: 340},
	"gpt-5-mini":                 {Provider: "openai", EnumValue: 337},
	"gpt-5-codex":                {Provider: "openai", ModelUID: "MODEL_CHAT_GPT_5_CODEX", EnumValue: 346},
	"gpt-5.2":                    {Provider: "openai", ModelUID: "MODEL_GPT_5_2_MEDIUM", EnumValue: 401},
	"gpt-5.2-low":                {Provider: "openai", ModelUID: "MODEL_GPT_5_2_LOW", EnumValue: 400},
	"gpt-5.2-high":               {Provider: "openai", ModelUID: "MODEL_GPT_5_2_HIGH", EnumValue: 402},
	"gpt-5.2-xhigh":              {Provider: "openai", ModelUID: "MODEL_GPT_5_2_XHIGH", EnumValue: 403},
	"gpt-5.3-codex":              {Provider: "openai", ModelUID: "gpt-5-3-codex-medium"},
	"gpt-5.4-none":               {Provider: "openai", ModelUID: "gpt-5-4-none"},
	"gpt-5.4-low":                {Provider: "openai", ModelUID: "gpt-5-4-low"},
	"gpt-5.4-medium":             {Provider: "openai", ModelUID: "gpt-5-4-medium"},
	"gpt-5.4-high":               {Provider: "openai", ModelUID: "gpt-5-4-high"},
	"gpt-5.4-xhigh":              {Provider: "openai", ModelUID: "gpt-5-4-xhigh"},
	"o3-mini":                    {Provider: "openai", EnumValue: 207},
	"o3":                         {Provider: "openai", ModelUID: "MODEL_CHAT_O3", EnumValue: 218},
	"o4-mini":                    {Provider: "openai", EnumValue: 264},
	"gemini-2.5-pro":             {Provider: "google", ModelUID: "MODEL_GOOGLE_GEMINI_2_5_PRO", EnumValue: 246},
	"gemini-2.5-flash":           {Provider: "google", ModelUID: "MODEL_GOOGLE_GEMINI_2_5_FLASH", EnumValue: 312},
	"gemini-3.0-pro":             {Provider: "google", ModelUID: "MODEL_GOOGLE_GEMINI_3_0_PRO_LOW", EnumValue: 412},
	"gemini-3.0-flash":           {Provider: "google", ModelUID: "MODEL_GOOGLE_GEMINI_3_0_FLASH_MEDIUM", EnumValue: 415},
	"gemini-3.1-pro-low":         {Provider: "google", ModelUID: "gemini-3-1-pro-low"},
	"gemini-3.1-pro-high":        {Provider: "google", ModelUID: "gemini-3-1-pro-high"},
	"deepseek-v3":                {Provider: "deepseek", EnumValue: 205},
	"deepseek-v3-2":              {Provider: "deepseek", EnumValue: 409},
	"deepseek-r1":                {Provider: "deepseek", EnumValue: 206},
	"grok-3":                     {Provider: "xai", ModelUID: "MODEL_XAI_GROK_3", EnumValue: 217},
	"grok-3-mini":                {Provider: "xai", EnumValue: 234},
	"swe-1.5":                    {Provider: "windsurf", ModelUID: "MODEL_SWE_1_5_SLOW", EnumValue: 369},
	"swe-1.5-fast":               {Provider: "windsurf", ModelUID: "MODEL_SWE_1_5", EnumValue: 359},
	"swe-1.6":                    {Provider: "windsurf", ModelUID: "swe-1-6"},
	"swe-1.6-fast":               {Provider: "windsurf", ModelUID: "swe-1-6-fast"},
	"arena-fast":                 {Provider: "windsurf", ModelUID: "arena-fast"},
	"arena-smart":                {Provider: "windsurf", ModelUID: "arena-smart"},
}

var windsurfModelAliases = map[string]string{
	"claude-3-5-sonnet-20240620": "claude-3.5-sonnet",
	"claude-3-5-sonnet-20241022": "claude-3.5-sonnet",
	"claude-3-7-sonnet-20250219": "claude-3.7-sonnet",
	"claude-sonnet-4-20250514":   "claude-4-sonnet",
	"claude-opus-4-20250514":     "claude-4-opus",
	"claude-opus-4-1-20250805":   "claude-4.1-opus",
	"claude-sonnet-4-5-20250929": "claude-4.5-sonnet",
	"claude-opus-4-5-20251101":   "claude-4.5-opus",
	"claude-opus-4-7":            "claude-opus-4-7-medium",
	"claude-opus-4.7":            "claude-opus-4-7-medium",
	"claude-opus-4-7-latest":     "claude-opus-4-7-medium",
	"claude-sonnet-4-6":          "claude-sonnet-4.6",
	"claude-sonnet-4-6-thinking": "claude-sonnet-4.6-thinking",
	"claude-opus-4-6":            "claude-opus-4.6",
	"claude-opus-4-6-thinking":   "claude-opus-4.6-thinking",
	"gpt-4o-2024-11-20":          "gpt-4o",
	"gpt-4o-2024-08-06":          "gpt-4o",
	"gpt-4o-mini-2024-07-18":     "gpt-4o-mini",
	"gpt-4.1-2025-04-14":         "gpt-4.1",
	"gpt-4.1-mini-2025-04-14":    "gpt-4.1-mini",
	"gpt-4.1-nano-2025-04-14":    "gpt-4.1-nano",
	"gpt-5-4-none":               "gpt-5.4-none",
	"gpt-5-4-low":                "gpt-5.4-low",
	"gpt-5-4-medium":             "gpt-5.4-medium",
	"gpt-5-4-high":               "gpt-5.4-high",
	"gpt-5-4-xhigh":              "gpt-5.4-xhigh",
	"opus-4.6":                   "claude-opus-4.6",
	"opus-4.6-thinking":          "claude-opus-4.6-thinking",
	"sonnet-4.6":                 "claude-sonnet-4.6",
	"sonnet-4.6-thinking":        "claude-sonnet-4.6-thinking",
	"ws-opus":                    "claude-opus-4.6",
	"ws-opus-thinking":           "claude-opus-4.6-thinking",
	"ws-sonnet":                  "claude-sonnet-4.6",
	"ws-sonnet-thinking":         "claude-sonnet-4.6-thinking",
}

func resolveWindsurfCanonicalModelID(model string) string {
	normalized := normalizeWindsurfModelIdentifier(model)
	if normalized == "" {
		return ""
	}
	if alias, ok := windsurfModelAliases[normalized]; ok {
		return alias
	}
	return normalized
}

func lookupWindsurfStaticModel(model string) (windsurfStaticModelInfo, bool) {
	info, ok := windsurfStaticModels[resolveWindsurfCanonicalModelID(model)]
	return info, ok
}

func buildWindsurfResolvedModel(requestedModel string, modelID string, cfg *WindsurfModelConfig) WindsurfResolvedModel {
	canonical := resolveWindsurfCanonicalModelID(modelID)
	if canonical == "" {
		canonical = resolveWindsurfCanonicalModelID(requestedModel)
	}

	resolved := WindsurfResolvedModel{
		RequestedModel: strings.TrimSpace(requestedModel),
		CanonicalModel: canonical,
		UpstreamModel:  strings.TrimSpace(modelID),
	}
	if resolved.UpstreamModel == "" {
		resolved.UpstreamModel = canonical
	}

	if info, ok := lookupWindsurfStaticModel(canonical); ok {
		resolved.Provider = info.Provider
		resolved.ModelUID = info.ModelUID
		resolved.EnumValue = info.EnumValue
	}

	if cfg != nil {
		if resolved.Provider == "" {
			resolved.Provider = normalizeWindsurfProvider(cfg.Provider)
		}
		if resolved.ModelUID == "" {
			resolved.ModelUID = strings.TrimSpace(cfg.ModelUID)
		}
	}

	if resolved.Provider == "" {
		resolved.Provider = inferWindsurfModelProvider(resolved.UpstreamModel)
	}
	return resolved
}

func normalizeWindsurfProvider(provider string) string {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	switch normalized {
	case "", "unknown":
		return ""
	case "model-provider-openai", "openai":
		return "openai"
	case "model-provider-anthropic", "anthropic":
		return "anthropic"
	case "model-provider-google", "google":
		return "google"
	case "model-provider-xai", "xai":
		return "xai"
	case "model-provider-deepseek", "deepseek":
		return "deepseek"
	case "model-provider-windsurf", "windsurf":
		return "windsurf"
	default:
		return normalized
	}
}
