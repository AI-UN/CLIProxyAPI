package helps

import (
	"strings"

	"github.com/tidwall/gjson"
)

var defaultClaudeBuiltinToolNames = []string{
	"web_search",
	"code_execution",
	"text_editor",
	"computer",
	"bash",
	"memory",
	"web_fetch",
	"tool_search_tool_regex",
	"advisor",
	"mcp_toolset",
}

func IsClaudeCustomToolType(toolType string) bool {
	return strings.TrimSpace(toolType) == "custom"
}

func IsClaudePreservedTypedToolType(toolType string) bool {
	toolType = strings.TrimSpace(toolType)
	return toolType != "" && toolType != "custom"
}

// NewClaudePreservedToolRegistry returns tool names that must not be rewritten.
// Known built-in names provide a fallback for history-only requests. Explicit
// custom declarations override that fallback, while opaque typed declarations
// are preserved even when their type is not yet known to this version.
func NewClaudePreservedToolRegistry(body []byte) map[string]bool {
	registry := make(map[string]bool, len(defaultClaudeBuiltinToolNames))
	for _, name := range defaultClaudeBuiltinToolNames {
		registry[name] = true
	}

	tools := gjson.GetBytes(body, "tools")
	if !tools.Exists() || !tools.IsArray() {
		return registry
	}
	tools.ForEach(func(_, tool gjson.Result) bool {
		name := tool.Get("name").String()
		if name == "" {
			return true
		}
		if IsClaudePreservedTypedToolType(tool.Get("type").String()) {
			registry[name] = true
		} else {
			delete(registry, name)
		}
		return true
	})
	return registry
}
