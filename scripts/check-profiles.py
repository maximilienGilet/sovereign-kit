#!/usr/bin/env python3
"""Fail closed on malformed published route profiles without third-party dependencies."""
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PROFILES = ROOT / "profiles"


def error(path: Path, message: str) -> None:
    print(f"{path.relative_to(ROOT)}: {message}", file=sys.stderr)


def validate(path: Path) -> bool:
    try:
        profile = json.loads(path.read_text())
    except json.JSONDecodeError as exc:
        error(path, f"invalid JSON: {exc.msg}")
        return False
    required = {"schemaVersion", "id", "displayName", "route", "provider", "model", "limits", "evidence"}
    unknown = set(profile) - required
    missing = required - set(profile)
    if unknown or missing:
        error(path, f"unknown keys {sorted(unknown)}; missing keys {sorted(missing)}")
        return False
    if profile["schemaVersion"] != 1 or not re.fullmatch(r"[a-z0-9]+(?:-[a-z0-9]+)*", profile["id"]):
        error(path, "requires schemaVersion 1 and a kebab-case id")
        return False
    route = profile["route"]
    if set(route) != {"transport", "baseUrl", "endpointBinding", "fallback"}:
        error(path, "route must contain exactly transport, baseUrl, endpointBinding, fallback")
        return False
    if route["transport"] not in {"ssh-loopback", "https-pinned"} or route["fallback"] != "deny":
        error(path, "route must use a supported transport and fallback=deny")
        return False
    if route["transport"] == "ssh-loopback" and (route["baseUrl"] != "http://127.0.0.1:30000/v1" or route["endpointBinding"] != "loopback"):
        error(path, "ssh-loopback routes must use the fixed loopback endpoint")
        return False
    auth = profile["provider"].get("authentication", {})
    if auth.get("mode") not in {"none", "environment"}:
        error(path, "provider authentication mode must be none or environment")
        return False
    limits = profile["limits"]
    if not all(isinstance(limits.get(key), int) and limits[key] > 0 for key in ("contextWindow", "maxOutputTokens", "maxConcurrentRequests")):
        error(path, "limits require positive contextWindow, maxOutputTokens, maxConcurrentRequests")
        return False
    if not profile["evidence"].get("benchmarks"):
        error(path, "evidence requires at least one benchmark reference")
        return False
    return True


def main() -> int:
    files = sorted(PROFILES.glob("*/profile.json"))
    if not files:
        print("No published profiles found.", file=sys.stderr)
        return 1
    valid = all(validate(path) for path in files)
    if not valid:
        return 1
    print(f"Profile validation passed ({len(files)} published profile(s)).")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
