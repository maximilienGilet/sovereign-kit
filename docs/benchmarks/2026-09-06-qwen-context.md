# Qwen endpoint benchmark — 2026-09-06

These measurements were collected from the user's existing endpoint through its
local SSH tunnel. They are observations, not a throughput guarantee or a v1
qualification. The live API reported `unsloth/Qwen3.8-27B-GGUF`, a 262,144-token
context limit, and a 17,548,181,504-byte model. Exact deployed recipe, GPU,
KV precision and runtime revision were not independently recorded; do not
attribute these numbers to a specific recipe variant without repeating the
test with that metadata.

## Method

Streaming chat completions, temperature zero, thinking disabled, prompt cache
disabled. Synthetic repeated JavaScript queue-processing code with a request for
an engineering review; this is not a representative repository-quality test.
The model was already loaded: “cold” means no reused prompt tokens, not cold boot.
Wall time starts after prompt construction/tokenization. Generation throughput
comes from the server's `predicted_per_second`, not tokens divided by total wall
time. First-token latency is measured at the client through the tunnel.

| Input tokens | Output tokens | First token (s) | Generation (tok/s) | Total (s) |
|---:|---:|---:|---:|---:|
| 181 | 855 | 1.255 | 124.24 | 8.106 |
| 4,143 | 512 | 2.756 | 118.91 | 6.977 |
| 16,428 | 512 | 7.345 | 114.25 | 11.806 |
| 32,819 | 512 | 13.210 | 105.05 | 18.076 |
| 131,128 — run 1 | 512 | 80.141 | 70.94 | 87.348 |
| 131,128 — run 2 | 512 | 79.257 | 70.37 | 86.519 |

Every row reported zero cached prompt tokens. The first row finished naturally;
the others hit the intentionally short 512-token benchmark output cap.
For the repeated 128k case, prompt processing took 77.239 seconds at 1,697.70
tokens/s, versus 7.262 seconds generating the response. Health was `ok` afterward.

## Two concurrent requests

Two 8,249-token prompts each generated 512 tokens. First-token times were 5.863
and 7.481 seconds. Server generation rates were 69.46 and 87.91 tok/s; individual
wall times were 13.219 and 13.293 seconds. The batch took 14.078 seconds including
prompt preparation, for 72.74 output tokens/s end to end.

## Interpretation and limits

Large uncached context primarily increases time to the first token. A generation
rate of 70 tok/s does not mean a 128k-context request responds immediately.
These runs do not validate 262k input, two concurrent 128k requests, 16k output,
reasoning quality, prompt-cache gains, long-term stability, or cold startup.
Keep recipe status experimental/lab. Capture the full deployed configuration
and repeat representative cold/warm coding workloads before publishing
recipe-specific performance promises.
