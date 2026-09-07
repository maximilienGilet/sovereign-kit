# Unified Terminal Application Implementation Plan

> **For agentic workers:** Use subagent-driven-development task-by-task with independent review. The user has approved implementation; do not pause between tasks.

**Goal:** One continuous interactive terminal application from setup to connected clients, with no duplicate recipe metrics or empty custom dashboard.

**Architecture:** The CLI composition layer owns a single root Bubble Tea model and embeds existing models and Huh forms. Existing sequential setup services run through a cancellable prompt/event bridge rather than nested terminal programs. Read-only prompt history may be replayed on Back before approval of mutation; the cost/identity mutation boundary locks replay. Extract connection establishment from Start so the root can own and supervise its tunnel.

**Tech Stack:** Existing Go, Bubble Tea, Bubbles, Huh, Lip Gloss, Charm ANSI/term; no new dependencies.

## Global Constraints

- Spec: docs/superpowers/specs/2026-09-03-unified-terminal-application-design.md, including review correction: obtain configuration replacement approval before any paid creation.
- One active recipe, no comparison or invented performance. Preserve per-GPU VRAM and per-instance disk qualifiers.
- One terminal owner; child screens return decisions rather than quitting the root.
- Keep loopback/fail-closed route and explicit cost, identity mutation and host-key confirmations.
- No automatic retry of paid creation. Retain ambiguous/known paid state and warn before exit. Do not destroy remote resources.
- Mask secrets; no secret logs. Do not run live provider operations in tests or verification.
- Work in the existing feat/recipe-picker-tui checkout selected earlier. Preserve all pre-existing edits. Do not commit code that overlaps pre-existing work; task reports/diff snapshots replace commit ranges for this dirty worktree.
- Prefix shell commands with rtk. Use apply_patch for edits. Follow TDD and record red/green evidence.

## File ownership

- Task 1: internal/catalogui/cockpit.go, picker.go, model.go, related tests and embedded.go.
- Task 2: internal/setup/orchestrator.go and progress.go/tests; internal/cli/start.go and connection tests.
- Task 3: new internal/cli/application*.go files and their tests, plus narrow rental_review.go embedded adapter. Reuse private form builders in prompts.go without duplicating business validation.
- Task 4: cmd/sovkit/main.go and tests, internal/cli/prompts.go/accessibility helpers, README usage documentation, integration tests. Narrow integration fixes to prior files require coordination.

### Task 1: Unduplicated cockpit and embeddable recipe screen

**Interfaces:** Existing standalone NewPicker remains unchanged. Add:

```go
func NewEmbeddedPicker(entries []Entry) Model
type PickerResultMsg struct { Value string; Cancelled bool }
```

Embedded mode sends PickerResultMsg on choose/cancel rather than tea.Quit; transient i/esc handling and progress commands remain local. Standalone picker tests still pass. Root can render the entire picker view without adding a second header/footer.

- [x] Write failing tests counting the configured context numerator, output value and concurrency total in dashboard body (not sidebar/evidence), and tests asserting custom views have no UNKNOWN/slots/metrics and retain action at 30x10 through 320x80.
- [x] Add failing embedded choose/cancel tests: execute the returned command and assert PickerResultMsg, never tea.QuitMsg; opening/closing i must not complete the picker.
- [x] Run `rtk go test ./internal/catalogui -count=1` and record red output.
- [x] Remove heroFigures and move output to capacityZone; retain one configured slot total and unmeasured throughput. Add responsive custom ASCII welcome before minimal/compact/wide metric rendering. Use plain ASCII art colored yellow; preserve full textual label/action. Update obsolete hero composition assertions to the new single-placement behavior.
- [x] Implement embedded result adapter; reuse selected/cancelled fields and existing navigation, no new framework.
- [x] Run catalog tests green; inspect renders at 72x24,134x30,150x34. Save report with changed files and red/green output. No code commit.

### Task 2: Observable setup mutations and reusable connection lifecycle

**Interfaces produced:**

```go
// internal/setup/progress.go
type Progress struct { Stage string; InstanceID int }
type ProgressObserver interface { SetupProgress(Progress) }
// Stage constants: ProgressSearching, ProgressCreating, ProgressCreated,
// ProgressWaiting, ProgressHostKeys, ProgressLaunching, ProgressSaving.
// ProgressCreating emitted immediately before CreateInstance; Created after ID.

// internal/cli/start.go
func Connect(ctx context.Context, output io.Writer, configPath string, deps StartDependencies) (Tunnel, error)
```

Progress contains no secrets, raw host keys or endpoints. RunVast optionally notifies an operator implementing ProgressObserver. Existing Operator signatures stay intact. Notification does not bypass confirmation or validation. Errors after creation keep existing warning semantics. Connect performs current validation, tunnel start and bounded health loop; ownership transfers only on success. All failure exits stop the started tunnel. Start uses Connect and keeps its public behavior and dashboard callback.

- [x] Add red tests observing ordered progress around a fake API: declined cost emits no Creating; creation error after send emits Creating but not Created; success emits Created ID before post-create failures. Existing setup tests cover exact command/validation requirements.
- [x] Add red Connect tests: healthy returns running owned tunnel; timeout/cancel/tunnel exit stop it; no dashboard launch in Connect. Preserve Start tests.
- [x] Run focused tests red.
- [x] Extract existing connection loop rather than copy it, using the ownership pattern:

```go
owned := false
defer func() { if !owned { _ = tunnel.Stop() } }()
// After the existing successful health and tunnel-exit checks:
owned = true
return tunnel, nil
```

- [x] Emit progress from RunVast at real operation boundaries only. Keep synchronous existing operator decisions and provider semantics.
- [x] Run `rtk go test ./internal/setup ./internal/cli -count=1`, record green and self-review. No code commit.

