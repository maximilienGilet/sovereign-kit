#!/usr/bin/env python3
"""A deliberately small client-owned, read-only retrieval example."""
from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.request


def main() -> int:
    parser = argparse.ArgumentParser(description="Answer from explicitly allowed documents only.")
    parser.add_argument("--endpoint", default="http://127.0.0.1:30000/v1")
    parser.add_argument("--model", default="qwen3.8-27b-nvfp4")
    parser.add_argument("--documents", required=True, help="JSON list of {id, text} records.")
    parser.add_argument("--allow-id", action="append", required=True, help="Document ID approved for this request.")
    parser.add_argument("--question", required=True)
    args = parser.parse_args()

    try:
        documents = json.load(open(args.documents, encoding="utf-8"))
        allowed = [doc for doc in documents if doc.get("id") in set(args.allow_id)]
    except (OSError, json.JSONDecodeError) as error:
        print(f"Cannot read documents: {error}", file=sys.stderr)
        return 1
    if not allowed:
        print("No explicitly allowed documents were found.", file=sys.stderr)
        return 1

    context = "\n\n".join(f"[source:{doc['id']}]\n{doc['text']}" for doc in allowed)
    prompt = f"Answer only from these supplied sources. Cite source IDs.\n\n{context}\n\nQuestion: {args.question}"
    request = urllib.request.Request(
        f"{args.endpoint.rstrip('/')}/chat/completions",
        data=json.dumps({"model": args.model, "messages": [{"role": "user", "content": prompt}]}).encode(),
        headers={"Content-Type": "application/json", "Authorization": "Bearer local-qwen-tunnel"},
    )
    try:
        with urllib.request.urlopen(request, timeout=60) as response:
            body = json.load(response)
        print(body["choices"][0]["message"]["content"])
    except urllib.error.HTTPError as error:
        print(f"Endpoint rejected the request: HTTP {error.code}", file=sys.stderr)
        return 1
    except (urllib.error.URLError, KeyError, IndexError, TypeError) as error:
        print(f"Retrieval request failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
