# Qwen Solo Uncensored llama.cpp Implementation Plan

> **For agentic workers:** Use executing-plans to implement this plan task-by-task. User approved the article-aligned route on 2026-09-04.

**Goal:** Deploy the exact article GGUF through the existing SSH-only provisioning flow.

**Architecture:** Pin the CUDA base, llama.cpp source and GGUF content. A bounded bootstrap installs Python/curl before the existing Python reconciliation lock; a background Python supervisor downloads, verifies, builds and execs llama-server. The supervisor and final server share one PID and the existing log/readiness polling. Resume recognizes either phase, never an uncertain dead launch.

**Tech Stack:** Go, Python 3, CUDA 13.3, llama.cpp, existing Charm TUI.

## Global Constraints

- No Cloudflare, public inference listener, provider mutation during development, or silent checkpoint migration.
- Context 262144; parallel 1; K/V q8_0; batch 2048; ubatch 512; native draft-mtp depth 2 and p-min 0.
- Model SHA256 ba36dc3c2b2ff5e0aa5d71092a8894546996a6a119ae391803dda07cdc08516d.
- CUDA image and source/model revisions from the adjacent research note, not mutable tags.
- No claim of GPU success before a real completion test.

### Task 1: Recipe contract

Files: internal/recipe/recipe.go, internal/recipe/llama_test.go, recipes/qwen-solo-uncensored.toml, recipes/builtin_test.go.

- [x] Add tests that reject missing source/hash/native-MTP contracts and unsafe filenames. Run `rtk go test ./internal/recipe ./recipes` and observe failures.
- [x] Add SourceRevision and SHA256 fields; require the reviewed native-MTP contract for llama recipes. Add a separate Uncensored recipe (user clarification); preserve Solo, Studio and existing checkpoints. New optional JSON fields are omitted for old recipes to preserve remote fingerprints.
- [x] Re-run recipe tests.

### Task 2: Launcher and resume

Files: internal/setup/llama.go, internal/setup/llama_test.go, internal/setup/reconcile.go, internal/setup/system.go.

Interface: `ControlledLlamaCommand(recipe.Recipe) (string, error)` produces an SSH-safe supervised background launch. `prepareLlama(ctx, ssh)` ensures Python/curl under a bounded package lock. Existing readiness reads sovkit-llama-cpp.log/pid.

- [x] Test command argv and failures by executing the supervisor with controlled local external-command doubles; validate hash rejection, exact launch arguments, no launch after build failure.
- [x] Implement pinned download/hash verification, source checkout, CUDA120a compilation and exec. No unsafe shell interpolation.
- [x] Test real reconciliation script with synthetic /proc entries for the bootstrap, matching llama-server, and foreign server; then add matching recognition. Initial matching tests failed before implementation.
- [x] Wire both Launch and Reconcile and verify readiness/log snapshots. Initial launch/readiness test failed before implementation.

### Task 3: Regression and handoff

Files: README.md and this checklist.

- [x] Run `rtk go test ./... -count=1` (511 passing), build and diff check.
- [x] Document fresh-image requirement, bounded bootstrap and GPU validation still required. No instance created or destroyed.

Remaining external validation: real CUDA compilation, short completion/tool calls and increasing context occupancy on a chosen paid RTX 5090 offer. The article's throughput is not a local test result.
