# Download progress implementation plan

**Goal:** Implement the user-approved animated provisioning bar, honest download counters, provider detail and time since last change.

**Architecture:** Keep the existing Bubble Tea timer/cube. Add a segmented step bar and a separate measured model-transfer bar. Retrieve optional Vast daemon log snapshots while waiting for the instance; never interpret missing logs as failure or download completion as readiness.

**Constraints:** No paid instance creation, no recipe/timeout changes, no user-profile writes. Preserve the dirty worktree. Logs must be bounded, sanitized, cancellable and unable to leak the Vast token to storage hosts. Unknown totals stay indeterminate. Byte totals do not imply model readiness. Existing model hash verification remains mandatory.

## Tasks

- [x] Vast adapter: documented request_logs contract, strict HTTPS S3 path, bounded reads, async same-URL polling, no credential forwarding. Optional GetDaemonLogs(context.Context,int)(string,error), every 30 seconds with 3-second deadline; failures never fail readiness.
- [x] Model instrumentation: real embedded Python downloader exercised at mocked network boundary; SOVKIT_DOWNLOAD JSON counters at start, bounded intervals and EOF. Unknown/inconsistent Content-Length stays indeterminate. Hash verification unchanged.
- [x] TUI: step shimmer, measured transfer bar, unchanged-polling age, responsive sizing and frozen errors covered. Long log snapshots preserve newest counters, verification markers and provider detail.
- [x] Integration: 665 tests pass across 15 packages; 482 race-enabled tests pass across cli/setup/vast. CLI built and installed with install-macos.sh. Independent scoped review approved after two long-log truncation issues were fixed and regression-tested.

No real Vast/GPU test or paid instance creation was performed. Provider log availability during loading remains provider-dependent. Image pull percentage is intentionally not invented. Existing running remote downloaders need a new launch to emit the new structured counters. No existing Pi/OMP profile was changed and no image rebuild is needed for this launcher-side instrumentation.

## Approved visual behavior

Completed milestone segments are cyan; active segment shimmers; future segments are dim. The overall segmented bar represents milestones, not elapsed-time percentage. Model transfer displays actual received bytes and percentage only when a valid total exists. Image loading remains indeterminate unless reliable counters are available. Provider logs augment—not replace—the last observed provider status. Narrow terminals retain the most recent status and safety controls.
