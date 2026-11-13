package executor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/MoeForge/moego/ojson"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
)

// ompSystemConventionsOpening is the exact OMP opening reported in #5751.
const ompSystemConventionsOpening = "<system-conventions>\nRFC 2119: MUST, REQUIRED, SHOULD, RECOMMENDED, MAY, OPTIONAL.\n</system-conventions>"

// ompRFC2119KeywordsLine is the newer wording reported in oh-my-pi #18706.
const ompRFC2119KeywordsLine = "RFC 2119 keywords: MUST, REQUIRED, SHOULD, RECOMMENDED, MAY, OPTIONAL. `NEVER` = `MUST NOT`; `AVOID` = `SHOULD NOT`."

const ompSystemPromptOpening = ompSystemConventionsOpening + "\n" + ompRFC2119KeywordsLine

func antigravityDefaultsAuth(baseURL string) *cliproxyauth.Auth {
	return &cliproxyauth.Auth{
		Attributes: map[string]string{"base_url": baseURL},
		Metadata: map[string]any{
			"access_token": "token-123",
			"expired":      time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			"project_id":   "project-1",
		},
	}
}

func newAntigravityDefaultsUpstream(t *testing.T) (*httptest.Server, <-chan []byte) {
	t.Helper()
	bodies := make(chan []byte, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, errRead := io.ReadAll(r.Body)
		if errRead != nil {
			t.Errorf("read upstream body: %v", errRead)
			return
		}
		bodies <- body
		switch {
		case strings.Contains(r.URL.Path, "countTokens"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"totalTokens":42}`))
		case strings.Contains(r.URL.Path, "streamGenerateContent"):
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {}\n\n"))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"OK"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":1,"totalTokenCount":11}}}`))
		}
	}))
	t.Cleanup(server.Close)
	return server, bodies
}

func antigravityContentTexts(body []byte, path string) []string {
	var texts []string
	ojson.GetBytes(body, path).ForEach(func(_, item ojson.Result) bool {
		item.Get("parts").ForEach(func(_, part ojson.Result) bool {
			if part.Get("text").Type == ojson.String {
				texts = append(texts, part.Get("text").String())
			}
			return true
		})
		return true
	})
	return texts
}

// antigravityToolPropertyType reads the probe tool property type from either
// schema key; chat-completions translation renames parameters to
// parametersJsonSchema, Responses translation does not.
func antigravityToolPropertyType(body []byte) string {
	for _, schema := range []string{"parameters", "parametersJsonSchema"} {
		if typ := ojson.GetBytes(body, "request.tools.0.functionDeclarations.0."+schema+".properties.q.type"); typ.Exists() {
			return typ.String()
		}
	}
	return ""
}

func assertNoDefaultFingerprint(t *testing.T, label, text string) {
	t.Helper()
	for _, phrase := range defaultAntigravitySensitiveWords {
		if strings.Contains(text, phrase) {
			t.Fatalf("%s retained raw default fingerprint %q: %q", label, phrase, text)
		}
	}
}

// TestAntigravityDefaultSensitiveWordsObfuscateWithoutConfig covers the user
// report: the OMP opening and the Claude Agent SDK identity must be obfuscated
// even though no sensitive words are configured.
func TestAntigravityDefaultSensitiveWordsObfuscateWithoutConfig(t *testing.T) {
	systemText := ompSystemPromptOpening + "\nYou are a Claude agent, built on Anthropic's Claude Agent SDK." +
		"\n<system-directive> and system_directive and <system_conventions>.\nUnrelated guidance stays."
	payload := []byte(fmt.Sprintf(`{"request":{"systemInstruction":{"parts":[{"text":%q}]},"contents":[{"role":"user","parts":[{"text":"Claude Agent SDK and RFC 2119 remain user content"}]}],"tools":[{"functionDeclarations":[{"description":"RFC 2119 tool","parameters":{"type":"object","properties":{"q":{"type":"string"}}}}]}]}}`, systemText))

	executors := map[string]*AntigravityExecutor{
		"nil config":   NewAntigravityExecutor(nil),
		"empty config": NewAntigravityExecutor(&config.Config{}),
	}
	for name, executor := range executors {
		t.Run(name, func(t *testing.T) {
			got := executor.obfuscateSensitiveWords(payload)
			gotSystem := ojson.GetBytes(got, "request.systemInstruction.parts.0.text").String()
			assertNoDefaultFingerprint(t, "system instruction", gotSystem)
			if !strings.Contains(gotSystem, "\u034F") {
				t.Fatalf("system instruction has no CGJ marker: %q", gotSystem)
			}
			if !strings.Contains(gotSystem, "Unrelated guidance stays.") {
				t.Fatalf("nonmatching system text changed: %q", gotSystem)
			}
			if content := ojson.GetBytes(got, "request.contents.0.parts.0.text").String(); content != "Claude Agent SDK and RFC 2119 remain user content" {
				t.Fatalf("user content = %q, want unchanged", content)
			}
			if desc := ojson.GetBytes(got, "request.tools.0.functionDeclarations.0.description").String(); desc != "RFC 2119 tool" {
				t.Fatalf("tool description = %q, want unchanged", desc)
			}
		})
	}
}

