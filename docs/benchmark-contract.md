# Benchmark contract

Publish a performance or capacity position only after measuring the exact profile.

Every record must identify the GPU or managed endpoint, runtime image digest or provider hostname, model and revision, quantization, context/KV settings, request shape, concurrency order, HTTP result, timings, and exclusions.

- **Solo:** one capacity test, one realistic harness workload, a concurrency/TTFT sweep, and OOM/recovery observations.
- **Studio:** rerun both existing synthetic workloads against the digest-locked recipe, then add a realistic harness workload.
- **Lite:** capture endpoint/region/retention review, rate limits, authentication path, no-fallback behavior, compatibility, latency, and errors.

Never commit client prompts, active hosts, raw logs containing client material, tokens, identities, or credentials.
