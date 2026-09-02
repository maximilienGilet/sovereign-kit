# Operations

Use this runbook for every deployment. It does not provision a host or make provider, legal, retention, or client-workload decisions.

## Start

### Remote host

1. Follow [Server setup](server.md) and start `./server/run-sglang.sh` on the GPU host.
2. From a second SSH session on that host, confirm the service is reachable only on loopback:

   ```bash
   curl --fail --silent --show-error http://127.0.0.1:30000/v1/models
   ss -ltnp '( sport = :30000 )'
   ```

   `curl` must return model JSON; `ss` must show a loopback listener. Do not proceed if the inference service listens on a public/LAN address.

### Mac

3. Open the strict tunnel in a dedicated terminal:

   ```bash
   sovkit-tunnel <ssh-host> <ssh-port> <tunnel-user> \
     <identity-file> <known-hosts-file>
   ```

4. In a second Mac terminal, run:

   ```bash
   curl --fail --silent --show-error http://127.0.0.1:30000/v1/models
   sovkit doctor
   ```

   The diagnostic checks the Pi/OpenCode provider locks, Pi extensions, available executables, intentional path overrides, and the local endpoint. Any `FAIL` blocks use. Resolve a warning before client work or record why the deliberate override is safe.

5. Start `pi-sovereign` or `opencode-sovereign`. In Pi, run `/subagents-models`; every role must resolve to `sovereign-qwen/qwen3.8-27b-nvfp4`. In OpenCode, confirm the same selected model.
6. Start with a non-sensitive smoke prompt before opening a project containing client material.

The tunnel has no daemon, restart loop, or health monitor. Keep its terminal visible; restart it manually after a disconnect.

## Stop

1. Exit Pi or OpenCode.
2. Stop the tunnel with `Ctrl-C` in its terminal.
3. Stop the remote SGLang process with `Ctrl-C` in its launch terminal.
4. Stop or destroy the GPU instance according to the provider/deployment procedure.
5. Confirm that billing and persistent-disk charges have ended as intended.

Stopping the tunnel only disconnects the Mac. It does not stop the remote process, delete the model cache, destroy the instance, or end provider billing.

## Troubleshooting

### `sovkit-tunnel` exits immediately

Read its error. It fails for unreadable identity/known-hosts files, a changed host key, an unusable SSH identity, or a failed remote port forward.

Check local permissions and verify the host key through an independent channel. Never bypass a host-key failure with `StrictHostKeyChecking=no` or by deleting the known-hosts entry without verification.

### Port 30000 is already in use on the Mac

```bash
lsof -nP -iTCP:30000 -sTCP:LISTEN
```

Stop the process that owns the local port, then restart the tunnel. Port `30000` is fixed in this release: the rendered Pi/OpenCode configuration, tunnel, and server recipe must stay aligned. Do not choose a new port in only one component.

### The remote `/v1/models` check fails

On the GPU host, inspect the terminal running `run-sglang.sh`, then check:

```bash
nvidia-smi
df -h "$HOME/.cache/huggingface"
ss -ltnp '( sport = :30000 )'
```

Common causes are GPU/container-toolkit failures, insufficient VRAM, a failed model download, insufficient disk, or a server process that exited. Keep the inference port closed while diagnosing.

### The Mac `/v1/models` check fails while the tunnel is open

First make the same request on the remote host. If it succeeds remotely, confirm that `sovkit-tunnel` targets the same remote SSH host and port and remains running. If the server returns `401` or `403`, stop: this shared V1 profile is keyless and does not support an authenticated SGLang server.

### `sovkit doctor` reports a provider-lock failure

Do not start a client. Rerun the installer from a reviewed checkout after preserving intentional local changes:

```bash
./install-macos.sh --upgrade
sovkit doctor
```

Inspect `PI_CODING_AGENT_DIR` and `SOVEREIGN_OPENCODE_CONFIG`; both deliberately replace the standard configuration and therefore require an operator review.

### Pi or OpenCode shows another provider/model

Exit the client. Launch `pi-sovereign` or `opencode-sovereign`, not bare `pi` or `opencode`, then rerun the diagnostic. In Pi, rerun `/subagents-models`. Never solve this by enabling a second provider or fallback.

### OpenCode is unavailable

```bash
npm install --global opencode-ai@1.18.25
opencode --version
```

Then launch it through `opencode-sovereign`. The wrapper confirms that an executable exists but does not enforce its installed version; use the pinned release and run the non-sensitive smoke prompt first.

## Record after a deployment

Keep a private deployment record containing the approved provider/region, GPU type, context/concurrency settings, model revision, image digest, host checkout commit, start/stop time, destruction outcome, and the checks performed. Do not commit records that reveal client names, active hosts, prompts, logs, tokens, identities, or credentials.
