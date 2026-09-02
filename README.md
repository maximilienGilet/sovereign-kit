<p align="center">
  <img src="assets/sovereign-kit-logo.png" width="156" alt="Blue glass cube, the Sovereign Kit mark">
</p>

# Sovereign Kit

A small reference setup for using **one private Qwen model** from Pi, pi-subagents, Oh-My-Pi, and OpenCode on a Mac.

```text
Pi / OpenCode on a Mac
        │
        ▼
127.0.0.1:30000
        │ strict SSH tunnel
        ▼
127.0.0.1:30000 on a GPU host
        │
        ▼
SGLang + Qwen
```

The point is simple: the coding clients have one configured model route and cannot silently fall back to OpenAI, Anthropic, DeepSeek, OpenRouter, or another model provider.

## What is included

- isolated Pi profile with `pi-subagents` and Oh-My-Pi;
- locked OpenCode configuration;
- `sovkit-tunnel`, a local-only SSH forward with strict host-key verification;
- `sovkit doctor`, a local configuration and endpoint check;
- one Linux GPU-host SGLang command using a digest-pinned image and pinned Qwen revision;
- historical benchmark reports for the reference model.

The reference model is `RadixArk/Qwen3.8-27B-NVFP4`. This is one route, not a general model gateway or deployment platform.

## What it is not

Sovereign Kit does not provision GPUs, create SSH users, expose a public API, manage billing, make a rented GPU legally sovereign, prove GDPR compliance, sandbox agent tools, or validate a provider contract.

The local client configuration and checks are structurally tested. Historical benchmarks exist on an RTX PRO 6000 S. A live Pi/OpenCode session against the current digest-pinned server recipe has not yet been completed: use a non-sensitive smoke test before client work.

## Before starting

You need:

