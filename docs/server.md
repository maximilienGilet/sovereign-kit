# Server setup

This is a **reference runbook** for a Linux GPU host. It creates one Qwen/SGLang process on remote loopback (`127.0.0.1:30000`). The Mac reaches it only through the SSH tunnel described in [Quick start](quickstart.md).

**Validation status:** the model settings have historical synthetic evidence on an RTX PRO 6000 S. The exact digest-locked container recipe below has not yet been live re-benchmarked. Complete this runbook and your own smoke test before putting client material through it.

For an optional Vast.ai host-selection worksheet—not a one-click template—see [Vast.ai](providers/vast.md).

## 1. Decide the host boundary

Before creating a host, assign an operator who can stop or destroy it and approve all of the following for the intended material:

- provider, region, host/storage terms, retention, logs, and deletion process;
- a Linux AMD64 GPU with enough VRAM for the selected context and concurrency;
- persistent disk for the model cache, runtime, and logs;
- a dedicated unprivileged SSH account and an out-of-band host-key verification path.

The published Studio evidence used an RTX PRO 6000 S with 96 GB VRAM. It does **not** establish capacity for a different GPU, image, workload, or concurrent-user service. Read the [route profile](profiles.md) and [benchmark contract](benchmark-contract.md) before selecting limits.

Do not create a public inference-port mapping or firewall rule. The inference process must not listen on `0.0.0.0`.

## 2. Check host prerequisites

Connect using the dedicated SSH account. Do not use `root`. On the GPU host, run:

```bash
uname -m
docker --version
nvidia-smi
```

Proceed only when:

- `uname -m` reports `x86_64` or `amd64`;
- Docker works for the SSH user;
- `nvidia-smi` reports the intended NVIDIA GPU.

Then verify Docker can pass the GPU into a container. Replace no values in this command:

```bash
docker run --rm --gpus all nvidia/cuda:12.8.0-base-ubuntu24.04 nvidia-smi
```

It must print the same GPU family inside the container. If it fails, install or repair NVIDIA Container Toolkit according to your host/provider’s documentation. Do not continue by exposing the host’s inference port as a workaround.

The server script mounts `$HOME/.cache/huggingface`. Ensure that filesystem has sufficient free space and that the SSH account can write it. If your selected model requires Hugging Face authentication, configure that credential in the host account’s local credential store; never commit it, put it in a template, or paste it into a startup URL.

## 3. Obtain and inspect the recipe

On the GPU host, use a reviewed checkout or release:

```bash
git clone https://github.com/maximilienGilet/sovereign-kit.git
cd sovereign-kit
git rev-parse HEAD
cat server/image.lock
```

Record the checkout commit and the image digest outside this repository if they identify a client deployment. The OCI digest in `server/image.lock` prevents the container tag from moving; it does not prove that the image, model code, GPU driver, or model artifact is safe or compatible.

Inspect the command before starting it:

```bash
sed -n '1,220p' server/run-sglang.sh
```

The reference script pins the model revision, uses `--trust-remote-code`, shares the host network namespace, and tells SGLang itself to bind only to `127.0.0.1:30000`. `--trust-remote-code` executes model-provided Python on the GPU host. Review the model revision and restrict server egress as appropriate for the deployment.

## 4. Start SGLang

Run the command in a terminal that remains open:

```bash
./server/run-sglang.sh
```

The first start may download the model into the Hugging Face cache. Treat a running process alone as insufficient: wait for its logs to indicate that the HTTP server is ready, then use a second SSH terminal to verify the loopback endpoint:

```bash
curl --fail --silent --show-error http://127.0.0.1:30000/v1/models
```

A successful check exits `0` and returns JSON with a `data` list. Confirm the configured Qwen model appears. A connection failure means the server did not start or is not listening; inspect the first terminal’s logs rather than changing the bind address.

Confirm that the listener is loopback-only:

```bash
ss -ltnp '( sport = :30000 )'
```

The local address must be `127.0.0.1:30000` (or the equivalent IPv4 loopback display). If it is reachable on a public or LAN address, stop the process, remove any port mapping/firewall exposure, and correct the launch configuration before continuing.

## 5. Connect the Mac

Do not copy the host, port, user, identity, or host key into this repository. Verify the SSH host fingerprint through an independent approved channel, then follow [Quick start](quickstart.md#3-prepare-the-ssh-trust-files) from the Mac.

The current shared Pi + OpenCode V1 route is deliberately **keyless at the SGLang API layer**. SSH plus loopback bindings protect transit and network exposure. The local forwarded port is still accessible to other processes under the same macOS account.

The shipped Pi profile cannot consume a deployment API secret. Do not add SGLang API authentication to this shared route. An authenticated OpenCode-only or authenticated Pi route requires a separate compatible profile and live validation; it is outside this recipe.

## 6. Stop and recover

Press `Ctrl-C` in the terminal running `run-sglang.sh`. Because the Docker command uses `--rm`, the container is removed after it exits; the model cache remains in `$HOME/.cache/huggingface`.

If startup fails:

1. keep the inference port closed;
2. read the SGLang/Docker error in the launch terminal;
3. check GPU visibility, available VRAM, disk space, and model-download access;
4. record the failure without client prompts, credentials, active hostnames, or private logs;
5. adjust capacity only after measuring the exact host/workload under the [benchmark contract](benchmark-contract.md).

Stopping the server does not automatically destroy the GPU instance or stop provider billing. Follow [Operations](operations.md#stop) and the provider’s instance-destruction procedure.
