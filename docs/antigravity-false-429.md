# Antigravity false-429 compatibility

## Symptom and evidence

Antigravity can return `429 RESOURCE_EXHAUSTED` with the generic message `Resource has been exhausted (e.g. check quota).` even when a neutral request succeeds with the same account and model. That response alone does **not** identify the cause or prove that quota remains available.

[CLIProxyAPI issue #5751](https://github.com/router-for-me/CLIProxyAPI/issues/5751) and [oh-my-pi issue #11699](https://github.com/can1357/oh-my-pi/issues/11699) contain reported controlled comparisons where changing only coding-agent text in `systemInstruction` changed the result from 429 to 200. A recurring example is:

```text
<system-conventions>
RFC 2119: MUST, REQUIRED, SHOULD, RECOMMENDED, MAY, OPTIONAL.
```

These reports support a content-sensitive upstream rejection. They do not establish Google's internal filtering mechanism or intent, and they do not mean every Antigravity 429 is content-related. The official client succeeding is a useful comparison, but it may send a different system instruction.

This has recurred with newer wording: merged [OMP PR #13379](https://github.com/can1357/oh-my-pi/pull/13379), commit [`6274932f46f7`](https://github.com/can1357/oh-my-pi/commit/6274932f46f73a29ab927b5f6d4e6aedbf19659d), changed `RFC 2119:` to `RFC 2119 keywords:`. [OMP #18706](https://github.com/can1357/oh-my-pi/issues/18706), reported on November 10, describes that new wording failing while the previous wording succeeds. Matching the `RFC 2119` phrase covers both variants without maintaining whole prompt templates.

The OMP message `Provider requested 1800000ms wait, exceeds retry.maxDelayMs (300000ms)` is a separate concern: [OMP PR #11693](https://github.com/can1357/oh-my-pi/pull/11693) traces this 30-minute delay to local quota-classification heuristics for a detail-free Google error. [OMP PR #11924](https://github.com/can1357/oh-my-pi/pull/11924) proposes distinguishing an OMP-estimated wait from an actual provider deadline. Do not infer a provider-supplied 30-minute deadline from that wording alone.

## Compared remedies

Repository and PR states below were checked on 2026-11-13. Reported live results belong to the linked authors; they are not live-provider verification of this fork.

| Remedy and primary source | Strength | Limitation / status |
| --- | --- | --- |
| Configure `sensitive-words`, as reported in [CPA #5751](https://github.com/router-for-me/CLIProxyAPI/issues/5751) | No code change; works centrally for clients using the proxy; preserves non-matching prompt text | Operator must discover and maintain the phrases. Old releases use U+200B, which later reports found insufficient for some identities |
| Client extension or prompt-tag rename in [OMP #11699](https://github.com/can1357/oh-my-pi/issues/11699); shipped wording change in [#13379](https://github.com/can1357/oh-my-pi/pull/13379) | Very small, supported by controlled prompt-only comparisons; #13379 merged | Client-specific and brittle: [#18706](https://github.com/can1357/oh-my-pi/issues/18706) reports the replacement wording becoming a new trigger. Replacing the whole system prompt discards unrelated instructions |
| Default system-only fingerprints in [CPA PR #6280](https://github.com/router-for-me/CLIProxyAPI/pull/6280), original commit [`248d1124b3c6`](https://github.com/hertzhzzz/CLIProxyAPI/commit/248d1124b3c61d84fab35d10a489bef3807e700c), and downstream replay [`92c1ee5906ff`](https://github.com/router-for-me/CLIProxyAPI/commit/92c1ee5906ffbc8908ddb340a83be0e4977636bb) | Central default mitigation for OMP and Claude Agent SDK text; user messages and tools stay untouched | PR open, not merged into the inspected upstream main. Older implementation rebuilds a matcher and uses the older obfuscation API; port the intent, not the old code |
| Provider-side system-only obfuscation in [OMP PR #11730](https://github.com/can1357/oh-my-pi/pull/11730), commit [`1d03e330accc`](https://github.com/can1357/oh-my-pi/commit/1d03e330accc29c6610a6ac0c0b7bd5162126d40) | Antigravity-scoped; configurable `RFC 2119` default; author reports full-prompt live A/B success without changing `requestType` | Closed without merge. Client-side alternative, not a CPA fix; uses literal case-sensitive U+200B handling |
| Remove `requestType: "agent"`, proposed in [OMP PR #11742](https://github.com/can1357/oh-my-pi/pull/11742) and included in [#11693](https://github.com/can1357/oh-my-pi/pull/11693) | Reported to avoid the problematic agent-mode route without rewriting prompt text | Both PRs closed without merge. Changes the upstream routing contract; equivalence to the official client is not established here. Not selected for this fork |
| Generic 429 retry reclassification in [OMP PR #11693](https://github.com/can1357/oh-my-pi/pull/11693) | Avoids treating ambiguous boilerplate as definitive daily exhaustion; retains structured quota signals in the revised proposal | Closed without merge. A retry sends the same triggering content and cannot repair deterministic rejection |
| Newer capacity classification in [OMP PR #12684](https://github.com/can1357/oh-my-pi/pull/12684), head [`97e3d4ce335b`](https://github.com/can1357/oh-my-pi/commit/97e3d4ce335b4b77d157469df9bc83c47167c927) | Open; bounded capacity backoff instead of the 30-minute quota heuristic; revised guard preserves account-scoped credit/quota signals | Changes recovery, not rejected content. Early revisions accidentally suppressed real credit exhaustion, illustrating why generic 429 reclassification is not selected here |
| Nonce and system-to-user fold fallback in [OMP PR #11918](https://github.com/can1357/oh-my-pi/pull/11918), head [`d5b7cbb24bb2`](https://github.com/can1357/oh-my-pi/commit/d5b7cbb24bb2627930a71594fc167f47aececeb6) | Open; varies convention tags and can recover without discarding prompt text; later revision makes the nonce session-stable | Folding moves higher-priority system instructions into a user turn and adds stateful retries. Nonces can affect prompt caching; changing tags alone does not cover the latest RFC-only trigger. Not ported |
| Retry once as `requestType: "chat"` in [OMP PR #12437](https://github.com/can1357/oh-my-pi/pull/12437), behavior commit [`8304e6e39a56`](https://github.com/can1357/oh-my-pi/commit/8304e6e39a5658b59b10547595f5f13bb2270c51) | Open; leaves healthy agent requests unchanged; author reports successful full-prompt streaming and tool-use comparisons | Additional upstream attempt and a different routing contract based on an ambiguous 429; downstream capability/metering equivalence is not established. Not ported |
| Fingerprint warning in [CPA PR #5790](https://github.com/router-for-me/CLIProxyAPI/pull/5790) | Makes known unmasked text discoverable; keeps payload changes opt-in | Open; diagnostic only, does not repair requests or establish the cause of an arbitrary 429 |
| v8 configuration automation in [ai-launcher PR #14](https://github.com/Ak1Ena/ai-launcher/pull/14), commit [`506abbeaf896`](https://github.com/Ak1Ena/ai-launcher/commit/506abbeaf8966e53d4168c811638a3ee5f2fae3c) | Merged; applies the existing mechanism during setup and preserves a nonempty user list | Requires that launcher and the v8 config API; preserving an incomplete user list can leave the trigger unhandled |
| U+034F per-subword obfuscation and literal replacement in upstream commit [`9a7e1e6b4127`](https://github.com/router-for-me/CLIProxyAPI/commit/9a7e1e6b412747729dedecefd7e23bd2e3ee1d3d), addressing [CPA #7428](https://github.com/router-for-me/CLIProxyAPI/issues/7428) | Already on this patch's upstream base; handles the newer reported U+200B limitation and supports precise identity replacement | Still opt-in upstream: an empty configuration creates no matcher. This is the residual gap addressed here |
| Independent proxy masking in [antigravity-claude-proxy-go PR #156](https://github.com/gustavokch/antigravity-claude-proxy-go/pull/156), commit [`4917878f311b`](https://github.com/gustavokch/antigravity-claude-proxy-go/commit/4917878f311be581a32640348947b8cbc001793f) | Recent independent report: U+034F masking changed the captured OMP request from 429 to successful completion; preserves the native identity block | Open, not deployed in the report; its phrase set is now covered by this fork's defaults apart from `Claude Agent SDK`, which only this fork adds; separate tri-state configuration. Its retry/recovery changes are unrelated and are not ported here |

### Detailed comparison with antigravity-claude-proxy-go #156

This comparison pins [PR #156](https://github.com/gustavokch/antigravity-claude-proxy-go/pull/156) to head `4917878f311be581a32640348947b8cbc001793f` and was first written against this fork at `e08a46343f2ed5952dda61914fcbf1d5bace4e97`, where only six default phrases were present. The coverage table reflects the fork's **current** defaults; the historical gap is noted below it. The PR is still open. It contains two separate commits: [`d1e6746`](https://github.com/gustavokch/antigravity-claude-proxy-go/commit/d1e674643632b5baf74a98405603c9d0568cabd0) changes recovery/error delivery; [`4917878`](https://github.com/gustavokch/antigravity-claude-proxy-go/commit/4917878f311be581a32640348947b8cbc001793f) adds masking.

With both mask sets extended, the defaults now overlap almost completely:

| Coverage | Phrases |
| --- | --- |
| Both (eight) | `RFC 2119`, `system-conventions`, `system_conventions`, `system-directive`, `system_directive`, `MUST, REQUIRED, SHOULD, RECOMMENDED, MAY, OPTIONAL`, `Hermes Agent`, `Nous Research` |
| Only #156 | none |
| Only this fork | `Claude Agent SDK` |

The [pinned mask implementation](https://github.com/gustavokch/antigravity-claude-proxy-go/blob/4917878f311be581a32640348947b8cbc001793f/internal/format/system_mask.go#L11-L80) explicitly ports CLIProxyAPI's algorithm: case-insensitive literal matching, longest phrase first, and U+034F after the first rune of each alphanumeric subword. It is not a new obfuscation algorithm.

Historically the gap mattered for the current OMP line: at `e08a4634` our default masked only the two subwords in `RFC 2119`, while #156 also masked the six subwords in the keyword enumeration. The fork now masks both, so neither side covers more of that line. Its broader phrase coverage was a plausible extra safeguard rather than proved necessity: the reported successful full-request A/B changed the complete configured mask, not one phrase at a time, so that evidence does not show keyword-enumeration masking is required in addition to masking `RFC 2119`.

The extension is therefore a low-cost parity alignment with an independently reported mask, not a verified requirement.

| Boundary | #156 | This fork |
| --- | --- | --- |
| Configuration | Absent/null uses defaults; `[]` disables; a nonempty list replaces defaults | Configured words are additive; empty list keeps defaults |
| Exact replacement | Mask only; no phrase-to-replacement map in `SystemMask` | Existing `sensitive-words-replace` supports neutral rewrites, deletion, and identity opt-out |
| Reconfiguration | Atomically swaps a compiled mask at startup/config save | Existing per-executor matcher cache keys configured words/replacements |
| Rewritten text | Client system parts only, before joining with native identity text | Translated system-instruction text only, before final user payload rules |
| Native identity | Existing builder prepends its agy identity and an ignore-copy; new mask deliberately excludes both | This patch adds no identity blocks or official-client envelope changes |

Sources: [configuration semantics](https://github.com/gustavokch/antigravity-claude-proxy-go/blob/4917878f311be581a32640348947b8cbc001793f/internal/config/antigravity.go#L5-L19), [builder and atomic mask](https://github.com/gustavokch/antigravity-claude-proxy-go/blob/4917878f311be581a32640348947b8cbc001793f/internal/format/builder.go#L22-L69), and this fork's `AntigravityExecutor.getSensitiveWordMatcher` / `helps.ObfuscateSensitiveWordsInSystemInstructionDoc`.

The external builder also strips lines containing `x-anthropic-billing-header`, injects native identity text, derives native-looking request/session metadata, and manages output limits. These are existing builder behaviors, **not additional innovations introduced by #156**. CLIProxyAPI already filters Claude attribution system blocks on its Claude-to-Antigravity path; the rules are not identical (block-prefix filtering versus case-insensitive line filtering). Neither difference justifies replacing system instructions or porting the other builder wholesale.

The three complementary #156 phrases are now part of this fork's built-in defaults, so no configuration is needed to get them. An empty or absent `sensitive-words` list keeps the full set active. The full keyword enumeration is one literal phrase, not six globally masked words; punctuation/spacing changes can prevent a match. Broader masking changes more system text and is not a guarantee against normalization or future filtering. To opt a phrase out, map it to itself with `sensitive-words-replace`; see the configuration section. No deployment settings were changed during this comparison.


#### Recovery and error delivery are a separate change

The first commit in #156 adds a request-local recovery window capped at `min(10s, maxWait)`. It is armed after certain failures or blocked account selection, shared across subsequent retries, and disarmed before the first upstream event is forwarded. A recovery delay that would consume the remaining budget is refused instead of hidden inside a long wait. Healthy initial attempts do not start this timer. See the [pinned dispatcher](https://github.com/gustavokch/antigravity-claude-proxy-go/blob/4917878f311be581a32640348947b8cbc001793f/internal/accounts/dispatcher.go#L447-L641) and [recovery window](https://github.com/gustavokch/antigravity-claude-proxy-go/blob/4917878f311be581a32640348947b8cbc001793f/internal/accounts/recovery.go#L27-L127).

This is not safe to copy directly into CLIProxyAPI: after the window is armed, its cancellation context also covers `StreamGenerateContent`. The timer can cancel a retry whose connection is established but has not delivered its first event. CLIProxyAPI's repository policy forbids that post-connection timeout. Limiting scheduler waits or admission to another attempt is a different design from timing out a connected request.

The same PR improves failure visibility: typed HTTP 429 and retry advice before response commitment; a classified SSE error after commitment; propagation of empty-stream completion errors; and stopping when downstream writes or flushes fail. Current CLIProxyAPI already has typed streaming errors, incomplete-stream detection, and trusted retry-header handling. The patch we implemented intentionally leaves all of those mechanisms unchanged. These are largely parity improvements for that proxy, not missing functionality proved by the false-429 report.

One narrower item merits a separate regression investigation: this checkout's Claude SSE handler ignores `Write` errors in `sdk/api/handlers/claude/code_handlers.go` and does not supply the error-returning flush hook used by the OpenAI Responses handler. That suggests it can keep consuming until request cancellation after a downstream write fails. This is source-based inference, not a reproduced defect; no fix or test was added during this comparison.

The [latest review comment on the pinned head](https://github.com/gustavokch/antigravity-claude-proxy-go/pull/156#issuecomment-6839498084) reports three outstanding problems: credential/project-discovery retry paths that do not arm the window, retry advice that ignores a longer session-pin constraint when shared throttling exists, and malformed sensitive-word configuration being saved before rejection. No later commit was present when checked. These are external reviewer reports, not locally reproduced results.

The author reports live success for one captured OMP request, but the PR still lists deployed interactive OMP/tool-use and requesting-client controlled-429 visibility as open acceptance gates. That is stronger evidence than mocks alone, not proof that every added phrase is necessary or that the whole recovery patch is production-ready. No source code, deployment configuration, or refs were changed for this comparison; no provider calls or test/build reruns were performed.

## Fork behavior

This patch ports the six default phrases from CPA PR #6280 onto upstream main `1aaf56bc6a0fc6fce19049ba7ae8554c882304a6`, then extends the list with the three phrases independently used by antigravity-claude-proxy-go #156. Nine defaults are active:

- `Claude Agent SDK`
- `system-conventions`
- `system_conventions`
- `system-directive`
- `system_directive`
- `RFC 2119`
- `MUST, REQUIRED, SHOULD, RECOMMENDED, MAY, OPTIONAL`
- `Hermes Agent`
- `Nous Research`

The keyword enumeration is matched as one literal phrase; `MUST`, `SHOULD`, and the other keywords are not masked where they appear alone. The six-phrase set was the fork's initial state at `e08a4634`; the extra three are the later extension described above.

The existing Antigravity matcher inserts U+034F after the first rune of each matching alphanumeric subword. Matching is case-insensitive. Only text in `request.systemInstruction` or `request.system_instruction` is processed; ordinary conversation content and tool schemas are not rewritten. The rules are literal phrase matches, not a reconstruction of Google's filter.

Configured `sensitive-words` remain additive. `sensitive-words-replace` retains its existing longest-match, single-pass behavior and takes precedence for matching targets. Effective matchers are cached; the default phrase list is combined with configured words only when the matcher must be rebuilt. Runtime defaults do not modify or persist the user's configuration.

The shared matcher is used by non-streaming, streaming, and token-count paths. Processing remains before final user payload rules: an explicit payload override can reintroduce a literal phrase, and a filtered system instruction stays removed. Those intentional overrides may therefore restore the upstream rejection.

There is no change to OAuth, project discovery, endpoints, `requestType`, retries, cooldowns, credits, or real quota errors. No generic 429 is relabeled as a content rejection.

## Configuration

On this fork, the nine defaults need no configuration. An empty or absent `sensitive-words` list does **not** disable built-in handling.

On an unpatched upstream release with the current matcher, the corresponding manual workaround uses the v8 layout:

```yaml
config-version: 8
oauth:
  providers:
    antigravity:
      sensitive-words:
        - "Claude Agent SDK"
        - "system-conventions"
        - "system_conventions"
        - "system-directive"
        - "system_directive"
        - "RFC 2119"
        - "MUST, REQUIRED, SHOULD, RECOMMENDED, MAY, OPTIONAL"
        - "Hermes Agent"
        - "Nous Research"
```

Merge the provider settings into the existing configuration; do not replace the whole file. Legacy-layout files place `antigravity` at the top level instead of under `oauth.providers`.

For an additional, specifically observed identity rejection, current upstream also supports literal replacement. For example, [CPA #7428](https://github.com/router-for-me/CLIProxyAPI/issues/7428) reports the Codex identity sentence below; it is **not** added to this patch's default list:

```yaml
oauth:
  providers:
    antigravity:
      sensitive-words-replace:
        "You are Codex, an agent based on GPT-5.": "You are a helpful coding assistant."
```

To preserve one built-in phrase verbatim, map that exact phrase to itself using `sensitive-words-replace`, for example `"RFC 2119": "RFC 2119"`. This deliberately opts that phrase out of obfuscation and can reintroduce the false 429. An empty replacement string instead deletes the matched phrase; it is not an opt-out.

## Limits and verification

This is a narrow compatibility mitigation, not a guarantee against future upstream filtering. Invisible characters change the transmitted system text and can affect exact-string instructions or downstream comparisons. Use literal replacements or an identity replacement when that tradeoff is inappropriate.

**Account risk:** merged [OMP PR #18622](https://github.com/can1357/oh-my-pi/pull/18622) paused its built-in Antigravity provider after reports of Google account restrictions. Using an OpenAI-compatible proxy does not establish permission to use the service from third-party clients and does not remove that risk. This patch neither re-enables OMP's disabled provider nor verifies Google's terms or enforcement policy. The [independent proxy PR](https://github.com/gustavokch/antigravity-claude-proxy-go/pull/156) explicitly warns about the same risk.

Regression coverage uses a controlled local upstream to inspect the real executor request, including default handling, preserved user/tool content, explicit replacement precedence, and final payload override/filter ordering. Such checks prove the outbound request contract, not Google's current acceptance policy. Live-provider verification requires the affected account, model, and client payload; do not publish tokens or full private prompts when diagnosing it.

If a neutral prompt also fails, or structured `ErrorInfo` / `RetryInfo` identifies a real quota limit, investigate that evidence rather than assuming this mitigation applies. Increasing the retry-delay cap or rotating credentials does not remove a deterministic content trigger.
