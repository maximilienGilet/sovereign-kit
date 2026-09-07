# Qwen Solo: official-derived GGUF verification

Verified 2026-09-04. Research only; no production configuration or provider changes.

## Recommendation

Use **`unsloth/Qwen3.8-27B-GGUF` / `Qwen3.8-27B-UD-Q4_K_XL.gguf`**, pinned below, for the proposed official-derived Qwen Solo rebuild. This is the publisher's documented 27B four-bit example. Its GGUF directly contains the native MTP head; **no separate MTP sidecar is required** for the existing llama.cpp embedded-MTP strategy. Retain the runtime as a candidate and benchmark on the actual RTX 5090 before declaring performance or production readiness. [Unsloth guide](https://unsloth.ai/docs/models/qwen3.8.md), [pinned artifact](https://huggingface.co/unsloth/Qwen3.8-27B-GGUF/blob/4ca720788d1e01f1bff70c033e0d0028fd02e502/Qwen3.8-27B-UD-Q4_K_XL.gguf).

## Artifact pin

Repository revision, read from the public Hugging Face API:

`4ca720788d1e01f1bff70c033e0d0028fd02e502`

| Role | Filename | Exact bytes | Published SHA-256 |
| --- | --- | ---: | --- |
| Recommended | `Qwen3.8-27B-UD-Q4_K_XL.gguf` | 17,559,178,144 | `3f227079003add2511437e5b1e94812e363385225bf6a9b47b0054a72bc8b01e` |
| Smaller alternative | `Qwen3.8-27B-UD-Q4_K_M.gguf` | 16,464,440,224 | `322e194ff79741c7baa497c240f677f54b201b0efab44ca8e50f122b39123482` |

