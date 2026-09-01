# Benchmarks

These reports record synthetic tests on a specific GPU, server build, model quantization, context length, and request pattern. They are not hardware recommendations, cost estimates, or promises of model quality.

| Report | What it tested | Result |
|---|---|---|
| [262K single-agent](benchmark-262k-rtx-pro-6000-sglang.md) | One request with 246K input and 8,192 output tokens | HTTP 200 without OOM; about 48.2 tok/s decode near the limit |
| [128K principal + 4 × 32K workers](benchmark-shared-128k-plus-4x32k.md) | Five concurrent requests totaling 256K input tokens | All requests completed; the principal TTFT was about 60 seconds because the prefills competed |

The reports use a historical SGLang development image. They are evidence of what was measured, not a production launch recipe. Use [server setup](../docs/server.md) for the reviewed reference contract.

Before choosing a GPU, model revision, context budget, or concurrency limit, benchmark the actual harness and workload you plan to run.
