# Route profiles

A route profile defines one approved inference path: transport, endpoint binding, model identity, authentication mode, configured ceilings, and an explicit no-fallback rule.

It does not define a business workload, an agent permission model, a tenant, a cost promise, or a legal/compliance guarantee.

## Published profile

| Profile | Status | Use |
|---|---|---|
| [Studio — Qwen on RTX PRO 6000](../profiles/studio-qwen-pro6000/README.md) | Historical synthetic evidence; production rerun required | Long-context private route for the validated Pi/OpenCode adapter configuration |

## Planned, not published profiles

- **Solo RTX 5090:** requires a pinned runtime, exact context/concurrency settings, and target-harness benchmarks before release.
- **Lite:** requires a chosen explicit endpoint, region, model/version, authentication path, no-fallback policy, and latency/error evidence. A managed European API does not inherit self-hosted claims.

## Workloads

The route itself can serve any compatible client, but V1 only ships validated coding-client adapters. A future assistant, research, or automation workload needs a separate contract for egress, documents, retrieval, tool calls, identities, approvals, and audit. Private inference is not private autonomous agents.
