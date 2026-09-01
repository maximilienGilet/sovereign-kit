# Architecture and measured limits

## Shared-endpoint topology

```text
MacBook
  Pi / Oh-My-Pi principal
  ├─ Pi subagent A
  ├─ Pi subagent B
  ├─ Pi subagent C
  └─ Pi subagent D
         │ all use one provider/model
         ▼
127.0.0.1:30000 (SSH local forward)
         ▼
SGLang, bound to remote loopback
         ▼
Qwen3.8-27B NVFP4
```

This is request concurrency inside one SGLang server, not five co-hosted copies of the model.

## Validated benchmarks

### Long-context principal capacity

On an RTX PRO 6000 S (96 GB), Qwen3.8-27B NVFP4/SGLang completed:

```text
Input       246,000 tokens
Output      8,192 tokens maximum
Total       254,192 / 262,144 tokens
Result      HTTP 200, no OOM and no truncation
Decode      ~48.2 tok/s near the context limit
```

### Shared principal plus workers

On the same GPU/server class:

```text
Principal   128K input + 4,096 output
Workers     4 × (32K input + 1,024 output)
Concurrent  5 requests
Result      all HTTP 200, no OOM
```

Observed output decode after first token:

```text
Principal   ~55.7 tok/s
Workers     ~34.6–51.3 tok/s each
```

The five prompts were deliberately submitted together. The principal received its first token after ~60 s because all large prefills competed. That is evidence for admission control, not a desired UX configuration.

## Production routing recommendation

1. Submit/pre-fill the principal first.
2. Let it produce its first token.
3. Dispatch worker tasks with bounded input and output budgets.
4. Prefer 16K–32K workers, four at most until workload-specific measurements say otherwise.
5. Provide compact worker summaries to the parent instead of full transcripts.

Large-context workload, real tool usage and high variance in model output must be benchmarked with the actual harness before setting hard SLAs.
