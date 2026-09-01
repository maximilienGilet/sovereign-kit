#!/usr/bin/env bash
# Run the reviewed SGLang reference configuration on a Linux GPU host with Docker + NVIDIA Container Toolkit.
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
image="$(<"$repo_dir/server/image.lock")"
model_path="${SOVKIT_MODEL_PATH:-RadixArk/Qwen3.8-27B-NVFP4}"
model_revision="${SOVKIT_MODEL_REVISION:-319f741cce68d7914884900c138a1fbb70a42f30}"
hf_home="${HF_HOME:-$HOME/.cache/huggingface}"

command -v docker >/dev/null 2>&1 || { printf 'Docker is required.\n' >&2; exit 1; }
mkdir -p "$hf_home"

exec docker run --rm \
  --gpus all \
  --network host \
  --ipc host \
  --mount "type=bind,source=$hf_home,target=/root/.cache/huggingface" \
  "$image" \
  sglang serve \
    --trust-remote-code \
    --model-path "$model_path" \
    --revision "$model_revision" \
    --context-length 262144 \
    --kv-cache-dtype fp8_e4m3 \
    --mem-fraction-static 0.85 \
    --attention-backend flashinfer \
    --chunked-prefill-size 2048 \
    --max-running-requests 5 \
    --cuda-graph-max-bs 5 \
    --reasoning-parser qwen3 \
    --tool-call-parser qwen3_coder \
    --host 127.0.0.1 \
    --port 30000
