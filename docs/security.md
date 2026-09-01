# Security boundary

Sovereign Kit gives the local coding clients one explicit model route. It is useful when a team needs to avoid an accidental model-provider fallback. It is not a legal, regulatory, or network-isolation certification.

## Controls in this repository

| Control | What it does |
|---|---|
| Pi provider configuration | Lists only the local `sovereign-qwen` endpoint |
| pi-subagents model scope | Restricts parent and workers to `sovereign-qwen/*` |
| OpenCode inline configuration | Overrides a repository's `opencode.json` and enables only `sovereign-qwen` |
| SSH tunnel | Forwards Mac loopback to GPU-host loopback |
| SSH options | Require a dedicated identity, explicit known-hosts file, and strict host-key checking |
| Installer permissions | Writes profile and OpenCode config as `600`; wrappers as `700` |

`PI_CODING_AGENT_DIR` and `SOVEREIGN_OPENCODE_CONFIG` let an operator replace the standard profile or configuration. They are intentional administrative overrides, not enforcement boundaries. Treat any replacement configuration as unverified until you inspect it.

## What remains outside the kit

- the provider contract, DPA, region, retention, disks, logs, staff access, and deletion process;
- network egress from agent tools, plugins, browsers, shells, Git, package managers, and MCP servers;
- the security and behaviour of Pi extensions, OpenCode plugins, model code, container images, and other dependencies;
- model quality, availability, cost, and capacity;
- whether a particular client workload is permitted on a particular deployment.

A private route does not automatically make a deployment legally sovereign or GDPR compliant.

## Required deployment checks

Before client material enters the route, confirm:

```text
[ ] SGLang is bound to 127.0.0.1 on the GPU host.
[ ] No public firewall rule exposes the inference port.
[ ] The tunnel is bound to 127.0.0.1 on the Mac.
[ ] The SSH host fingerprint was verified out of band.
[ ] A dedicated unprivileged SSH account and identity are in use.
[ ] Pi /subagents-models resolves every role to sovereign-qwen.
[ ] OpenCode is launched through opencode-sovereign.
[ ] Provider, region, contractual, retention, and deletion requirements are approved for this client.
[ ] Instance stop/destroy responsibility is assigned.
```

## Secrets and local state

Never commit API keys, SSH private keys, known-hosts files, active endpoints, client names, repository content, or private benchmark logs.

The default `local-qwen-tunnel` identifier is not a secret. It exists only for a keyless local SGLang endpoint. When SGLang authentication is available, use a unique deployment secret in local secret storage and preserve loopback binding.

Run these before committing or publishing:

```bash
python3 scripts/check-secrets.py
git diff --cached --check
```

Report a vulnerability privately as described in [SECURITY.md](../SECURITY.md).
