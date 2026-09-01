# Contributing to Sovereign Kit

Thanks for considering a contribution.

Sovereign Kit is an early, provider-neutral setup kit for a private AI inference route. The V1 harnesses are coding agents, but the core route can serve approved OpenAI-compatible clients. Contributions should improve reproducibility, clarity, or bounded technical controls — not expand the project into a generic compliance platform.

## Before opening an issue or pull request

- Read the [security boundary](docs/security.md) and the [security policy](SECURITY.md).
- Never include credentials, tokens, private keys, SSH host keys, production endpoints, client names, repository content, or private benchmark logs.
- Do not claim legal sovereignty, GDPR compliance, DPA coverage, security certification, or provider contractual guarantees without independently verifiable and appropriately scoped evidence.
- Keep provider-specific code behind clear adapters; the project core must not become tied to one GPU marketplace, model, or agent harness.

## Local checks

Run the checks relevant to your change before opening a pull request:

```bash
bash -n install-macos.sh bin/*
python3 scripts/check-profiles.py
python3 tests/test_render_profile.py
python3 tests/test_sovkit_doctor.py
python3 scripts/check-secrets.py
```

If you modify the installer or wrappers, run the isolated installer test described in the existing documentation and report exactly what was exercised. Do not report a real inference success unless the request completed through an approved private endpoint.

## Pull-request guidance

Keep pull requests small and explain:

1. the user or operator problem being addressed;
2. the security boundary affected, including residual risk;
3. how the change was validated;
4. any provider/model/harness assumption introduced.

## Roadmap discipline

The repository currently contains a validated Qwen/SGLang reference path and local Pi/OpenCode harness setup. Future provider adapters, lifecycle automation, dashboards, and a `sovkit` CLI must remain clearly labelled as planned until implemented and tested.
