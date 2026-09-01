# Internal text assistant

Use one approved chat client or a small internal UI to send text to the private route. This is a **single-user or client-owned access-control** pattern, not a shared chat product.

## When it fits

- summarize approved text;
- draft, rewrite, translate, or classify text;
- answer from text deliberately supplied in the current request.

## Route

```text
internal UI or script → local approved client → Sovereign Kit route → Qwen/SGLang
```

The UI must not expose the loopback endpoint to a LAN, browser audience, or other macOS accounts.

## Minimal connectivity check

After `sovkit doctor` passes:

```bash
curl --fail --silent --show-error \
  http://127.0.0.1:30000/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer local-qwen-tunnel' \
  -d '{
    "model": "qwen3.8-27b-nvfp4",
    "messages": [{"role": "user", "content": "Summarize this approved text."}]
  }'
```

## What the client still owns

- user login and permissions;
- conversation history and retention;
- UI security and browser exposure;
- document permission checks before text is sent;
- moderation, audit, and rate limits where required.

Do not present this pattern as multi-user tenant isolation or a managed chat service.
