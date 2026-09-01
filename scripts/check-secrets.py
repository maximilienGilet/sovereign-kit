#!/usr/bin/env python3
"""Fail on common credential material before committing or publishing."""
from pathlib import Path
import re
import subprocess
import sys

patterns = {
    "private key": re.compile(r"-----BEGIN(?: [A-Z]+)? PRIVATE KEY-----", re.I),
    "GitHub token": re.compile(r"\b(?:ghp|gho|github_pat)_[A-Za-z0-9_]+", re.I),
    "SSH public key": re.compile(r"\bssh-(?:rsa|ed25519|ecdsa)\s+[A-Za-z0-9+/=]{40,}"),
    "Vast API key": re.compile(r"\bvast(?:ai)?[_-]?(?:api)?[_-]?key\s*[:=]\s*[\"']?[A-Za-z0-9_-]{12,}", re.I),
}
files = subprocess.check_output(["git", "ls-files"], text=True).splitlines()
issues = []
for file_name in files:
    text = Path(file_name).read_text(errors="ignore")
    for label, pattern in patterns.items():
        if pattern.search(text):
            issues.append(f"{file_name}: possible {label}")
if issues:
    print("Secret scan failed:", *issues, sep="\n", file=sys.stderr)
    raise SystemExit(1)
print(f"Secret scan passed ({len(files)} tracked files).")
