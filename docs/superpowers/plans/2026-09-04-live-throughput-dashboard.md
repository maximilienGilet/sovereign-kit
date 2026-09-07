# Live Throughput Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the connected endpoint screen into the approved Focus + Inspector dashboard with a truthful ten-minute generation-throughput chart.

**Architecture:** Add a bounded, timestamp-based history component and an isolated Braille chart renderer under `internal/dashboardui`. Feed history from the existing three-second stats results, then reorganize only the main dashboard view around the chart and inspector. Keep the endpoint reader, tunnel lifecycle, integration flows, and provisioning UI unchanged.

**Tech Stack:** Go, Bubble Tea, Bubbles, Lip Gloss, Charm ANSI helpers, Go tests.

## Global Constraints

- Poll `/metrics` on the existing three-second cadence; do not add another timer or goroutine.
- Keep at most 200 samples and discard samples older than ten minutes.
- Missing generation telemetry is not numeric zero and must create a visual discontinuity.
- Use only local endpoint metrics; do not add a Vast mutation or public network path.
- Keep existing keyboard actions and optional integration behavior.
- The loading/provisioning ASCII animation is out of scope.
- Preserve unrelated changes in the shared dirty worktree; do not wholesale-stage, commit, reset, or push.

---

### Task 1: Bounded throughput history

**Files:**
- Create: `internal/dashboardui/throughput_history.go`
- Create: `internal/dashboardui/throughput_history_test.go`

**Interfaces:**
- Produces: `throughputSample{At time.Time, Generation *float64}`.
- Produces: `throughputHistory.add(time.Time, *float64)`, `reset()`, `snapshot(time.Time) []throughputSample`, and `summary(time.Time) throughputSummary`.
- `throughputSummary` exposes optional `Current`, `Average`, and `Peak` values calculated from reported samples only.

- [ ] **Step 1: Write failing bound and pruning tests**

Create tests that add 205 three-second samples, assert that only 200 remain, then advance `now` beyond ten minutes and assert that timestamp pruning removes expired samples.

- [ ] **Step 2: Run the focused history tests and confirm failure**

Run: `go test ./internal/dashboardui -run 'TestThroughputHistory' -count=1`

Expected: compilation failure because the history types do not exist.

- [ ] **Step 3: Implement the constant-memory history**

Use constants `throughputWindow = 10 * time.Minute` and `throughputCapacity = 200`. Copy numeric pointer values on insertion so later snapshots cannot mutate history. Prune by timestamp before applying the capacity bound.

- [ ] **Step 4: Write failing semantic tests**

Cover reported zero, nil/missing generation, current value from the newest sample, and average/peak calculated only from non-nil values. Assert that a newest nil sample makes `Current` nil without erasing historical average/peak.

- [ ] **Step 5: Implement summary semantics and rerun tests**

Run: `go test ./internal/dashboardui -run 'TestThroughputHistory' -count=1`

Expected: PASS.

---

### Task 2: Braille throughput chart

**Files:**
- Create: `internal/dashboardui/throughput_chart.go`
- Create: `internal/dashboardui/throughput_chart_test.go`

**Interfaces:**
- Consumes: `[]throughputSample`, a fixed `now`, and target width/height.
- Produces: `renderThroughputChart(samples []throughputSample, now time.Time, width, height int, interrupted bool) string`.
- Produces: `throughputCeiling([]throughputSample) float64` using a rounded 1/2/5 × 10ⁿ ceiling.

- [ ] **Step 1: Write failing chart tests**

Assert fixed visible width, labels `-10m`, `-5m`, and `now`, a zero-based y-axis, and a rounded ceiling that does not shrink while the peak remains inside the supplied window.

- [ ] **Step 2: Run the chart tests and confirm failure**

Run: `go test ./internal/dashboardui -run 'TestThroughputChart|TestThroughputCeiling' -count=1`

Expected: compilation failure because the renderer does not exist.

- [ ] **Step 3: Implement the isolated Braille canvas**

Map two horizontal and four vertical sub-cells into each Braille rune. Use the supplied timestamps for the fixed ten-minute x-domain and the rounded ceiling for y. Join adjacent reported samples with a line only when no missing sample separates them. Clamp all coordinates and sanitize dimensions before allocation.

- [ ] **Step 4: Add missing-data and geometry tests**

Render reported values with a nil sample between them and assert that the plot contains separate segments instead of a connecting stroke. Render twice at the same dimensions with changed values and assert identical line widths and height. Verify a fully reported zero window retains a baseline and a wholly missing window renders an explicit unavailable state.

- [ ] **Step 5: Add palette and interruption rendering**

Use existing `accent`, `muted`, and `bright` styles. An interrupted history remains visible but muted and includes `Telemetry interrupted`. Do not animate glyph positions or synthesize intermediate measurements.

