<p align="center">
  <img src="assets/sovereign-kit-mark.svg" width="104" alt="Sovereign Kit mark">
</p>

<h1 align="center">Sovereign Kit</h1>

<p align="center"><strong>A simple setup for private AI on your provider.</strong></p>

<p align="center">
  <a href="LICENSE">Apache-2.0</a> ·
  <a href="#quick-start">Quick start</a> ·
  <a href="docs/security-review.md">Security boundary</a> ·
  <a href="benchmarks/">Benchmark evidence</a>
</p>

Sovereign Kit connects the coding-agent tools you already use to an approved, privately accessed model endpoint on infrastructure you choose.

It currently ships a portable macOS setup for **Pi**, **Oh-My-Pi**, **pi-subagents**, and **OpenCode**. All use the same Qwen/SGLang endpoint through a loopback-only SSH tunnel; the supplied profiles prevent a silent fallback to OpenAI, Anthropic, DeepSeek, OpenRouter, or another model provider.

```text
Your MacBook                                      Your provider
─────────────                                     ─────────────
Pi / Oh-My-Pi / pi-subagents ─┐                   SGLang + approved model
OpenCode                       ├─ local tunnel ─▶ bound to 127.0.0.1 only
                               └─ provider lock
```

> **The promise is technical and bounded:** Sovereign Kit helps you establish and inspect a private execution path from supported local harnesses to an approved model endpoint. It does **not** certify legal sovereignty, GDPR compliance, a provider DPA, complete network confinement, or a particular performance/SLA.

## Why Sovereign Kit

Coding-agent adoption often stops at a reasonable question: *where does the repository context go?*

Sovereign Kit gives technical teams a small, reproducible reference setup for an answer they can operate themselves:

- choose the GPU provider or an existing SSH target;
- run an approved open-weight model behind a private route;
- connect Pi/OMP/pi-subagents and OpenCode to that one route;
- forbid unapproved model-provider fallback in the shipped profiles;
- retain a clear runbook, explicit residual risks, and measured capacity evidence.

It is a **setup kit**, not a SaaS control plane and not a compliance badge.

## What works today

| Capability | Status |
| --- | --- |
| Isolated Pi profile with strict `pi-subagents` model scope | Available |
| Oh-My-Pi extension in that isolated profile | Available |
| Locked-down OpenCode custom provider configuration | Available |
| Loopback-only SSH forwarding with strict host-key checking | Available |
| Qwen/SGLang reference launch contract | Available |
| Reproducible macOS installer | Available |
| Provider lifecycle automation and a generic `sovkit` CLI | Planned — not shipped |

The current reference model is `RadixArk/Qwen3.8-27B-NVFP4`; it is an implementation reference, not a requirement of the project’s long-term provider-neutral core.

## Quick start

### 1. Provision an approved private model endpoint

Prepare a Qwen/SGLang host following the [server launch contract](docs/server-launch.md). The inference service must bind to loopback on the GPU host — never expose its inference port publicly.

### 2. Install the local kit

```bash
git clone https://github.com/maximilienGilet/sovereign-kit.git
cd sovereign-kit
./install-macos.sh --with-opencode
```

The installer creates an isolated Pi profile and installs the wrappers under `~/.local/bin`. It refuses to overwrite an existing profile unless you pass `--upgrade`.

### 3. Establish the private route

Verify the remote host key out of band and use a dedicated unprivileged SSH account and identity:

```bash
sovkit-tunnel <ssh-host> <ssh-port> <tunnel-user> \
  <identity-file> <known-hosts-file>
```

The tunnel forwards only `127.0.0.1:30000` on your MacBook to `127.0.0.1:30000` on the remote host.

### 4. Use your harness

```bash
pi-sovereign
# or
opencode-sovereign
```

In Pi, run `/subagents-models` before client work. The parent and every worker must resolve to:

```text
sovereign-qwen/qwen3.8-27b-nvfp4
```

If another provider is shown, stop and correct the local profile before sending sensitive content.

## Security boundary

Sovereign Kit deliberately does a few things well, and documents what remains outside its reach.

**It helps control:** local provider selection, subagent model scope, private loopback transport, dedicated SSH identity use, and strict remote host-key verification.

**It cannot establish automatically:** the GPU provider’s contractual terms, region/data residency, storage and log retention, an applicable DPA, security of every plugin/tool invoked by an agent, or legal compliance for your client.

Read the [security review and operating checklist](docs/security-review.md) before use. Do not commit credentials, SSH material, endpoint addresses, or generated local agent state.

The reference launch uses `--trust-remote-code`; a pinned model revision is still executable supply-chain input and must be reviewed before use. See [server launch](docs/server-launch.md).

## Architecture and evidence

- [MacBook setup](docs/macbook-setup.md)
- [Server launch contract](docs/server-launch.md)
- [Architecture and measured limits](docs/architecture.md)
- [Security review and operating checklist](docs/security-review.md)
- [Benchmark evidence](benchmarks/)

The benchmark reports are evidence for their stated hardware, versions, and workloads — not a universal capacity or cost promise.

## Contributing

This project is early and intentionally narrow. Issues and pull requests that improve reproducibility, safety boundaries, provider-neutral adapters, and documentation are welcome.

Please read [CONTRIBUTING.md](CONTRIBUTING.md) and never include credentials, internal endpoints, host keys, client material, or sensitive proof-of-concepts in an issue. Use [SECURITY.md](SECURITY.md) for vulnerability reporting.

## License

[Apache License 2.0](LICENSE). You may use, modify, and distribute the code under its terms.
