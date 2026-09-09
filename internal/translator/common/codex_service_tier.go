package common

import (
	"strings"

	"github.com/tidwall/gjson"
)

// codexServiceTierAliases canonicalizes the client-facing speed identifiers that
// are known to name the same upstream tier. Codex exposes the fast lane as
// "fast" in config.toml and as "additional_speed_tiers", while the model catalog
// and the Responses payload both name it "priority".
var codexServiceTierAliases = map[string]string{
	"fast":     "priority",
	"priority": "priority",
}

// NormalizeCodexServiceTier resolves the service_tier value a Codex request
// should carry, reporting false when there is no tier to forward.
//
// The tier vocabulary is owned by the upstream model catalog, not by this proxy:
// clients populate their speed selector from the catalog the proxy serves, and
// that catalog grows new tiers without any change here. Values outside the known
// aliases are therefore forwarded unchanged rather than removed, because dropping
// a tier the client explicitly selected downgrades the request to the default
// lane with no error and no signal to the caller. An unusable value is left for
// the upstream to reject so the failure stays observable.
func NormalizeCodexServiceTier(serviceTier gjson.Result) (string, bool) {
	if serviceTier.Type != gjson.String {
		return "", false
	}

	trimmed := strings.TrimSpace(serviceTier.String())
	if trimmed == "" {
		return "", false
	}
	if canonical, ok := codexServiceTierAliases[strings.ToLower(trimmed)]; ok {
		return canonical, true
	}
	return trimmed, true
}
