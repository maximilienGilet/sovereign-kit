# Security policy

## Reporting a vulnerability

Do **not** open a public issue with credentials, endpoint addresses, host keys, client material, or a proof-of-concept that could expose an active deployment.

Contact the repository owner privately via GitHub, with a concise impact description and reproduction steps that omit secrets. The owner will acknowledge the report, assess scope, and coordinate remediation.

## Local security checks

Before a commit or publication:

```bash
python3 scripts/check-secrets.py
git diff --cached --check
```

Enable GitHub secret scanning for this repository in its GitHub security settings when available for the selected visibility and plan.
