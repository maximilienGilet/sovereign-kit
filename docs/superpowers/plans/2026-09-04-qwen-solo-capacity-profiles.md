# Qwen Solo Capacity Profiles Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship three deterministic Qwen Solo capacity recipes backed by the same model and precompiled runtime: Full Context, Dual, and Dual Max Lab.

**Architecture:** Extend llama.cpp recipe runtime configuration with explicit K/V cache types and validate them at the recipe boundary. Embed two additional recipe documents, pass the selected cache types through the existing launch manifest, and expose the resulting capacity trade-off in the Charm recipe cockpit.

**Tech Stack:** Go, TOML, Bubble Tea, Lip Gloss, embedded recipe assets, llama.cpp CLI.

## Global Constraints

- `qwen-solo-rtx5090` becomes Full Context with 1 request, 262,144 context tokens, 16,384 maximum output tokens, and `q8_0` K/V caches.
- Dual uses 2 requests, 131,072 context tokens per request, 16,384 maximum output tokens, and `q8_0` K/V caches.
- Dual Max Lab uses 2 requests, 262,144 context tokens per request, 16,384 maximum output tokens, and `q4_0` K/V caches.
- All three profiles share the exact model repository, revision, filename, checksum, runtime image, and llama.cpp source revision.
- Qwen Studio and Qwen Solo Uncensored behavior remains unchanged.
- Existing checkpoints are not migrated or silently reconfigured.

---

### Task 1: Typed llama.cpp KV-cache configuration

**Files:**
- Modify: `internal/recipe/recipe.go`
- Modify: `internal/recipe/llama_test.go`
- Modify: `internal/setup/llama.go`
- Modify: `internal/setup/llama_test.go`

**Interfaces:**
- Produces: `Runtime.KVCacheTypeK`, `Runtime.KVCacheTypeV`, and `Recipe.LlamaCacheTypes() (string, string)`.
- Consumes: the resolved cache types in the llama launch manifest and CLI arguments.

- [x] **Step 1: Write failing recipe and launcher tests**

Assert literal defaults `q8_0/q8_0`, explicit `q4_0/q4_0`, rejection of unsupported types, and exact `--cache-type-k` / `--cache-type-v` arguments from a real parsed recipe.

- [x] **Step 2: Run the focused tests and observe the expected failures**

Run: `rtk go test ./internal/recipe ./internal/setup -run 'LlamaCache|BuiltinOfficial|BuiltinDual' -count=1`

Expected: build or assertion failures because cache types are not represented and the launcher remains hard-coded to `q8_0`.

- [x] **Step 3: Add validated cache fields and defaults**

Add TOML fields `kv_cache_type_k` and `kv_cache_type_v` to `Runtime`. `LlamaCacheTypes` returns `q8_0` for empty values. For `gguf-text-generation`, validation accepts only `q8_0` and `q4_0`; mixed values remain valid if explicitly configured.

- [x] **Step 4: Pass cache types through the launch manifest**

Include `cache_type_k` and `cache_type_v` in the JSON sent to the remote launcher, then use those values for the two llama.cpp flags. Keep output, MTP, batching, GPU layers, and endpoint arguments unchanged.

- [x] **Step 5: Run package tests**

Run: `rtk go test ./internal/recipe ./internal/setup -count=1 -timeout=60s`

Expected: all tests pass.

---

### Task 2: Three embedded Qwen Solo profiles

**Files:**
- Modify: `recipes/qwen-solo-rtx5090.toml`
- Create: `recipes/qwen-solo-dual.toml`
- Create: `recipes/qwen-solo-dual-max.toml`
- Modify: `recipes/builtin.go`
- Modify: `recipes/builtin_test.go`
- Modify: `internal/recipe/recipe_test.go`
- Modify: `internal/catalogui/defaults_test.go`

**Interfaces:**
- Produces: `recipes.QwenSoloDual()`, `recipes.QwenSoloDualMax()`, and a stable five-entry `recipes.Builtin()` order.
- Consumes: the cache configuration from Task 1.

- [x] **Step 1: Write failing profile contract tests**

Assert each profile's literal ID, status, context, output, concurrency, and cache types. Compare the six shared model/runtime identity fields directly across all three. Assert built-in order: Studio, Full Context, Dual, Dual Max Lab, Uncensored.

- [x] **Step 2: Run tests and observe missing profiles / old two-slot behavior**