1. A Linux AMD64 NVIDIA GPU host approved for the intended material, with Docker and NVIDIA Container Toolkit working for an unprivileged SSH user.
2. A Mac with Git, standard `ssh`, and [Pi](https://github.com/badlogic/pi-mono). npm is needed only to install OpenCode automatically.
3. A dedicated SSH identity and a host fingerprint verified through an independent channel.
4. An assigned owner for provider approval, spend limit, and instance destruction.

Keep private keys, known-hosts files, active hostnames, client material, and private logs out of this repository.

## 1. Start the GPU host

On the GPU host:

```bash
uname -m
docker --version
nvidia-smi
docker run --rm --gpus all nvidia/cuda:12.8.0-base-ubuntu24.04 nvidia-smi
```

The last command must show the GPU inside a container. If it does not, repair the NVIDIA Container Toolkit setup; do not open an inference port as a workaround.

Then:

```bash
git clone https://github.com/maximilienGilet/sovereign-kit.git
cd sovereign-kit
git rev-parse HEAD
cat server/image.lock
./server/run-sglang.sh
```

Keep that terminal open. The first run can download the model. The command binds SGLang to remote loopback only. It uses `--trust-remote-code`, which executes model-provided Python on the GPU host: review the pinned model revision and container input before client use.

From a second SSH terminal, wait for the service to be ready and check it:

```bash
curl --fail --silent --show-error http://127.0.0.1:30000/v1/models
ss -ltnp '( sport = :30000 )'
```

The first command must return JSON; the second must show `127.0.0.1:30000`. Stop if the server is reachable on a LAN or public address.

## 2. Install the Mac clients

On the Mac:

```bash
pi --version
git --version
ssh -V
npm --version
git clone https://github.com/maximilienGilet/sovereign-kit.git
cd sovereign-kit
./install-macos.sh --with-opencode
```

The installer writes an isolated Pi profile, an OpenCode configuration, and four wrappers in `~/.local/bin`:

```text
pi-sovereign
opencode-sovereign
sovkit
sovkit-tunnel
```

If needed, add that directory to zsh’s path:

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
exec zsh
```

The installer refuses to overwrite an existing Pi profile. Use `--upgrade` only after saving intentional local changes; it backs up the Pi profile but overwrites the OpenCode config and wrappers.

## 3. Verify SSH and open the tunnel

Put the independently verified host key in a dedicated known-hosts file, outside the repository, alongside a dedicated private key:

```bash
chmod 600 ~/.ssh/qwen-sovereign_ed25519 ~/.ssh/qwen-sovereign_known_hosts
ssh-keygen -lf ~/.ssh/qwen-sovereign_known_hosts
```

Compare the displayed fingerprint to the approved value. A mismatch is a stop condition.

Open the route with deployment-specific values:

```bash
sovkit-tunnel <ssh-host> <ssh-port> <ssh-user> \
  ~/.ssh/qwen-sovereign_ed25519 \
  ~/.ssh/qwen-sovereign_known_hosts
```

Leave this terminal open. The tunnel listens only on Mac `127.0.0.1:30000` and fails if host verification or forwarding fails.

## 4. Verify and use it

In another Mac terminal:

```bash
curl --fail --silent --show-error http://127.0.0.1:30000/v1/models
sovkit doctor
```

Both must succeed. `sovkit doctor` must show no `FAIL`; inspect every warning before client work.

From a disposable or approved non-sensitive repository:

```bash
pi-sovereign
# or
opencode-sovereign
```

In Pi, run `/subagents-models`. The parent and every worker must show:

```text
sovereign-qwen/qwen3.8-27b-nvfp4
```

In OpenCode, confirm the same model. Start with a non-sensitive prompt. If another provider/model appears, or an endpoint check fails, stop. Do not use bare `pi`/`opencode`, enable a public-provider fallback, or open a public inference port.

The shipped route is **keyless at the SGLang API layer**. `local-qwen-tunnel` is a non-secret compatibility value, not access control. Do not add an SGLang API key to this shared Pi + OpenCode setup: the supplied Pi profile cannot consume it.

## Stop

1. Exit Pi or OpenCode.
2. Stop the tunnel with `Ctrl-C`.
3. Stop SGLang with `Ctrl-C`.
4. Stop or destroy the GPU instance and confirm billing has ended.

Closing the Mac client or tunnel does not stop the remote GPU.

## When something fails

| Problem | Safe response |
|---|---|
| Tunnel exits | Check permissions and verify the host fingerprint independently. Never disable strict host-key checking. |
| Port 30000 is occupied | Run `lsof -nP -iTCP:30000 -sTCP:LISTEN`, stop that process, restart the tunnel. |
| Server endpoint fails | Inspect server logs, `nvidia-smi`, GPU-container integration, model-download access, disk space, and loopback binding. |
| `sovkit doctor` reports config failure | Preserve changes and rerun `./install-macos.sh --upgrade`; inspect deliberate environment overrides. |
| Pi/OpenCode shows another provider | Stop. Launch the Sovereign wrapper, rerun `sovkit doctor`, and do not enable a fallback. |
| Endpoint returns `401`/`403` | Stop. This keyless shared route does not support an authenticated SGLang server. |

## Benchmarks

Historical synthetic evidence on an RTX PRO 6000 S (96 GB):

- [near-262K request](benchmarks/benchmark-262k-rtx-pro-6000-sglang.md): 246K input + 8,192 output completed at about 48.2 tok/s decode;
- [one 128K principal plus four 32K workers](benchmarks/benchmark-shared-128k-plus-4x32k.md): all requests completed; concurrent prefill made principal TTFT about 60 seconds.

These are not capacity, cost, quality, throughput, or availability promises. Rerun the workload on the exact GPU/image/model before a production claim.

## Development

```bash
bash -n install-macos.sh bin/* server/run-sglang.sh
python3 tests/test_sovkit_doctor.py
python3 scripts/check-secrets.py
git diff --check
```

See [SECURITY.md](SECURITY.md) for private vulnerability reporting and [CONTRIBUTING.md](CONTRIBUTING.md) for contribution rules.

## License

[Apache License 2.0](LICENSE).
