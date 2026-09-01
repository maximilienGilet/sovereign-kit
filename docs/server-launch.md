# Server launch contract

This repository is deliberately client-side. Provision the GPU only through an approved provider and client-specific operating procedure.

## Required SGLang properties

The client profile expects a Qwen server compatible with OpenAI Chat Completions and reachable through an SSH/VPN tunnel as local port `30000`.

Reference command used for the validated Qwen profile:

```bash
sglang serve \
  --trust-remote-code \
  --model-path RadixArk/Qwen3.8-27B-NVFP4 \
  --revision 319f741cce68d7914884900c138a1fbb70a42f30 \
  --context-length 262144 \
  --kv-cache-dtype fp8_e4m3 \
  --mem-fraction-static 0.85 \
  --attention-backend flashinfer \
  --chunked-prefill-size 2048 \
  --max-running-requests 5 \
  --cuda-graph-max-bs 5 \
  --reasoning-parser qwen3 \
  --tool-call-parser qwen3_coder \
  --host 127.0.0.1 \
  --port 30000
```

Parameters should be revalidated when changing the GPU, model revision, SGLang release, quantization, target concurrency, or context budget.

`--trust-remote-code` executes model-provided Python in the server environment. It is retained here only because the validated Qwen launch recipe requires it. Pin the Hugging Face revision (as above), inspect/re-approve any revision change, and run the server in an ephemeral, least-privilege environment. The documented revision is the Hugging Face commit resolved on 2026-09-01; revalidate it before a production rollout.

## Non-negotiable security settings

- `--host 127.0.0.1`, never `0.0.0.0`.
- No public inbound firewall rule for SGLang/vLLM ports.
- Use SSH local forwarding, a client-approved VPN, or Tailscale.
- Do not put API keys, SSH private keys, instance connection strings, or provider tokens in this repository.
- Destroy test instances when benchmarks finish.

## Important legal distinction

A private loopback endpoint protects network exposure. It does not, by itself, establish contractual or legal sovereignty. Confirm the contractual requirements for the specific client before processing their material.
