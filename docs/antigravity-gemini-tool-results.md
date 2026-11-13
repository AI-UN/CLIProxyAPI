# Antigravity Gemini tool-result boundary

## Symptom and scope

An OMP request through Antigravity Gemini can fail after a tool result with `400: Requests ending with a model turn are not supported.` The client's captured request ends with a `user` tool-result turn, but Antigravity normalization changes pure `functionResponse` turns to `model`. The referenced Argon patch reports a related failure: the endpoint accepts the request, streams thoughts, then aborts with `400 INVALID_ARGUMENT: Request contains an invalid argument.`

This fork normalizes that terminal turn for **all Antigravity Gemini-family targets** (including Gemini 4 Argon and Gemini 3.8 Flash), not only Argon. Claude targets on Antigravity, direct Gemini API/AI Studio, and Vertex AI paths are untouched.

## Normalization

Only the last turn is considered, and only when its parts are `functionResponse` values with non-empty names, optionally accompanied by whitespace-only text parts. The turn is then rewritten as:

- role `user` instead of `model`;
- the existing parts, followed by one `text` part containing the answered function names joined by newlines.

This boundary repair does not change earlier turns, tool call IDs, tool outputs, or thought signatures; existing translation and signature-replay behavior still applies. Turns containing non-empty text, media, thought parts, or `functionCall` parts are left unchanged, so mixed text/media tool-result turns keep their existing handling. Requests without a trailing pure tool-result turn pass through this repair untouched. The label reuses names already present in the request and adds no instructions or duplicated tool output.

## Reasoning-replay interaction

The reasoning-replay cache indexes requests by their pre-turn context so later turns can match stored thought signatures against historical content. The replay snapshot is taken **before** the label is applied, so the extra text part never enters the indexed context and `contextHash` matching for historical turns stays intact on subsequent turns.

## Where it applies

- non-streaming `Execute`;
- streaming `ExecuteStream`;
- non-streaming requests that the executor serves from an upstream stream;
- `CountTokens`, which normalizes the same contents before counting.

User-configured payload rules still run after this normalization and remain the final authority on the outgoing body.

## Evidence and limits

The original report came from a real failed Antigravity tool-use request, and the reporter confirmed the serving backend was Antigravity Gemini. Verification in this fork is limited to controlled tests against a fake upstream with synthetic payloads: no live Antigravity account was used, and no private request text, logs, or signatures are reproduced here. The change repairs one specific rejected request shape; it does not claim to fix every `400` response or to be the only cause of invalid-argument failures.

## Provenance

- [PR #7156](https://github.com/router-for-me/CLIProxyAPI/pull/7156), head [`ce03f2b9abd9`](https://github.com/router-for-me/CLIProxyAPI/commit/ce03f2b9abd9ca3ad1640dde4a7c2b7c0d102cb4), author Zhe Nan — original normalization with replay-drift avoidance, scoped to `gemini-4-argon`.
- [PR #6935](https://github.com/router-for-me/CLIProxyAPI/pull/6935) — predecessor revision of the same fix.
- This fork ports the behavior to the whole Antigravity Gemini family. A literal cherry-pick was not viable: the PR edits byte helpers built on `tidwall/sjson`, while current code normalizes through `moego/ojson` `Doc` helpers, so the rules were ported onto the refactored call sites.
