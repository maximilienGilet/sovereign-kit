# Choosing an inference route

A coding agent can call a model through several very different arrangements. They are not interchangeable, even when each one is described as "private", "European", or "zero retention".

This guide compares the public controls documented on 2026-09-01. It is a technical comparison, not legal, GDPR, or procurement advice. A provider's nationality is not proof of where a particular request is processed.

## Short version

| Route | Best when | Main trade-off |
|---|---|---|
| **Sovereign Kit with a self-managed model server** | You need one explicit route from coding agents to an approved model host | You operate the GPU, server, SSH, model lifecycle, capacity, and incident response |
| **Routed API, such as OpenRouter** | You need broad model choice and fast iteration | The router and selected provider are part of the data path; controls must be configured per request or account |
| **European API, such as Mistral EU or Scaleway** | You want a managed API with documented European processing properties | You still depend on a provider contract, endpoint scope, model availability, and API limits |
| **Closed-model API, such as OpenAI or Claude** | Model quality, tools, and time-to-value matter more than a self-managed route | Standard API defaults are not the same thing as an explicit private inference path |

## 1. Sovereign Kit: one explicit model route

Sovereign Kit is deliberately narrow. Pi, Oh-My-Pi, pi-subagents, and OpenCode call one local port. An SSH forward carries that traffic to one Qwen/SGLang process bound to loopback on a GPU host.

The local profiles include one model provider. Pi subagents are restricted to `sovereign-qwen/*`. The OpenCode wrapper injects a single-provider configuration that takes precedence over a project configuration.

That gives an operator a small, inspectable route:

```text
coding agent → Mac loopback → SSH tunnel → GPU-host loopback → approved model server
```

It does **not** prove the GPU provider's contractual terms, data region, disk handling, logs, egress, availability, or legal compliance. It also does not sandbox agent tools and plugins. Read [Architecture](architecture.md) and [Security](security.md) for the actual boundary.

Choose this path when the first problem is simple: "run this coding agent against this approved model host, without a silent model-provider fallback." Do not turn it into a general multi-provider router before that path is reliable and useful.

## 2. Routed APIs: OpenRouter

OpenRouter sits between the application and one of many model-provider endpoints. That makes experimentation easy, but it means the router and the selected upstream provider both matter.

### Controls documented by OpenRouter

