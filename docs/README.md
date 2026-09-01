# Documentation

Sovereign Kit has one tested route: Pi, Oh-My-Pi, pi-subagents, and OpenCode on a Mac use the same Qwen/SGLang endpoint through a local SSH forward.

Start with the path that matches your job.

| If you need to… | Read |
|---|---|
| See the whole route and its limits | [Architecture](architecture.md) |
| Get a working local setup quickly | [Quick start](quickstart.md) |
| Understand what the installer writes and how the clients are locked | [Local setup](local-setup.md) |
| Prepare the remote Qwen/SGLang host | [Server setup](server.md) |
| Start, verify, stop, and troubleshoot a deployment | [Operations](operations.md) |
| Review the security boundary before client work | [Security](security.md) |
| Integrate an OpenAI-compatible application | [OpenAI-compatible integrations](use-cases.md) |
| Compare self-managed, routed, European, and closed-model APIs | [Choosing an inference route](choosing-an-inference-route.md) |
| Select a published route profile and understand its boundary | [Route profiles](profiles.md) |
| Understand the evidence required before a performance claim | [Benchmark contract](benchmark-contract.md) |
| Review measured capacity, not product claims | [Benchmarks](../benchmarks/README.md) |

## Reading order for a first deployment

1. Read [Security](security.md).
2. Prepare the GPU host with [Server setup](server.md).
3. Follow [Quick start](quickstart.md) on the Mac.
4. Complete the checks in [Operations](operations.md) before sending client material.

The repository supplies local configuration and a reference server contract. It does not provision a GPU, create an SSH account, validate a provider contract, or decide whether a client workload is allowed on a chosen provider.
