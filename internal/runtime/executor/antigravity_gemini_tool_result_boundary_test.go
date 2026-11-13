package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/MoeForge/moego/ojson"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/cache"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
)

const (
	antigravityBoundaryJSONReply  = `{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":1,"totalTokenCount":4}}}`
	antigravityBoundaryCountReply = `{"totalTokens":42}`
)

// antigravityBoundaryUpstream captures requests and enforces the expected wire
// boundary contract; it does not emulate Google's model behavior.
type antigravityBoundaryUpstream struct {
	mu      sync.Mutex
	bodies  [][]byte
	stream  bool
	violate func(body []byte) string
	reply   func() string
}

func newAntigravityBoundaryUpstream(t *testing.T, stream bool, violate func(body []byte) string) (*antigravityBoundaryUpstream, *httptest.Server) {
	t.Helper()
	return startAntigravityBoundaryUpstream(t, &antigravityBoundaryUpstream{stream: stream, violate: violate})
}

func startAntigravityBoundaryUpstream(t *testing.T, upstream *antigravityBoundaryUpstream) (*antigravityBoundaryUpstream, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, errRead := io.ReadAll(r.Body)
		if errRead != nil {
			t.Errorf("read upstream request body: %v", errRead)
			return
		}
		upstream.mu.Lock()
		upstream.bodies = append(upstream.bodies, body)
		upstream.mu.Unlock()
		if upstream.violate != nil {
			if msg := upstream.violate(body); msg != "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"error":{"code":400,"message":"`+msg+`","status":"INVALID_ARGUMENT"}}`)
				return
			}
		}
		if strings.Contains(r.URL.Path, "countTokens") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, antigravityBoundaryCountReply)
			return
		}
		reply := antigravityBoundaryJSONReply
		if upstream.reply != nil {
			reply = upstream.reply()
		}
		if upstream.stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: "+reply+"\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, reply)
	}))
	return upstream, server
}

func (u *antigravityBoundaryUpstream) captured() [][]byte {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([][]byte(nil), u.bodies...)
}

func antigravityBoundaryAuth(baseURL string) *cliproxyauth.Auth {
	auth := testAntigravityAuth(baseURL)
	auth.Metadata["project_id"] = "project-1"
	return auth
}

// antigravityLabeledTailViolation models the upstream contract: the final turn
// must be a user turn that still carries the functionResponse and names the
// answered function(s) as text.
func antigravityLabeledTailViolation(body []byte, wantLabel string) string {
	contents := ojson.GetBytes(body, "request.contents")
	if !contents.IsArray() {
		return "request.contents is missing"
	}
	turns := contents.Array()
	if len(turns) == 0 {
		return "request.contents is empty"
	}
	tail := turns[len(turns)-1]
	if role := tail.Get("role").String(); role != "user" {
		return "final turn role " + role + " is not user"
	}
	var names []string
	labeled := false
	for _, part := range tail.Get("parts").Array() {
		if response := part.Get("functionResponse"); response.Exists() {
			names = append(names, response.Get("name").String())
		}
		if text := part.Get("text"); text.Exists() && strings.TrimSpace(text.String()) == wantLabel {
			labeled = true
		}
	}
	if len(names) == 0 {
		return "final turn lost its functionResponse part"
	}
	if !labeled {
		return "final turn is missing the answered function name label"
	}
	if joined := strings.Join(names, "\n"); joined != wantLabel {
		return "label " + wantLabel + " does not name " + joined
	}
	return ""
}

func antigravityNativeToolResultPayload() []byte {
	return []byte(`{"contents":[` +
		`{"role":"user","parts":[{"text":"run read"}]},` +
		`{"role":"model","parts":[{"functionCall":{"id":"call-1","name":"read","args":{"path":"a.txt"}},"thoughtSignature":"` + issue4959GeminiThoughtSignature() + `"}]},` +
		`{"role":"user","parts":[{"functionResponse":{"id":"call-1","name":"read","response":{"result":"contents"}}}]}` +
		`]}`)
}

