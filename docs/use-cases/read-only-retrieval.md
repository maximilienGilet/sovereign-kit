# Read-only retrieval-assisted answers

This runbook adds **client-owned retrieval** in front of the private inference route. The client selects documents the current user is allowed to see, sends only those passages to Qwen, and keeps the model read-only.

Sovereign Kit does not provide document ingestion, embeddings, a vector database, tenant isolation, or permission checks. It only receives the bounded context chosen by the client.

## Architecture

```text
user → client identity → permission check → retrieval → approved passages
     → private inference route → cited draft answer
```

The permission check comes before prompt construction. A system prompt cannot correct an unauthorized document that has already entered the context.

## Prerequisites

- working [server setup](../server.md), SSH tunnel, and passing `sovkit doctor`;
- a client-owned document source and user identity system;
- a policy that maps the current user to document IDs or scopes;
- a retention/deletion policy for prompts, answers, retrieval logs, and source copies.

Start read-only. Do not give the model write access to a source system, vector store, shell, browser, or business API.

## Test with the included example

The example uses a local JSON file, not a vector database. That is deliberate: it makes the authorization boundary visible.

Create a local non-sensitive fixture:

```json
[
  {"id": "travel-policy", "text": "Rail bookings are allowed for approved business travel."},
  {"id": "finance-private", "text": "This record must not be sent for this user."}
]
```

Open the route and verify it:

```bash
sovkit doctor
```

Then run the example with an explicit allowlist:

```bash
python3 examples/read-only-retrieval/answer.py \
  --documents documents.json \
  --allow-id travel-policy \
  --question 'What travel option is allowed?'
```

The program filters records by `--allow-id` before constructing the prompt. It sends neither `finance-private` nor any document omitted from the allowlist.

This is a reference shape, not a retrieval engine. Replace the JSON lookup with the client’s own search or vector retrieval only after its authorization boundary is enforceable.

## Build the production path

1. Authenticate the user in the client application.
2. Resolve the user’s document scopes or permissions server-side.
3. Retrieve only records inside those scopes.
4. Apply a strict context size limit and preserve source IDs.
5. Construct a prompt containing the question and selected passages only.
6. Send the request to one explicit model route, with no provider fallback.
7. Return an answer with source IDs or links the user can inspect.
8. Record only the audit information permitted by the client retention policy.

For a vector database, enforce tenant and access filters in the retrieval service. Do not accept a caller-provided `tenant_id` as the only protection. For stronger isolation, use separate indexes, collections, or deployments according to the client’s risk decision.

## Prompt and answer policy

Ask the model to answer only from supplied sources and cite source IDs. Treat that as a quality instruction, not a security control.

The client must decide what to do when:

- retrieval returns no allowed source;
- the answer has no citation;
- the model contradicts a source;
- the context is too large;
- an index is stale or permission metadata changes.

A safe initial behaviour is to return “no approved source found” rather than guessing.

## Data and operations

Document where each of these lives: original documents, extracted chunks, embeddings, vector metadata, prompt logs, answers, access logs, model server cache, and backups. The inference route’s provider/region review does not automatically cover a retrieval database or document store.

Before client work:

- run `sovkit doctor`;
- use a non-sensitive fixture to verify the real retrieval filter;
- inspect a generated prompt to confirm excluded documents are absent;
- verify the source IDs shown to the user map to the actual retrieved passages.

After client work, stop the tunnel and model server according to [Operations](../operations.md), then apply the client’s retention/deletion policy to retrieval and chat data.

## Troubleshooting

### An unauthorized source appears in the prompt

Stop the workflow. Fix filtering in the client retrieval layer; do not try to repair it with a stricter prompt.

### The answer uses facts outside supplied sources

Show source citations, reject or flag uncited answers, and use an application-level verification step for high-stakes content.

### The request is too slow or context is too large

Reduce retrieved chunks, shorten passages, or use a profile measured for the intended context. Do not infer capacity from the model name or GPU class; use the [benchmark contract](../benchmark-contract.md).

### The endpoint is unavailable

Check `sovkit doctor`, the SSH tunnel, and the remote SGLang service. Do not silently fail over to a public API.

## Out of scope

This guide does not ship a vector database, embeddings endpoint, document connector, shared tenant model, identity provider, permission engine, retention service, or regulated-records workflow.
