package helps

import (
	"bytes"
	"testing"

	"github.com/MoeForge/moego/ojson"
)

const (
	antigravityReadToolResult  = `{"functionResponse":{"id":"call-1","name":"read","response":{"result":"ok"}}}`
	antigravityWriteToolResult = `{"functionResponse":{"id":"call-2","name":"write","response":{"result":"ok"}}}`
)

func TestEnsureAntigravityGeminiToolResultBoundaryDoc(t *testing.T) {
	tests := []struct {
		name      string
		contents  string
		changed   bool
		wantRole  string
		wantParts int
		wantLabel string
	}{
		{
			name:      "terminal model role tool result becomes user with label",
			contents:  `[{"role":"model","parts":[` + antigravityReadToolResult + `]}]`,
			changed:   true,
			wantRole:  "user",
			wantParts: 2,
			wantLabel: "read",
		},
		{
			name:      "terminal user role tool result gains label",
			contents:  `[{"role":"user","parts":[` + antigravityReadToolResult + `]}]`,
			changed:   true,
			wantRole:  "user",
			wantParts: 2,
			wantLabel: "read",
		},
		{
			name:      "parallel tool results join their names",
			contents:  `[{"role":"model","parts":[` + antigravityReadToolResult + `,` + antigravityWriteToolResult + `]}]`,
			changed:   true,
			wantRole:  "user",
			wantParts: 3,
			wantLabel: "read\nwrite",
		},
		{
			name:      "whitespace sibling text is preserved before the label",
			contents:  `[{"role":"model","parts":[` + antigravityReadToolResult + `,{"text":" \n "}]}]`,
			changed:   true,
			wantRole:  "user",
			wantParts: 3,
			wantLabel: "read",
		},
		{
			name:     "non-whitespace sibling text is never re-labeled",
			contents: `[{"role":"model","parts":[` + antigravityReadToolResult + `,{"text":"continue"}]}]`,
		},
		{
			name:     "historical tool result followed by user turn is untouched",
			contents: `[{"role":"model","parts":[` + antigravityReadToolResult + `]},{"role":"user","parts":[{"text":"next"}]}]`,
		},
		{
			name:     "nested media tool result is untouched",
			contents: `[{"role":"model","parts":[{"functionResponse":{"name":"view_image","response":{"result":""},"parts":[{"inlineData":{"mimeType":"image/png","data":"AA=="}}]}}]}]`,
		},
		{
			name:     "missing function name is untouched",
			contents: `[{"role":"model","parts":[{"functionResponse":{"id":"call-1","response":{"result":"ok"}}}]}]`,
		},
		{
			name:     "thought sibling is untouched",
			contents: `[{"role":"model","parts":[{"thought":true,"text":"planning"},` + antigravityReadToolResult + `]}]`,
		},
		{
			name:     "function call sibling is untouched",
			contents: `[{"role":"model","parts":[` + antigravityReadToolResult + `,{"functionCall":{"name":"write","args":{}}}]}]`,
		},
		{
			name:     "plain text turn is untouched",
			contents: `[{"role":"user","parts":[{"text":"answer"}]}]`,
		},
		{
			name:     "empty parts is untouched",
			contents: `[{"role":"model","parts":[]}]`,
		},
		{
			name:     "non-array parts is untouched",
			contents: `[{"role":"model","parts":` + antigravityReadToolResult + `}]`,
		},
		{
			name:     "empty history is untouched",
			contents: `[]`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			payload := []byte(`{"request":{"contents":` + tc.contents + `}}`)
			doc := ojson.NewDoc(payload)
			changed := EnsureAntigravityGeminiToolResultBoundaryDoc(doc)
			if changed != tc.changed {
				t.Fatalf("changed = %t, want %t; doc=%s", changed, tc.changed, doc.Bytes())
			}
			got := doc.Bytes()
			if !tc.changed {
				if !bytes.Equal(got, payload) {
					t.Fatalf("document changed unexpectedly: %s", got)
				}
				return
			}
			turns := ojson.GetBytes(got, "request.contents").Array()
			last := turns[len(turns)-1]
			if role := last.Get("role").String(); role != tc.wantRole {
				t.Fatalf("role = %q, want %q; doc=%s", role, tc.wantRole, got)
			}
			parts := last.Get("parts").Array()
			if len(parts) != tc.wantParts {
				t.Fatalf("parts = %d, want %d; doc=%s", len(parts), tc.wantParts, got)
			}
			if label := parts[len(parts)-1].Get("text").String(); label != tc.wantLabel {
				t.Fatalf("label = %q, want %q; doc=%s", label, tc.wantLabel, got)
			}
			if again := EnsureAntigravityGeminiToolResultBoundaryDoc(doc); again {
				t.Fatalf("second call reported a change; doc=%s", doc.Bytes())
			}
			if !bytes.Equal(doc.Bytes(), got) {
				t.Fatalf("second call mutated the document: %s", doc.Bytes())
			}
		})
	}
}

func TestEnsureAntigravityGeminiToolResultBoundaryDocPreservesHistory(t *testing.T) {
	payload := []byte(`{"request":{"contents":[` +
		`{"role":"user","parts":[{"text":"run read"}]},` +
		`{"role":"model","parts":[{"functionCall":{"id":"call-1","name":"read","args":{"path":"a.txt"}},"thoughtSignature":"sig"}]},` +
		`{"role":"model","parts":[` + antigravityReadToolResult + `]}` +
		`]}}`)
	doc := ojson.NewDoc(payload)
	if !EnsureAntigravityGeminiToolResultBoundaryDoc(doc) {
		t.Fatalf("terminal tool-result turn was not labeled: %s", doc.Bytes())
	}
	got := doc.Bytes()
	for _, path := range []string{"request.contents.0", "request.contents.1", "request.contents.2.parts.0"} {
		if before, after := ojson.GetBytes(payload, path).Raw, ojson.GetBytes(got, path).Raw; before != after {
			t.Fatalf("%s mutated: before=%s after=%s", path, before, after)
		}
	}
}

func TestEnsureAntigravityGeminiToolResultBoundaryDocMissingInputs(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "nil doc"},
		{name: "no contents", payload: `{"request":{}}`},
		{name: "contents not an array", payload: `{"request":{"contents":{"role":"model"}}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var doc *ojson.Doc
			if tc.payload != "" {
				doc = ojson.NewDoc([]byte(tc.payload))
			}
			if EnsureAntigravityGeminiToolResultBoundaryDoc(doc) {
				t.Fatal("helper reported a change for an unsupported document")
			}
		})
	}
}