// antigravityGeminiTargetModels covers an ordinary Gemini target plus the bare
// and suffixed Argon names; the implementation lists none of them.
func antigravityGeminiTargetModels() []string {
	return []string{"gemini-3.8-flash", "gemini-3.8-flash-high", "gemini-2.5-flash", "gemini-4-argon", "gemini-4-argon-high"}
}

func assertAntigravityLabeledTail(t *testing.T, body []byte, wantLabel string) {
	t.Helper()
	if msg := antigravityLabeledTailViolation(body, wantLabel); msg != "" {
		t.Fatalf("%s; body=%s", msg, body)
	}
}

func TestAntigravityGeminiTerminalToolResultLabelNonStream(t *testing.T) {
	for _, model := range antigravityGeminiTargetModels() {
		t.Run(model, func(t *testing.T) {
			upstream, server := newAntigravityBoundaryUpstream(t, false, func(body []byte) string {
				return antigravityLabeledTailViolation(body, "read")
			})
			defer server.Close()

			payload := antigravityNativeToolResultPayload()
			exec := NewAntigravityExecutor(&config.Config{RequestRetry: 1})
			if _, errExec := exec.Execute(context.Background(), antigravityBoundaryAuth(server.URL), cliproxyexecutor.Request{
				Model:   model,
				Payload: payload,
			}, cliproxyexecutor.Options{
				SourceFormat:    sdktranslator.FormatGemini,
				ResponseFormat:  sdktranslator.FormatGemini,
				OriginalRequest: payload,
			}); errExec != nil {
				t.Fatalf("Execute() error = %v", errExec)
			}
			bodies := upstream.captured()
			if len(bodies) != 1 {
				t.Fatalf("upstream requests = %d, want 1", len(bodies))
			}
			assertAntigravityLabeledTail(t, bodies[0], "read")
		})
	}
}

func TestAntigravityGeminiTerminalToolResultLabelStream(t *testing.T) {
	for _, model := range antigravityGeminiTargetModels() {
		t.Run(model, func(t *testing.T) {
			upstream, server := newAntigravityBoundaryUpstream(t, true, func(body []byte) string {
				return antigravityLabeledTailViolation(body, "read")
			})
			defer server.Close()

			payload := antigravityNativeToolResultPayload()
			exec := NewAntigravityExecutor(&config.Config{RequestRetry: 1})
			result, errStream := exec.ExecuteStream(context.Background(), antigravityBoundaryAuth(server.URL), cliproxyexecutor.Request{
				Model:   model,
				Payload: payload,
			}, cliproxyexecutor.Options{
				SourceFormat:    sdktranslator.FormatGemini,
				ResponseFormat:  sdktranslator.FormatGemini,
				Stream:          true,
				OriginalRequest: payload,
			})
			if errStream != nil {
				t.Fatalf("ExecuteStream() error = %v", errStream)
			}
			for chunk := range result.Chunks {
				if chunk.Err != nil {
					t.Fatalf("stream chunk error = %v", chunk.Err)
				}
			}
			bodies := upstream.captured()
			if len(bodies) != 1 {
				t.Fatalf("upstream requests = %d, want 1", len(bodies))
			}
			assertAntigravityLabeledTail(t, bodies[0], "read")
		})
	}
}

func TestAntigravityGeminiTerminalToolResultLabelCountTokens(t *testing.T) {
	for _, model := range antigravityGeminiTargetModels() {
		t.Run(model, func(t *testing.T) {
			upstream, server := newAntigravityBoundaryUpstream(t, false, func(body []byte) string {
				return antigravityLabeledTailViolation(body, "read")
			})
			defer server.Close()

			payload := antigravityNativeToolResultPayload()
			exec := NewAntigravityExecutor(&config.Config{RequestRetry: 1})
			if _, errCount := exec.CountTokens(context.Background(), antigravityBoundaryAuth(server.URL), cliproxyexecutor.Request{
				Model:   model,
				Payload: payload,
			}, cliproxyexecutor.Options{
				SourceFormat:   sdktranslator.FormatGemini,
				ResponseFormat: sdktranslator.FormatGemini,
			}); errCount != nil {
				t.Fatalf("CountTokens() error = %v", errCount)
			}
			bodies := upstream.captured()
			if len(bodies) != 1 {
				t.Fatalf("upstream requests = %d, want 1", len(bodies))
			}
			assertAntigravityLabeledTail(t, bodies[0], "read")
		})
	}
}