// TestAntigravityDefaultSensitiveWordsCaseInsensitiveAndMerged covers matching
// case variants and adding configured words on top of the defaults.
func TestAntigravityDefaultSensitiveWordsCaseInsensitiveAndMerged(t *testing.T) {
	words := []string{"AcmeWidget"}
	cfg := &config.Config{Antigravity: config.AntigravityConfig{SensitiveWords: words}}
	executor := NewAntigravityExecutor(cfg)
	systemText := "claude agent sdk / SYSTEM-CONVENTIONS / system_conventions / System-Directive / " +
		"SYSTEM_DIRECTIVE / rfc 2119 / RFC 2119 keywords: MUST / acmewidget / unrelated text"
	payload := []byte(fmt.Sprintf(`{"request":{"systemInstruction":{"parts":[{"text":%q}]}}}`, systemText))

	got := ojson.GetBytes(executor.obfuscateSensitiveWords(payload), "request.systemInstruction.parts.0.text").String()
	for _, phrase := range []string{"claude agent sdk", "SYSTEM-CONVENTIONS", "system_conventions", "System-Directive", "SYSTEM_DIRECTIVE", "rfc 2119", "RFC 2119", "acmewidget"} {
		if strings.Contains(got, phrase) {
			t.Fatalf("system instruction retained raw %q: %q", phrase, got)
		}
	}
	if !strings.Contains(got, "unrelated text") {
		t.Fatalf("nonmatching text changed: %q", got)
	}
	if len(cfg.Antigravity.SensitiveWords) != 1 || cfg.Antigravity.SensitiveWords[0] != "AcmeWidget" {
		t.Fatalf("configured words mutated: %#v", cfg.Antigravity.SensitiveWords)
	}
}

// TestAntigravityDefaultSensitiveWordsReplacementsWin covers explicit
// replacements, including the identity mapping used to opt a default out.
func TestAntigravityDefaultSensitiveWordsReplacementsWin(t *testing.T) {
	replacements := map[string]string{
		"Claude Agent SDK":   "Claude SDK",
		"RFC 2119":           "RFC 2119",
		"system-conventions": "",
	}
	cfg := &config.Config{Antigravity: config.AntigravityConfig{SensitiveWordsReplace: replacements}}
	executor := NewAntigravityExecutor(cfg)
	systemText := "Built on Claude Agent SDK. <system-conventions> RFC 2119: MUST. <system_directive>"
	payload := []byte(fmt.Sprintf(`{"request":{"systemInstruction":{"parts":[{"text":%q}]}}}`, systemText))

	got := ojson.GetBytes(executor.obfuscateSensitiveWords(payload), "request.systemInstruction.parts.0.text").String()
	if !strings.Contains(got, "Claude SDK") || strings.Contains(got, "C\u034Flaude SDK") {
		t.Fatalf("explicit replacement lost or obfuscated: %q", got)
	}
	if !strings.Contains(got, "RFC 2119") || strings.Contains(got, "R\u034FFC 2119") {
		t.Fatalf("identity replacement did not opt out: %q", got)
	}
	if strings.Contains(got, "system-conventions") {
		t.Fatalf("empty replacement did not delete the phrase: %q", got)
	}
	if strings.Contains(got, "system_directive") {
		t.Fatalf("unreplaced default was left raw: %q", got)
	}
	if !strings.Contains(got, "MUST.") {
		t.Fatalf("nonmatching text changed: %q", got)
	}
	wantReplacements := map[string]string{"Claude Agent SDK": "Claude SDK", "RFC 2119": "RFC 2119", "system-conventions": ""}
	if !reflect.DeepEqual(cfg.Antigravity.SensitiveWordsReplace, wantReplacements) {
		t.Fatalf("replacement map mutated: %#v", cfg.Antigravity.SensitiveWordsReplace)
	}
}

