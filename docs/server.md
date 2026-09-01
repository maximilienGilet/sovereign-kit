# Server setup

This repository does not provision the GPU host. The operator must create the instance, restrict access, and destroy it when work finishes.

The local clients expect an OpenAI-compatible Qwen/SGLang endpoint at remote `127.0.0.1:30000`, reachable through SSH, VPN, or Tailscale.

## Reference launch contract

The following command is the reviewed reference for the published Qwen profile. Revalidate it whenever you change the GPU, server release, model revision, quantization, context budget, or concurrency.

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

`--host 127.0.0.1` is mandatory. Do not change it to `0.0.0.0` or add a public firewall rule for the inference port.

## SSH account

Use a dedicated, unprivileged SSH account for the tunnel. Do not use `root` for client work. Restrict the account and its authorized key to the forwarding it needs on the GPU host.

The local wrapper uses a dedicated identity, a dedicated known-hosts file, `StrictHostKeyChecking=yes`, `BatchMode=yes`, `IdentitiesOnly=yes`, and `ExitOnForwardFailure=yes`.

## Authentication

The reference configuration can work with a keyless SGLang endpoint because SSH carries the route and both ends bind only to loopback. That does not stop another process under the same Mac account from calling the forwarded port.

`opencode-sovereign` supports `QWEN_LOCAL_API_KEY` when server authentication is enabled. The shipped Pi profile does not inject that variable, so it is intended for a keyless SGLang endpoint. Do not enable API authentication for Pi until you have created and tested a compatible private Pi profile.

For an OpenCode-only authenticated deployment, use a unique secret for each deployment, keep it in local secret storage or a `600` ignored file, and export it as `QWEN_LOCAL_API_KEY` before running the wrapper.

## Supply-chain boundary

`--trust-remote-code` executes model-provided Python on the GPU host. The model revision is pinned in the command, but it remains executable supply-chain input. Review any revision change. Pin the container image by OCI digest and limit server egress before a real client deployment.

## Capacity settings

The reference settings are not universal defaults. In particular, `262144` context and five running requests were tested on an RTX PRO 6000 S 96 GB with a particular historical SGLang build. Read the [benchmarks](../benchmarks/README.md), then measure your real prompts and concurrency before setting production limits.