// TestAntigravityGemini3ProNonStreamTerminalToolResultLabel covers the
// streamed-upstream non-stream path used by gemini-3-pro.
func TestAntigravityGemini3ProNonStreamTerminalToolResultLabel(t *testing.T) {
	upstream, server := newAntigravityBoundaryUpstream(t, true, func(body []byte) string {
		return antigravityLabeledTailViolation(body, "read")
	})
	defer server.Close()

	payload := antigravityNativeToolResultPayload()
	exec := NewAntigravityExecutor(&config.Config{RequestRetry: 1})
	if _, errExec := exec.Execute(context.Background(), antigravityBoundaryAuth(server.URL), cliproxyexecutor.Request{
		Model:   "gemini-3-pro",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat:    sdktranslator.FormatGemini,
		ResponseFormat:  sdktranslator.FormatGemini,
		OriginalRequest: payload,
	}); errExec != nil {
		t.Fatalf("Execute() error = %v", errExec)
	}
	bodies := upstream.captured()
	if len(bodies) != 1 {
		t.Fatalf("upstream requests = %d, want 1", len(bodies))
	}
	assertAntigravityLabeledTail(t, bodies[0], "read")
}

// TestAntigravityClaudeTerminalToolResultNotLabeled pins that Claude targets,
// which share the send paths but reject synthetic user text, are untouched.
func TestAntigravityClaudeTerminalToolResultNotLabeled(t *testing.T) {
	upstream, server := newAntigravityBoundaryUpstream(t, true, nil)
	defer server.Close()

	payload := []byte(`{"model":"claude-opus-5-5-high","max_tokens":1024,"messages":[` +
		`{"role":"user","content":"run read"},` +
		`{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"read","input":{"path":"a.txt"}}]},` +
		`{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"contents"}]}` +
		`],"tools":[{"name":"read","input_schema":{"type":"object","properties":{"path":{"type":"string"}}}}]}`)
	exec := NewAntigravityExecutor(&config.Config{RequestRetry: 1})
	if _, errExec := exec.Execute(context.Background(), antigravityBoundaryAuth(server.URL), cliproxyexecutor.Request{
		Model:   "claude-opus-5-5-high",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat:    sdktranslator.FormatClaude,
		ResponseFormat:  sdktranslator.FormatClaude,
		OriginalRequest: payload,
	}); errExec != nil {
		t.Fatalf("Execute() error = %v", errExec)
	}
	bodies := upstream.captured()
	if len(bodies) != 1 {
		t.Fatalf("upstream requests = %d, want 1", len(bodies))
	}
	turns := ojson.GetBytes(bodies[0], "request.contents").Array()
	if len(turns) == 0 {
		t.Fatalf("request.contents empty; body=%s", bodies[0])
	}
	for _, part := range turns[len(turns)-1].Get("parts").Array() {
		if text := part.Get("text"); text.Exists() && strings.TrimSpace(text.String()) == "read" {
			t.Fatalf("claude terminal tool-result turn gained the synthetic label: %s; body=%s", part.Raw, bodies[0])
		}
	}
}

