<p align="center">
  <img src="assets/sovereign-kit-logo.png" width="156" alt="Blue glass cube, the Sovereign Kit mark">
</p>

# Sovereign Kit

A small reference setup for using **one private Qwen model** from Pi, pi-subagents, Oh-My-Pi, and OpenCode on a Mac.

```text
Pi / OpenCode on a Mac
        │
        ▼
127.0.0.1:30000
        │ strict SSH tunnel
        ▼
127.0.0.1:30000 on a GPU host
        │
        ▼
SGLang + Qwen
```

The point is simple: the coding clients have one configured model route and cannot silently fall back to OpenAI, Anthropic, DeepSeek, OpenRouter, or another model provider.

## Experimental: Qwen Solo capacity profiles and Uncensored

**Qwen Solo** uses `unsloth/Qwen3.8-27B-GGUF`, file `Qwen3.8-27B-UD-Q4_K_XL.gguf` (~17.56 GB): an optimized quantization derived from the official model, not an uncensored fine-tune. Its pinned header includes the native MTP block. Quantization and Unsloth's template adjustments mean it is not byte- or behavior-identical to the full-precision original. [Artifact verification](docs/superpowers/specs/2026-09-04-qwen-solo-official-verification.md).

The official-derived model has three explicit capacity profiles on RTX 5090:

- **Full Context:** 1 request × 262,144 tokens with `q8_0` K/V caches and 16,384 maximum output tokens.
- **Dual:** 2 requests × 131,072 tokens with `q8_0` K/V caches and 16,384 maximum output tokens per request.
- **Dual Max Lab:** 2 requests × 262,144 tokens with `q4_0` K/V caches and 16,384 maximum output tokens per request. GPU fit and long-context quality are unvalidated.

