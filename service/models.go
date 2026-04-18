package service

import (
	"claude2api/config"
	"sort"
	"strings"
)

type ResolvedModel struct {
	PublicID             string `json:"public_id"`
	UpstreamID           string `json:"upstream_id"`
	DisplayName          string `json:"display_name"`
	Tier                 string `json:"tier"`
	SupportsThinking     bool   `json:"supports_thinking"`
	Enabled              bool   `json:"enabled"`
	Visible              bool   `json:"visible"`
	SystemPromptOverride string `json:"system_prompt_override,omitempty"`
	PromptOverrideMode   string `json:"prompt_override_mode,omitempty"`
	Notes                string `json:"notes,omitempty"`
	VariantOf            string `json:"variant_of,omitempty"`
	VariantType          string `json:"variant_type,omitempty"`
	Source               string `json:"source"`
}

type ResolvedModelSelection struct {
	RequestedModel       string
	PublicID             string
	UpstreamID           string
	DisplayName          string
	Tier                 string
	Thinking             bool
	RemoveModelField     bool
	SystemPromptOverride string
	PromptOverrideMode   string
}

var builtinModelDefinitions = []config.ModelDefinition{
	{
		PublicID:         "claude-3-7-sonnet-20250219",
		UpstreamID:       "claude-3-7-sonnet-20250219",
		DisplayName:      "Claude 3.7 Sonnet",
		Tier:             "free",
		SupportsThinking: true,
		Enabled:          true,
		Visible:          true,
	},
	{
		PublicID:         "claude-sonnet-4-20250514",
		UpstreamID:       "claude-sonnet-4-20250514",
		DisplayName:      "Claude Sonnet 4.5",
		Tier:             "free",
		SupportsThinking: true,
		Enabled:          true,
		Visible:          true,
	},
	{
		PublicID:         "claude-sonnet-4-6",
		UpstreamID:       "claude-sonnet-4-6-20260217",
		DisplayName:      "Claude Sonnet 4.6",
		Tier:             "free",
		SupportsThinking: false,
		Enabled:          true,
		Visible:          true,
	},
	{
		PublicID:         "claude-sonnet-4-6-20260217",
		UpstreamID:       "claude-sonnet-4-6-20260217",
		DisplayName:      "Claude Sonnet 4.6 (Legacy ID)",
		Tier:             "free",
		SupportsThinking: false,
		Enabled:          true,
		Visible:          true,
		Notes:            "Legacy upstream-compatible model ID",
	},
	{
		PublicID:         "claude-haiku-4-5",
		UpstreamID:       "claude-sonnet-4-20250514",
		DisplayName:      "Claude Haiku 4.5",
		Tier:             "free",
		SupportsThinking: false,
		Enabled:          true,
		Visible:          true,
		Notes:            "Mapped to current stable upstream until dedicated haiku upstream ID is configured",
	},
	{
		PublicID:         "claude-opus-3",
		UpstreamID:       "claude-opus-4-20250514",
		DisplayName:      "Claude Opus 3",
		Tier:             "pro",
		SupportsThinking: false,
		Enabled:          true,
		Visible:          true,
		Notes:            "Alias kept for admin-side maintenance",
	},
	{
		PublicID:         "claude-opus-4-6",
		UpstreamID:       "claude-opus-4-20250514",
		DisplayName:      "Claude Opus 4.6",
		Tier:             "pro",
		SupportsThinking: true,
		Enabled:          true,
		Visible:          true,
		Notes:            "Alias kept for admin-side maintenance",
	},
	{
		PublicID:         "claude-opus-4-7",
		UpstreamID:       "claude-opus-4-20250514",
		DisplayName:      "Claude Opus 4.7",
		Tier:             "pro",
		SupportsThinking: true,
		Enabled:          true,
		Visible:          true,
		Notes:            "Alias kept for admin-side maintenance",
	},
	{
		PublicID:         "claude-opus-4-20250514",
		UpstreamID:       "claude-opus-4-20250514",
		DisplayName:      "Claude Opus 4 (Legacy ID)",
		Tier:             "pro",
		SupportsThinking: true,
		Enabled:          true,
		Visible:          true,
		Notes:            "Legacy upstream-compatible model ID",
	},
}

func GetResolvedModels() []ResolvedModel {
	merged := make(map[string]config.ModelDefinition)
	for _, item := range builtinModelDefinitions {
		merged[item.PublicID] = normalizeModelDefinition(item)
	}
	for _, item := range config.ConfigInstance.ModelDefinitions {
		if strings.TrimSpace(item.PublicID) == "" {
			continue
		}
		merged[item.PublicID] = normalizeModelDefinition(item)
	}

	resolved := make([]ResolvedModel, 0, len(merged)*2)
	for _, item := range merged {
		resolved = append(resolved, buildResolvedModel(item, false))
		if item.SupportsThinking && item.Enabled && item.Visible {
			resolved = append(resolved, buildResolvedModel(item, true))
		}
	}

	sort.Slice(resolved, func(i, j int) bool {
		return resolved[i].PublicID < resolved[j].PublicID
	})

	return resolved
}