func TestAntigravityGeminiTerminalToolResultHonorsPayloadRules(t *testing.T) {
	models := []config.PayloadModelRule{{Name: "gemini-3.8-flash-high"}}
	rules := []struct {
		name  string
		cfg   config.PayloadConfig
		check func(t *testing.T, body []byte)
	}{
		{
			name: "override wins over the label",
			cfg: config.PayloadConfig{Override: []config.PayloadRule{{Models: models, Params: map[string]any{
				"contents.2.role":         "model",
				"contents.2.parts.1.text": "custom_override",
			}}}},
			check: func(t *testing.T, body []byte) {
				t.Helper()
				if got := ojson.GetBytes(body, "request.contents.2.role").String(); got != "model" {
					t.Fatalf("role = %q, want the configured model role; body=%s", got, body)
				}
				if got := ojson.GetBytes(body, "request.contents.2.parts.1.text").String(); got != "custom_override" {
					t.Fatalf("label = %q, want the configured text; body=%s", got, body)
				}
			},
		},
		{
			name: "filter removes the label without restoring it",
			cfg: config.PayloadConfig{Filter: []config.PayloadFilterRule{{Models: models, Params: []string{
				"contents.2.role",
				"contents.2.parts.1",
			}}}},
			check: func(t *testing.T, body []byte) {
				t.Helper()
				if role := ojson.GetBytes(body, "request.contents.2.role"); role.Exists() {
					t.Fatalf("filtered role was restored: %s; body=%s", role.Raw, body)
				}
				parts := ojson.GetBytes(body, "request.contents.2.parts").Array()
				if len(parts) != 1 || parts[0].Get("text").Exists() {
					t.Fatalf("filtered label was restored: %s; body=%s", ojson.GetBytes(body, "request.contents.2.parts").Raw, body)
				}
			},
		},
	}
	for _, rule := range rules {
		for _, path := range []string{"execute", "count"} {
			t.Run(rule.name+"/"+path, func(t *testing.T) {
				upstream, server := newAntigravityBoundaryUpstream(t, false, nil)
				defer server.Close()

				payload := antigravityNativeToolResultPayload()
				exec := NewAntigravityExecutor(&config.Config{RequestRetry: 1, Payload: rule.cfg})
				switch path {
				case "execute":
					if _, errExec := exec.Execute(context.Background(), antigravityBoundaryAuth(server.URL), cliproxyexecutor.Request{
						Model:   "gemini-3.8-flash-high",
						Payload: payload,
					}, cliproxyexecutor.Options{
						SourceFormat:    sdktranslator.FormatGemini,
						ResponseFormat:  sdktranslator.FormatGemini,
						OriginalRequest: payload,
					}); errExec != nil {
						t.Fatalf("Execute() error = %v", errExec)
					}
				default:
					if _, errCount := exec.CountTokens(context.Background(), antigravityBoundaryAuth(server.URL), cliproxyexecutor.Request{
						Model:   "gemini-3.8-flash-high",
						Payload: payload,
					}, cliproxyexecutor.Options{
						SourceFormat:   sdktranslator.FormatGemini,
						ResponseFormat: sdktranslator.FormatGemini,
					}); errCount != nil {
						t.Fatalf("CountTokens() error = %v", errCount)
					}
				}
				bodies := upstream.captured()
				if len(bodies) != 1 {
					t.Fatalf("upstream requests = %d, want 1", len(bodies))
				}
				rule.check(t, bodies[0])
			})
		}
	}
}

