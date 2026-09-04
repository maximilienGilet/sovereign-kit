# Endpoint Dashboard Implementation Plan

> **For agentic workers:** Use subagent-driven-development for implementation and scoped review.

**Goal:** Deliver the approved endpoint-first dashboard and explicit, verified client profile setup.

**Architecture:** A dedicated endpoint/profile service supplies a Bubble Tea dashboard. The application owns all cancellable workers and the SSH tunnel. Persist recipe-derived metadata when saving the route and match it against live model discovery; unknown limits block profile generation.

**Tech Stack:** Go, Bubble Tea/Bubbles/Lip Gloss, existing clipboard library, existing Pi integration packages.

## Completion — 2026-09-04

Implemented in `0c34ef8`, with review fixes in `1b66f1c` and `f4f0c98`.
Task review and final integrated review identified five concrete issues; all were
fixed and approved in scoped re-reviews. No open findings remain for this task.

Final independent verification: `go test -count=1 ./...` passed 535 tests in 15
packages; the complete Python suite passed 13 tests; CLI build and shell syntax
checks passed. Installed Pi 0.84.2 accepted a generated temporary profile and its
local placeholder authentication in offline mode. No live profile installation,
package download, endpoint smoke test, or remote mutation was performed.

Existing configurations without verified model limits remain usable for generic
endpoint connections, but cannot install a model-specific profile until those
limits are available. Existing unrelated working-tree changes were preserved;
this verification does not claim clean-checkout or merge readiness.

## Global Constraints

- Specification: docs/superpowers/specs/2026-09-04-endpoint-dashboard-design.md.
- No live profile writes, paid deployment, remote changes or package installation during development.
- No automatic client launch, even for OpenCode. Existing dirty changes must be preserved.
- Explicit confirmation before installing/updating profiles; ready profiles skip that step.
- Preserve unrelated files/settings; backup before mutation; fail closed on unsafe paths, unreadable configuration, unknown model identity/limits or ambiguous models.
- Shell commands start with rtk; use apply_patch for edits. Stay on current feature branch unless user chooses otherwise.

### Task 1: Endpoint-first dashboard and confirmed integration setup

**Files and ownership:**
- Add focused endpoint discovery and profile inspection/installation files under internal/clientprofile/ with tests.
- Replace client-launch dashboard logic in internal/dashboardui/; keep rendering/state transitions separated into small files.
- Integrate through internal/cli/application_connection.go, application.go, application_view.go and focused tests.
- Persist optional model metadata in internal/config/config.go and internal/setup/orchestrator.go, with tests.
- Update standalone/headless entry points and README to avoid exposing the old launcher.
- Update install-macos.sh and its temporary-home tests: installation of the tool must not bypass dashboard consent or generate hard-coded client profiles.

**Interfaces:** Define endpoint metadata and profile inspection results in clientprofile, consumed by dashboardui/cli. Discovery receives a context and configured base URL, returns the actual unique model ID and verified limits or a visible reason installation is unavailable. Profile operations take resolved target paths and verified model metadata; installation is a distinct operation reachable only from confirmation. Keep system dependencies injectable at HTTP, filesystem-root, clipboard and package-command boundaries.

- [ ] Write failing behavior tests. Use httptest for bounded discovery (one model, multiple models, malformed/failed responses, cancellation), real temporary directories for absent/ready/different/incomplete/unreadable profiles and backup preservation, and Bubble Tea Update messages for confirmation/cancel/copy/late worker results. A basic confirmation test must assert that selecting an integration invokes inspection but installation count stays zero until explicit positive confirmation.
- [ ] Run focused tests and record the expected RED result before implementing each behavior.
- [ ] Implement bounded live `/v1/models` discovery without redirects away from the configured endpoint. Persist optional recipe-derived model ID/context/output metadata on successful setup and use it only when the actual discovered model matches. Existing configurations without metadata remain loadable. If reliable endpoint metadata is used as a fallback, verify its meaning against local source/docs and record that evidence; never guess model limits.
- [ ] Implement profile inspection as read-only. Compare endpoint, model/default selection, required dependencies and verified limits. Resolve installation and command paths identically. Represent absent, ready, incomplete, different and unreadable distinctly.
- [ ] Implement confirmed installation with cancellable bounded package commands, staging/backups and preservation of unrelated settings, credentials, sessions and packages. Reject unsafe/symlink paths, recheck state before mutation, and keep the previous profile recoverable on failure. Verify the installed profile before returning ready. Use existing pinned package versions; do not install packages during tests outside temporary fixtures.
- [ ] Implement the endpoint-first dashboard, raw endpoint/model copying, generic connection guidance, integration inspection, cancel-default confirmation, installation progress/error/retry, and ready-only manual launch command copying. Keep escape/back inside the dashboard and reserve disconnect/quit for tunnel lifecycle actions. No client launch commands remain reachable.
- [ ] Bind workers to the application connection context/generation, cancel and join them on disconnect, and ignore stale results. Keep inspection/network/package work out of Update/View. Use a scrollable narrow layout and retain useful headers/footer actions.
- [ ] Run all Go tests, build, and focused race tests for changed worker lifecycle. Document actual evidence and any unavailable live validations. Do not claim profile readiness based solely on a directory or a successful installer exit.
- [ ] Commit only task-owned changes, leaving unrelated work untouched, and write an implementation report with RED/GREEN evidence and modified file list.

### Task 2: Review and final verification

- [ ] Review the implementation against every acceptance check in the specification, particularly destructive writes, cancellation, model identity and bypasses of confirmation.
- [ ] Fix concrete findings through the implementer and re-run affected tests.
- [ ] Run the complete Go suite and build on the final working tree; update documentation and report limits honestly. No real installation is part of verification.
