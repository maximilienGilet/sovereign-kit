# Lite European API candidate

This is a **candidate profile**, not a released provider integration. Its purpose is a lower-operations route for smaller workloads when a self-managed GPU is not the right fit.

Lite is still one explicit route: one provider origin, one model/version, one authentication path, and no automatic fallback. It is not a generic router and it does not inherit the privacy, region, retention, or performance claims of the self-managed Studio route.

## Choose the contract before the provider

Select and record these facts before writing any client configuration:

| Decision | Required record |
|---|---|
| Provider endpoint | Exact HTTPS origin and API version |
| Processing location | Provider’s current documented region for the selected endpoint, plus the date checked |
| Model | Exact identifier and version/revision policy |
| Authentication | Secret source, rotation owner, and which client adapters can consume it |
| Data handling | Current retention/training policy and the contract required for the intended material |
| Capacity | Rate limit, context ceiling, output cap, latency, and error behaviour measured for the account |
| Failure | Explicit user-visible failure; no fallback to another provider |
| Cost | Input/output pricing basis, spend ceiling, and owner |

Provider nationality alone does not establish processing region. An endpoint region alone does not establish retention, DPA coverage, or a legal compliance conclusion. See [Choosing an inference route](../choosing-an-inference-route.md).

## Release gates

Do not ship a Lite profile, wrapper, or example until:

1. the chosen provider has an OpenAI-compatible endpoint verified with the target client;
2. the exact base URL and selected model are pinned in a non-secret profile;
3. credentials are kept outside Git and delivered to every supported client without weakening its provider lock;
4. the endpoint fails closed when unavailable or rate-limited;
5. Pi compatibility is tested if Pi is claimed. The current shared Pi route cannot simply inherit an authenticated endpoint;
6. a real workload measures latency, rate limits, context handling, and error recovery;
7. the provider/region/retention review is recorded with its source date;
8. a cost range is calculated from realistic token volumes, not headline per-token pricing.

## Initial use

Start Lite with generic OpenAI-compatible applications or an OpenCode-only adapter if its secret delivery is tested. Do not describe it as a Pi route, shared-team service, or sovereign deployment until those specific properties are implemented and measured.
