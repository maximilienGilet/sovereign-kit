# Custom provider and safe exit implementation plan

> **For agentic workers:** Use test-driven development for each bounded task and review integration before release.

**Goal:** Preserve users' normal Pi environment and offer explicit remote-instance lifecycle choices on TUI exit.

**Architecture:** Provider-only configuration merges retain the existing inspection, confirmation, backup and rollback boundary. Exact-instance stop/destruction capabilities verify remote results; the Bubble Tea root owns exit confirmation and asynchronous shutdown.

**Tech Stack:** Go, Bubble Tea/Lip Gloss, JSON configuration, Vast HTTP API.

## Global constraints

- Do not mutate live Vast instances or install real integration settings during implementation.
- Preserve unrelated dirty files and user customizations. No automatic profile installation.
- Stop and quit is the default remote action; leave running is explicit, destruction gets a second confirmation, Escape cancels.
- Errors and unknown states never report success or automatically close the TUI.
- Existing precompiled Solo recipes remain pinned; no image rebuild is needed.
- Dashboard redesign and remote idle watchdog are separate subsequent work. Output limit is user-managed.

## Task 1: Provider-only installation

- [x] Add failing behavior tests in internal/clientprofile for retaining settings, skills, credentials and other providers, while selecting the model by launch flags.
- [x] Update Resolve/managed/render/install so normal Pi models.json alone is merged, with existing backups and stale-inspection protection. Verify actual Pi/OMP CLI behavior using installed source/docs.
- [x] Retain explicit inspection/confirmation; no package install or default-model mutation. Update integration labels and tests.
- [x] Run clientprofile/dashboardui tests and inspect generated provider with the actual installed Pi registry offline.

## Task 2: Verified remote stop

- [x] Add HTTP boundary tests for StopInstance request, API rejection, invalid ID and wrong response ID.
- [x] Add setup recovery tests for observed stopped state, timeout, cancellation and ambiguous requests. Keep checkpoints on stop.
- [x] Extend InstanceRecovery with Stop(context.Context) error; expose construction for saved route lifecycle operations.
- [x] Run Vast/setup tests without touching live infrastructure.

## Task 3: Exit dialog

- [x] Add CLI behavior tests for q/Ctrl+C, default stop, explicit leave, second destruction approval, cancel and failed stop retry.
- [x] Add application_exit.go to own choices and lifecycle operation; resolve only the displayed instance from saved config/checkpoint and credentials.
- [x] Integrate Update/View/cleanup, join ongoing setup and connection workers before remote mutation; do not exit until capability confirms success.
- [x] Preserve recoverability after errors and small-terminal interaction; do not claim forced process termination can show a prompt.

## Task 4: Integration verification

- [x] Review owned diffs, run full Go tests, Python tests isolated from the user's live endpoint and race tests on changed subsystems.
- [x] Build and install local CLI only; profiles/instances remain unchanged.
- [x] Record evidence and limitations, including preserved old resume snapshots and provider-dependent storage charges after stopping.

## Verification record

- 599 Go tests passed across 15 packages; 462 tests passed with race detection
  across clientprofile, dashboardui, cli, setup and vast.
- 14 Python tests passed with the missing-tunnel fixture isolated from the user's
  live endpoint using SOVKIT_ENDPOINT_URL=http://127.0.0.1:0/v1/models.
- Real installed OMP 18.1.6 loaded generated default and named-profile providers
  in temporary homes. No production profile files were modified.
- Focused lifecycle review approved after regression fixes for checkpoint-path
  refresh, recovery refreshed during save/cancel, and short-terminal errors.
- install-macos.sh rebuilt and installed the CLI and wrappers. No instances were
  created, stopped, destroyed or otherwise modified during this work.
- Existing precompiled image remains publicly published and pinned in both Solo
  recipes. Existing saved deployment snapshots are not migrated.
- Scope limitations: dashboard redesign/idle watchdog remain subsequent work.
  Stopped instances require restart in Vast before reconnecting. Legacy doctor
  integration checks remain isolated-profile oriented; current provider inspection
  is the connected TUI. Existing subagent model preferences are intentionally kept.