func antigravityReplayTurnPayload(model, extraCall, extraOutput string) []byte {
	return []byte(`{` +
		`"model":"` + model + `",` +
		`"session_id":"antigravity-terminal-tool-result-replay",` +
		`"input":[` +
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"start"}]},` +
		`{"type":"function_call","call_id":"call_1","name":"TaskList","arguments":"{}"},` +
		`{"type":"function_call_output","call_id":"call_1","output":"running"}` + extraCall + extraOutput +
		`],` +
		`"tools":[` +
		`{"type":"function","name":"TaskList","parameters":{"type":"object","properties":{}}},` +
		`{"type":"function","name":"exec_command","parameters":{"type":"object","properties":{"cmd":{"type":"string"}}}}` +
		`]` +
		`}`)
}

// antigravityContentTextParts returns the text parts of a content.
func antigravityContentTextParts(content ojson.Result) []string {
	var texts []string
	for _, part := range content.Get("parts").Array() {
		if text := part.Get("text"); text.Exists() {
			texts = append(texts, text.String())
		}
	}
	return texts
}

// antigravityContentWithFunctionCall returns the model content that issues the
// named tool call, which reasoning replay restores.
func antigravityContentWithFunctionCall(body []byte, name string) (ojson.Result, bool) {
	for _, content := range ojson.GetBytes(body, "request.contents").Array() {
		for _, part := range content.Get("parts").Array() {
			if call := part.Get("functionCall"); call.Exists() && call.Get("name").String() == name {
				return content, true
			}
		}
	}
	return ojson.Result{}, false
}

func TestAntigravityGeminiTerminalToolResultKeepsReplayAcrossTurns(t *testing.T) {
	signature := issue4959GeminiThoughtSignature()
	for _, stream := range []bool{false, true} {
		for _, model := range []string{"gemini-3.8-flash-high", "gemini-4-argon-high"} {
			name := model
			if stream {
				name += "/stream"
			} else {
				name += "/non-stream"
			}
			t.Run(name, func(t *testing.T) {
				cache.ClearAntigravityReasoningReplayCache()
				t.Cleanup(cache.ClearAntigravityReasoningReplayCache)

				turn := 0
				upstream, server := startAntigravityBoundaryUpstream(t, &antigravityBoundaryUpstream{
					stream: stream,
					reply: func() string {
						turn++
						if turn == 1 {
							return `{"response":{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"native-call-2","name":"exec_command","args":{"cmd":"echo hi"}},"thoughtSignature":"` + signature + `"}]},"finishReason":"STOP"}]}}`
						}
						return `{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"all done"}]},"finishReason":"STOP"}]}}`
					},
				})
				defer server.Close()

				exec := NewAntigravityExecutor(&config.Config{RequestRetry: 1})
				opts := cliproxyexecutor.Options{
					SourceFormat:   sdktranslator.FormatOpenAIResponse,
					ResponseFormat: sdktranslator.FormatOpenAIResponse,
					Stream:         stream,
				}
				runTurn := func(payload []byte) {
					t.Helper()
					if stream {
						result, errStream := exec.ExecuteStream(context.Background(), antigravityBoundaryAuth(server.URL), cliproxyexecutor.Request{Model: model, Payload: payload}, opts)
						if errStream != nil {
							t.Fatalf("ExecuteStream() error = %v", errStream)
						}
						for chunk := range result.Chunks {
							if chunk.Err != nil {
								t.Fatalf("stream chunk error = %v", chunk.Err)
							}
						}
						return
					}
					if _, errExec := exec.Execute(context.Background(), antigravityBoundaryAuth(server.URL), cliproxyexecutor.Request{Model: model, Payload: payload}, opts); errExec != nil {
						t.Fatalf("Execute() error = %v", errExec)
					}
				}

				runTurn(antigravityReplayTurnPayload(model, "", ""))
				runTurn(antigravityReplayTurnPayload(model,
					`,{"type":"function_call","call_id":"native-call-2","name":"exec_command","arguments":"{\"cmd\":\"echo hi\"}"},`,
					`{"type":"function_call_output","call_id":"native-call-2","output":"hi"}`,
				))

				bodies := upstream.captured()
				if len(bodies) != 2 {
					t.Fatalf("upstream requests = %d, want 2", len(bodies))
				}
				assertAntigravityLabeledTail(t, bodies[0], "TaskList")
				assertAntigravityLabeledTail(t, bodies[1], "exec_command")

				replayed, found := antigravityContentWithFunctionCall(bodies[1], "exec_command")
				if !found {
					t.Fatalf("replayed exec_command call missing; body=%s", bodies[1])
				}
				if got := replayed.Get("parts.0.thoughtSignature").String(); got != signature {
					t.Fatalf("replayed thoughtSignature = %q, want %q (contextHash drift); body=%s", got, signature, bodies[1])
				}

				turns := ojson.GetBytes(bodies[1], "request.contents").Array()
				for index, content := range turns[:len(turns)-1] {
					for _, text := range antigravityContentTextParts(content) {
						if strings.TrimSpace(text) == "TaskList" || strings.TrimSpace(text) == "exec_command" {
							t.Fatalf("historical contents[%d] leaked the terminal label %q; body=%s", index, text, bodies[1])
						}
					}
				}
			})
		}
	}
}
