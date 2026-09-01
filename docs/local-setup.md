# Local setup

`install-macos.sh` creates a self-contained Pi profile and installs three wrappers. It does not create a GPU server or an SSH account.

## Install commands

```bash
./install-macos.sh
./install-macos.sh --with-opencode
./install-macos.sh --upgrade --with-opencode
```

`--with-opencode` runs `npm install --global opencode-ai@1.18.25` before the Pi profile is changed. `--upgrade` moves an existing Sovereign Kit Pi profile to `agent.backup.<timestamp>`, then installs a fresh profile.

Without `--upgrade`, the installer refuses to overwrite an existing Pi profile. It **does** overwrite `~/.config/opencode/sovereign.json` and the three wrappers in `~/.local/bin` without making backups. Copy local changes elsewhere before rerunning the installer. There is no uninstaller.

## Files written

| Path | Purpose | Mode |
|---|---|---:|
| `~/.pi/profiles/sovereign/agent/settings.json` | Pi defaults and strict subagent model scope | `600` |
| `~/.pi/profiles/sovereign/agent/models.json` | The only Pi model provider: local `sovereign-qwen` | `600` |
| `~/.pi/profiles/sovereign/agent/npm/` | Isolated `pi-subagents@0.62.0` and `oh-my-pi@0.2.0` packages | inherited from profile |
| `~/.config/opencode/sovereign.json` | OpenCode's single-provider configuration | `600` |
| `~/.local/bin/pi-sovereign` | Starts Pi with the isolated profile | `700` |
| `~/.local/bin/sovkit-tunnel` | Opens the strict local SSH forward | `700` |
| `~/.local/bin/opencode-sovereign` | Starts OpenCode with its inline locked configuration | `700` |

Set `PI_SOVEREIGN_DIR` before installation if you need the Pi profile elsewhere. Set `SOVEREIGN_OPENCODE_CONFIG` before running `opencode-sovereign` if you need a different local OpenCode configuration path.

`pi-sovereign` also honours `PI_CODING_AGENT_DIR`. These overrides are for deliberate local testing or administration. They bypass the standard installed path, so they also bypass the route guarantee described by this documentation unless you inspect the replacement configuration yourself.

## What the profiles lock

### Pi and subagents

The Pi profile has one provider, `sovereign-qwen`, at `http://127.0.0.1:30000/v1`. Its default model is `qwen3.8-27b-nvfp4`.

`pi-subagents` enforces the allowlist `sovereign-qwen/*`. A worker cannot select a second provider through the normal subagent configuration.

### OpenCode

`opencode-sovereign` reads `~/.config/opencode/sovereign.json` and exports it as `OPENCODE_CONFIG_CONTENT` before launching OpenCode. This wins over a project-level `opencode.json` on the supported `opencode-ai@1.18.25` release.

The installer installs that version only with `--with-opencode`. The wrapper only checks that `opencode` exists; it does not enforce or inspect its version. If you installed it separately, check `opencode --version` and run a non-sensitive smoke test before trusting the route.

The wrapper sets `QWEN_LOCAL_API_KEY=local-qwen-tunnel` only when that environment variable is absent. This placeholder works with a keyless SGLang endpoint. When server authentication is enabled, export the real deployment-specific key in your shell first.

## Authentication caveat

The shipped Pi profile uses the literal non-secret identifier `local-qwen-tunnel`. It supports a keyless SGLang endpoint reached through the SSH tunnel.

`opencode-sovereign` can use a server key through `QWEN_LOCAL_API_KEY`. The current Pi wrapper does **not** inject that variable into its model configuration. Do not enable API authentication for a shared Pi deployment unless you create and test a compatible Pi profile for that deployment. Keep such a profile and its secret outside this repository.

## Upgrades and removal

Keep backups until you have started Pi successfully. To remove the local setup, delete only the paths listed above after stopping Pi, OpenCode, and the tunnel. Removing them does not stop or destroy the remote GPU server.
