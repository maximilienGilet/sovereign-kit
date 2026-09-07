# Provisioning Forge Animation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pair an honest indeterminate Magnetic Pulse animation with the measured Crystal Forge construction animation.

**Architecture:** Add a pure fixed-size forge renderer and let `provisioningLoader` select its mode from existing provider/download state. Keep download parsing and the Bubble Tea clock unchanged; store only a per-run measured-progress high-water mark in the loader.

**Tech Stack:** Go, Bubble Tea, Bubbles, Lip Gloss, Braille terminal rendering.

## Global Constraints

- Magnetic Pulse must never imply a percentage or persistent completion.
- Crystal Forge construction must be driven only by trustworthy measured bytes.
- The renderer must keep a fixed 41×19 footprint and never add timers or goroutines.
- Compact, `NO_COLOR`, and accessible output must retain truthful textual status.
- No paid Vast instance is created by automated verification.

---

### Task 1: Pure Forge Renderer

**Files:**
- Create: `internal/cli/provisioning_forge.go`
- Create: `internal/cli/provisioning_forge_test.go`
- Modify: `internal/cli/provisioning_cube.go`

**Interfaces:**
- Produces: `provisioningForge(mode forgeMode, ratio float64, elapsed, wave time.Duration, color bool, marker string) string`
- Produces: fixed 41×19 rendered geometry for both forge modes.

- [ ] Write failing tests proving Magnetic Pulse preserves identical stripped geometry across frames while its continuous inward lighting changes, plus monotonic measured construction, stable built cells, frozen markers, and structural `NO_COLOR` output.
- [ ] Run `rtk go test ./internal/cli -run 'TestProvisioningForge' -count=1` and verify failures are caused by the missing renderer.
- [ ] Implement the pure renderer by reusing the existing crystal geometry and deterministic ordered material cells.
- [ ] Re-run the focused tests and keep existing cube tests green.

### Task 2: Loader State and Responsive Composition

**Files:**
- Modify: `internal/cli/provisioning_loader.go`
- Modify: `internal/cli/provisioning_logs.go`
- Modify: `internal/cli/provisioning_transfer.go`
- Modify: `internal/cli/provisioning_transfer_test.go`
- Modify: `internal/cli/provisioning_loader_test.go`

**Interfaces:**
- Consumes: `provisioningForge(...)` from Task 1.
- Produces: monotonic per-run `transferRatio`, retained completion state, and responsive non-duplicated transfer presentation.

- [ ] Add failing integration tests for high-water progress, completed-crystal retention, no full transfer bar beside the large motif, compact real meter, and unknown-total honesty.
- [ ] Run the focused CLI tests and verify the new assertions fail for the existing loader.
- [ ] Add loader state transitions and select Magnetic Pulse or Crystal Forge without changing the existing clock.
- [ ] Split transfer presentation into numeric detail and an optional compact meter.
- [ ] Re-run focused CLI tests and responsive overflow tests.

### Task 3: Accessible Output and Documentation

**Files:**
- Modify: `internal/cli/accessible.go`
- Modify: `internal/cli/accessible_test.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: existing `ServerLogs` snapshots and `setup.ParseDownloadProgress`.
- Produces: append-only measured download status without animation frames.

- [ ] Add a failing accessible-output test for one truthful download line per changed measurement and no percentage for unknown totals.
- [ ] Run the focused accessible test and verify the missing behavior.
- [ ] Implement accessible download announcements and update the README semantics.
- [ ] Run `rtk go test ./internal/cli ./internal/setup -count=1`.
- [ ] Run `rtk go test ./... -count=1 -timeout=90s` and the relevant race suite.
- [ ] Request an independent code review and resolve every confirmed high/medium-priority issue.
