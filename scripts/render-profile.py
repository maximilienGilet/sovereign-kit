#!/usr/bin/env python3
"""Render the shared, keyless SSH-loopback route into Pi and OpenCode adapters."""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path


def reject(message: str) -> None:
    print(f"Cannot render profile: {message}", file=sys.stderr)
    raise SystemExit(1)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--profile", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    try:
        profile = json.loads(Path(args.profile).read_text())
    except (OSError, json.JSONDecodeError) as error:
        reject(str(error))
    route, provider, model, limits = (profile.get(key) for key in ("route", "provider", "model", "limits"))
    if not all(isinstance(value, dict) for value in (route, provider, model, limits)):
        reject("profile is missing route, provider, model, or limits")
    if route.get("transport") != "ssh-loopback" or route.get("baseUrl") != "http://127.0.0.1:30000/v1" or route.get("endpointBinding") != "loopback":
        reject("Pi/OpenCode V1 requires the fixed SSH loopback route")
    if route.get("fallback") != "deny" or provider.get("protocol") != "openai-compatible":
        reject("Pi/OpenCode V1 requires an OpenAI-compatible fail-closed route")
    if provider.get("authentication", {}).get("mode") != "none":
        reject("Pi/OpenCode V1 requires a keyless route because Pi secret injection is unsupported")
    provider_id, model_id, base_url = provider.get("id"), model.get("id"), route.get("baseUrl")
    if not all(isinstance(value, str) and value for value in (provider_id, model_id, base_url)):
        reject("provider id, model id, and base URL are required")
    context, output = limits.get("contextWindow"), limits.get("maxOutputTokens")
    if not all(isinstance(value, int) and value > 0 for value in (context, output)):
        reject("positive contextWindow and maxOutputTokens are required")

    pi_settings = {
        "theme": "dark", "defaultProvider": provider_id, "defaultModel": model_id,
        "defaultThinkingLevel": "high", "packages": ["npm:pi-subagents@0.62.0", "npm:oh-my-pi@0.2.0"],
        "subagents": {"defaultModel": f"{provider_id}/{model_id}", "defaultThinking": "high",
                      "modelScope": {"enforce": True, "strict": True, "allow": [f"{provider_id}/*"]}},
    }
    pi_models = {"providers": {provider_id: {"baseUrl": base_url, "api": "openai-completions", "apiKey": "local-qwen-tunnel",
        "compat": {"supportsDeveloperRole": False, "supportsReasoningEffort": False},
        "models": [{"id": model_id, "name": f"{model_id} — sovereign route", "reasoning": True,
                    "input": model.get("input", ["text"]), "contextWindow": context, "maxTokens": output,
                    "cost": {"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0}}]}}}
    opencode = {"$schema": "https://opencode.ai/config.json", "model": f"{provider_id}/{model_id}",
        "enabled_providers": [provider_id], "provider": {provider_id: {"npm": "@ai-sdk/openai-compatible",
        "name": "Private Qwen via local tunnel", "options": {"baseURL": base_url, "apiKey": "{env:QWEN_LOCAL_API_KEY}"},
        "models": {model_id: {"name": f"{model_id} (private)", "limit": {"context": context, "output": output}}}}}}
    root = Path(args.output)
    (root / "pi").mkdir(parents=True, exist_ok=True)
    (root / "opencode").mkdir(parents=True, exist_ok=True)
    for path, value in ((root / "pi/settings.json", pi_settings), (root / "pi/models.json", pi_models), (root / "opencode/sovereign.json", opencode)):
        path.write_text(json.dumps(value, indent=2) + "\n")
    print(f"Rendered {profile['id']} to {root}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
