package common

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestNormalizeCodexServiceTier(t *testing.T) {
	tests := []struct {
		name     string
		rawJSON  string
		want     string
		wantForw bool
	}{
		{name: "priority is canonical", rawJSON: `{"service_tier":"priority"}`, want: "priority", wantForw: true},
		{name: "fast alias canonicalizes to priority", rawJSON: `{"service_tier":"fast"}`, want: "priority", wantForw: true},
		{name: "alias match ignores case and padding", rawJSON: `{"service_tier":"  Fast "}`, want: "priority", wantForw: true},
		{name: "tier unknown to this build is forwarded", rawJSON: `{"service_tier":"ultra"}`, want: "ultra", wantForw: true},
		{name: "unknown tier keeps its original casing", rawJSON: `{"service_tier":"Flex"}`, want: "Flex", wantForw: true},
		{name: "unknown tier is trimmed", rawJSON: `{"service_tier":" standard "}`, want: "standard", wantForw: true},
		{name: "missing tier has nothing to forward", rawJSON: `{}`},
		{name: "empty tier has nothing to forward", rawJSON: `{"service_tier":""}`},
		{name: "blank tier has nothing to forward", rawJSON: `{"service_tier":"   "}`},
		{name: "null tier has nothing to forward", rawJSON: `{"service_tier":null}`},
		{name: "boolean tier has nothing to forward", rawJSON: `{"service_tier":true}`},
		{name: "numeric tier has nothing to forward", rawJSON: `{"service_tier":1}`},
		{name: "object tier has nothing to forward", rawJSON: `{"service_tier":{"id":"priority"}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, forward := NormalizeCodexServiceTier(gjson.Get(tt.rawJSON, "service_tier"))
			if forward != tt.wantForw {
				t.Fatalf("NormalizeCodexServiceTier(%s) forward = %v, want %v", tt.rawJSON, forward, tt.wantForw)
			}
			if got != tt.want {
				t.Fatalf("NormalizeCodexServiceTier(%s) = %q, want %q", tt.rawJSON, got, tt.want)
			}
		})
	}
}
