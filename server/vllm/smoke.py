import json
import os
from typing import get_args

from huggingface_hub import hf_hub_download
from vllm.config.speculative import MTPModelTypes
from vllm.model_executor.layers.quantization.turboquant.config import TurboQuantConfig
from vllm.model_executor.models.registry import ModelRegistry
from vllm.v1.attention.backends.turboquant_attn import TurboQuantAttentionBackend

repository = os.environ["SOVKIT_MODEL_REPOSITORY"]
revision = os.environ["SOVKIT_MODEL_REVISION"]
path = hf_hub_download(repository, "config.json", revision=revision)
with open(path, encoding="utf-8") as source:
    config = json.load(source)

architectures = config.get("architectures", [])
quantization = config.get("quantization_config", {})
ignored = quantization.get("ignore", [])
if architectures != ["Qwen3_5ForConditionalGeneration"]:
    raise SystemExit(f"unexpected model architecture: {architectures}")
if not any(str(entry).startswith("mtp") for entry in ignored):
    raise SystemExit("pinned checkpoint does not preserve the MTP head")
if "qwen3_5_mtp" not in get_args(MTPModelTypes):
    raise SystemExit("vLLM image does not support qwen3_5_mtp")
if architectures[0] not in ModelRegistry.get_supported_archs():
    raise SystemExit(f"vLLM image does not support {architectures[0]}")
if not TurboQuantAttentionBackend.supports_kv_cache_dtype("turboquant_4bit_nc"):
    raise SystemExit("vLLM image does not support turboquant_4bit_nc")
turboquant = TurboQuantConfig.from_cache_dtype("turboquant_4bit_nc", 128)
if turboquant.key_mse_bits != 4 or turboquant.effective_value_quant_bits != 4:
    raise SystemExit("TurboQuant preset is not 4-bit keys and values")
print("SOVKIT_VLLM_IMAGE_CONTRACT_OK")