- OpenRouter says it does not store prompt or completion content unless the account opts in, and it does not use inputs or outputs for model training. It retains request metadata such as token counts and latency. [Data collection](https://openrouter.ai/docs/guides/privacy/data-collection) · [Privacy policy](https://openrouter.ai/privacy)
- The upstream provider receives the request. Provider data policies vary by endpoint; OpenRouter says its policy labels are informative rather than a definitive third-party policy source. [Provider logging](https://openrouter.ai/docs/guides/privacy/provider-logging)
- `provider.zdr: true` limits a request to endpoints declared Zero Data Retention. `provider.data_collection: "deny"` filters for providers that do not collect user data. These settings do not cover external tools such as web search. [ZDR](https://openrouter.ai/docs/guides/features/zdr) · [Provider selection](https://openrouter.ai/docs/guides/routing/provider-selection)
- `provider.only` can allowlist exact provider slugs. `provider.order` plus `allow_fallbacks: false` can prevent an implicit fallback. A base provider slug can cover several endpoints or regions, so use the complete endpoint slug when the deployment needs a specific one. [Provider selection](https://openrouter.ai/docs/guides/routing/provider-selection)
- EU in-region routing is an Enterprise feature enabled on request through `https://eu.openrouter.ai`. OpenRouter says prompts and completions then stay in the chosen EU or US region. Its documentation does not describe a per-inference geographic attestation returned to the caller. [Provider logging](https://openrouter.ai/docs/guides/privacy/provider-logging)

### Practical reading

OpenRouter can be a good managed routing layer when its account features, endpoint allowlist, fallback policy, region, retention setting, and commercial terms match the workload.

It is not the same thing as Sovereign Kit's single inspected route. A generic OpenRouter request can be automatically balanced across eligible providers. Make routing explicit rather than assuming a privacy setting or a European company name answers every question.

## 3. Managed European API routes

### Mistral regional inference

Mistral documents `https://api.eu.mistral.ai` for inference processed in EU/EFTA data centres. Its model catalogue includes `codestral` and `devstral` for code and software-engineering work. [Regional inference](https://docs.mistral.ai/inference/regional-inference) · [Models](https://docs.mistral.ai/models)

The EU endpoint and Zero Data Retention are separate controls. Regional inference alone does not state that no data is retained. Mistral also documents limits: the regional endpoint costs 1.1× the standard rate; account, billing, analytics, and other control-plane data may sit outside the inference geography; Agents, Batch, and Files APIs are unavailable on the regional endpoint. [Regional inference](https://docs.mistral.ai/inference/regional-inference)

Use the EU hostname explicitly and check model availability against that hostname before rollout. Do not rely on a fallback to the global endpoint if the region matters.

### Scaleway Generative APIs

Scaleway documents its serverless endpoint, `https://api.scaleway.ai/v1`, as hosted in Paris, France, with a stated commitment to remain in Europe if more EU locations are added. Its catalogue includes several coding-oriented models, including Devstral and Qwen Coder variants. [FAQ](https://www.scaleway.com/en/docs/generative-apis/faq/) · [Supported models](https://www.scaleway.com/en/docs/generative-apis/reference-content/supported-models/)

Scaleway states that serverless requests use Zero Data Retention by default and that prompts and outputs are not read, reused, analysed, or used to train the models. Its public policy documents temporary HTTP content retention after abnormal errors or malicious activity for up to two weeks, plus aggregated anonymised metrics for up to six months. [Data privacy](https://www.scaleway.com/en/docs/generative-apis/reference-content/data-privacy/)

Serverless is still a public Internet API, not a private VPC endpoint. Scaleway recommends Dedicated Deployment when a deployment needs a single region, private networking, access control, a fixed model version, or a model of its own. Dedicated capacity is billed while allocated. [FAQ](https://www.scaleway.com/en/docs/generative-apis/faq/)

### Practical reading

A European managed API may be the fastest defensible route when self-hosting is unnecessary. It can also be a better fit than a broad router for a client that needs one provider, one documented region, and fewer moving parts.

It still needs workload-specific validation: model availability, rate limits, retention exception handling, tool calls, network access, contract, DPA, and an explicit fallback policy.

## 4. Closed-model APIs: OpenAI and Claude

### OpenAI API

OpenAI says API inputs and outputs are not used to train its models by default. Standard API abuse-monitoring logs may retain prompts, outputs, and metadata for up to 30 days. Some API features also create persistent application state. [Your data](https://developers.openai.com/api/docs/guides/your-data)

For eligible customers, Modified Abuse Monitoring and Zero Data Retention require approval and additional contractual conditions. OpenAI also documents regional project controls, including an EU endpoint, for eligible services under the relevant retention configuration. These controls have endpoint and service limits; they do not cover every system datum, third-party service, or request flow. [Your data](https://developers.openai.com/api/docs/guides/your-data)

### Anthropic API and Claude Enterprise

Anthropic says commercial prompts, data, and results are not used to train its models by default. It states that business/Enterprise content is governed by customer agreements rather than the consumer privacy policy. Claude Enterprise advertises data-retention controls, but the public sources reviewed here do not specify a universal Zero Data Retention mode or a per-request European processing selection for the direct Anthropic API. [Enterprise](https://claude.com/solutions/enterprise) · [Commercial terms](https://www.anthropic.com/legal/commercial-terms) · [Privacy policy](https://www.anthropic.com/legal/privacy)

### Practical reading

The standard OpenAI and Claude API defaults do not meet Sovereign Kit's strict technical idea of a private inference route. Commercial controls may improve the posture substantially, but they require an endpoint-by-endpoint, region-by-region, and contract-by-contract review.

That does not make these APIs unsuitable. They are often the right choice when their model capability, managed tooling, and speed outweigh the need to operate a specific private route.

## Decision guide

Use this order instead of picking from vendor claims:

1. **What must be true about the route?** Decide whether you need an explicit approved host, a documented EU inference endpoint, no durable content retention, a particular model, or only a commercial no-training commitment.
2. **What can leave the developer machine?** Include repositories, prompts, tool outputs, web-search queries, Git remotes, and package-manager traffic, not only model text.
3. **Who owns operations?** A self-managed route trades API simplicity for model and infrastructure control. A managed API does the opposite.
4. **What is the explicit fallback?** Keep it inside the same approved provider/region, or fail closed. Do not let the SDK or router silently choose a different route.
5. **What needs to be written down?** Record the endpoint, model/version, region, retention setting, fallback, provider terms, client approval, and the stop/deletion process.

For Sovereign Kit's current scope, the answer is intentionally modest: one coding agent route, one approved model endpoint, and no silent model-provider fallback. A company-wide multi-model router is a different product with different security, tenancy, billing, policy, and support obligations.
