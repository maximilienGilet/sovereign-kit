# Contributing to Sovereign Kit

Sovereign Kit is one small reference route: Pi and OpenCode on a Mac reach one private Qwen/SGLang server through SSH loopback. Contributions should make that route clearer, safer to operate, or more reproducible.

Do not turn it into a general multi-model router, compliance product, dashboard, GPU marketplace integration, or agent platform without a separate design and evidence.

## Rules

- Never include credentials, private keys, host keys, active endpoints, client material, private benchmark logs, or production identifiers.
- Do not claim legal sovereignty, GDPR compliance, security certification, DPA coverage, provider guarantees, performance, or cost without appropriately scoped evidence.
- Keep the server loopback-bound and the client route fail-closed. Do not add a public-provider fallback.
- Keep the shared Pi + OpenCode route keyless unless a Pi-compatible authenticated route has been implemented and live-tested.

## Checks

Run before opening a pull request:

```bash
bash -n install-macos.sh bin/* server/run-sglang.sh
python3 tests/test_sovkit_doctor.py
python3 scripts/check-secrets.py
git diff --check
```

Describe the operator problem, the route/security impact, and what you actually validated. Do not describe a real inference path as tested unless it completed against an approved private endpoint.