Input and output share each profile's per-request context ceiling. The profiles reuse the same verified model file only while it remains available on the same instance or persistent storage; a new ephemeral instance downloads it again. The separate **Qwen Solo Uncensored** recipe retains the HauhauCS Q4_K_P GGUF (~17.92 GB), 4 × 65,536 context and 4,096 maximum output. Qwen Studio is unchanged. The llama.cpp recipes use the optimization starting point from [John Paul Wile's single-5090 configuration](https://johnpaulwile.substack.com/p/i-ran-qwen-38-27b-on-a-single-rtx), including native MTP depth 2 and batch/microbatch 2048/512. These are experimental configurations, **not validated maximum-context workloads or a 133 tok/s guarantee**.

The Solo recipes pin CUDA 13.3.1 by image digest, llama.cpp by source commit, and their model by revision plus SHA256. The reviewed image contains the precompiled CUDA 120a runtime; provisioning downloads the selected GGUF and verifies it before loading. Package repositories remain live; image/source/model pins do not make the complete build bit-reproducible. Choose an RTX 5090 host with a CUDA 13.3-compatible driver. Download progress and server logs appear in provisioning (`l` expands them). A failed checksum, runtime validation, or missing native-MTP option stops deployment.

Inference stays on `127.0.0.1:30000` through the existing strict SSH tunnel; no Cloudflare. The client model ID is `unsloth/Qwen3.8-27B-GGUF` for Solo and `HauhauCS/Qwen3.8-27B-Uncensored-HauhauCS-Aggressive-MTP-GGUF` for Uncensored; use the corresponding ID instead of the reference NVFP4 ID in client settings. First verify `/v1/models`, a short chat completion and tool calling, then test increasingly long prompts. Configuring a context ceiling alone does not establish memory fit at full occupancy.

Use a **new deployment with this recipe**, not an old SGLang checkpoint. Resume deliberately retains the original model/runtime pins and never converts an existing paid instance. During an Uncensored build/download, resume observes the same supervisor; after it exits unexpectedly, the retained marker prevents a duplicate launch. Quitting the client does not stop provider billing. No instance is automatically destroyed.

Sources and remaining validation gates: [runtime verification](docs/superpowers/specs/2026-09-04-qwen-runtime-verification.md).

## What is included

- isolated Pi profile with `pi-subagents` and Oh-My-Pi;
- locked OpenCode configuration;
- `sovkit-tunnel`, a local-only SSH forward with strict host-key verification;
- `sovkit doctor`, a local configuration and endpoint check;
- one Linux GPU-host SGLang command using a digest-pinned image and pinned Qwen revision;
- historical benchmark reports for the reference model.

The reference model is `RadixArk/Qwen3.8-27B-NVFP4`. This is one route, not a general model gateway or deployment platform.

## What it is not

Sovereign Kit can rent a selected Vast GPU after explicit approval; it does not manage a fleet, create SSH users, expose a public API, manage billing, make a rented GPU legally sovereign, prove GDPR compliance, sandbox agent tools, or validate a provider contract.

The local client configuration and checks are structurally tested. Historical benchmarks exist on an RTX PRO 6000 S. A live Pi/OpenCode session against the current digest-pinned server recipe has not yet been completed: use a non-sensitive smoke test before client work.

## Before starting

You need:

1. A Linux AMD64 NVIDIA GPU host approved for the intended material, with Docker and NVIDIA Container Toolkit working for an unprivileged SSH user.
2. A Mac with Git, Go, and standard `ssh`. [Pi](https://github.com/badlogic/pi-mono) and OpenCode are optional integrations; npm is needed for their package installation.
3. A dedicated SSH identity and a host fingerprint verified through an independent channel.
4. An assigned owner for provider approval, spend limit, and instance destruction.

Keep private keys, known-hosts files, active hostnames, client material, and private logs out of this repository.

## Primary journey

On the Mac, open the terminal application:

```bash
sovkit
```

With terminal input and output, bare `sovkit` opens Home. From there, set up a Vast or Manual SSH route and connect to the private endpoint dashboard. It shows the configured base URL and the actual model from `/v1/models`; Enter copies raw values. Its live generation graph covers the latest ten minutes of values reported by the server. Current, average, and peak summaries use reported generation values only: a reported zero is real idle activity, while missing reports remain `not reported` and leave gaps in the graph. Narrow terminals omit the plot but retain those textual summaries. Telemetry is not persisted between CLI runs, and the dashboard does not infer latency. Saving a route does not claim it is healthy. Existing configuration is replaced only after explicit confirmation. The API key is requested without echo, used only for setup, and never stored; `VAST_API_KEY` is also supported.

Direct commands remain available: `sovkit setup` opens setup, `sovkit start` connects an existing route, and `sovkit help` always prints help. `catalog`, `dashboard`, `tunnel`, and `doctor` remain explicit tools.

Interrupted Vast provisioning is saved locally. Reopening the TUI offers **Resume**, **Destroy**, or **Quit** before starting another setup. `sovkit resume` continues the pending deployment; `sovkit resume INSTANCE_ID` imports an older instance after explicit recipe and SSH identity selection. Resuming never creates another paid instance. SSH verification remains strict, and an uncertain or mismatched remote launch is refused rather than duplicated. Destroy requires confirmation; quitting does not stop provider billing. Keep the adjacent `.pending.json` checkpoint until provisioning or verified destruction completes. The checkpoint contains deployment inputs, not API credentials.

Vast API keys entered in setup or resume are saved separately at `<config-path>.vast-api-key`, an owner-only (`0600`) **plaintext** file. Do not share or commit this file. Subsequent runs reuse it; `VAST_API_KEY` overrides it without being persisted. To replace a saved key, remove that credential file and enter the replacement on the next run. Older versions did not save prompted keys, so one final entry is required after upgrading.

If an older instance has no local checkpoint, press **i** on the TUI home screen to recover it, or run `sovkit resume`: the recovery flow asks for its Vast instance ID. Saving an API key alone cannot identify which instance to resume. Recovery requires recipe and SSH identity confirmation and never rents another instance.

During inference-server startup, the TUI shows a **SERVER LOGS** panel. Press **l** to expand it, use arrows or PgUp/PgDn to scroll, and **l** or **Esc** to return. The bounded snapshot (up to 16 KiB / 200 lines) is refreshed by read-only SSH checks with a two-second interval; the latest logs remain available after errors. Known API credentials and common token patterns are redacted. Ctrl+C stops local polling, not the paid instance.

The pinned SGLang image is launched via `python3 -m sglang.launch_server`, not the unavailable `sglang serve` executable. Resume recognizes only the old launcher's exact `nohup` executable-not-found failure, with a matching recipe fingerprint, no detected server and a free serving port. It archives that log as `sovkit-sglang.log.missing-command` before one corrected dispatch. Other uncertain launches remain blocked.

Every Vast recipe, including built-ins, now opens an offer browser before the separate paid deployment review. Use ↑/↓ to browse, `f` to search and select countries (Space toggles, Enter applies, Esc discards), `s` to switch price/reliability sort, `r` to refresh, and `i` for full inspection. Country selections are a union; “All countries” resets the filter. On narrow terminals, Enter opens details before a second Enter chooses the offer. `?` shows contextual help. Browsing, filtering, inspecting and refreshing never rent an instance; the following review starts on Cancel.

Searches load at most 100 provider offers, with eligible counts shown separately. Refine countries when the limit is reached. Country and reliability are provider-declared, not independently verified; missing values remain unknown. Hourly prices retain the provider's precision. Monthly figures are compute-only estimates at 730 hours and exclude storage, egress and tax. Capacity bars show available VRAM per GPU and disk per instance against the recipe requirement; they are not performance scores.

During provisioning, the Magnetic Pulse animation sends a continuous light wave from the fixed crystal cage toward its nucleus to show indeterminate container activity without inventing progress. Once the launcher reports trustworthy model bytes, Crystal Forge builds the crystal from the center outward at the measured percentage; the numeric byte count remains visible and the construction never moves backward. The milestone bar still represents completed setup stages, not elapsed-time progress. Small terminals use a compact loader, and accessible mode prints measured status lines without animation. A temporarily empty Vast status remains pending until the readiness timeout; an explicit offline status remains an error.

`ACCESSIBLE=1 sovkit setup`, `TERM=dumb`, or redirected input/output selects line-oriented prompts instead of fullscreen graphics. Numbered choices use one answer per line; paid actions default to **No** and require an explicit `yes`. EOF cancels an unfinished response. Passwords are never reprinted, including when read from a pipe. Bare `sovkit` prints help in this mode; `sovkit start` prints connection status and keeps the local tunnel open until interrupted. Launch clients explicitly from another terminal. The catalog becomes plain text and the dashboard command provides guidance rather than opening a fullscreen interface.

The text offer browser supports numbered choices plus `f` (comma-separated ISO country codes or `all`), `s`, `r`, `i` (then an offer number), and `q` to cancel. Empty searches and temporary errors keep these controls available; EOF exits without approving a paid action.

In the interactive application, Esc goes back without disconnecting the dashboard; Ctrl+C requests exit. No coding client is launched by the dashboard. Keep it open while using the endpoint from another application. Press `d` to disconnect; quitting or disconnecting stops the **local** tunnel and cancels profile setup workers. It never destroys a remote Vast instance or ends its billing. If creation was requested, check the instance in Vast even after a failure or cancellation. Stop or destroy it explicitly and confirm billing has ended.

After an error involving an instance created in the current session, the application offers a separate destruction action for that exact instance. Confirmation defaults to Cancel and warns that its data will be lost. The application stops its local work, sends the request, and verifies absence before showing destruction as confirmed. A timeout, permission failure or network error keeps the result unconfirmed and the billing warning visible; retry checks the instance again first. Accessible setup offers the same explicit confirmation after a failed creation workflow. No instance is destroyed automatically, and existing charges are not reversed. A saved configuration pointing to a destroyed instance must be replaced before reuse.

For either provider, verify the host fingerprint through an independent channel before trusting it. The route remains strict: do not disable host-key checking, expose a public endpoint, or enable a provider fallback.

## 1. Start the GPU host

On the GPU host:

```bash
uname -m
docker --version
nvidia-smi
docker run --rm --gpus all nvidia/cuda:12.8.0-base-ubuntu24.04 nvidia-smi
```

The last command must show the GPU inside a container. If it does not, repair the NVIDIA Container Toolkit setup; do not open an inference port as a workaround.

Then:

```bash
git clone https://github.com/maximilienGilet/sovereign-kit.git
cd sovereign-kit
git rev-parse HEAD
cat server/image.lock
./server/run-sglang.sh
```

Keep that terminal open. The first run can download the model. The command binds SGLang to remote loopback only. It uses `--trust-remote-code`, which executes model-provided Python on the GPU host: review the pinned model revision and container input before client use.

From a second SSH terminal, wait for the service to be ready and check it:

```bash
curl --fail --silent --show-error http://127.0.0.1:30000/v1/models
ss -ltnp '( sport = :30000 )'
```

The first command must return JSON; the second must show `127.0.0.1:30000`. Stop if the server is reachable on a LAN or public address.

## 2. Install the Mac CLI and optional integrations

On the Mac:

```bash
pi --version
git --version
ssh -V
npm --version
git clone https://github.com/maximilienGilet/sovereign-kit.git
cd sovereign-kit
./install-macos.sh --with-opencode
```

The installer builds the CLI and installs wrappers in `~/.local/bin`. It never creates or replaces client profiles. `--with-opencode` explicitly installs the pinned OpenCode CLI; omit it if not wanted.

```text
pi-sovereign
opencode-sovereign
sovkit
sovkit-tunnel
```

If needed, add that directory to zsh’s path:

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
exec zsh
```

Connect with `sovkit start`. Generic OpenAI-compatible instructions come first: use the shown base URL, discovered model ID, and the non-secret placeholder `local-qwen-tunnel` if an API key is required. The loopback endpoint is keyless and protected by the SSH route; this placeholder is not a Vast API key.

Selecting Pi, standalone Oh My Pi (OMP), or OpenCode first inspects its exact destination without writing. Compatible providers immediately offer a manual launch command. Missing or different providers require explicit confirmation, defaulting to Cancel. Install the selected client separately if missing. No extensions or packages are installed by this step. Existing managed files receive recoverable backups; unrelated settings, credentials, skills, sessions and packages are retained.

Profile setup requires verified context/output limits matching the discovered model. New deployments save these from the selected recipe. Older routes without metadata remain usable for generic connections, but profile setup is unavailable until verified deployment metadata is present; no Studio model or limits are substituted. Press `r` on the main dashboard to retry discovery.

Only verified providers unlock Enter to copy the launch command. Copying never starts a client or disconnects the tunnel. Paste the command in another terminal from your project directory, and keep the dashboard open. Pi uses `PI_CODING_AGENT_DIR` or `~/.pi/agent`, changing only `models.json`. OMP uses its normal agent directory (default `~/.omp/agent`) and active models YAML/JSON file. Their commands select the Sovereign model for this session, without changing saved defaults or subagent policies. Existing subagents can therefore retain their own configured models. Legacy isolated profiles are not deleted or migrated. OpenCode retains its separate config via `SOVEREIGN_OPENCODE_CONFIG` or `~/.config/opencode/sovereign.json`. Unsafe/symlink targets are refused.

In the TUI, `q` or `Ctrl+C` opens an exit dialog for the displayed Vast instance: **Stop and quit** (default), **Leave running and quit**, **Destroy** (second confirmation), or **Cancel**. Remote stop/destruction must be verified before exit; errors remain visible and retryable. Stopping retains the instance and may retain storage charges. Start it again in Vast before reconnecting; automatic remote restart and idle shutdown are not implemented. A killed process, closed terminal or lost Mac cannot guarantee this interactive shutdown—check Vast directly in that case.

## 3. Verify SSH and open the tunnel

Put the independently verified host key in a dedicated known-hosts file, outside the repository, alongside a dedicated private key:

```bash
chmod 600 ~/.ssh/qwen-sovereign_ed25519 ~/.ssh/qwen-sovereign_known_hosts
ssh-keygen -lf ~/.ssh/qwen-sovereign_known_hosts
```

Compare the displayed fingerprint to the approved value. A mismatch is a stop condition.

Open the route with deployment-specific values:

```bash
sovkit-tunnel <ssh-host> <ssh-port> <ssh-user> \
  ~/.ssh/qwen-sovereign_ed25519 \
  ~/.ssh/qwen-sovereign_known_hosts
```

Leave this terminal open. The tunnel listens only on Mac `127.0.0.1:30000` and fails if host verification or forwarding fails.

## 4. Verify and use it

In another Mac terminal:

```bash
curl --fail --silent --show-error http://127.0.0.1:30000/v1/models
sovkit doctor
```

Both must succeed. `sovkit doctor` must show no `FAIL`; inspect every warning before client work.

From a disposable or approved non-sensitive repository:

```bash
pi-sovereign
# or
opencode-sovereign
```

In Pi, run `/subagents-models`. The parent and every worker must show:

```text
sovereign-qwen/qwen3.8-27b-nvfp4
```

In OpenCode, confirm the same model. Start with a non-sensitive prompt. If another provider/model appears, or an endpoint check fails, stop. Do not use bare `pi`/`opencode`, enable a public-provider fallback, or open a public inference port.

The shipped route is **keyless at the SGLang API layer**. `local-qwen-tunnel` is a non-secret compatibility value, not access control. Do not add an SGLang API key to this shared Pi + OpenCode setup: the supplied Pi profile cannot consume it.

## Stop

1. Exit Pi or OpenCode.
2. Stop the tunnel with `Ctrl-C`.
3. Stop SGLang with `Ctrl-C`.
4. Stop or destroy the GPU instance and confirm billing has ended.

Closing the Mac client or tunnel does not stop the remote GPU.

## When something fails

| Problem | Safe response |
|---|---|
| Tunnel exits | Check permissions and verify the host fingerprint independently. Never disable strict host-key checking. |
| Port 30000 is occupied | Run `lsof -nP -iTCP:30000 -sTCP:LISTEN`, stop that process, restart the tunnel. |
| Server endpoint fails | Inspect server logs, `nvidia-smi`, GPU-container integration, model-download access, disk space, and loopback binding. |
| `sovkit doctor` reports config failure | Connect with `sovkit start`, inspect the integration destination and confirm a profile update if needed. Check deliberate environment overrides; the CLI installer does not repair profiles. |
| Pi/OpenCode shows another provider | Stop. Launch the Sovereign wrapper, rerun `sovkit doctor`, and do not enable a fallback. |
| Endpoint returns `401`/`403` | Stop. This keyless shared route does not support an authenticated SGLang server. |

## Benchmarks

Historical synthetic evidence on an RTX PRO 6000 S (96 GB):

- [near-262K request](benchmarks/benchmark-262k-rtx-pro-6000-sglang.md): 246K input + 8,192 output completed at about 48.2 tok/s decode;
- [one 128K principal plus four 32K workers](benchmarks/benchmark-shared-128k-plus-4x32k.md): all requests completed; concurrent prefill made principal TTFT about 60 seconds.

These are not capacity, cost, quality, throughput, or availability promises. Rerun the workload on the exact GPU/image/model before a production claim.

## Development

```bash
bash -n install-macos.sh bin/* server/run-sglang.sh
python3 tests/test_sovkit_doctor.py
python3 scripts/check-secrets.py
git diff --check
```

See [SECURITY.md](SECURITY.md) for private vulnerability reporting and [CONTRIBUTING.md](CONTRIBUTING.md) for contribution rules.

## License

[Apache License 2.0](LICENSE).
