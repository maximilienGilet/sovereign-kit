# Any OpenAI-compatible application

Many applications call an OpenAI-compatible API but expose the settings under different names. Look for fields such as:

| Application setting | Sovereign Kit value |
|---|---|
| API base URL / endpoint | `http://127.0.0.1:30000/v1` |
| API key | `local-qwen-tunnel` |
| Model | `qwen3.8-27b-nvfp4` |

Use the application’s custom-provider or OpenAI-compatible-provider setting. Keep the base URL on local loopback and disable every automatic fallback provider.

## Verify

1. Start the SSH tunnel.
2. Run `sovkit doctor` and resolve any failure.
3. Send a non-sensitive test prompt from the application.
4. Stop the tunnel and confirm the application fails rather than switching to another provider.

The provided Pi and OpenCode integrations are the V1 validated adapters. For another application, this recipe configures the endpoint but does not prove the application’s tool permissions, prompt storage, browser exposure, or multi-user access controls.
