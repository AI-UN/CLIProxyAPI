package thinking_test

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	"github.com/tidwall/gjson"
)

// Clients that always emit the reasoning level field and leave it blank must be
// treated as "unspecified": no validation error, and no empty level forwarded.
func TestApplyThinkingBlankLevelIsUnspecified(t *testing.T) {
	tests := []struct {
		name       string
		modelInfo  *registry.ModelInfo
		format     string
		body       string
		effortPath string
		keepPath   string
	}{
		{
			name:       "openai blank reasoning_effort",
			modelInfo:  &registry.ModelInfo{ID: "gpt-5.1", Type: "openai", Thinking: &registry.ThinkingSupport{Levels: []string{"none", "low", "medium", "high", "xhigh"}}},
			format:     "openai",
			body:       `{"model":"gpt-5.1","reasoning_effort":"","messages":[{"role":"user","content":"hi"}]}`,
			effortPath: "reasoning_effort",
			keepPath:   "messages.0.role",
		},
		{
			name:       "openai null reasoning_effort",
			modelInfo:  &registry.ModelInfo{ID: "gpt-5.1", Type: "openai", Thinking: &registry.ThinkingSupport{Levels: []string{"none", "low", "medium", "high", "xhigh"}}},
			format:     "openai",
			body:       `{"model":"gpt-5.1","reasoning_effort":null,"messages":[{"role":"user","content":"hi"}]}`,
			effortPath: "reasoning_effort",
			keepPath:   "messages.0.role",
		},
		{
			name:       "codex blank reasoning.effort",
			modelInfo:  &registry.ModelInfo{ID: "gpt-5.1-codex", Type: "codex", Thinking: &registry.ThinkingSupport{Levels: []string{"none", "low", "medium", "high", "xhigh"}}},
			format:     "codex",
			body:       `{"model":"gpt-5.1-codex","reasoning":{"effort":" ","summary":"auto"}}`,
			effortPath: "reasoning.effort",
			keepPath:   "reasoning.summary",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			out, err := thinking.ApplyThinkingWithModelInfo(body, body, tc.modelInfo.ID, tc.format, tc.format, tc.format, tc.modelInfo)
			if err != nil {
				t.Fatalf("ApplyThinkingWithModelInfo() error = %v", err)
			}
			if got := gjson.GetBytes(out, tc.effortPath); got.Exists() {
				t.Fatalf("%s = %s, want removed; body=%s", tc.effortPath, got.Raw, out)
			}
			if !gjson.GetBytes(out, tc.keepPath).Exists() {
				t.Fatalf("%s missing after normalization; body=%s", tc.keepPath, out)
			}
		})
	}
}

// A blank Gemini 3 thinkingLevel is dropped without touching the 2.5 budget.
func TestStripEmptyThinkingLevelKeepsBudget(t *testing.T) {
	out := thinking.StripEmptyThinkingLevel([]byte(`{"generationConfig":{"thinkingConfig":{"thinkingLevel":"","thinkingBudget":8192}}}`), "gemini")
	if got := gjson.GetBytes(out, "generationConfig.thinkingConfig.thinkingLevel"); got.Exists() {
		t.Fatalf("thinkingLevel = %s, want removed; body=%s", got.Raw, out)
	}
	if got := gjson.GetBytes(out, "generationConfig.thinkingConfig.thinkingBudget").Int(); got != 8192 {
		t.Fatalf("thinkingBudget = %d, want 8192; body=%s", got, out)
	}
}

// A reasoning object left empty by the blank level must not be forwarded.
func TestStripEmptyThinkingLevelDropsEmptyContainer(t *testing.T) {
	out := thinking.StripEmptyThinkingLevel([]byte(`{"model":"gpt-5.1","reasoning":{"effort":""}}`), "codex")
	if got := gjson.GetBytes(out, "reasoning"); got.Exists() {
		t.Fatalf("reasoning = %s, want removed; body=%s", got.Raw, out)
	}
	kept := thinking.StripEmptyThinkingLevel([]byte(`{"reasoning":{}}`), "codex")
	if !gjson.GetBytes(kept, "reasoning").Exists() {
		t.Fatalf("untouched body was rewritten: %s", kept)
	}
}
