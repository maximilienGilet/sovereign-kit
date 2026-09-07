# Terminal polish implementation plan

> Execute the approved refinements in the existing feature checkout, preserving all existing changes. Do not commit unrelated work or mutate a live instance.

**Goal:** Complete the provisioning-to-dashboard flow and apply the user's visual feedback.

**Architecture:** Preserve the root-owned setup and connection workers. Presentation changes stay within the terminal renderers; metrics remain measured server data.

**Tech stack:** Go, Bubble Tea, Lip Gloss, Braille rasterization.

## Constraints

- No remote creation, destruction, restart, or personal profile writes during validation.
- Keep explicit destructive confirmation, captured instance identity, and cancellation.
- Preserve the ten-minute chart, missing-data semantics, keyboard navigation, and small terminal support.
- Crystal stays recognizable in each stage; only measured download bytes build it progressively.

## Task 1: Continuous connection and compact logs

- [x] Test a successful Vast setup transitioning into one owned connection without another Enter; failed setup must not connect. Keep manual route save explicit.
- [x] Replace the successful Vast `saved` transition with `beginConnection`, initialize the verification loader, and preserve cancellation/exit behavior.
- [x] Test compact logs with mixed and download-only snapshots; omit machine progress lines only in compact rendering, not raw logs.
- [x] Run CLI tests.

## Task 2: Focus + Inspector layout

- [x] Test a large dashboard bounding its content width, a taller chart, folded logs toggle, and retained endpoint actions at small sizes.
- [x] Bound dashboard content to 140 columns, center its rendered view (without changing model dimensions), use available height for the graph, and place session and endpoint cards side by side on wide screens.
- [x] Keep logs folded by default with a keyboard toggle, and show truthful waiting/idle labels without synthetic metrics.
- [x] Run dashboard tests and verify terminal geometry.

## Task 3: Crystal-based stage animations

- [x] Test that all indefinite stages retain a substantive Braille crystal silhouette, remain bounded, vary by stage, and freeze without color.
- [x] Replace standalone dots and SSH bridge with crystal-based lighting and restrained peripheral trails; retain measured bottom-up construction.
- [x] Increase core size/contrast, reduce cage dominance, replace diagonal hatching with spatial stippling and facet lighting.
- [x] Run animation tests and inspect rendered text frames. Browser policy blocked HTML capture review; no workaround attempted.

## Task 4: Review and evidence

- [x] Verify quit/destruction modal geometry and cancellation at multiple terminal sizes (existing modal/exit tests pass).
- [x] Record benchmark conditions and results in `docs/benchmarks/2026-09-06-qwen-context.md`; no unverified recipe-specific hardware claims.
- [x] Review the changed files, run full Go tests, targeted race tests and vet. Independent review found and verified fixes for manual-route auto-connect and stale-generation animation timers.
- [x] Build/install the CLI: `rtk proxy bash install-macos.sh` succeeded; existing profiles unchanged. Handoff notes distinguish automated verification from untested live provisioning.

## Verification evidence

- `rtk proxy go test ./... -count=1`: all 17 packages passed.
- `rtk proxy go test -race ./internal/cli ./internal/dashboardui ./internal/terminalui -count=1`: all three packages passed.
- `rtk proxy go vet ./...`: exit 0.
- `rtk git diff --check`: clean.
- Fixture-only ANSI captures generated under `/tmp/sovkit-polish-previews`.
- Actual rented-instance provisioning was not rerun. Recipe performance labels remain unchanged because the benchmark did not record the deployed recipe identity and runtime settings.
