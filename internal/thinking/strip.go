// Package thinking provides unified thinking configuration processing.
package thinking

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// StripThinkingConfig removes thinking configuration fields from request body.
//
// This function is used when a model doesn't support thinking but the request
// contains thinking configuration. The configuration is silently removed to
// prevent upstream API errors.
//
// Parameters:
//   - body: Original request body JSON
//   - provider: Provider name (determines which fields to strip)
//
// Returns:
//   - Modified request body JSON with thinking configuration removed
//   - Original body is returned unchanged if:
//   - body is empty or invalid JSON
//   - provider is unknown
//   - no thinking configuration found
func StripThinkingConfig(body []byte, provider string) []byte {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body
	}

	var paths []string
	switch provider {
	case "claude":
		paths = []string{"thinking", "output_config.effort"}
	case "gemini":
		paths = []string{"generationConfig.thinkingConfig"}
	case "antigravity":
		paths = []string{"request.generationConfig.thinkingConfig"}
	case "interactions":
		paths = []string{
			"generation_config.thinking_level",
			"generation_config.thinkingLevel",
			"generation_config.thinking_budget",
			"generation_config.thinkingBudget",
			"generation_config.thinking_summaries",
			"generation_config.thinkingSummaries",
			"generation_config.thinking_config",
			"generation_config.thinkingConfig",
		}
	case "openai":
		paths = []string{"reasoning_effort", "reasoning"}
	case "kimi":
		paths = []string{
			"reasoning_effort",
			"thinking",
		}
	case "codex", "xai":
		paths = []string{"reasoning"}
	default:
		return body
	}

	result := body
	for _, path := range paths {
		result, _ = sjson.DeleteBytes(result, path)
	}

	// Avoid leaving an empty output_config object for Claude when effort was the only field.
	if provider == "claude" {
		if oc := gjson.GetBytes(result, "output_config"); oc.Exists() && oc.IsObject() && len(oc.Map()) == 0 {
			result, _ = sjson.DeleteBytes(result, "output_config")
		}
	}
	return result
}

// emptyEffortPaths lists the request fields that carry a discrete thinking
// level for a provider format.
func emptyEffortPaths(provider string) []string {
	switch provider {
	case "claude":
		return []string{"output_config.effort"}
	case "gemini":
		return []string{"generationConfig.thinkingConfig.thinkingLevel", "generationConfig.thinkingConfig.thinking_level"}
	case "antigravity":
		return []string{"request.generationConfig.thinkingConfig.thinkingLevel", "request.generationConfig.thinkingConfig.thinking_level"}
	case "interactions":
		return []string{
			"generation_config.thinking_level",
			"generation_config.thinkingLevel",
			"generation_config.thinking_config.thinking_level",
			"generation_config.thinking_config.thinkingLevel",
			"generation_config.thinkingConfig.thinking_level",
			"generation_config.thinkingConfig.thinkingLevel",
		}
	case "openai":
		return []string{"reasoning_effort"}
	case "kimi":
		return []string{"reasoning_effort", "thinking.effort"}
	case "codex", "xai":
		return []string{"reasoning.effort"}
	default:
		return nil
	}
}

// emptyEffortContainers lists the objects that only exist to carry a thinking
// level and must not be left behind empty once a blank level is removed.
func emptyEffortContainers(provider string) []string {
	switch provider {
	case "claude":
		return []string{"output_config"}
	case "gemini":
		return []string{"generationConfig.thinkingConfig"}
	case "antigravity":
		return []string{"request.generationConfig.thinkingConfig"}
	case "interactions":
		return []string{"generation_config.thinking_config", "generation_config.thinkingConfig"}
	case "codex", "xai":
		return []string{"reasoning"}
	default:
		return nil
	}
}

// StripEmptyThinkingLevel removes blank thinking level fields from a request body.
//
// Some clients always emit the level field and leave it empty (or null) to mean
// "use the provider default". An empty level is not a valid value for any
// provider, so it is dropped here instead of being validated or forwarded.
func StripEmptyThinkingLevel(body []byte, provider string) []byte {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body
	}

	result := body
	stripped := false
	for _, path := range emptyEffortPaths(provider) {
		value := gjson.GetBytes(result, path)
		if !value.Exists() {
			continue
		}
		if value.Type != gjson.Null && !(value.Type == gjson.String && strings.TrimSpace(value.String()) == "") {
			continue
		}
		result, _ = sjson.DeleteBytes(result, path)
		stripped = true
	}
	if !stripped {
		return body
	}

	// Do not leave behind a container object that only held the blank level.
	for _, path := range emptyEffortContainers(provider) {
		container := gjson.GetBytes(result, path)
		if container.Exists() && container.IsObject() && len(container.Map()) == 0 {
			result, _ = sjson.DeleteBytes(result, path)
		}
	}
	return result
}
