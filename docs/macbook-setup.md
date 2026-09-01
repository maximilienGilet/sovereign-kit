# Sovereign Kit on macOS

## 1. Prerequisites

- macOS with an SSH client (included by default).
- Pi CLI installed and available as `pi`.
- Node.js/npm if installing OpenCode with `--with-opencode`; otherwise install `opencode-ai@1.18.25` separately.
- GitHub access to clone this repository.
- A separately provisioned Qwen/SGLang server that is reachable over SSH.

This repository does **not** create GPU infrastructure and contains no Vast API key, SSH identity, GitHub token, or server address.

## 2. Install the profile

```bash
git clone https://github.com/<you>/sovereign-kit.git
cd sovereign-kit
chmod +x install-macos.sh
./install-macos.sh --with-opencode
```

The installer creates:

```text
~/.pi/profiles/sovereign/agent/
  settings.json
  models.json
  npm/                         # Pi packages isolated to this profile
~/.local/bin/pi-sovereign
~/.local/bin/sovkit-tunnel
~/.local/bin/opencode-sovereign
~/.config/opencode/sovereign.json
```

If `~/.local/bin` is not in your shell path, add this once:

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
exec zsh
```

## 3. Connect privately

The GPU server must bind SGLang to loopback. Do not expose port 30000 to the Internet.

Use a dedicated, unprivileged tunnel user and SSH identity. Verify the server host key out of band, then save that fingerprint in a dedicated known-hosts file (do **not** accept a first-use key blindly).

```bash
sovkit-tunnel \
  <ssh-host> <ssh-port> <tunnel-user> \
  ~/.ssh/qwen-sovereign_ed25519 \
  ~/.ssh/qwen-sovereign_known_hosts
```

This command deliberately binds its local port to `127.0.0.1`, so no other machine on your LAN can call it. It requires `StrictHostKeyChecking=yes`, the dedicated identity and the verified host-key file. Keep it running while using Pi.

## 4. Start a sovereign harness

### Pi / Oh-My-Pi

```bash
pi-sovereign
```

Before starting client work, run Pi’s model inspector:

```text
/subagents-models
```

Expected model for the parent and all workers:

```text
sovereign-qwen/qwen3.8-27b-nvfp4
```

If another provider appears, stop. Do not send client content until the profile is corrected.

### OpenCode

```bash
opencode-sovereign
```

The wrapper sets OpenCode’s inline config, whose precedence is higher than a project `opencode.json`. Only `sovereign-qwen/qwen3.8-27b-nvfp4` is enabled. It reads `QWEN_LOCAL_API_KEY` only to satisfy the OpenAI-compatible client; the wrapper defaults it to a non-secret local identifier for keyless SGLang. If SGLang API authentication is enabled, export the deployment-specific key in your shell; do not put it in this repository or OpenCode config.

## 5. Stop

Exit Pi, then interrupt the SSH tunnel with `Ctrl-C`. This only disconnects your MacBook; it does not destroy or stop the remote GPU server.