These are **published full-file checksums**, not locally recomputed checksums: this investigation downloaded headers, not the entire models. Production download must verify the full SHA-256. Source: [HF model API with blob metadata](https://huggingface.co/api/models/unsloth/Qwen3.8-27B-GGUF?blobs=true), [XL file metadata](https://huggingface.co/unsloth/Qwen3.8-27B-GGUF/blob/4ca720788d1e01f1bff70c033e0d0028fd02e502/Qwen3.8-27B-UD-Q4_K_XL.gguf), [M file metadata](https://huggingface.co/unsloth/Qwen3.8-27B-GGUF/blob/4ca720788d1e01f1bff70c033e0d0028fd02e502/Qwen3.8-27B-UD-Q4_K_M.gguf).

## Verified findings

### Official lineage, not an uncensored fine-tune

The official Qwen repository distributes the post-trained Qwen3.8-27B model and documents MTP. Unsloth's model card names `Qwen/Qwen3.8-27B` as its base; the HF API classifies the relationship as `base_model:quantized:Qwen/Qwen3.8-27B`. Both inspected GGUF headers identify organization `Qwen` and that same original repository. This supports the intended choice of a quantization of the official model, rather than an uncensored/abliterated derivative. [Official Qwen card](https://huggingface.co/Qwen/Qwen3.8-27B), [Unsloth card](https://huggingface.co/unsloth/Qwen3.8-27B-GGUF/blob/4ca720788d1e01f1bff70c033e0d0028fd02e502/README.md), [HF API](https://huggingface.co/api/models/unsloth/Qwen3.8-27B-GGUF?blobs=true).

Do not describe it as byte-identical or behaviorally identical to official full precision: quantization changes representation, and Unsloth explicitly documents developer-role and tool-formatting changes. The publisher's source declaration is not a complete reproducible tensor-by-tensor audit of its conversion pipeline. **Confidence: MEDIUM for complete provenance; strong first-party declaration, no independent full conversion audit.** [Unsloth card](https://huggingface.co/unsloth/Qwen3.8-27B-GGUF/blob/4ca720788d1e01f1bff70c033e0d0028fd02e502/README.md).

### Embedded native MTP: directly inspected

HTTP range requests read the first 16 MiB of **both pinned files**. A minimal little-endian GGUF parser read every metadata entry and tensor descriptor; descriptor parsing ended at byte 10,996,621. Both reported:

```text
GGUF version: 3
tensor count: 866
metadata entries: 50
general.architecture: qwen35
general.base_model.count: 1
general.base_model.0.organization: Qwen
general.base_model.0.repo_url: https://huggingface.co/Qwen/Qwen3.8-27B
qwen35.block_count: 65
qwen35.nextn_predict_layers: 1
```

Both contain 15 `blk.64.*` descriptors, including the attention/FFN tensors and:

```text
blk.64.nextn.eh_proj.weight          [10240, 5120]
blk.64.nextn.enorm.weight            [5120]
blk.64.nextn.hnorm.weight            [5120]
blk.64.nextn.shared_head_norm.weight [5120]
```

This is direct artifact-header evidence of an embedded native MTP block, not an inference from filenames. Sources: [pinned XL binary](https://huggingface.co/unsloth/Qwen3.8-27B-GGUF/resolve/4ca720788d1e01f1bff70c033e0d0028fd02e502/Qwen3.8-27B-UD-Q4_K_XL.gguf), [pinned M binary](https://huggingface.co/unsloth/Qwen3.8-27B-GGUF/resolve/4ca720788d1e01f1bff70c033e0d0028fd02e502/Qwen3.8-27B-UD-Q4_K_M.gguf). The publisher separately confirms MTP availability in its [guide](https://unsloth.ai/docs/models/qwen3.8.md) and [maintainer discussion](https://huggingface.co/unsloth/Qwen3.8-27B-GGUF/discussions/12). **Confidence: HIGH for embedded-header presence.**

The repository also publishes `MTP/mtp-Qwen3.8-27B-Q4_0.gguf` (1,369,590,656 bytes). Its existence does **not** mean the inspected main artifacts lack MTP. [Sidecar metadata](https://huggingface.co/unsloth/Qwen3.8-27B-GGUF/blob/4ca720788d1e01f1bff70c033e0d0028fd02e502/MTP/mtp-Qwen3.8-27B-Q4_0.gguf).

### Existing runtime's loading path is compatible in source

At llama.cpp revision `86b351fd64d5ebbf1ba795ffd60c8f4a8c958613`, `common/speculative.cpp` lines 2560–2565 create the MTP draft context against the target model itself. `common/common.cpp` line 1317 identifies an MTP context as sharing the main model's weights. `src/models/qwen35.cpp` supports a one-block Qwen35 MTP graph and asserts `n_layer_nextn == 1`, matching the inspected headers. Sources: [speculative loading](https://github.com/ggml-org/llama.cpp/blob/86b351fd64d5ebbf1ba795ffd60c8f4a8c958613/common/speculative.cpp#L2560), [shared-model context](https://github.com/ggml-org/llama.cpp/blob/86b351fd64d5ebbf1ba795ffd60c8f4a8c958613/common/common.cpp#L1317), [Qwen35 MTP graph](https://github.com/ggml-org/llama.cpp/blob/86b351fd64d5ebbf1ba795ffd60c8f4a8c958613/src/models/qwen35.cpp#L484).

**No source-level sidecar compatibility blocker was identified.** This supports a direct artifact swap using `--spec-type draft-mtp`; it does not establish that an untested deployment has passed load, tool-use, long-context, or performance tests. One MTP block is not the same as a maximum speculative draft depth; a draft-depth setting of two must still be validated in the actual service.

## Performance and acceptance limits

Unsloth advises roughly 1–2 GB extra headroom for MTP. Use that as a planning caveat, not an exact VRAM prediction for this service. No RTX 5090 benchmark was performed here; **do not promise 133 tokens/s** or transfer NVFP4/B200 throughput figures to GGUF/CUDA on a 5090. [Publisher guide](https://unsloth.ai/docs/models/qwen3.8.md).

Before promotion: verify downloaded SHA-256, load the pinned runtime, confirm active embedded MTP in logs, test representative tool calls and thinking controls, compare MTP on/off on identical prompts, and measure prefill/decode throughput and VRAM at the chosen context/concurrency. Keep quant, engine, cache format, context length, and draft depth in benchmark records.

## Source evaluation and method

- **Qwen:** primary authority for official model identity and architecture; not an independent evaluator of its own quality.
- **Unsloth:** primary authority for its quant provenance and recommendations; commercial/maintainer performance claims were not treated as independent guarantees.
- **Hugging Face:** primary artifact registry metadata; public hashes were cross-checked against file pages.
- **llama.cpp source:** exact runtime implementation, preferable to generic current-version instructions for compatibility.

Method: official card → quant card/API → pinned binary header → exact runtime loading/model source. Overall confidence is **MEDIUM for deployment readiness**, despite strong evidence for artifact identity and embedded MTP, because no full download checksum, GPU execution, or workload benchmark occurred. This note does not approve or implement a runtime migration.
