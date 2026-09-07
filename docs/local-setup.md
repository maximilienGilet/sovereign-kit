# Local setup

Run `./install-macos.sh` to build the CLI and install its helpers in `~/.local/bin`.
This does not install client profiles or change your settings. `--with-opencode`
explicitly installs OpenCode 1.18.25; `--upgrade` is accepted without replacing profiles.

## Add an optional client

Connect using `sovkit start`, select **Pi**, **Oh My Pi (OMP)** or **OpenCode**, and
inspect the destination. Confirm the update only if needed. Enter then copies a
launch command to use in another terminal from your project directory.

- Pi: `PI_CODING_AGENT_DIR` or `~/.pi/agent`; only `models.json` is managed.
- OMP: `PI_CODING_AGENT_DIR`, or the agent directory beneath `PI_CONFIG_DIR`
  (default `~/.omp/agent`); existing `models.yml`, `models.yaml`, or `models.json`
  is used following the client's precedence. Unsupported YAML fails closed.
  Active named `OMP_PROFILE` / `PI_PROFILE` selections are retained and take
  precedence over the agent-dir override, as in OMP itself.
- OpenCode: `SOVEREIGN_OPENCODE_CONFIG` or `~/.config/opencode/sovereign.json`;
  this remains a separate provider-restricted configuration.

Pi and OMP retain settings, default models, skills, extensions, authentication and
other providers. Only the Sovereign provider/model is merged; no packages are
installed. A backup is retained before updating existing managed files. Changes
since inspection require another confirmation. Existing isolated profiles remain
on disk but are no longer selected by default. No customizations are copied out
of them automatically.

The copied commands select the actual served model for that invocation. Subagents
and extensions retain their configured policies; this is not a guarantee that all
their traffic uses Sovereign. `pi-sovereign` also uses the normal Pi directory,
selecting the sole registered Sovereign model or the explicit `SOVKIT_MODEL`.
It no longer reads the legacy `PI_SOVEREIGN_DIR` override.

The older `sovkit doctor` helper still checks legacy isolated integration policies;
use the connected TUI's integration inspection to validate these normal-directory
providers. Generic endpoint health is independent of optional integrations.

## Leaving the TUI

`q`/`Ctrl+C` prompts for stop, leave running, destruction, or cancellation when a
Vast instance is known. Stop is selected by default; destruction has an additional
confirmation defaulting to Cancel. A verified remote result is required before
exit. On failure, the error stays visible and can be retried.

Stopping retains disk and the saved route; storage charges may remain. On the
next connection, the TUI checks the actual Vast status first. A stopped instance
offers **Restart and connect**, with Cancel selected by default and a warning that
GPU billing resumes. It then waits for readiness, refreshes SSH coordinates while
preserving the pinned host keys, and restores the exact inference recipe before
opening the dashboard. New deployments save the complete recipe locally; older
deployments must match a built-in recipe's remote fingerprint. Unknown recipes
or changed host keys are refused, not guessed. Interrupted restarts retain a
local pending flag so reconnecting can finish restoring the server without
issuing another start request to an already running instance.

When Vast returns `resources_unavailable` with an explicit queued acknowledgement,
the TUI displays a resource-waiting state and polls without resubmitting the start.
An existing remote start intent is also observed on reconnect, not submitted again.
Actual `running` status and SSH coordinates are required before continuing.
Waiting is bounded to 15 minutes; ending local observation does not cancel the
remote queue. Use **Stop instance and quit** (which verifies queue cancellation)
or explicitly destroy the instance. **Leave running and quit** retains queued
starts and can resume GPU billing later. Other refusals remain errors with their
sanitized provider reason; they are never treated as successful starts.

The TUI does not run a remote inactivity watchdog. Headless connection remains
local-only and does not automatically restart paid instances.
Closing/killing the terminal or losing connectivity
cannot guarantee a prompt or successful remote shutdown; check Vast directly.

No instances or real client provider files are modified just by installing the
CLI. The local API placeholder `local-qwen-tunnel` is not a Vast API key: the
loopback endpoint is protected by the private SSH route.
