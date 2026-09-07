# Qwen3.8-27B NVFP4 runtime verification

**Scope correction after reading the article:** the article uses llama.cpp and the HauhauCS Q4_K_P GGUF, not this NVFP4/SGLang route. The earlier findings below remain a separate alternative, not a reproduction of the article. See the final section for the exact article model.

Verified 2026-09-04. Scope: one RTX 5090, existing SSH access; no provider changes or Cloudflare configuration performed.

## Verified findings

The concrete replacement candidate is the official cookbook image, pinned to the registry digest observed today:

```text
lmsysorg/sglang:dev-qwen38-27b-dflash2@sha256:616a3e97f45191af975896cfa644279096cb31bd408a071c2e99ca7209c3cafe
```

The [SGLang Qwen3.8-27B cookbook](https://docs.sglang.io/cookbook/autoregressive/Qwen/Qwen3.8-27B) explicitly names that tag and alternatively pins source commit `1cf2b8c54d81802abc15dcf23a29b9cc687bc01e`. It reports RTX 5090 validation at 8192 input / 1024 output tokens, concurrency one. Its no-speculation setting is `--mem-fraction-static 0.90`; SM120 uses `--attention-backend flashinfer`. It recommends NVFP4 (~16.5 GB weights), `--chunked-prefill-size 2048`, and parsers `qwen3` / `qwen3_coder`. DSpark uses a separate draft checkpoint and different memory allocation; do not copy its settings into a no-speculation launch. These are upstream-tested settings, not measurements from this host. **Confidence: medium for deployment success; high for what the documentation states.**

[Docker Hub tag metadata](https://hub.docker.com/v2/repositories/lmsysorg/sglang/tags/dev-qwen38-27b-dflash2) was fetched directly using HTTPS. It returned an active OCI index, last pushed `2026-08-22T03:26:48.506535Z`, with the index digest above. Linux/amd64 manifest: `sha256:b91d664a8e4825afc16ab831c6035a6c88ac20ef8bd26da4fe2b9813a9f44376`; compressed size 14,676,200,256 bytes. Linux/arm64 is also present. No local image pull or inspection was performed. **Confidence: high for registry identity, not runtime behavior.**

The [model configuration](https://huggingface.co/RadixArk/Qwen3.8-27B-NVFP4/raw/main/config.json) declares `model_type: qwen3_5`, architecture `Qwen3_5ForConditionalGeneration`, and `transformers_version: 5.12.1`. Quantization is ModelOpt `MIXED_PRECISION`, not a uniform FP4 tensor layout. The [publisher's model card](https://huggingface.co/RadixArk/Qwen3.8-27B-NVFP4) identifies NVFP4 MLP/head, FP8 attention weights, BF16 MTP/vision tensors, and SGLang support. The card's example uses four GPUs; the single-5090 evidence comes from the runtime cookbook, not that example. **Confidence: high for model metadata.**

At the cookbook's exact commit, [SGLang dependencies](https://raw.githubusercontent.com/sgl-project/sglang/1cf2b8c54d81802abc15dcf23a29b9cc687bc01e/python/pyproject.toml) pin Transformers `5.12.1`, FlashInfer `0.6.17`, torch `2.13.0`, and sglang-kernel `0.4.6.post1`. Its [model implementation](https://raw.githubusercontent.com/sgl-project/sglang/1cf2b8c54d81802abc15dcf23a29b9cc687bc01e/python/sglang/srt/models/qwen3_5.py) implements the Qwen3.5 architecture family. This supports replacing the old runtime, rather than changing the model's declared architecture to bypass its error. These are source dependencies, not independently inspected contents of the Docker image. **Confidence: high for source support.**

The pinned source [Dockerfile](https://raw.githubusercontent.com/sgl-project/sglang/1cf2b8c54d81802abc15dcf23a29b9cc687bc01e/docker/Dockerfile) defaults to CUDA `13.0.3` (build arguments can override this). [NVIDIA's compatibility table](https://docs.nvidia.com/deploy/cuda-compatibility/minor-version-compatibility.html) requires driver branch 580 or newer for CUDA 13.x minor-version compatibility and warns that PTX / newer features may need newer drivers. Verify the actual image CUDA and host driver before launching; this research does not establish that the existing host meets them. **Confidence: high for NVIDIA requirement.**

## Confidence assessment

Overall: **medium** for this deployment. The model publisher, runtime maintainer, Docker registry, and NVIDIA independently establish their respective facts, but three independent successful reproductions of this exact combination were not available. SGLang's performance claims are first-party, not independently benchmarked here.

## Methodology and limitations

Compared live model metadata, the runtime's model-specific cookbook, immutable-commit source, registry metadata, and NVIDIA documentation. Registry request was read-only. No claim is made that a stable numbered SGLang release or vLLM image supports this exact mixed checkpoint: this verification found a concrete documented SGLang development image instead. The mutable model `main` revision should be separately pinned for reproducibility.

Remaining gates: image dependency inspection, driver compatibility, model load, readiness, actual completion and tool-call smoke tests, VRAM/long-context measurements. A valid registry digest does not demonstrate a successful GPU launch. No claim is made that the article's peak speed or long context has been reproduced.

## Article-aligned llama.cpp / GGUF verification

The exact model exists publicly and does not require substitution. [Hugging Face API](https://huggingface.co/api/models/HauhauCS/Qwen3.8-27B-Uncensored-HauhauCS-Aggressive-MTP-GGUF) returned revision `993a5971fda8f30dd1b7eb2654792ba4415c7460`. The [immutable file tree](https://huggingface.co/api/models/HauhauCS/Qwen3.8-27B-Uncensored-HauhauCS-Aggressive-MTP-GGUF/tree/993a5971fda8f30dd1b7eb2654792ba4415c7460?recursive=false&expand=false) identifies:

```text
repo: HauhauCS/Qwen3.8-27B-Uncensored-HauhauCS-Aggressive-MTP-GGUF
revision: 993a5971fda8f30dd1b7eb2654792ba4415c7460
file: Qwen3.8-27B-Uncensored-HauhauCS-Aggressive-Q4_K_P.gguf
size_bytes: 17923393664
sha256: ba36dc3c2b2ff5e0aa5d71092a8894546996a6a119ae391803dda07cdc08516d
```

Its LFS content hash matches the publisher's [SHA256SUMS](https://huggingface.co/HauhauCS/Qwen3.8-27B-Uncensored-HauhauCS-Aggressive-MTP-GGUF/raw/993a5971fda8f30dd1b7eb2654792ba4415c7460/SHA256SUMS). This verifies published identity; the 17.92 GB file was not downloaded or locally hashed.

The pinned [publisher README](https://huggingface.co/HauhauCS/Qwen3.8-27B-Uncensored-HauhauCS-Aggressive-MTP-GGUF/raw/993a5971fda8f30dd1b7eb2654792ba4415c7460/README.md) says every text GGUF preserves the native NextN head. Embedded MTP uses the target file alone with current upstream llama.cpp; **FastMTP is a different optional route requiring a separate sidecar and runtime patch**. Do not add that sidecar/patch when reproducing the article's native MTP. The publisher describes K_P as a standard GGUF with custom per-tensor quantization, not a new runtime tensor format. These compatibility statements are publisher claims, not an independent binary inspection. Its cited reference GPU is RTX PRO 6000 / RTX 6000 Ada, not proof of RTX 5090 throughput.

A concrete current upstream source candidate is [`86b351fd64d5ebbf1ba795ffd60c8f4a8c958613`](https://github.com/ggml-org/llama.cpp/commit/86b351fd64d5ebbf1ba795ffd60c8f4a8c958613), returned by the GitHub API for master on 2026-09-04 (commit timestamp 08:28:23 UTC). At this exact revision, [common/arg.cpp](https://github.com/ggml-org/llama.cpp/blob/86b351fd64d5ebbf1ba795ffd60c8f4a8c958613/common/arg.cpp) defines `--spec-type`, `--spec-draft-n-max`, and `--spec-draft-p-min`; [common/speculative.cpp](https://github.com/ggml-org/llama.cpp/blob/86b351fd64d5ebbf1ba795ffd60c8f4a8c958613/common/speculative.cpp) maps `draft-mtp` to its native MTP implementation. This is an immutable **candidate source pin**, not the article's unknown build revision and not a GPU-tested recommendation.

Article-provided settings to preserve in the implementation plan: native draft-MTP depth 2 / p-min 0, context 262144, parallel 1, q8_0 K/V, batch 2048 / microbatch 512, flash attention on, all GPU layers, Jinja, no warmup, CUDA 13.3 and architecture 120a. Their full combined fit, performance, CUDA toolchain availability, and host compatibility remain unverified in this research. A bounded smoke test and then a long-context test must establish those claims. Overall confidence: **high for exact model identity and upstream flag presence; medium for publisher-reported embedded MTP compatibility; unverified for article performance reproduction.**

## Published CUDA 13.3 build base (follow-up)

The CUDA toolchain image is available. [Docker Hub metadata](https://hub.docker.com/v2/repositories/nvidia/cuda/tags/13.3.1-devel-ubuntu24.04) verified this exact reference on 2026-09-04:

```text
nvidia/cuda:13.3.1-devel-ubuntu24.04@sha256:4ff859525f99de5782aa73607ce24219b07dddd48d12b97c1c301d7e1cfb0a87
linux/amd64 manifest: sha256:03c372fd9c65fe7739279f8c65473b315dc61efaaffab03e1e65bc7be7aee61e
compressed amd64 size: 4120505252 bytes
```

The tag was last pushed 2026-07-28. Direct read-only Registry V2 manifest/config requests for that amd64 digest confirmed `CUDA_VERSION=13.3.1`, installation of `cuda-minimal-build-13-3`, and NVIDIA's entrypoint. The image history explicitly purges curl after key installation and does not explicitly install Python, Git, or CMake. **Do not rely on these executables being present.** Source-build/reconciliation setup must first ensure `python3`, `curl`, `ca-certificates`, `git`, `cmake`, `build-essential`, and `libssl-dev` (plus whichever compiler the chosen CUDA build requires). Bootstrap must precede Python-dependent reconciliation, not be hidden behind it. Registry metadata is not a full installed-package inspection.

The [upstream CUDA Dockerfile at the selected commit](https://github.com/ggml-org/llama.cpp/blob/86b351fd64d5ebbf1ba795ffd60c8f4a8c958613/.devops/cuda.Dockerfile) explicitly installs its build dependencies including Python, Git, CMake, build-essential and libssl-dev; its default is GCC 14. Its server-only final stage installs curl but not Python; the full stage installs Python. A development CUDA base and a ready-made llama.cpp server image are not interchangeable deployment prerequisites.

For comparison, official [GHCR server-cuda](https://github.com/ggml-org/llama.cpp/pkgs/container/llama.cpp) was registry-verified at index digest `sha256:7f87a3bbe3143cdb857f5c84f82d2c15528be70ca0e7c5ade9a47d56b791f93f` (amd64 `sha256:fc76b629103635823c4ba847acc493d09da5ad279c3de9ebe2d6338770ad120a`). Its inspected config identifies build `b10795`, source `6703d7894c70e8b076ce4608157d056e42e6889c`, created 2026-09-04, **CUDA 12.8.1**, and entrypoint `/app/llama-server`. It is therefore not the article's CUDA 13.3 source build; no recommendation is made to silently swap it in.

The selected llama.cpp [argument definitions](https://github.com/ggml-org/llama.cpp/blob/86b351fd64d5ebbf1ba795ffd60c8f4a8c958613/common/arg.cpp) provide `--hf-repo` and `--hf-file`, but inspection found no `--hf-revision`. For reproducibility, fetch the GGUF through its immutable `resolve/<revision>/<file>` URL, verify the published SHA256, then launch with a local `--model` path. Do not invent a revision flag.

Confidence: high for published image identity and inspected config fields; actual CUDA compile, `120a` targeting, driver compatibility, and launch still require runtime validation. All registry operations here were read-only and downloaded metadata only.
