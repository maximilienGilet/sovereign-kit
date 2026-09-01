# Operations

Use this checklist each time you open a route for client work.

## Start

1. Start the remote SGLang service using [Server setup](server.md).
2. Open the local tunnel in a dedicated terminal:

   ```bash
   sovkit-tunnel <ssh-host> <ssh-port> <tunnel-user> \
     <identity-file> <known-hosts-file>
   ```

3. In a second terminal, run the local diagnostic:

   ```bash
   sovkit doctor
   ```

   It checks the profile, provider locks, extensions, executables, overrides, and `http://127.0.0.1:30000/v1/models`. A local endpoint warning means the tunnel or remote service is unavailable; a configuration failure blocks use.

4. Start `pi-sovereign` or `opencode-sovereign`.
5. In Pi, run `/subagents-models` and confirm that every role resolves to `sovereign-qwen/qwen3.8-27b-nvfp4`.

Do not start client work until all five checks pass.

## Stop

1. Exit Pi or OpenCode.
2. Stop the tunnel with `Ctrl-C` in its terminal.
3. Stop the remote SGLang process.
4. Stop or destroy the GPU instance according to the deployment procedure.

Stopping the tunnel only disconnects the Mac. It does not stop the remote server or end provider billing.

## Troubleshooting

### `sovkit-tunnel` exits immediately

Read the error. The wrapper fails on unreadable identity/known-hosts files, a changed host key, an unusable SSH identity, or a failed port forward.

Check local file permissions and verify the host key out of band. Do not work around a host-key error by disabling strict checking.

### Port 30000 is already in use

```bash
lsof -nP -iTCP:30000 -sTCP:LISTEN
```

Stop the process that owns the local port. Port `30000` is fixed in this release: the Pi profile, OpenCode configuration, tunnel, and server must all use it. The tunnel has no background service, restart loop, or automated health check; keep its terminal open and restart it manually after a disconnect.

### `curl /v1/models` fails while the tunnel is running

First, check the remote SGLang process and its loopback listener on the GPU host. Then confirm that the tunnel targets the same remote port.

If server authentication is enabled, retry with the locally stored `QWEN_LOCAL_API_KEY`. Never paste it into a shell history you plan to share or into a repository file.

### Pi shows another provider or model

Exit Pi. Run it through `pi-sovereign`, not bare `pi`, then run `/subagents-models` again. If the profile was edited, reinstall with `--upgrade` after preserving any local changes you need.

### OpenCode reports a missing configuration or executable

Run the installer again, or install the pinned client:

```bash
npm install --global opencode-ai@1.18.25
```

Then start it with `opencode-sovereign`, not bare `opencode`.

## Record after a deployment

Record the model revision, server image digest, GPU type, provider/region approval, context limit, concurrency limit, and the checks performed. Keep that record outside the repository if it identifies a client or an active endpoint.
