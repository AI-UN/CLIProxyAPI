package helps

import "testing"

func TestClaudeToolTypeClassification(t *testing.T) {
	tests := []struct {
		name          string
		toolType      string
		wantCustom    bool
		wantPreserved bool
	}{
		{name: "empty", toolType: ""},
		{name: "custom", toolType: "custom", wantCustom: true},
		{name: "trimmed custom", toolType: " custom ", wantCustom: true},
		{name: "builtin", toolType: "web_search_20250305", wantPreserved: true},
		{name: "unknown typed", toolType: "custom_builtin_20250401", wantPreserved: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsClaudeCustomToolType(tt.toolType); got != tt.wantCustom {
				t.Fatalf("IsClaudeCustomToolType(%q) = %v, want %v", tt.toolType, got, tt.wantCustom)
			}
			if got := IsClaudePreservedTypedToolType(tt.toolType); got != tt.wantPreserved {
				t.Fatalf("IsClaudePreservedTypedToolType(%q) = %v, want %v", tt.toolType, got, tt.wantPreserved)
			}
		})
	}
}

func TestClaudePreservedToolRegistry_DefaultSeedFallback(t *testing.T) {
	registry := NewClaudePreservedToolRegistry(nil)
	for _, name := range defaultClaudeBuiltinToolNames {
		if !registry[name] {
			t.Fatalf("default builtin %q missing from fallback registry", name)
		}
	}
}

func TestClaudePreservedToolRegistry_DeclarationOverrides(t *testing.T) {
	registry := NewClaudePreservedToolRegistry([]byte(`{
		"tools": [
			{"type": "custom", "name": "bash"},
			{"name": "web_search"},
			{"type": "custom_builtin_20250401", "name": "special_builtin"},
			{"type": "mcp_toolset", "name": "mcp_toolset"}
		]
	}`))

	if registry["bash"] {
		t.Fatal("expected explicit type=custom declaration to override builtin fallback")
	}
	if registry["web_search"] {
		t.Fatal("expected explicit untyped declaration to override builtin fallback")
	}
	if !registry["special_builtin"] {
		t.Fatal("expected unknown typed tool to be preserved")
	}
	if !registry["mcp_toolset"] {
		t.Fatal("expected typed builtin tool to be preserved")
	}
}
