# OpenAI-compatible integrations

Sovereign Kit exposes one local OpenAI-compatible route:

```text
http://127.0.0.1:30000/v1
```

Use it in any application that lets you set an OpenAI-compatible base URL, API key, and model name. The current V1 route is keyless at the model server; `local-qwen-tunnel` is a non-secret placeholder required by many SDKs.

Before any integration, open the tunnel and run:

```bash
sovkit doctor
```

Then configure the application with:

```text
OPENAI_BASE_URL=http://127.0.0.1:30000/v1
OPENAI_API_KEY=local-qwen-tunnel
OPENAI_MODEL=qwen3.8-27b-nvfp4
```

Do not add a public-provider fallback. If the local route fails, fix the tunnel or server.

## Recipes

- [Python OpenAI SDK](integrations/openai-python.md)
- [JavaScript OpenAI SDK](integrations/openai-javascript.md)
- [Any OpenAI-compatible application](integrations/generic-openai-compatible.md)

The endpoint is local to the Mac account running the tunnel. It is not a public endpoint for a browser frontend or a shared multi-user application.