func ResolveModel(requested string) ResolvedModelSelection {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = "claude-3-7-sonnet-20250219"
	}

	isThinking := strings.HasSuffix(requested, "-think")
	baseID := strings.TrimSuffix(requested, "-think")

	definitions := make(map[string]config.ModelDefinition)
	for _, item := range builtinModelDefinitions {
		definitions[item.PublicID] = normalizeModelDefinition(item)
	}
	for _, item := range config.ConfigInstance.ModelDefinitions {
		if strings.TrimSpace(item.PublicID) == "" {
			continue
		}
		definitions[item.PublicID] = normalizeModelDefinition(item)
	}

	selected, exists := definitions[baseID]
	if !exists {
		selected = normalizeModelDefinition(config.ModelDefinition{
			PublicID:         baseID,
			UpstreamID:       baseID,
			DisplayName:      baseID,
			Tier:             "unknown",
			SupportsThinking: isThinking,
			Enabled:          true,
			Visible:          true,
		})
	}

	thinking := isThinking && selected.SupportsThinking
	publicID := selected.PublicID
	if thinking {
		publicID += "-think"
	}

	return ResolvedModelSelection{
		RequestedModel:       requested,
		PublicID:             publicID,
		UpstreamID:           selected.UpstreamID,
		DisplayName:          selected.DisplayName,
		Tier:                 selected.Tier,
		Thinking:             thinking,
		RemoveModelField:     shouldRemoveModelField(selected.UpstreamID),
		SystemPromptOverride: selected.SystemPromptOverride,
		PromptOverrideMode:   normalizePromptMode(selected.PromptOverrideMode),
	}
}

func GetAdminModelSummaries() []map[string]interface{} {
	models := GetResolvedModels()
	result := make([]map[string]interface{}, 0, len(models))
	for _, item := range models {
		result = append(result, map[string]interface{}{
			"id":                     item.PublicID,
			"public_id":              item.PublicID,
			"upstream_id":            item.UpstreamID,
			"display_name":           item.DisplayName,
			"tier":                   item.Tier,
			"supports_thinking":      item.SupportsThinking,
			"enabled":                item.Enabled,
			"visible":                item.Visible,
			"variant_of":             item.VariantOf,
			"variant_type":           item.VariantType,
			"source":                 item.Source,
			"has_system_prompt":      strings.TrimSpace(item.SystemPromptOverride) != "",
			"prompt_override_mode":   item.PromptOverrideMode,
			"system_prompt_override": item.SystemPromptOverride,
			"notes":                  item.Notes,
		})
	}
	return result
}

func shouldRemoveModelField(upstreamID string) bool {
	switch upstreamID {
	case "claude-sonnet-4-20250514", "claude-sonnet-4-6-20260217":
		return true
	default:
		return false
	}
}

func buildResolvedModel(item config.ModelDefinition, thinking bool) ResolvedModel {
	publicID := item.PublicID
	variantOf := ""
	variantType := "base"
	if thinking {
		variantOf = item.PublicID
		variantType = "thinking"
		publicID = item.PublicID + "-think"
	}

	return ResolvedModel{
		PublicID:             publicID,
		UpstreamID:           item.UpstreamID,
		DisplayName:          item.DisplayName,
		Tier:                 item.Tier,
		SupportsThinking:     item.SupportsThinking,
		Enabled:              item.Enabled,
		Visible:              item.Visible,
		SystemPromptOverride: item.SystemPromptOverride,
		PromptOverrideMode:   normalizePromptMode(item.PromptOverrideMode),
		Notes:                item.Notes,
		VariantOf:            variantOf,
		VariantType:          variantType,
		Source:               "config",
	}
}

func normalizeModelDefinition(item config.ModelDefinition) config.ModelDefinition {
	item.PublicID = strings.TrimSpace(item.PublicID)
	item.UpstreamID = strings.TrimSpace(item.UpstreamID)
	item.DisplayName = strings.TrimSpace(item.DisplayName)
	item.Tier = strings.TrimSpace(strings.ToLower(item.Tier))
	item.Notes = strings.TrimSpace(item.Notes)
	item.SystemPromptOverride = strings.TrimSpace(item.SystemPromptOverride)
	item.PromptOverrideMode = normalizePromptMode(item.PromptOverrideMode)

	if item.UpstreamID == "" {
		item.UpstreamID = item.PublicID
	}
	if item.DisplayName == "" {
		item.DisplayName = item.PublicID
	}
	if item.Tier == "" {
		item.Tier = "unknown"
	}
	if !item.Enabled && !item.Visible && item.PublicID != "" && item.UpstreamID != "" {
		item.Enabled = true
		item.Visible = true
	}
	return item
}

func normalizePromptMode(mode string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "replace" {
		return "replace"
	}
	return "append"
}
