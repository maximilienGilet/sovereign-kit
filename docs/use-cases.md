# Use cases

Sovereign Kit is a private inference route, not a coding-agent product. The current repository ships validated client adapters only for Pi and OpenCode, but the loopback endpoint uses an OpenAI-compatible protocol and can be consumed by other approved clients.

The route boundary remains the same:

```text
approved client → Mac loopback → SSH tunnel → GPU-host loopback → approved model server
```

A compatible client does not automatically become a supported Sovereign Kit adapter. Validate its provider lock, secret handling, tool behavior, and egress before client work.

## 1. Coding agents — validated adapter path

Pi, Oh-My-Pi, pi-subagents, and OpenCode are the V1 adapters. Follow [Quick start](quickstart.md), then use `sovkit doctor` and Pi’s `/subagents-models` check before working with client material.

## 2. Internal text assistant — client-owned UI

An internal chat UI or service can call the local OpenAI-compatible endpoint if the client is deliberately configured with this exact base URL and model. The example below is a connectivity shape, not a full application or access-control solution:

```bash
curl --fail --silent --show-error \
  http://127.0.0.1:30000/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer local-qwen-tunnel' \
  -d '{
    "model": "qwen3.8-27b-nvfp4",
    "messages": [{"role": "user", "content": "Summarize this approved text."}]
  }'
```

The UI, user identity, document permissions, conversation storage, and retention are outside the current kit. Do not point a shared browser application directly at this keyless V1 endpoint.

## 3. Retrieval-assisted answers — client-side retrieval

A client may retrieve approved documents in its own environment, build a bounded context, and send that context to the route for generation. Sovereign Kit does not currently provide embeddings, a vector database, document ingestion, tenant isolation, or permission filtering.

Keep retrieval read-only at first. The caller must enforce document permissions before material enters the model context. A shared index needs tenant-scoped enforcement in the retrieval service, not a prompt instruction.

## 4. Automation — client executes actions

A workflow can ask the model for structured advice or a proposed tool call, then validate and execute the resulting action inside its own worker. Sovereign Kit does not execute shell commands, browser actions, web search, MCP tools, or business-system writes on behalf of the model.

For sensitive actions, keep human approval and business-rule validation outside the model.

## Guides

- [Coding agents](quickstart.md) — the V1 adapter path for Pi and OpenCode.
- [Internal text assistant](use-cases/internal-text-assistant.md) — client-owned UI and access control.
- [Read-only retrieval-assisted answers](use-cases/read-only-retrieval.md) — client-owned permissions and retrieval.
- [Controlled automation](use-cases/controlled-automation.md) — model proposal, client validation, client execution.

## Not included in V1

- shared multi-user chat;
- managed RAG or a shared vector store;
- browser, shell, web-search, or MCP execution;
- multi-provider routing and automatic fallbacks;
- tenant identity, quotas, audit logging, or policy management.

Those need separate identity, egress, secret, storage, and approval controls. Private inference is not private autonomous agents.
