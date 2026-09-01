# 262K single-agent benchmark: historical, not a production recipe

> **Security note:** this historical capture uses a development image and does not pin a Hugging Face revision. Do not reproduce it as-is for client workloads. The reviewed deployment recipe is in [`docs/server-launch.md`](../docs/server-launch.md); it pins the model revision and adds tunnel and review requirements.

Date: 2026-09-01. Synthetic test only. No client code or context was sent.

## Goal

Test whether one Qwen3.8-27B server could accept a request near its native 262,144-token window and then produce a long completion.

## Infrastructure

- GPU: NVIDIA RTX PRO 6000 Blackwell Server Edition, 97,887 MiB VRAM.
- Runtime: SGLang image `lmsysorg/sglang:dev-qwen38-27b-dflash2`.
- Model: `RadixArk/Qwen3.8-27B-NVFP4`, an NVFP4 quantization of Qwen3.8-27B.
- Endpoint: bound to `127.0.0.1:30000` on the pod.
- Concurrency: 1.

## Server command

```bash
sglang serve \
  --trust-remote-code \
  --model-path RadixArk/Qwen3.8-27B-NVFP4 \
  --context-length 262144 \
  --kv-cache-dtype fp8_e4m3 \
  --mem-fraction-static 0.85 \
  --attention-backend flashinfer \
  --chunked-prefill-size 2048 \
  --max-running-requests 1 \
  --cuda-graph-max-bs 1 \
  --reasoning-parser qwen3 \
  --tool-call-parser qwen3_coder \
  --host 127.0.0.1 --port 30000
```

## Generated workload

- Prompt tokenized locally with the checkpoint tokenizer: **246,000 exact tokens**.
- `max_tokens`: **8,192**.
- API response: HTTP 200, `finish_reason: length`.
- Server-reported usage: `prompt_tokens: 246000`, `completion_tokens: 8192`, `total_tokens: 254192`.

## Observed measurements

- After loading weights: 20.14 GiB used, 74.13 GiB available.
- Mamba pool: 0.54 GiB convolution state + 27.70 GiB SSM state.
- Reserved FP8 KV pool: 1,035,364 tokens, K 15.80 GiB + V 15.80 GiB.
- After graph capture: about 12.33 GiB available.
- During the near-254K-token request: 85,749 / 97,887 MiB used, GPU at 100%.
- Decode at roughly 246K–254K context: about 48.2 tok/s, from SGLang logs.

## Result

The server accepted a 246K-token input and generated 8,192 output tokens without OOM or truncation. This RTX PRO 6000 S 96 GB configuration can therefore carry **one** 262K-total-context agent with this runtime and model.

This synthetic workload validates capacity and observed decode speed. It does not validate reasoning quality on a real codebase.

## Limits and follow-up

- The runtime reported that no KV FP8 scaling factors were supplied and used 1.0. Compare quality against BF16 KV or calibrated quantization on a real coding workload.
- This test does not validate multiple agents, 262K useful tokens of mixed code, or long-context quality.
- The test instance was destroyed after the result.
