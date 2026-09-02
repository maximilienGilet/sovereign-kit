# Quick start

This is the shortest supported path from a prepared GPU host to a local Pi or OpenCode session on macOS. It uses the shared **keyless** Studio route. Read [Security](security.md) first, and complete [Server setup](server.md) before starting on the Mac.

## 1. Check Mac prerequisites

You need macOS, the standard `ssh` client, Git, and [Pi](https://github.com/badlogic/pi-mono) available as `pi`. Node.js/npm are needed only when the installer should install OpenCode.

```bash
pi --version
git --version
ssh -V
# only when using --with-opencode
npm --version
```

Each command must return successfully. Install the missing dependency and open a new shell before continuing. This repository does not install Pi itself.

## 2. Clone and install

```bash
git clone https://github.com/maximilienGilet/sovereign-kit.git
cd sovereign-kit
./install-macos.sh --with-opencode
```

The installer prints the installed Pi profile and OpenCode configuration paths, then the wrapper commands. If it says an existing Pi profile would be overwritten, stop and decide whether `--upgrade` is appropriate; it backs up the Pi profile but not the OpenCode configuration or wrappers. See [Local setup](local-setup.md#upgrades-and-removal).

Without `--with-opencode`, install the pinned OpenCode client before using its wrapper:

```bash
npm install --global opencode-ai@1.18.25
opencode --version
```

The wrappers are installed in `~/.local/bin`. If the shell cannot find them, add that directory once and open a new zsh session:

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
exec zsh
```

## 3. Prepare the SSH trust files

Obtain the GPU host’s SSH fingerprint through a channel you trust. Do **not** accept an unknown key at first connection as a substitute for verification. Create a dedicated known-hosts file containing the verified key and use a dedicated private identity for the tunnel.

Keep both files outside the repository and limit their permissions:

```bash
chmod 600 ~/.ssh/qwen-sovereign_ed25519 ~/.ssh/qwen-sovereign_known_hosts
ssh-keygen -lf ~/.ssh/qwen-sovereign_known_hosts
```

The final command prints the fingerprints saved in that file. Compare them with the independently verified value. A mismatch or a changed host key is a stop condition, not a prompt to disable strict checking.

## 4. Open the tunnel

Use the deployment-specific SSH host, port, and unprivileged user:

```bash
sovkit-tunnel <ssh-host> <ssh-port> <tunnel-user> \
  ~/.ssh/qwen-sovereign_ed25519 \
  ~/.ssh/qwen-sovereign_known_hosts
```

Leave this terminal open. The wrapper binds only `127.0.0.1:30000` on the Mac and fails rather than falling back when the identity, known-hosts file, host key, or port forward is invalid.

## 5. Verify the local route

In a second terminal, check the forwarded endpoint:

```bash
curl --fail --silent --show-error http://127.0.0.1:30000/v1/models
sovkit doctor
```

`curl` must return JSON with a `data` list and exit `0`. `sovkit doctor` must report no failures. It may warn about an intentional path override or a missing OpenCode executable; resolve or explicitly inspect every warning before client work.

Do **not** set `QWEN_LOCAL_API_KEY` for the shared Studio profile: its SGLang route is keyless and the shipped Pi profile uses the non-secret compatibility identifier `local-qwen-tunnel`. If `/v1/models` returns `401` or `403`, stop: the server does not match the shared V1 route. Do not use Pi against it until a compatible authenticated profile has been built and live-tested.

## 6. Start a harness and smoke-test it

From a disposable or approved non-sensitive repository, launch one client:

```bash
pi-sovereign
# or
opencode-sovereign
```

In Pi, run `/subagents-models`. The parent and every worker must show:

```text
sovereign-qwen/qwen3.8-27b-nvfp4
```

In OpenCode, confirm the selected model is the same value. Start with a non-sensitive prompt, such as asking the client to explain a small disposable local file. If another provider/model appears, or the request fails, stop and follow [Operations](operations.md) and [Development harness](development-harness.md). Do not work around a failure by launching bare `pi`/`opencode`, opening a public port, or adding a public-provider fallback.

## 7. End the session

Exit Pi or OpenCode, stop the tunnel with `Ctrl-C`, then stop and destroy the remote GPU host according to the deployment procedure. Closing the Mac client or the tunnel alone does not stop provider billing.
