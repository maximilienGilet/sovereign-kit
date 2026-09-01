# Quick start

This is the shortest path from a prepared GPU host to a local Pi or OpenCode session. It assumes you have already read [Security](security.md) and have an approved Qwen/SGLang host.

## 1. Check prerequisites on the Mac

You need:

- macOS with the standard `ssh` client;
- [Pi](https://github.com/badlogic/pi-mono) installed and available as `pi`;
- Git;
- Node.js and npm only if you want the installer to install OpenCode.

```bash
pi --version
git --version
ssh -V
```

For OpenCode, also check:

```bash
npm --version
```

## 2. Clone and install

```bash
git clone https://github.com/maximilienGilet/sovereign-kit.git
cd sovereign-kit
./install-macos.sh --with-opencode
```

Without `--with-opencode`, the installer still writes the OpenCode configuration and wrapper. Install the pinned client yourself before using it:

```bash
npm install --global opencode-ai@1.18.25
```

If `~/.local/bin` is not on your shell path, add it once:

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
exec zsh
```

See [Local setup](local-setup.md) for the files created and upgrade behaviour.

## 3. Prepare the SSH trust files

Do not accept an unknown host key at first connection. Obtain the GPU host fingerprint through a channel you trust, then create a dedicated known-hosts file containing that verified key.

Keep the tunnel identity and known-hosts file outside the repository. Restrict their permissions:

```bash
chmod 600 ~/.ssh/qwen-sovereign_ed25519 ~/.ssh/qwen-sovereign_known_hosts
```

The exact host entry depends on the hostname and SSH port. Verify it before use:

```bash
ssh-keygen -lf ~/.ssh/qwen-sovereign_known_hosts
```

## 4. Open the tunnel

```bash
sovkit-tunnel <ssh-host> <ssh-port> <tunnel-user> \
  ~/.ssh/qwen-sovereign_ed25519 \
  ~/.ssh/qwen-sovereign_known_hosts
```

Leave this terminal open. The command fails instead of falling back if the identity, known-hosts file, host key, or forward cannot be established.

## 5. Check the local endpoint

In another terminal:

```bash
curl --fail --silent --show-error http://127.0.0.1:30000/v1/models
```

If the server requires authentication, use a deployment-specific secret for an endpoint check and for OpenCode:

```bash
export QWEN_LOCAL_API_KEY='<deployment secret>'
curl --fail --silent --show-error \
  -H "Authorization: Bearer $QWEN_LOCAL_API_KEY" \
  http://127.0.0.1:30000/v1/models
```

The shipped Pi profile is keyless and does not consume `QWEN_LOCAL_API_KEY`. Do not continue with Pi against an authenticated server until you have built and tested a compatible Pi profile. See [Local setup](local-setup.md#authentication-caveat).

## 6. Start a harness

```bash
pi-sovereign
# or
opencode-sovereign
```

In Pi, run `/subagents-models`. The parent and every worker must show `sovereign-qwen/qwen3.8-27b-nvfp4`. If they do not, stop and read [Operations](operations.md).