- [ ] **Step 6: Rerun focused chart tests**

Run: `go test ./internal/dashboardui -run 'TestThroughputChart|TestThroughputCeiling' -count=1`

Expected: PASS.

---

### Task 3: Feed and reset live history

**Files:**
- Modify: `internal/dashboardui/model.go`
- Modify: `internal/dashboardui/layout_test.go`
- Create: `internal/dashboardui/history_integration_test.go`

**Interfaces:**
- Consumes: existing `statsResultMsg` and `endpointstats.Snapshot.CheckedAt`.
- Produces: `Model.throughput throughputHistory` and session/endpoint reset behavior.

- [ ] **Step 1: Write failing model-update tests**

Send multiple `statsResultMsg` values and assert generation values enter history with their `CheckedAt` timestamps. Assert a nil generation value creates a missing sample. Assert `tea.WindowSizeMsg` preserves all samples.

- [ ] **Step 2: Write failing lifecycle tests**

Verify that telemetry problems retain history, disconnect retains the frozen history without adding samples, reconnection clears it before new sampling, and a discovered endpoint identity change clears it. A stats result received while unhealthy must not enter history.

- [ ] **Step 3: Run the integration tests and confirm failure**

Run: `go test ./internal/dashboardui -run 'TestDashboardThroughputHistory' -count=1`

Expected: failure because the model does not yet retain throughput history.

- [ ] **Step 4: Integrate history into the existing update loop**

Add samples only for healthy accepted `statsResultMsg` values, using `CheckedAt` when present and the model clock fallback when absent. Keep telemetry failures as missing samples only when the server was queried successfully enough to timestamp the result. Track the healthy transition and endpoint identity so reconnect/new-endpoint resets are deterministic.

- [ ] **Step 5: Rerun model integration tests**

Run: `go test ./internal/dashboardui -run 'TestDashboardThroughputHistory' -count=1`

Expected: PASS.

---

### Task 4: Focus + Inspector responsive layout

**Files:**
- Modify: `internal/dashboardui/view.go`
- Modify: `internal/dashboardui/layout_test.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: history snapshot/summary, existing endpoint/session facts, and existing `metricsBody` values.
- Produces: wide Focus + Inspector composition and vertical compact composition.

- [ ] **Step 1: Write failing wide-layout tests**

At width 120+, assert the status strip precedes the performance area, the chart appears left of an inspector, and the view contains current/average/peak generation values, prompt throughput, KV, context, output, instance, and endpoint exactly once each.

- [ ] **Step 2: Write failing responsive tests**

At medium width, assert chart and inspector stack without horizontal overflow. At the existing smallest supported size, assert textual current/average/peak values remain while the Braille plot is omitted. Preserve selectable connection actions and scrolling.

- [ ] **Step 3: Run layout tests and confirm failure**

Run: `go test ./internal/dashboardui -run 'Test.*FocusInspector|Test.*ThroughputLayout' -count=1`

Expected: failure against the old Activity/Connection card grid.

- [ ] **Step 4: Implement the new view composition**

Split rendering into focused helpers for status strip, throughput panel, inspector, session, and endpoint actions. Remove metric duplication from the old `activityBody`/`metricsBody` paths used by the main page. Keep detail/integration pages and key handling unchanged.

- [ ] **Step 5: Update README dashboard copy**

Document the ten-minute live generation graph, reported-value-only summaries, missing-data gaps, and responsive text fallback. Do not claim persistence or measured latency.

- [ ] **Step 6: Run dashboard package tests**

Run: `go test ./internal/dashboardui -count=1`

Expected: PASS.

---

### Task 5: Review and full verification

**Files:**
- Review only: all files changed by Tasks 1–4.

- [ ] **Step 1: Request an independent code review**

Ask the reviewer to check metric truthfulness, missing-versus-zero semantics, fixed ten-minute timing, chart bounds, duplicate facts, terminal overflow, stale async results, and preservation of existing dashboard actions.

- [ ] **Step 2: Resolve every Critical or Important finding test-first**

For each accepted finding, add or strengthen a failing regression test, implement the smallest correction, and rerun the focused package tests.

- [ ] **Step 3: Run complete verification**

Run:

```text
go test ./... -count=1 -timeout=90s
go test -race ./internal/dashboardui ./internal/endpointstats -count=1 -timeout=120s
go vet ./internal/dashboardui ./internal/endpointstats
```

Expected: every command exits zero with no test, race, or vet failure.

- [ ] **Step 4: Install the verified local binary**

Run the repository's macOS installer only after the final verification. Confirm that existing Pi/OpenCode profiles remain unchanged and do not perform any Vast instance action.
