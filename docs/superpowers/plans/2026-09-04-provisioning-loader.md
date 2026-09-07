# Provisioning Loader Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Animate real provisioning progress and allow confirmed, verified destruction after a session-owned instance fails.

**Architecture:** Keep the single Bubble Tea root. Setup publishes actual progress and an opaque recovery capability tied to the created instance; a separate lifecycle adapter verifies deletion. A bounded animation model renders the rings, active-label shimmer and compact spinners without driving operational state.

**Tech Stack:** Go, Bubble Tea, Bubbles spinner, Lip Gloss, existing HTTP client and injected clocks.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-09-04-provisioning-loader-design.md`, user approved and requested review plus implementation.
- No live creation or destruction, including instance 49798002 already destroyed by the user.
- Preserve the dirty feature checkout and existing edits; no commits or branch changes in this implementation.
- One TUI program, no fake percentage, no automatic destruction or rental retry.
- Tests first, deterministic external fakes, bounded cancellation-aware operations; never equate authentication/network failures with absence.
- Render at 30×10, 72×24, 108×30, 150×34 and large widths. Accessible mode is unanimated, EOF never confirms.
- All shell commands prefixed with `rtk`; file edits via apply_patch.

## Task 1: Safe lifecycle capability and transient waiting

**Files:** Create `internal/vast/destroy.go`, `internal/vast/destroy_test.go`, `internal/setup/recovery.go`, `internal/setup/recovery_test.go`; modify `internal/setup/orchestrator.go` and its tests. No UI changes in this task.

**Interfaces:**
```go
// setup; callback owns credentials, target and lifecycle verification.
type InstanceRecovery struct {
    InstanceID int
    Destroy func(context.Context) error
}
type RecoveryObserver interface { InstanceCreated(InstanceRecovery) }
type InstanceDestroyer interface {
    DestroyInstance(context.Context, int) error
    InstanceExists(context.Context, int) (bool, error)
}
```

`vast.Client` implements the last two methods. After positive CreateInstance success, RunVast publishes the recovery capability to an optional observer, only if the actual API supports InstanceDestroyer. Its callback targets that exact immutable ID, checks existence before DELETE, then verifies absence with bounded polling. It owns a separate execution lock to reject concurrent destruction attempts. Do not extend the required VastAPI interface or break existing fake providers.

- [x] Write failing adapter tests using httptest for exact method/path/auth, positive ID validation, accepted/rejected/malformed DELETE, positive/absent/malformed targeted GET and forbidden/network errors. Official DELETE contract: `DELETE /api/v0/instances/{id}` with bearer token, no body, `200` and `success:true` required. Verification: SDK uses `GET /api/v0/instances/{id}/?owner=me`, with `{"instances":null}` meaning absent, object with exact matching ID meaning present. Missing field, malformed JSON, wrong ID, generic 404 and auth/network errors never prove absence. Avoid listing pagination entirely. Sources: https://docs.vast.ai/api-reference/instances/destroy-instance and https://github.com/vast-ai/vast-cli/blob/master/vastai/api/instances.py .
```go
// A 403 response must not return (false, nil).
exists, err := client.InstanceExists(context.Background(), 987)
if err == nil || exists { t.Fatalf("forbidden lookup cannot establish absence: %v %v", exists, err) }
```
- [x] Run `rtk go test ./internal/vast -run 'Destroy|InstanceExists' -count=1`, verify expected missing-feature failures, implement the smallest strict decoder and run again.
- [x] Write failing recovery tests: exact ID, precheck absence avoids DELETE, accepted DELETE then still present is not success, disappearance succeeds, canceled/timeout/auth/malformed fails, ambiguous retry rechecks first, concurrent callback rejected. Use injected setup.Clock and existing polling options, requiring positive interval and timeout before side effects.
```go
// Callback registered on observer after creation; no automatic invocation.
if observer.recovery.InstanceID != 987 || observer.recovery.Destroy == nil { t.Fatal("missing exact-target recovery") }
```
- [x] Implement recovery publication immediately after successful creation, preserving it even when later polling fails. Wrap all errors without leaking credentials to UI; no response body echo.
- [x] Add orchestrator tests for blank/whitespace status → loading → running, persistent blank timeout, cancellation and existing offline errors. Run to see premature unusable-status failure, then permit blank only as bounded transient state.
- [x] Run focused setup/vast suites. Self-review and report RED/GREEN evidence. No commit; root captures task diff against pre-task snapshot.

## Task 2: Loader, shimmer, error recovery UI and accessible flow

**Files:** Create `internal/cli/provisioning_loader.go`, `internal/cli/provisioning_loader_test.go`, `internal/cli/application_recovery.go`, `internal/cli/application_recovery_test.go`, `internal/cli/accessible_recovery.go`, `internal/cli/accessible_recovery_test.go`. Modify application root/view/navigation/connection/prompter files, offer_browser model/view, setup.go, accessible.go, setup/progress.go and related tests. Keep rendering separated from operation lifecycles; add a focused integration test file if needed.

**Interfaces:** Consume Task 1's InstanceRecovery/RecoveryObserver. applicationPrompter preserves a recovery snapshot under applicationSession.mu and emits an event; root merges it at worker completion/cancellation like current progress. AccessiblePrompter records it synchronously for setupVast error handling. Credentials stay in the callback, not UI fields.

- [x] Write failing frame and timer tests. At distinct animation times the ring geometry/shading and active label change but display width remains fixed; completed steps do not shimmer; prompts/errors/stale generations do not schedule animation. Tests exercise root Update/View, not just helpers. Tiny frames retain operation/ID/warning/footer. Plain/no-color output remains legible without shimmer.
```go
// Animation may never manufacture success.
if strings.Contains(ansi.Strip(m.View()), "LIVE") { t.Fatal("waiting is not a verified route") }
```
- [x] Implement a focused reusable loader model with a single tagged tick chain. Use elapsed wall time, gentle concentric-ring pulse and a narrow cyan/white highlight traversing the active label with a pause; stage transition briefly radiates a wave. Only working/connecting/destroying screens animate. Bubbles spinner handles compact waiting; root generation plus loader epoch rejects stale ticks. Prompts/exit confirmations suspend and restart safely. No extra rendering goroutine.
- [x] Wire setup.Progress into actual stage tracking, emit named Hugging Face search/inspect progress, and animate offer searches only while loading. Stop compact spinner after selection, Close or request completion. Do not mark Save as ready; show stable check around verified connection success without delaying controls.
- [x] Write failing recovery integration tests for session ownership, default cancel, EOF, repeated enter, worker stopped before DELETE, canceled provisioning context independent from cleanup context, uncertain retry, error redaction, late result handling and shutdown. Exercise original error through confirmation and injected recovery callback to visible result.
```go
// Cancellation is the default action on the destruction confirmation.
m.Update(tea.KeyMsg{Type: tea.KeyEnter})
if calls != 0 { t.Fatal("default action destroyed an instance") }
```
- [x] Implement error action only when root has matching positive instanceID plus recovery capability. Show explicit confirmation with data loss warning. After confirmation cancel/join any session and local connection, start a dedicated bounded operation context and worker, disable duplicate dispatch. On verified success clear paid-warning state, invalidate session capability and prevent reuse of matching saved route in this session without deleting unrelated files. Keep original error plus recovery diagnostic on failures; preserve exact ID and warning. Exit during uncertain destruction cancels/joins local operation and prints uncertainty, not success.
- [x] Add accessible progress lines and post-error recovery confirmation/retry loop. Empty input/EOF decline; read errors never imply consent. Redact provider secrets in all printed diagnostics. Return an honest error for failed setup even if cleanup succeeded, without reprinting a stale active-billing claim as the final result.
- [x] Add terminal fixture with fake delayed progress/recovery. Check widths and controls, error/default cancel/destruction success, loading animation and terminal restoration. Never invoke real provider dependencies.
- [x] Run `rtk go test ./internal/cli ./internal/setup ./internal/vast -count=1`, self-review, report evidence and await independent review.

## Final validation

- [x] Review both tasks against spec and lifecycle safety, including parked findings.
- [x] Run `rtk go test ./... -count=1`.
- [x] Run `rtk go test -race ./internal/cli ./internal/setup ./internal/vast -count=1`.
- [x] Run `rtk go vet ./...` and `rtk git diff --check`.
- [x] Record a fake-provider PTY smoke check and accurate remaining limitations. Update spec/plan completion; keep all changes uncommitted.
