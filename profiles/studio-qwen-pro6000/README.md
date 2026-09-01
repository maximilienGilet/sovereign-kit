# Studio — Qwen on RTX PRO 6000

This is the first **route profile**. It defines one fail-closed, OpenAI-compatible Qwen route through SSH loopback. It is the canonical description of the current Pi and OpenCode configuration.

## Evidence status

The linked reports are historical synthetic measurements on an RTX PRO 6000 S. They show a 246K-input / 8,192-output request and a 128K principal plus four 32K workers completed without OOM. They do not validate the current digest-locked image, real repository quality, a multi-user service, or an SLO.

Before a production claim, rerun the benchmark contract on the exact GPU, image digest, model revision, and workload.

## Boundary

The route is workload-neutral at the protocol layer. V1 validates only the Pi and OpenCode adapters. It is not a shared-project system, a tenant boundary, a RAG service, or a tool-execution platform.