Run: `rtk go test ./recipes ./internal/recipe ./internal/catalogui -run 'QwenSolo|Builtin|Default' -count=1`

Expected: failures because Full Context still has two slots and the new embedded profiles do not exist.

- [x] **Step 3: Update Full Context and add both variants**

Set Full Context to `1 × 262144`, `q8_0/q8_0`, and its recommended Solo copy. Add Dual as `2 × 131072`, `q8_0/q8_0`; add Dual Max Lab as `2 × 262144`, `q4_0/q4_0`, with explicit unvalidated-memory and cache-quality warnings.

- [x] **Step 4: Embed and return the profiles in stable order**

Add two `go:embed` variables and exported parsing functions. Return all five launchable recipes without changing Custom Hugging Face placement in the catalog.

- [x] **Step 5: Run recipe and catalog boundary tests**

Run: `rtk go test ./recipes ./internal/recipe ./internal/catalogui -count=1 -timeout=60s`

Expected: all tests pass.

---

### Task 3: Capacity trade-off in the recipe cockpit

**Files:**
- Modify: `internal/catalogui/model.go`
- Modify: `internal/catalogui/defaults.go`
- Modify: `internal/catalogui/cockpit.go`
- Modify: `internal/catalogui/cockpit_test.go`
- Modify: `internal/catalogui/picker_test.go`
- Modify: `internal/catalogui/picker_readability_test.go`

**Interfaces:**
- Consumes: recipe cache types from Tasks 1–2.
- Produces: `Entry.KVCacheTypeK`, `Entry.KVCacheTypeV`, compact summary text, and cockpit `KV CACHE` metric.

- [x] **Step 1: Write failing rendering tests**

For every Solo variant, render wide and compact dashboards and assert the literal cache label, context, and request count are visible. Verify narrow dimensions remain within width and height.

- [x] **Step 2: Run focused rendering tests and observe missing KV information**

Run: `rtk go test ./internal/catalogui -run 'SoloCapacity|KVCache|Readability' -count=1`

Expected: assertions fail because `Entry` does not carry cache precision.

- [x] **Step 3: Map and render cache precision**

Copy resolved cache types into each recipe entry. Render `KV CACHE q8_0 K / q8_0 V` or `q4_0 K / q4_0 V` in both cockpit layouts while preserving existing clipping and overflow safeguards.

- [x] **Step 4: Run all catalog tests**

Run: `rtk go test ./internal/catalogui -count=1 -timeout=60s`

Expected: all tests pass.

---

### Task 4: Documentation, regression verification, and local install

**Files:**
- Modify: `README.md`
- Modify: `server/llama/README.md`
- Modify: `docs/superpowers/plans/2026-09-04-qwen-solo-capacity-profiles.md`

**Interfaces:**
- Consumes: completed Tasks 1–3.
- Produces: accurate operator guidance and a tested locally installed CLI.

- [x] **Step 1: Update runtime guidance**

Document the three exact profiles, explain that output tokens share the context ceiling, state that model reuse only applies while the verified file remains on the same instance/storage, and label Dual Max Lab as unvalidated.

- [x] **Step 2: Run the full suite**

Run: `rtk go test ./... -count=1 -timeout=90s`

Expected: all packages pass.

- [x] **Step 3: Run race tests on changed Go packages**

Run: `rtk go test -race ./internal/recipe ./recipes ./internal/setup ./internal/catalogui -count=1 -timeout=120s`

Expected: all packages pass without race reports.

- [x] **Step 4: Install locally**

Run: `rtk proxy sh install-macos.sh`

Expected: installation succeeds without modifying Pi or OpenCode profiles.

- [x] **Step 5: Record evidence**

Mark completed checkboxes and append exact test counts. Record that verification did not create, stop, resume, or destroy a Vast instance.

## Verification Evidence

- `rtk go test ./... -count=1 -timeout=90s`: 762 tests passed in 16 packages.
- `rtk go test -race ./internal/recipe ./recipes ./internal/setup ./internal/catalogui -count=1 -timeout=120s`: 265 tests passed in 4 packages, with no race report.
- Independent review: no material issue remaining after review fixes.
- `rtk git diff --check`: passed.
- `rtk proxy sh install-macos.sh`: installed successfully in `/Users/maximiliengilet/.local/bin`; existing Pi and OpenCode profiles were not changed.
- No Vast instance was created, stopped, resumed, or destroyed during verification.
