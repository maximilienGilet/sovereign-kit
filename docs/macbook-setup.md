# Setup MacBook

## 1. Prerequisites

- macOS with an SSH client (included by default).
- Pi CLI installed and available as `pi`.
- GitHub access to clone this repository.
- A separately provisioned Qwen/SGLang server that is reachable over SSH.

This repository does **not** create GPU infrastructure and contains no Vast API key, SSH identity, GitHub token, or server address.

## 2. Install the profile

```bash
git clone https://github.com/<you>/qwen-sovereign-harness.git
cd qwen-sovereign-harness
chmod +x install-macos.sh
./install-macos.sh
```

The installer creates:

```text
~/.pi/profiles/sovereign/agent/
  settings.json
  models.json
  npm/                         # Pi packages isolated to this profile
~/.local/bin/pi-sovereign
~/.local/bin/qwen-sovereign-tunnel
```

If `~/.local/bin` is not in your shell path, add this once:

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
exec zsh
```

## 3. Connect privately

The GPU server must bind SGLang to loopback. Do not expose port 30000 to the Internet.

```bash
qwen-sovereign-tunnel <ssh-host> <ssh-port>
```

This command deliberately binds its local port to `127.0.0.1`, so no other machine on your LAN can call it. Keep it running while using Pi.

## 4. Start the sovereign harness

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

## 5. Stop

Exit Pi, then interrupt the SSH tunnel with `Ctrl-C`. This only disconnects your MacBook; it does not destroy or stop the remote GPU server.
