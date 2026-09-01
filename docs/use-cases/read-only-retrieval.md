# Read-only retrieval-assisted answers

Use a client-owned retrieval step to select approved passages, then ask the private route to draft an answer from that bounded context.

## When it fits

- internal policy or procedure lookup;
- project-document Q&A;
- answering from a curated set of client documents.

## Route

```text
user → client-owned permission check → client-owned retrieval → bounded context → Sovereign Kit route
```

Sovereign Kit only performs generation. It does not currently ship embeddings, ingestion, a vector database, document storage, access controls, or a shared index.

## Safe first version

1. Keep retrieval read-only.
2. Enforce permissions before a passage reaches the prompt.
3. Put document identifiers and source excerpts in the client request deliberately.
4. Ask the model to cite only the supplied sources.
5. Treat the answer as a draft; preserve source links for human verification.

## Do not assume

- a prompt can enforce document permissions;
- one vector index is safely shared between clients or teams;
- the model will refuse unsupported claims without an application-level check;
- retrieval traffic has the same retention or region policy as the inference route.

A multi-user or shared-document service needs tenant-scoped retrieval, identity, retention, egress, and audit controls outside this kit.
