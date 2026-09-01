# Shared Qwen benchmark: 128K principal + 4 × 32K workers

Date: 2026-09-01. Synthetic workload, with no client code or context. The instance was destroyed after the test.

## Infrastructure and server

- GPU: RTX PRO 6000 S, 97,887 MiB.
- Observed price: $1.4333/h.
- Runtime: SGLang `lmsysorg/sglang:dev-qwen38-27b-dflash2`.
- Model: `RadixArk/Qwen3.8-27B-NVFP4`.
- Server context limit: 262,144 tokens.
- KV cache: FP8 E4M3; attention backend: FlashInfer.
- `max-running-requests=5`, `cuda-graph-max-bs=5`.

## Concurrent workload

All five requests were sent at the same time to the same OpenAI-compatible endpoint:

| Role | Exact prompt | Requested output |
|---|---:|---:|
| Principal | 128,000 tokens | 4,096 tokens |
| Workers 1–4 | 32,000 tokens each | 1,024 tokens each |

Total active input: 256,000 tokens. Total requested output: 8,192 tokens.

## Results

| Role | HTTP | TTFT | End-to-end time | Output | Decode after first token |
|---|---:|---:|---:|---:|---:|
| Principal | 200 | 60.165 s | 133.687 s | 4,096 | 55.712 tok/s |
| Worker 1 | 200 | 9.862 s | 33.028 s | 1,024 | 44.203 tok/s |
| Worker 2 | 200 | 13.079 s | 33.028 s | 1,024 | 51.332 tok/s |
| Worker 3 | 200 | 3.406 s | 33.027 s | 1,024 | 34.570 tok/s |
| Worker 4 | 200 | 6.720 s | 33.028 s | 1,024 | 38.924 tok/s |

- Aggregate worker throughput over their 33.028-second window: **124.02 tok/s**.
- Aggregate throughput for the full workload (8,192 tokens / 133.687 seconds): **61.28 tok/s**.
- Memory during the workload: 85,837 / 97,887 MiB; about 11.77 GiB remained.

## Result

One Qwen server accepted and served the selected workload without OOM or deadlock: a 128K principal and four 32K workers. The workers returned in about 33 seconds while the principal continued its long generation.

The principal's roughly 60-second TTFT came from competing with four 32K prefills. A production workload should therefore prioritise the principal rather than submit five large prompts at the same instant.

This benchmark validates capacity and concurrency for synthetic prompts. It does not validate quality on real repositories or choose the best scheduler for the harness.
