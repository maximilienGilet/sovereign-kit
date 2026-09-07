FROM vllm/vllm-openai@sha256:2286e8533ca8b6bc777594bae30524f1426ba46ca21797524e06df6a94b06635

USER root

ARG MODEL_REPOSITORY=gittensor-model-hub/Qwen3.8-27B-NVFP4-RTX5090-LMHead4
ARG MODEL_REVISION=f7866ff274f7b2ab34d9d828a7bccff51919b711

ENV SOVKIT_MODEL_REPOSITORY=${MODEL_REPOSITORY}
ENV SOVKIT_MODEL_REVISION=${MODEL_REVISION}

LABEL org.opencontainers.image.source="https://github.com/maximilienGilet/sovereign-kit" \
      org.opencontainers.image.base.digest="sha256:2286e8533ca8b6bc777594bae30524f1426ba46ca21797524e06df6a94b06635" \
      ai.sovereign-kit.model="gittensor-model-hub/Qwen3.8-27B-NVFP4-RTX5090-LMHead4@f7866ff274f7b2ab34d9d828a7bccff51919b711" \
      ai.sovereign-kit.kv-cache="turboquant_4bit_nc" \
      ai.sovereign-kit.speculative="qwen3_5_mtp:4" \
      ai.sovereign-kit.patch-provenance="none; native vLLM 0.28.0 support"

COPY smoke.py /opt/sovereign-kit/smoke.py
RUN python3 /opt/sovereign-kit/smoke.py