func TestAntigravityDefaultSensitiveWordsConfigChangesDoNotMutateWords(t *testing.T) {
	var nilExecutor *AntigravityExecutor
	if matcher := nilExecutor.getSensitiveWordMatcher(); matcher != nil {
		t.Fatal("nil executor must leave payloads unchanged")
	}

	cfg := &config.Config{}
	executor := NewAntigravityExecutor(cfg)
	payload := []byte(`{"request":{"systemInstruction":{"parts":[{"text":"custom-cache-word Claude Agent SDK"}]}}}`)
	before := ojson.GetBytes(executor.obfuscateSensitiveWords(payload), "request.systemInstruction.parts.0.text").String()
	if !strings.Contains(before, "custom-cache-word") || strings.Contains(before, "Claude Agent SDK") {
		t.Fatalf("default handling = %q", before)
	}

	words := make([]string, 1, 8)
	words[0] = "custom-cache-word"
	cfg.Antigravity.SensitiveWords = words
	after := ojson.GetBytes(executor.obfuscateSensitiveWords(payload), "request.systemInstruction.parts.0.text").String()
	if strings.Contains(after, "custom-cache-word") || strings.Contains(after, "Claude Agent SDK") {
		t.Fatalf("updated word list not applied: %q", after)
	}
	for i, word := range words[:cap(words)] {
		if i > 0 && word != "" {
			t.Fatalf("defaults were appended into caller slice at %d: %#v", i, words[:cap(words)])
		}
	}
}

// TestAntigravityDefaultSensitiveWordsReachUpstream sends real executor paths
// and checks the upstream body: defaults are obfuscated in the system
// instruction only, while user content and tool schemas stay untouched.
func TestAntigravityDefaultSensitiveWordsReachUpstream(t *testing.T) {
	server, bodies := newAntigravityDefaultsUpstream(t)
	auth := antigravityDefaultsAuth(server.URL)
	const userText = "Claude Agent SDK stays in user content"
	tools := `"tools":[{"type":"function","name":"run_probe","description":"Diagnostic tool","parameters":{"type":"object","properties":{"q":{"type":"string"}}}}]`
	responsesPayload := fmt.Sprintf(`{"model":"gemini-3.7-flash","instructions":%q,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":%q}]}],%s,"max_output_tokens":128,"stream":%t}`,
		ompSystemPromptOpening, userText, tools, false)
	chatTools := `"tools":[{"type":"function","function":{"name":"run_probe","description":"Diagnostic tool","parameters":{"type":"object","properties":{"q":{"type":"string"}}}}}],`
	chatPayload := func(stream bool) string {
		return fmt.Sprintf(`{"model":"gemini-3.7-flash","messages":[{"role":"system","content":%q},{"role":"user","content":%q}],%s"max_tokens":128,"stream":%t}`,
			ompSystemPromptOpening, userText, chatTools, stream)
	}

	cases := []struct {
		name    string
		path    string
		payload string
		format  sdktranslator.Format
		stream  bool
	}{
		{name: "responses execute", path: "execute", payload: responsesPayload, format: sdktranslator.FormatOpenAIResponse},
		{name: "chat execute", path: "execute", payload: chatPayload(false), format: sdktranslator.FormatOpenAI},
		{name: "responses stream", path: "stream", payload: responsesPayload, format: sdktranslator.FormatOpenAIResponse, stream: true},
		{name: "chat stream", path: "stream", payload: chatPayload(true), format: sdktranslator.FormatOpenAI, stream: true},
		{name: "responses count", path: "count", payload: responsesPayload, format: sdktranslator.FormatOpenAIResponse},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			executor := NewAntigravityExecutor(&config.Config{RequestRetry: 1})
			req := cliproxyexecutor.Request{Model: "gemini-3.7-flash", Payload: []byte(tc.payload)}
			opts := cliproxyexecutor.Options{SourceFormat: tc.format, ResponseFormat: tc.format, Stream: tc.stream}
			switch tc.path {
			case "execute":
				if _, errExec := executor.Execute(context.Background(), auth, req, opts); errExec != nil {
					t.Fatalf("Execute() error = %v", errExec)
				}
			case "stream":
				result, errStream := executor.ExecuteStream(context.Background(), auth, req, opts)
				if errStream != nil {
					t.Fatalf("ExecuteStream() error = %v", errStream)
				}
				for chunk := range result.Chunks {
					if chunk.Err != nil {
						t.Fatalf("stream chunk error = %v", chunk.Err)
					}
				}
			case "count":
				if _, errCount := executor.CountTokens(context.Background(), auth, req, opts); errCount != nil {
					t.Fatalf("CountTokens() error = %v", errCount)
				}
			}

			body := <-bodies
			systemText := ojson.GetBytes(body, "request.systemInstruction.parts.0.text").String()
			assertNoDefaultFingerprint(t, "upstream system instruction", systemText)
			if !strings.Contains(systemText, "\u034F") {
				t.Fatalf("upstream system instruction has no CGJ marker: %s", body)
			}
			texts := antigravityContentTexts(body, "request.contents")
			if !reflect.DeepEqual(texts, []string{userText}) {
				t.Fatalf("upstream contents = %#v, want unchanged user text", texts)
			}
			if desc := ojson.GetBytes(body, "request.tools.0.functionDeclarations.0.description").String(); desc != "Diagnostic tool" {
				t.Fatalf("upstream tool description = %q, want unchanged", desc)
			}
			if typ := antigravityToolPropertyType(body); !strings.EqualFold(typ, "string") {
				t.Fatalf("upstream tool schema changed: %s", body)
			}
			for _, text := range texts {
				if strings.Contains(text, "\u034F") {
					t.Fatalf("obfuscation leaked into contents: %q", text)
				}
			}
		})
	}
}

