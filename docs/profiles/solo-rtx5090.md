# Solo RTX 5090 candidate

This is a **candidate profile**, not a released configuration. Its intended use is one developer working through a private route with a short-to-moderate active context and deliberately limited concurrency.

It must not be described as a 262K-context profile, a multi-user service, or a substitute for the Studio RTX PRO 6000 profile.

## Historical signal only

An earlier isolated measurement used `RadixArk/Qwen3.8-27B-NVFP4` on an RTX 5090 with FP8 KV cache, FlashInfer, EAGLE/MTP, and ReplaySSM. For 8K input, 1K output, and concurrency one, it observed approximately 139.42 end-to-end tokens/second and 157.7 decode tokens/second.

That was an experimental server configuration, not the current digest-locked recipe. It does not establish quality, context capacity, stability, cost, client compatibility, or a production SLO.

## Release gates

Do not publish a Solo profile JSON, installer adapter, template, or performance claim until all of these are recorded against one exact configuration:

1. GPU SKU, driver, CUDA/runtime image identity, and model revision/quantization.
2. A supported context ceiling with an explicit OOM boundary and recovery behaviour.
3. A Pi and OpenCode smoke test through the real loopback SSH route.
4. A realistic single-developer coding workload, not only synthetic generation.
5. A concurrency and TTFT sweep. Solo may intentionally set concurrency to one.
6. A sustained run long enough to observe model download/cache behaviour and GPU stability.
7. Startup, stop, and destroy cost observations for the selected provider/host class.

## Initial design constraints

- one approved Qwen deployment;
- one local SSH loopback route;
- no fallback provider;
- no public inference port;
- no shared-team or multi-tenant claim;
- explicit context and concurrency ceiling derived from the release measurements.

Use the [benchmark contract](../benchmark-contract.md) for the required record format and [Vast.ai worksheet](../providers/vast.md) if Vast is the chosen host marketplace.
