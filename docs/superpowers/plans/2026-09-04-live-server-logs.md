# Live server logs implementation plan

**Goal:** Display bounded, redacted server logs during provisioning, with `l` opening a scrollable view and errors retaining the last snapshot.

**Architecture:** StrictSSHLauncher polls the fixed engine log path and readiness in one cancellable SSH loop, with a two-second interval. A ServerLogs observer transports sanitized snapshots through the existing session event channel. The root model owns a bounded snapshot and a Charm viewport; no provider or SSH operation runs from View. No extra polling goroutine is necessary.

**Constraints:** No live provider operations during development; preserve strict SSH checking and unrelated work. Reads are limited to 16 KiB / 200 lines; cancellation joins the poller. No new dependencies. Logs never imply readiness.

## Backend
- [x] Add tests for callbacks, executed shell response framing, redaction, process exit and cancellation.
- [x] Add internal/setup/server_logs.go: ServerLogs, ServerLogObserver, sanitized fixed-path snapshot reads and a cancellable two-second polling loop.
- [x] Wire StrictSSHLauncher and production operator callback; deliver snapshots before readiness/exit decisions and retain them on failure.

## TUI
- [x] Add a regression test for panel visibility, expanded scroll view and preservation on error.
- [x] Add internal/cli/provisioning_logs.go with session callbacks, bounded snapshots and viewport updates.
- [x] Render compact logs below the existing loader, support l/Esc and scrolling without replacing the provisioning state, retain error logs and mask secrets.

## Verification
- [x] Full suite: 497 passing tests across 14 packages; setup/CLI race suite: 314 passing tests. README updated. No live instance connection, mutation or automatic relaunch performed.