// TestAntigravityDefaultSensitiveWordsUserPayloadRulesAppliedLast enforces the
// payload-finality contract: built-in obfuscation runs first, then user rules
// may replace or remove the system instruction without restoration.
func TestAntigravityDefaultSensitiveWordsUserPayloadRulesAppliedLast(t *testing.T) {
	const rawOverride = ompSystemPromptOpening + "\nClaude Agent SDK reintroduced by user rule"
	const userText = "hello"
	modelRules := []config.PayloadModelRule{{Name: "gemini-3.7-flash", Protocol: "antigravity"}}
	payload := []byte(fmt.Sprintf(`{"model":"gemini-3.7-flash","instructions":%q,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":%q}]}],"max_output_tokens":128}`, ompSystemPromptOpening, userText))

	rules := []struct {
		name string
		cfg  config.PayloadConfig
	}{
		{name: "override", cfg: config.PayloadConfig{Override: []config.PayloadRule{{
			Models: modelRules,
			Params: map[string]any{"systemInstruction.parts.0.text": rawOverride},
		}}}},
		{name: "filter", cfg: config.PayloadConfig{Filter: []config.PayloadFilterRule{{
			Models: modelRules,
			Params: []string{"systemInstruction.parts.0.text"},
		}}}},
	}
	paths := []string{"execute", "stream", "count"}
	for _, rule := range rules {
		for _, path := range paths {
			t.Run(rule.name+"/"+path, func(t *testing.T) {
				server, bodies := newAntigravityDefaultsUpstream(t)
				auth := antigravityDefaultsAuth(server.URL)
				executor := NewAntigravityExecutor(&config.Config{RequestRetry: 1, Payload: rule.cfg})
				req := cliproxyexecutor.Request{Model: "gemini-3.7-flash", Payload: payload}
				opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatOpenAIResponse, ResponseFormat: sdktranslator.FormatOpenAIResponse, Stream: path == "stream"}
				switch path {
				case "execute":
					if _, errExec := executor.Execute(context.Background(), auth, req, opts); errExec != nil {
						t.Fatalf("Execute() error = %v", errExec)
					}
				case "stream":
					result, errStream := executor.ExecuteStream(context.Background(), auth, req, opts)
					if errStream != nil {
						t.Fatalf("ExecuteStream() error = %v", errStream)
					}
					for chunk := range result.Chunks {
						if chunk.Err != nil {
							t.Fatalf("stream chunk error = %v", chunk.Err)
						}
					}
				case "count":
					if _, errCount := executor.CountTokens(context.Background(), auth, req, opts); errCount != nil {
						t.Fatalf("CountTokens() error = %v", errCount)
					}
				}

				body := <-bodies
				systemText := ojson.GetBytes(body, "request.systemInstruction.parts.0.text")
				switch rule.name {
				case "override":
					if systemText.String() != rawOverride {
						t.Fatalf("user override was overwritten or obfuscated: %q", systemText.String())
					}
					if strings.Contains(systemText.String(), "\u034F") {
						t.Fatalf("built-in obfuscation was reapplied after the user rule: %q", systemText.String())
					}
				case "filter":
					if systemText.Exists() {
						t.Fatalf("filtered system instruction was restored: %q", systemText.String())
					}
				}
			})
		}
	}
}
