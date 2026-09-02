# Security boundary

Sovereign Kit gives local coding clients one explicit model route and helps avoid an accidental public-model-provider fallback. It is not a legal, regulatory, network-isolation, or security certification.

## Controls in this repository

| Control | What it does | What it does not do |
|---|---|---|
| Pi provider configuration | Lists only local `sovereign-qwen` | Sandbox Pi tools, shell commands, extensions, or project instructions |
| `pi-subagents` model scope | Restricts normal parent/worker selection to `sovereign-qwen/*` | Prevent a deliberately replaced profile from changing the route |
| OpenCode inline configuration | Overrides a repository `opencode.json` and enables only `sovereign-qwen` on the supported release | Validate all OpenCode/plugin behaviour on future versions |
| SSH tunnel | Forwards Mac loopback to GPU-host loopback | Protect another process under the same Mac account from using the local port |
| SSH options | Require dedicated identity, explicit known-hosts file, strict host-key checking, batch mode, and forwarding failure | Prove that the remote host or provider is trustworthy |
| Installer permissions | Writes configuration `600` and wrappers `700` | Protect copied secrets or broader user-account access |

`PI_CODING_AGENT_DIR` and `SOVEREIGN_OPENCODE_CONFIG` intentionally replace the standard profile/configuration. They are administrative overrides, not enforcement boundaries. Treat any replacement as unverified until it has been inspected and tested.

## Required checks before client material

Perform these actions at deployment time, not only during initial installation:

```text
[ ] The GPU host is approved for the intended provider, region, retention, storage, access, and deletion requirements.
[ ] SGLang listens on 127.0.0.1:30000 on the GPU host.
[ ] No provider firewall rule, Docker port mapping, Instance Portal, or public URL exposes inference.
[ ] The Mac tunnel listens only on 127.0.0.1:30000.
[ ] The SSH fingerprint was verified through an independent channel.
[ ] A dedicated unprivileged SSH account and dedicated identity are in use.
[ ] `curl http://127.0.0.1:30000/v1/models` succeeds on both ends of the tunnel.
[ ] `sovkit doctor` reports no failures; any warnings were deliberately reviewed.
[ ] Pi `/subagents-models` resolves every role to `sovereign-qwen/qwen3.8-27b-nvfp4`.
[ ] OpenCode is launched through `opencode-sovereign` and shows the same model.
[ ] Instance stop/destroy responsibility and spend limit are assigned.
```

## Authentication and local state

The shipped Studio route is keyless at the SGLang API layer. `local-qwen-tunnel` is a non-secret compatibility identifier used by clients that require an API-key field. Do not mistake it for access control.

Do not add SGLang API authentication to the shared Pi + OpenCode V1 route: the shipped Pi configuration cannot inject a deployment secret. An authenticated route requires a separate compatible Pi profile and live validation. Even on the keyless route, other processes under the same macOS account can reach the forwarded local port.

Never commit API keys, SSH private keys, known-hosts files, active endpoints, client names, repository content, private benchmark logs, or raw terminal output containing any of them. Keep secrets in local secret storage and make non-versioned deployment records private.

Before committing or publishing, run:

```bash
python3 scripts/check-secrets.py
git diff --cached --check
```

## What remains outside the kit

- provider contract, DPA, region, retention, disks, logs, staff access, and deletion process;
- network egress from agent tools, plugins, browsers, shells, Git, package managers, and MCP servers;
- security and behaviour of Pi extensions, OpenCode plugins, model code, container images, and dependencies;
- model quality, availability, cost, capacity, and availability of a rented GPU;
- workload-specific permission, privacy, source-code, and tool-execution decisions.

A private route does not automatically make a deployment legally sovereign or GDPR compliant. Report a vulnerability privately as described in [SECURITY.md](../SECURITY.md).
