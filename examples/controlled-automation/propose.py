#!/usr/bin/env python3
"""Request and validate a proposal. This program never executes an action."""
from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.request


def main() -> int:
    parser = argparse.ArgumentParser(description="Get one validated proposal from a private route.")
    parser.add_argument("--endpoint", default="http://127.0.0.1:30000/v1")
    parser.add_argument("--model", default="qwen3.8-27b-nvfp4")
    parser.add_argument("--allowed-action", action="append", required=True)
    parser.add_argument("--input", required=True, help="Approved task input.")
    args = parser.parse_args()
    instruction = (
        "Return JSON only with action, requires_approval, and draft. "
        f"Allowed actions: {', '.join(args.allowed_action)}. "
        "Set requires_approval to true. Task: " + args.input
    )
    request = urllib.request.Request(
        f"{args.endpoint.rstrip('/')}/chat/completions",
        data=json.dumps({"model": args.model, "messages": [{"role": "user", "content": instruction}]}).encode(),
        headers={"Content-Type": "application/json", "Authorization": "Bearer local-qwen-tunnel"},
    )
    try:
        with urllib.request.urlopen(request, timeout=60) as response:
            completion = json.load(response)["choices"][0]["message"]["content"]
        proposal = json.loads(completion)
        if proposal.get("action") not in args.allowed_action or proposal.get("requires_approval") is not True:
            raise ValueError("proposal is outside the allowed action set or lacks approval")
    except urllib.error.HTTPError as error:
        print(f"Endpoint rejected the request: HTTP {error.code}", file=sys.stderr)
        return 1
    except (urllib.error.URLError, KeyError, IndexError, TypeError, json.JSONDecodeError, ValueError) as error:
        print(f"Proposal rejected: {error}", file=sys.stderr)
        return 1
    print(json.dumps(proposal, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
