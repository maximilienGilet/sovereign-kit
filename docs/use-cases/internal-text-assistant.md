# Internal text assistant

This runbook turns one Sovereign Kit route into a small, client-owned text assistant. It is for drafting, summarising, translating, classifying, or answering from text explicitly supplied in a request.

It is **not** a shared chat product, a document platform, or an access-control system. Sovereign Kit supplies the inference route; the client application owns every user-facing and business control.

## Architecture

```text
approved user
  → client-owned UI or script
  → local approved client on the Mac
  → 127.0.0.1:30000 SSH forward
  → GPU-host loopback SGLang/Qwen
```

The inference port must remain bound to loopback on both machines. Do not expose it through a LAN address, reverse proxy, browser application, or public firewall rule.

## What you need

- a working [server setup](../server.md) and open `sovkit-tunnel`;
- a passing `sovkit doctor` result;
- an approved text-only workflow;
- a client-owned place to enforce login, permissions, retention, and audit.

The shared Pi + OpenCode V1 route is keyless at the model API layer. This pattern is suitable for a local, approved client on the same Mac account. It is not suitable for pointing a shared browser UI directly at the endpoint.

## Start and verify the route

On the GPU host, start the digest-locked reference recipe:

```bash
./server/run-sglang.sh
```

On the Mac, open the tunnel in one terminal:

```bash
sovkit-tunnel <ssh-host> <ssh-port> <tunnel-user> \
  <identity-file> <known-hosts-file>
```

In another terminal:

```bash
sovkit doctor
```

Do not continue if it reports a failure. A local endpoint failure normally means the tunnel or remote SGLang service is not running.

## Run the supplied minimal client

The example is a standard-library Python program. It sends only the supplied `--text` value and does not save conversation history.

```bash
python3 examples/internal-text-assistant/assistant.py \
  --text 'Rewrite this approved paragraph in clear French: ...'
```

Expected behaviour is one text answer on standard output. The example is intentionally small: it demonstrates the OpenAI-compatible request shape, not an application design.

To point it at a non-default local test endpoint, use:

```bash
python3 examples/internal-text-assistant/assistant.py \
  --endpoint http://127.0.0.1:30000/v1 \
  --model qwen3.8-27b-nvfp4 \
  --text 'Approved text only.'
```

The installed route, `sovkit doctor`, and the example must all refer to the same local endpoint and model identity.

## Build a real client safely

Keep this boundary in the client application:

| Concern | Owner |
|---|---|
| Model inference route | Sovereign Kit |
| User identity and session | Client application |
| Permission to send a document | Client application before prompt construction |
| Conversation history and deletion | Client application storage policy |
| Prompt construction and source selection | Client application |
| Rate limits and abuse controls | Client application or private gateway |
| Audit trail | Client application or client platform |

For a first production workflow, send a request with one approved task and only the text necessary to perform it. Display that text, or a clear reference to it, to the user before submission where appropriate.

Do not rely on a system prompt to enforce permissions. If a user cannot read a document, its content must never be placed in the request.

## Data flow and retention

Record, for the specific client deployment:

1. where the UI runs;
2. where request text is assembled and stored;
3. the Qwen server host, provider/region approval, image digest, model revision, and shutdown process;
4. whether the UI, application logs, reverse proxy, tunnel client, GPU host, or provider retains request/response content;
5. how a user can delete locally stored conversation content.

The route does not establish a provider contract, data-processing agreement, legal residency claim, client-side storage policy, or user access policy. Review those separately before sending client material.

## Operational checks

Before a client session:

1. Confirm `sovkit doctor` passes.
2. Confirm the SSH terminal remains open and has no host-key warning.
3. Send a non-sensitive test prompt through the real client.
4. Verify the expected model identity in the client configuration.
5. Record the active image digest and model revision outside the repository if the record identifies a client deployment.

After a session:

1. Stop the client.
2. Stop the SSH tunnel with `Ctrl-C`.
3. Stop the remote SGLang process.
4. Stop or destroy the GPU instance according to the client’s agreed procedure.
5. Apply the client application’s history/log deletion policy.

Stopping the tunnel does not stop the server or provider billing.

## Troubleshooting

### `sovkit doctor` cannot reach the endpoint

Verify the server process on the GPU host, then restart `sovkit-tunnel`. Do not bypass a changed SSH host key or change the server bind to `0.0.0.0`.

### The example reports HTTP 401 or 403

The shared V1 Pi + OpenCode route must be keyless at the SGLang API layer. An authenticated endpoint is outside this supported route until secret injection for Pi has been implemented and validated.

### The example reports an unexpected response

Check that the target is an OpenAI-compatible chat-completions endpoint and that the configured model exists. Do not silently add a public API fallback.

### A response appears to contain information a user should not see

Stop the workflow. Treat it as an application permission or context-selection failure, not a prompt-tuning problem. Remove the unauthorized source from the client-side retrieval/context builder before resuming.

## Capacity and cost

The published Studio evidence is historical synthetic work, not an assistant SLO or cost promise. The profile can hold long contexts, but concurrent prefills can materially increase first-token time. Measure realistic prompt size, expected simultaneous users, output length, GPU runtime, idle time, storage, and support effort before selecting a cost model.

See [Route profiles](../profiles.md) and the [benchmark contract](../benchmark-contract.md) before making capacity or price commitments.

## Out of scope

This runbook does not provide:

- user accounts or multi-user tenancy;
- a shared chat frontend;
- document ingestion, embeddings, or a vector database;
- browser, shell, web-search, MCP, or tool execution;
- automatic model/provider fallback;
- compliance certification or a legal residency guarantee.
