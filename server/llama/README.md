# Precompiled Solo runtime

This image builds the pinned llama.cpp revision once, outside rented instances.
The three official-derived capacity profiles and Qwen Solo Uncensored use the
anonymously verified image
`ghcr.io/maximiliengilet/sovereign-kit-llama@sha256:a9ceb65277f2f4dfd4cc3b5896f4398703e323249dd9bf87eb20b17b9f467239`
from source `86b351fd64d5ebbf1ba795ffd60c8f4a8c958613`.
Rebuild/reinstall the CLI to pick up the embedded recipe changes before creating
a new instance. Existing saved deployments are not migrated (see below).

It targets Linux amd64, CUDA 13.3.1 and Blackwell architecture 120a, with native
MTP support. CPU-native tuning is disabled so binaries do not inherit the build
runner's CPU instruction set. It contains no model weights or API credentials.

The dedicated `Publish llama CUDA image` workflow builds the image, checks its
command-line options without a GPU, verifies distinct SSH host identities for
separate containers, then publishes a commit-tagged image to GHCR. Host keys
generated during package installation are removed in that same image layer;
missing keys are generated at container startup. Existing keys survive restarts.

Only a successfully checked, anonymously accessible image digest should enter
a recipe. `runtime.precompiled = true` selects the bundled binary at
`/opt/sovereign-kit/llama/llama-server`; a missing or mismatched runtime fails
without attempting package installation or compilation. GGUF download and
SHA256 verification still happen on the instance.

Saved deployments retain their original recipe snapshot. An existing deployment
using the old CUDA development image is not migrated by changing built-in
recipes. This prevents resume from silently changing its deployment identity.

The final image intentionally retains the verified CUDA development base for
now, so it is larger than a runtime-only image. Ubuntu package repositories are
live at build time; the published image digest freezes the resulting artifact,
but rebuilding these sources is not guaranteed to reproduce the same bytes.

Build and smoke tests do **not** validate GPU model loading, maximum context,
throughput, or Vast's full provisioning integration. Those require a separate
authorized deployment test.

## Solo capacity profiles

Qwen Solo Full Context requests 1 slot with 262,144 context tokens and `q8_0`
K/V caches. Qwen Solo Dual requests 2 slots with 131,072 context tokens per slot
and `q8_0` K/V caches. Qwen Solo Dual Max Lab requests 2 slots with 262,144
context tokens per slot and `q4_0` K/V caches; its memory fit and long-context
quality are deliberately labelled unvalidated. All three advertise 16,384
maximum output tokens. Input plus output must fit within each per-slot context.
Qwen Solo Uncensored remains at 4 slots with 65,536 context tokens per request
and 4,096 output tokens.
`serve.context_window` is the per-request limit published to client profiles;
`serve.max_running_requests` controls `--parallel`. The launcher allocates their
product as `--ctx-size` and explicitly caps each slot with
`--kv-unified-per-slot`. Validation allows 1–4 slots, at most 262,144 tokens per
slot and 524,288 aggregate context tokens. This is a configuration guardrail,
not a measured hardware capacity. The selected cache types are passed directly
to llama.cpp; no fallback silently changes a profile.
This bounds token capacity, not all GPU memory: per-sequence MTP/recurrent state
adds overhead, so simultaneous full-context loading and performance require validation.

`--metrics` enables read-only server counters over the existing private tunnel.
No extra public port is opened. Existing processes and saved recipe snapshots
are not automatically changed; applying the new layout requires a controlled
server reconfiguration/restart and updating the matching client context metadata.
Do not update only the client limits or only the resume fingerprint.
