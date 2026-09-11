// Package thinking provides unified thinking configuration processing.
package thinking

import (
	"strings"

	"github.com/MoeForge/moego/ojson"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/util"
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
	if len(body) == 0 || !ojson.ValidBytes(body) {
		return body
	}
	doc := ojson.NewDoc(body)
	if !stripThinkingFieldsDoc(doc, provider) {
		return body
	}
	return doc.Bytes()
}

// StripThinkingFieldsDoc removes thinking configuration fields from doc in place.
func StripThinkingFieldsDoc(doc *ojson.Doc, format string, keepTopLevelEffort ...bool) {
	_ = stripThinkingFieldsDoc(doc, format, keepTopLevelEffort...)
}

func stripThinkingFieldsDoc(doc *ojson.Doc, format string, keepTopLevelEffort ...bool) bool {
	if doc == nil {
		return false
	}
	provider := strings.ToLower(strings.TrimSpace(format))
	if provider == "openai-response" {
		provider = "codex"
	}
	keepEffort := len(keepTopLevelEffort) > 0 && keepTopLevelEffort[0]

	var paths []string
	switch provider {
	case "claude":
		if keepEffort {
			paths = []string{"thinking"}
		} else {
			paths = []string{"thinking", "output_config.effort"}
		}
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
		if keepEffort {
			paths = []string{"reasoning"}
		} else {
			paths = []string{"reasoning_effort", "reasoning"}
		}
	case "kimi", "kimi-ai", "kimi.ai", "kimi.com":
		if keepEffort {
			paths = []string{"thinking"}
		} else {
			paths = []string{"reasoning_effort", "thinking"}
		}
	case "codex", "xai":
		if !keepEffort {
			paths = []string{"reasoning"}
		}
	default:
		return false
	}

	modified := false
	for _, path := range paths {
		if doc.Get(path).Exists() {
			if doc.Delete(path) == nil {
				modified = true
			}
		}
	}

	// Avoid leaving an empty output_config object for Claude when effort was the only field.
	if provider == "claude" && !keepEffort {
		if oc := doc.Get("output_config"); oc.Exists() && oc.IsObject() && len(oc.Map()) == 0 {
			if util.DeleteJSONDocIfExists(doc, "output_config") == nil {
				modified = true
			}
		}
	}
	return modified
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
	case "kimi", "kimi-ai", "kimi.ai", "kimi.com":
		return []string{"reasoning_effort", "thinking.effort"}
	case "codex", "xai":
		return []string{"reasoning.effort"}
	default:
		return nil
	}
}

// emptyEffortContainers lists objects that must not be left empty after their
// blank effort field is removed.
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
	case "kimi", "kimi-ai", "kimi.ai", "kimi.com":
		return []string{"thinking"}
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
	if len(body) == 0 || !ojson.ValidBytes(body) {
		return body
	}
	doc := ojson.NewDoc(body)
	if !stripEmptyThinkingLevelDoc(doc, provider) {
		return body
	}
	return doc.Bytes()
}

func stripEmptyThinkingLevelDoc(doc *ojson.Doc, provider string) bool {
	if doc == nil {
		return false
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "openai-response" {
		provider = "codex"
	}

	modified := false
	for _, path := range emptyEffortPaths(provider) {
		value := doc.Get(path)
		if !value.Exists() {
			continue
		}
		if value.Type != ojson.Null && !(value.Type == ojson.String && strings.TrimSpace(value.String()) == "") {
			continue
		}
		if doc.Delete(path) == nil {
			modified = true
		}
	}
	if !modified {
		return false
	}

	for _, path := range emptyEffortContainers(provider) {
		container := doc.Get(path)
		if container.Exists() && container.IsObject() && len(container.Map()) == 0 {
			if util.DeleteJSONDocIfExists(doc, path) == nil {
				modified = true
			}
		}
	}
	return modified
}