### Task 3: Root application, prompt bridge and guarded navigation

**Interfaces produced:**

```go
type ApplicationDependencies struct {
    Setup SetupDependencies
    Start StartDependencies
    LaunchClient dashboardui.Launcher // Optional; production uses raw tea.ExecProcess.
}
func RunApplication(ctx context.Context, input io.Reader, output io.Writer,
    configPath, defaultUser, entry string, deps ApplicationDependencies) error
```

Entry is "home", "setup" or "start". RunApplication is interactive only; task 4 handles selection/fallback. New files: application.go (root state/update/runner), application_view.go (frame), application_prompts.go (SetupPrompter/WorkloadPrompter/Operator/IdentityOperator bridge), application_navigation.go (history/back/confirmations), application_connection.go (Connect and dashboard lifecycle), application_test.go plus focused bridge tests. Keep each responsibility separate.

The bridge runs existing Setup with a supplied prompter in a worker context. Each prompt carries a typed kind, immutable data, response channel and session ID. Root builds/runs embedded Huh fields, never Form.Run. Keep only root-thread-owned form pointers. A worker command sends requests/status and receives decisions with cancellation. Root awaits the next message via tea.Cmd; no unbounded busy loop or goroutines reading terminal input.

For Back, cancel/wait the previous read-only run before replaying accepted answers up to the target prompt. Drop dependent answers after the edited step. Replay only deterministic prior prompt answers; provider searches/inspection may rerun with fresh results. Cache selections by stable value and validate against new options. Require fresh cost and fingerprint confirmation. After accepting cost or identity mutation, disable replay/back into the workflow; after ProgressCreating retain paid-risk state even if canceled. This conservative boundary preserves the spec's safe-back qualification.

Before starting any replacement setup, check the existing configuration path and require an explicit replacement confirmation; declining returns home without starting Setup. Pass io.Discard or a bounded redacting event writer to the background Setup, never the real terminal. Surface sanitized errors and typed progress; no API key in views/errors. Error state offers safe Back/retry only before mutation; after mutation or ambiguous creation, show warning/ID and exit guidance, not restart.

Connection uses Connect in a session context. Root owns successful Tunnel, subscribes to Done, stops it on disconnect/exit, cancels/waits in-flight work on cleanup. Healthy dashboard uses the existing dashboard model and external client launcher; adapt its completion/exit behavior as necessary without nested programs. Root checks route health on client return before allowing another launch; stale results cannot mark a later session healthy. During client execution keep the tunnel alive. Never claim health solely from configuration.

- [x] Write red root tests for entry states, refusal of replacement before RunVast, embedded recipe completion remaining inside root, Back preserving values/invalidation, and edit fields accepting literal q.
- [x] Write red worker tests for cancel/unblock, late request/result rejection, and paid-state retention after ProgressCreating/Created. Simulated callbacks must count real orchestration calls, not test-only state setters.
- [x] Implement root/frame with consistent title/step/footer; use full recipe/rental views without duplicate chrome. Huh forms are embedded tea.Models and resized with the available body area. Use viewport for lengthy confirmations; controls visible at minimum30x10, resize notice below minimum without hidden submissions.
- [x] Implement provider/manual/Vast/HF/offer/cost/identity/host-key prompts using existing form builders and validation. Add rental review result adapter if needed; standalone review behavior remains intact.
- [x] Implement connection events and client handoff, then tests for tunnel failure/disconnect/exit and post-client health recheck.
- [x] Run focused tests after each red/green increment, then `rtk go test ./internal/cli ./internal/catalogui ./internal/dashboardui -count=1`. Report concrete transition coverage and remaining concerns. No code commit.

### Task 4: CLI wiring, accessible fallback and end-to-end verification

**Interfaces consumed:** RunApplication and ApplicationDependencies, existing legacy Setup/Start. Extend private cmd/sovkit application injectable callbacks with a terminal-app runner and interactive detector so tests need no real TTY.

```go
// Detector must check BOTH streams and accessible environment settings.
func UseTerminalApplication(input io.Reader, output io.Writer, getenv func(string) string) bool
```

- [x] Write red routing tests: interactive bare command→home, setup→setup, start→start; explicit help never starts UI; noninteractive/ACCESSIBLE/TERM=dumb bare command→help; original injected setup/start paths remain testable.
- [x] Wire production setup dependencies once and share them between legacy Setup and RunApplication. Do not create divergent provider configurations. For explicit start outside interactive mode keep textual connection status and lifetime until cancellation rather than opening dashboard with os.Stdin; require explicit client commands in that mode. Non-TTY must never create a fullscreen picker/review. Installed Huh accessible input reprints password values and ignores injected streams: use a small line-oriented prompter for this fallback, sharing existing validation and labels; use existing Charm term.ReadPassword for terminal secrets, never echo secrets from pipes. Keep Huh embedded for rich mode. Test injected input/output, EOF and secret non-disclosure.
- [x] Add integration tests driving real root + fake setup/connection services through manual and Vast/custom paths. Confirm replacement refusal calls neither create nor save, repeated submit calls create once, and exiting after creation never destroys remote resources. Verify one terminal runner across transitions.
- [x] Update usage documentation for bare sovkit, direct commands, accessible mode and exit/billing semantics.
- [x] Run full Go tests/vet, race tests for the new bridge/lifecycle, contributor checks, and inspect text/PTTY views without initiating any live provider operation. Preserve unrelated modifications; report working-tree changes rather than committing overlapping files.
- [x] Independent full-scope review against the spec, fix important findings, rerun affected tests and final suite before delivery.
