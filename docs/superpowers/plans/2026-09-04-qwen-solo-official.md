# Qwen Solo official-derived GGUF Implementation Plan

**Goal:** Replace the old Solo SGLang recipe with the verified official-derived Unsloth GGUF; keep Uncensored and Studio separate.

**Architecture:** Reuse the existing llama.cpp native-MTP launcher, verified CUDA image and source pins. Only the Solo artifact and decision metadata change. Persisted checkpoints retain their snapshots; no provider mutation or silent migration.

**Tech stack:** Go/TOML, existing llama.cpp supervisor and Charm TUI.

## Constraints

- Unsloth Qwen3.8-27B UD-Q4_K_XL, revision `4ca720788d1e01f1bff70c033e0d0028fd02e502`, SHA256 `3f227079003add2511437e5b1e94812e363385225bf6a9b47b0054a72bc8b01e`.
- MTP depth two, q8_0 K/V, batch 2048 / microbatch 512, context 262144, one slot; measured GPU performance still outstanding.
- Preserve the recipe ID `qwen-solo-rtx5090`; never rewrite an old checkpoint using the new builtin.

## Steps

- [x] Update recipe/catalog consumer tests to expect llama.cpp, official-derived repository and 262144 context. Add a launcher integration assertion that the builtin Solo is accepted by ControlledLlamaCommand and passes its artifact pins. Five expected failures observed before editing the recipe.
- [x] Update recipes/qwen-solo-rtx5090.toml with the verified pins and experimental evidence. Preserve Uncensored/Studio bytes. Remove the Uncensored-specific download size from the shared log copy.
- [x] Update README with both variants, model IDs, unchanged legacy resume behavior and GPU measurement caveats.
- [x] Run full Go tests (512 passing), build and diff check. Existing recovery tests pass; no changes to checkpoint handling.

GPU model load, tool calls, MTP speedup and context occupancy tests remain external validation steps. No paid instance was modified.
