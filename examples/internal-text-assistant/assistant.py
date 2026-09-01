#!/usr/bin/env python3
"""Minimal client-owned text assistant for a Sovereign Kit route."""
from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.request


def main() -> int:
    parser = argparse.ArgumentParser(description="Send approved text to one private OpenAI-compatible route.")
    parser.add_argument("--endpoint", default="http://127.0.0.1:30000/v1")
    parser.add_argument("--model", default="qwen3.8-27b-nvfp4")
    parser.add_argument("--text", required=True, help="Text already approved for this request.")
    args = parser.parse_args()

    payload = json.dumps({
        "model": args.model,
        "messages": [{"role": "user", "content": args.text}],
    }).encode()
    request = urllib.request.Request(
        f"{args.endpoint.rstrip('/')}/chat/completions",
        data=payload,
        headers={
            "Content-Type": "application/json",
            "Authorization": "Bearer local-qwen-tunnel",
        },
    )
    try:
        with urllib.request.urlopen(request, timeout=60) as response:
            body = json.load(response)
    except urllib.error.HTTPError as error:
        print(f"Endpoint rejected the request: HTTP {error.code}", file=sys.stderr)
        return 1
    except urllib.error.URLError as error:
        print(f"Endpoint is unavailable: {error.reason}", file=sys.stderr)
        return 1

    try:
        print(body["choices"][0]["message"]["content"])
    except (KeyError, IndexError, TypeError):
        print("Endpoint returned an unexpected OpenAI-compatible response.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
