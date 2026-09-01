# Server setup

This repository does not provision the GPU host. The operator creates the instance, restricts access, and destroys it when work finishes.

The local clients expect an OpenAI-compatible Qwen/SGLang endpoint at remote `127.0.0.1:30000`, reached through SSH, VPN, or Tailscale.

## Digest-locked container recipe

The Linux AMD64 SGLang image is pinned by OCI digest in [`server/image.lock`](../server/image.lock). On a Linux GPU host with Docker and NVIDIA Container Toolkit:

```bash
git clone https://github.com/maximilienGilet/sovereign-kit.git
cd sovereign-kit
./server/run-sglang.sh
```

The script reads the lock file, mounts `$HOME/.cache/huggingface`, pins the Qwen model revision, and binds SGLang to `127.0.0.1:30000`. Do not change that address to `0.0.0.0` or open an inference firewall rule.

The digest prevents an image tag from moving. It is **not yet a live re-benchmark of this exact image**; validate the image, GPU, model download, endpoint, and workload before client use.

## SSH account

Use a dedicated unprivileged SSH account. Do not use `root`. Restrict the account and authorized key to the forwarding it needs.

The local wrapper requires a dedicated identity and known-hosts file, `StrictHostKeyChecking=yes`, `BatchMode=yes`, `IdentitiesOnly=yes`, and `ExitOnForwardFailure=yes`.

## Authentication boundary

The shared Pi + OpenCode V1 route is deliberately keyless at the SGLang API layer: SSH and loopback bindings protect network transit and exposure. The local port remains available to processes under the same Mac account.

The shipped Pi profile does not accept a deployment secret. Do not add SGLang API authentication to this shared V1 route. An authenticated OpenCode-only deployment is outside the supported recipe until Pi secret injection has been implemented and tested.

## Supply-chain boundary

`--trust-remote-code` executes model-provided Python on the GPU host. The model revision and container image are pinned, but both remain supply-chain inputs. Review revisions, restrict server egress, and record the image digest used for each client deployment.

## Capacity settings

The reference settings are not universal defaults. `262144` context and five running requests were tested on an RTX PRO 6000 S 96 GB with a historical SGLang build. Read the [benchmarks](../benchmarks/README.md), then measure real prompts and concurrency before setting production limits.
