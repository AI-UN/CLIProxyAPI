package responses

import (
	"context"
	"testing"

	"github.com/tidwall/gjson"
)

func TestConvertOpenAIResponsesRequestToOpenAIChatCompletions_ForwardsServiceTier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		tierJSON   string
		want       string
		wantExists bool
	}{
		{name: "priority reaches the provider", tierJSON: `"priority"`, want: "priority", wantExists: true},
		{name: "tier unknown to this build reaches the provider", tierJSON: `"ultra"`, want: "ultra", wantExists: true},
		{name: "flex reaches the provider", tierJSON: `"flex"`, want: "flex", wantExists: true},
		{name: "absent tier stays absent"},
		{name: "non-string tier is omitted", tierJSON: `true`},
		{name: "null tier is omitted", tierJSON: `null`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			request := `{"model":"provider-model","input":[{"type":"message","role":"user","content":"hi"}]}`
			if tt.tierJSON != "" {
				request = `{"model":"provider-model","service_tier":` + tt.tierJSON + `,"input":[{"type":"message","role":"user","content":"hi"}]}`
			}

			out := ConvertOpenAIResponsesRequestToOpenAIChatCompletions("provider-model", []byte(request), false)
			serviceTier := gjson.GetBytes(out, "service_tier")
			if serviceTier.Exists() != tt.wantExists {
				t.Fatalf("service_tier exists = %v, want %v. Outgoing: %s", serviceTier.Exists(), tt.wantExists, out)
			}
			if tt.wantExists && serviceTier.String() != tt.want {
				t.Fatalf("service_tier = %q, want %q. Outgoing: %s", serviceTier.String(), tt.want, out)
			}
		})
	}
}

// A provider may serve a lower tier than the one requested. The Responses
// response must report what was served, not what was asked for, so a downgrade
// is visible to the caller instead of being masked by the request echo.
func TestConvertOpenAIChatCompletionsResponseToOpenAIResponses_ReportsServedServiceTier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		providerTier   string
		wantTier       string
		wantTierExists bool
	}{
		{name: "provider downgrade is reported", providerTier: `,"service_tier":"default"`, wantTier: "default", wantTierExists: true},
		{name: "provider confirmation is reported", providerTier: `,"service_tier":"priority"`, wantTier: "priority", wantTierExists: true},
		{name: "silent provider falls back to the requested tier", wantTier: "priority", wantTierExists: true},
	}

	// The outgoing request carries the requested tier, matching what the request
	// translator now forwards.
	const outgoing = `{"model":"provider-model","service_tier":"priority","messages":[{"role":"user","content":"hi"}]}`

	for _, tt := range tests {
		t.Run(tt.name+" (non-stream)", func(t *testing.T) {
			t.Parallel()

			reply := `{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"provider-model"` + tt.providerTier +
				`,"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`

			var param any
			out := ConvertOpenAIChatCompletionsResponseToOpenAIResponsesNonStream(
				context.Background(), "provider-model", nil, []byte(outgoing), []byte(reply), &param)

			serviceTier := gjson.GetBytes(out, "service_tier")
			if serviceTier.Exists() != tt.wantTierExists {
				t.Fatalf("service_tier exists = %v, want %v. Response: %s", serviceTier.Exists(), tt.wantTierExists, out)
			}
			if serviceTier.String() != tt.wantTier {
				t.Fatalf("service_tier = %q, want %q. Response: %s", serviceTier.String(), tt.wantTier, out)
			}
		})

		t.Run(tt.name+" (stream)", func(t *testing.T) {
			t.Parallel()

			chunks := []string{
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"provider-model"` + tt.providerTier +
					`,"choices":[{"index":0,"delta":{"role":"assistant","content":"ok"}}]}`,
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"provider-model"` + tt.providerTier +
					`,"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
				`data: [DONE]`,
			}

			var param any
			var completed gjson.Result
			for _, line := range chunks {
				for _, chunk := range ConvertOpenAIChatCompletionsResponseToOpenAIResponses(
					context.Background(), "provider-model", nil, []byte(outgoing), []byte(line), &param) {
					event, data := parseOpenAIResponsesSSEEvent(t, chunk)
					if event == "response.completed" {
						completed = data
					}
				}
			}
			if !completed.Exists() {
				t.Fatal("no response.completed event emitted")
			}

			serviceTier := completed.Get("response.service_tier")
			if serviceTier.Exists() != tt.wantTierExists {
				t.Fatalf("response.service_tier exists = %v, want %v. Event: %s", serviceTier.Exists(), tt.wantTierExists, completed.Raw)
			}
			if serviceTier.String() != tt.wantTier {
				t.Fatalf("response.service_tier = %q, want %q. Event: %s", serviceTier.String(), tt.wantTier, completed.Raw)
			}
		})
	}
}
