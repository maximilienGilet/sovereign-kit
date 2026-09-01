# Route profiles

A route profile defines one approved inference path: transport, endpoint binding, model identity, authentication mode, configured ceilings, and an explicit no-fallback rule.

It does not define a business workload, an agent permission model, a tenant, a cost promise, or a legal/compliance guarantee.

Published profiles are checked with:

```bash
python3 scripts/check-profiles.py
```

The checker rejects unknown top-level keys, route fallbacks, unsupported transports, malformed limits, and profiles without benchmark evidence. It complements the JSON Schema at [`profiles/schema/profile-v1.schema.json`](../profiles/schema/profile-v1.schema.json).

## Published profile

| Profile | Status | Use |
|---|---|---|
| [Studio — Qwen on RTX PRO 6000](../profiles/studio-qwen-pro6000/README.md) | Historical synthetic evidence; production rerun required | Long-context private route for the validated Pi/OpenCode adapter configuration |

## Planned, not published profiles

- **[Solo RTX 5090 candidate](profiles/solo-rtx5090.md):** a fast individual-route design under measurement; no released configuration or capacity claim yet.
- **[Lite European API candidate](profiles/lite-eu-api.md):** lower-operations design awaiting one selected endpoint, model, region review, secret path, and measured failure/capacity behaviour. It does not inherit self-hosted claims.

## Workloads

The route itself can serve any compatible client, but V1 only ships validated coding-client adapters. A future assistant, research, or automation workload needs a separate contract for egress, documents, retrieval, tool calls, identities, approvals, and audit. Private inference is not private autonomous agents.
