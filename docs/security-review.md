# Security review — 2026-09-01

## Scope

Review of the client-side Pi/Oh-My-Pi profile, the installer, SSH local-forwarding helper and published server launch contract before initial GitHub publication.

## Controls verified

| Control | Status | Evidence |
|---|---|---|
| No private key, GitHub token, Vast key or SSH public key staged | Pass | Pre-publication regex scan of every tracked file |
| SSH forward binds locally only | Pass | `-L 127.0.0.1:30000:127.0.0.1:30000` |
| Server contract binds SGLang remotely to loopback | Pass | `--host 127.0.0.1` |
| Profile excludes external model providers | Pass | `models.json` has only `sovereign-qwen` |
| Parent and workers are constrained to the same provider | Pass | strict `modelScope` allowlist |
| Dependencies are version-pinned | Pass | `pi-subagents@0.62.0`, `oh-my-pi@0.2.0` |
| Installer works in a clean temporary home | Pass | end-to-end smoke test on this Linux host; macOS must still be exercised on a real MacBook |

## Residual risks and operating rules

1. **SGLang authentication:** the reference endpoint is keyless. Loopback plus authenticated SSH are mandatory but do not prevent another process under the same macOS account from calling the local port. Enable SGLang API authentication with a unique deployment secret whenever the deployed SGLang version supports it; keep the secret in macOS Keychain or a local ignored `600` file. Never expose port 30000 publicly.
2. **Remote model code:** `--trust-remote-code` is executable supply-chain input. The documented Hugging Face revision is pinned; review and re-pin every upgrade. Pin the GPU container by OCI digest and restrict server egress before client workloads.
3. **Pi egress:** `modelScope` prevents model-provider fallback; it is not a network sandbox. Audit/allowlist Pi plugins and tools, and enforce OS/network egress policy if client data must be contractually confined to the tunnel.
4. **Rented GPU sovereignty:** network privacy is not sufficient for a contractual sovereignty claim. Validate provider, region, contract/DPA, retention, disk handling, logs, egress and access controls per client.
5. **Pi packages:** version pinning reduces drift but does not replace source review. Upgrade deliberately and rerun the installer smoke test plus this review.
6. **SSH trust:** use a dedicated unprivileged tunnel account, identity and verified known-hosts file. The client helper enforces `StrictHostKeyChecking=yes`; do not disable it.
7. **Runtime verification:** before processing client material, use `/subagents-models` and stop if parent or workers resolve to another provider.

## Required checks before each client deployment

```text
[ ] GPU endpoint is loopback-only
[ ] Tunnel is local-loopback-only
[ ] SSH host fingerprint has been verified
[ ] /subagents-models shows sovereign-qwen for all roles
[ ] Client sovereignty/contractual requirements are approved
[ ] Instance lifecycle/auto-stop and deletion procedure are defined
```
