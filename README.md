# Qwen Sovereign Harness

A portable **Pi + Oh-My-Pi + pi-subagents** profile for a privately accessed Qwen3.8-27B SGLang server.

The purpose is simple:

```text
Pi principal agent ─┐
Pi subagents       ├─→ one private SGLang / Qwen endpoint
Oh-My-Pi           ┘
```

The profile deliberately exposes only `sovereign-qwen`. `pi-subagents` uses a strict model scope so workers cannot silently fall back to DeepSeek, OpenAI, Anthropic, or another provider.

## What this repository contains

- `install-macos.sh` — installs the isolated profile and the two Pi packages.
- `profile/` — Qwen-only Pi configuration.
- `bin/pi-sovereign` — starts Pi using that isolated profile.
- `bin/qwen-sovereign-tunnel` — a loopback-only SSH tunnel to SGLang.
- `docs/` — operating procedure, security model, and benchmark results.

## Quick start on a MacBook

```bash
git clone https://github.com/<you>/qwen-sovereign-harness.git
cd qwen-sovereign-harness
./install-macos.sh
```

When a Qwen pod is running, keep an SSH tunnel in one terminal:

```bash
qwen-sovereign-tunnel <ssh-host> <ssh-port>
```

Then start the harness in another:

```bash
pi-sovereign
```

Use `/subagents-models` inside Pi to inspect the resolved model mapping. It must report `sovereign-qwen/qwen3.8-27b-nvfp4` for the parent and workers.

## Server contract

The profile assumes a server with this contract:

```text
Protocol : OpenAI Chat Completions compatible
Endpoint : http://127.0.0.1:30000/v1 (on the client, through SSH)
Model    : qwen3.8-27b-nvfp4
Security : remote SGLang binds only to 127.0.0.1:30000
```

The `apiKey` in `profile/models.json` is only a local placeholder required by Pi to list a keyless OpenAI-compatible server. It is not a secret and is not an authentication barrier. Network isolation is provided by loopback binding plus SSH/VPN.

## Recommended harness policy

Use one shared endpoint, but do not submit every large prompt at once:

1. Send the principal agent its large context first.
2. Wait until it receives its first token.
3. Admit bounded subagents (typically 16K–32K context, bounded output).
4. Return worker summaries to the principal rather than copying raw transcripts wholesale.

This protects principal-agent TTFT while preserving shared GPU utilization. See [architecture](docs/architecture.md).

## Security boundary

Open-weight inference on a rented GPU is not automatically contractually sovereign. Validate region, provider agreement/DPA, storage, logs, egress, access controls, retention and deletion against each client requirement. Never commit credentials, SSH private keys, API keys, or generated Pi `auth.json`.

The server recipe uses `--trust-remote-code`; treat a model revision as executable supply-chain input. The launch documentation pins the reviewed revision and requires re-approval for upgrades.

## Docs

- [MacBook setup](docs/macbook-setup.md)
- [Server launch contract](docs/server-launch.md)
- [Architecture and measured limits](docs/architecture.md)
- [Security review and operating checklist](docs/security-review.md)
- [Benchmark evidence](benchmarks/)
