# Dashboard polish evidence

Task 2 implemented in `internal/dashboardui/model.go`, `view.go`, `polish_test.go`, and `layout_test.go`, preserving the preexisting changes in those files.

- `View` renders a local model copy at a maximum 140 columns and adds horizontal padding based on the actual terminal width. The model retains its real terminal dimensions; root rendering must not center the dashboard again.
- Wide layouts pair session and endpoint cards. The chart grows from six to fourteen rows with available terminal height; compact terminals retain their four-row plot and endpoint actions.
- Server logs start folded. `l` toggles the existing snapshot locally, including after a tunnel disconnect; the action hint advertises it. Folded session events retain their latest event. Expanding logs does not fetch anything or run a command.
- Unknown request counters show “Waiting for telemetry”; measured zero active and queued requests show “Idle”. Missing throughput remains unavailable, never synthetic zero. Interrupted telemetry remains explicitly identified.

## Red / green

Observed failures before each implementation:

- Width/centering test: `content is not centered: "PRIVATE ENDPOINT"`.
- Folded-log test: `logs expanded by default`.
- Wide-chart/lower-cards test: `[120 40]: chart height 6, want 10..14`.
- Waiting/idle test: `unknown activity mislabeled`, first for wide, then separately for compact rendering.

Each targeted test passed after its corresponding implementation. The preexisting snapshot sanitization test now presses `l` before checking expanded log contents, matching the new default.

Final command: `rtk go test ./internal/dashboardui -race -count=1` — passed (63 tests reported by RTK). Tests cover 80×24, 120×40, 180×55, and 300×90, folded and expanded logs, terminal bounds, and keyboard navigation keeping OpenCode visible. The tall layouts check a 10–14-row visible chart and paired lower cards. Existing missing-data, ten-minute-history, modal, and compact endpoint tests also pass.

## Limits

All validation is offline with explicit test fixtures. No live server calls, integrations, clipboard writes, or deployments were exercised. Arbitrarily long expanded logs remain scrollable; the chart uses a conservative height reserve to preserve room for the lower cards. No metrics aggregation or history semantics changed.

## Tall-terminal refinement

The complete View now receives balanced vertical margins when the terminal is at least 40 rows tall and all unscrolled content fits. An overflowing view retains its existing viewport and scrolling behavior. Actual model dimensions remain unchanged.

Red evidence: the new 300×90 centering test failed with `unbalanced vertical margins: top=0 bottom=0 height=34`. After implementation it passes, including visible endpoint actions and equal top/bottom margins. An additional regression check covers an oversized expanded log snapshot at scroll offsets zero and twenty, ensuring no top padding and a 40-row viewport. Final `rtk go test ./internal/dashboardui -race -count=1` passes (65 tests reported by RTK).
