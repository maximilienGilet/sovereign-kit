# Qwen Solo Capacity Profiles Design

## Goal

Expose three deterministic Qwen Solo recipes so users can choose between maximum context, reliable concurrency, and an experimental maximum-concurrency configuration on one RTX 5090.

The variants share the same official-derived Unsloth GGUF, immutable model revision, checksum, precompiled llama.cpp image, native MTP settings, endpoint behavior, and 16,384-token maximum output. They differ only in slot count, per-slot context, and KV-cache precision.

## Recipes

### Qwen Solo — Full Context

- Status: experimental, recommended among the Solo capacity profiles.
- Parallel requests: 1.
- Context per request: 262,144 tokens.
- Maximum output: 16,384 tokens, included within the per-request context ceiling.
- K/V cache: `q8_0`.
- Intent: preserve maximum context and higher KV-cache precision with enough memory margin to load on a 32 GB RTX 5090.

### Qwen Solo — Dual

- Status: experimental.
- Parallel requests: 2.
- Context per request: 131,072 tokens.
- Maximum output: 16,384 tokens, included within each request's context ceiling.
- Aggregate llama.cpp context: 262,144 tokens.
- K/V cache: `q8_0`.
- Intent: support two concurrent agents while retaining the current cache precision and approximately the same aggregate KV capacity as Full Context.

### Qwen Solo — Dual Max Lab

- Status: lab.
- Parallel requests: 2.
- Context per request: 262,144 tokens.
- Maximum output: 16,384 tokens, included within each request's context ceiling.
- Aggregate llama.cpp context: 524,288 tokens.
- K/V cache: `q4_0` for both K and V.
- Intent: test two full-context requests on a 32 GB RTX 5090.
- Warning: GPU fit and quality at long context are unvalidated. Lower KV precision can affect output quality, and CUDA/MTP overhead may still cause an out-of-memory failure.

## Recipe Picker

The three profiles appear as separate entries in the main recipe list. Their summary line exposes the decision directly:

- `1 × 262K · KV q8 · recommended Solo profile`
- `2 × 131K · KV q8 · concurrent`
- `2 × 262K · KV q4 · LAB / unvalidated memory fit`

The detail cockpit keeps showing configured context, output, and concurrency. It must also show KV-cache precision so the Lab trade-off is not hidden. These are configuration profiles of one model, not benchmark comparisons with other models.

## Runtime Boundary

KV-cache types become explicit recipe fields rather than being hard-coded in the llama.cpp launcher. Validation accepts only cache formats supported by the pinned runtime and supplies safe defaults for older/custom recipes. The launcher passes the selected K/V values to `--cache-type-k` and `--cache-type-v`.

No automatic fallback changes a running recipe. If Dual Max Lab fails to load, provisioning reports the CUDA allocation error and recommends Full Context or Dual. A retry with a different profile is an explicit user choice, keeping checkpoints and client metadata deterministic.

## Storage and Downloads

All three recipes use the same model revision and filename. On the same retained instance, the verified model file is reusable; selecting another capacity profile must not require another model download. A newly created ephemeral instance still downloads the model normally because it has no shared persistent storage.

## Compatibility

The existing `qwen-solo-rtx5090` recipe ID becomes Full Context so existing selection references move to the safe one-slot configuration. Dual and Dual Max Lab receive new stable IDs. Qwen Solo Uncensored and Qwen Studio remain unchanged.

Existing deployment checkpoints retain their snapshotted launch configuration and are not silently converted to another capacity profile.

## Verification

Automated tests must prove:

- all three recipes parse and validate with the exact literal limits above;
- Full Context launches `1 × 262144` with `q8_0` K/V;
- Dual launches `2 × 131072` with `q8_0` K/V;
- Dual Max Lab launches `2 × 262144` with `q4_0` K/V;
- the three variants share the exact model identity, checksum, runtime image, and source revision;
- picker summaries and detail views expose concurrency, context, and KV precision without overflow;
- existing custom llama.cpp recipes receive the documented default cache precision;
- Qwen Studio and Qwen Solo Uncensored behavior does not change.

The automated suite cannot establish GPU memory fit or long-context quality. Dual Max Lab remains labelled unvalidated until a real RTX 5090 deployment completes model loading and progressively larger inference tests.
