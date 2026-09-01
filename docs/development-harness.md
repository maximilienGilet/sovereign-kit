# Development harness: Pi, subagents, and OpenCode

This is the specific development path built on top of the generic endpoint. It uses an isolated Pi profile, pi-subagents, Oh-My-Pi, and a locked OpenCode configuration so normal client configuration cannot select a public model provider.

**Validation status:** the installed configuration, wrappers, provider allowlists, and diagnostic are structurally tested. A live Pi and OpenCode session against the current digest-pinned SGLang recipe has not yet been completed. Treat this as a prepared runbook, not a claim of live end-to-end validation.

## Before opening a project

1. Complete [Quick start](quickstart.md) through the tunnel step.
2. In a second terminal, run:

   ```bash
   sovkit doctor
   ```

   Continue only if it reports no failures. A failure to reach `127.0.0.1:30000/v1/models` means the tunnel or remote server is unavailable.

3. Use the shared V1 keyless route. The shipped Pi profile cannot use an authenticated SGLang endpoint. See [Local setup](local-setup.md#authentication-caveat).

## Pi and subagents

From the repository you want to work in:

```bash
pi-sovereign
```

Pi must start with the isolated profile at:

```text
~/.pi/profiles/sovereign/agent
```

Inside Pi, run:

```text
/subagents-models
```

The parent and every available worker must show:

```text
sovereign-qwen/qwen3.8-27b-nvfp4
```

If a different provider or model appears, stop. Do not begin client work. Check that `PI_CODING_AGENT_DIR` is unset or points to the installed Sovereign Kit profile, then rerun `sovkit doctor`.

### First non-sensitive prompt

Use a disposable repository or an approved non-sensitive task first. Ask Pi to inspect a small local file and explain it. Confirm the displayed model remains the approved Qwen route before using project secrets or client code.

The profile locks normal pi-subagents selection to `sovereign-qwen/*`. It does not sandbox shell commands, Git remotes, browser traffic, extensions, or tool permissions. Review the project and Pi tool settings separately.

## OpenCode

From the repository you want to work in:

```bash
opencode-sovereign
```

The wrapper injects the installed configuration through `OPENCODE_CONFIG_CONTENT`. On the supported configuration, this takes precedence over a repository-level `opencode.json`, so an untrusted project cannot re-enable OpenAI, Anthropic, DeepSeek, OpenRouter, or another provider through its local config.

Start with a non-sensitive prompt and verify the UI shows:

```text
sovereign-qwen/qwen3.8-27b-nvfp4
```

If OpenCode is missing:

```bash
npm install --global opencode-ai@1.18.25
opencode --version
```

Then retry. The wrapper checks only that `opencode` exists; use the pinned version above and run the smoke prompt before trusting the route.

## Working in a real project

1. Open the tunnel and keep its terminal visible.
2. Run `sovkit doctor`.
3. Start either `pi-sovereign` or `opencode-sovereign` from the project directory.
4. Check the selected model before the first prompt.
5. Begin with a non-sensitive task.
6. Keep client credentials out of prompts, terminal output, Git diffs, and commits.
7. Stop the client and tunnel when finished.

A project can still contain unsafe instructions, plugins, tools, or remotes. The provider lock prevents an unapproved model route; it is not a sandbox or source-code trust decision.

## Troubleshooting

| Symptom | Action |
|---|---|
| `sovkit doctor` endpoint failure | Check the remote SGLang process, then restart `sovkit-tunnel`. Do not change either bind address to `0.0.0.0`. |
| Pi shows a different model | Stop. Inspect `PI_CODING_AGENT_DIR`, reinstall if necessary, then rerun `/subagents-models`. |
| OpenCode starts with another provider | Stop. Run `opencode-sovereign`, not bare `opencode`; inspect `SOVEREIGN_OPENCODE_CONFIG` and remove unintended overrides. |
| Host-key warning | Stop. Verify the expected host key through an independent channel; never accept a changed key blindly. |
| API authentication enabled on server | Do not use the shared Pi profile. Either return to the keyless V1 route or build and test a separate Pi-compatible authenticated deployment. |

## End a session

Exit Pi or OpenCode, then stop the tunnel with `Ctrl-C`. Stop the remote model process and destroy or stop the GPU instance according to the deployment procedure. Closing the local client alone does not stop server billing.
