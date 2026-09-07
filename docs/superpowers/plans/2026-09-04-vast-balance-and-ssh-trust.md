# Vast Balance and SSH Trust Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Display the authenticated Vast balance in every TUI header and automatically pin first-use SSH keys for Vast-created instances without an approval screen.

**Architecture:** Add a narrow account-balance method to the existing Vast HTTP client. A dedicated application balance component owns credential discovery, asynchronous reads, 60-second refreshes, stale-value retention, and header rendering. The Vast orchestration path replaces interactive host-key approval with first-use persistence and strict checkpoint comparison; manual routes keep requiring their existing verified `known_hosts` file.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, `net/http`, existing checkpoint/trust-store abstractions.

## Global Constraints

- Show `VAST  $12.34` right-aligned on every full-size screen.
- Fetch at startup and every 60 seconds without blocking input.
- Preserve the last successful value across transient failures; render no fallback amount.
- Never expose the API key or provider errors in the header.
- A saved Vast host-key digest mismatch is a hard failure.
- Manual SSH setup continues to require a readable, user-verified `known_hosts` file.

---

### Task 1: Vast account balance boundary

**Files:**
- Create: `internal/vast/account.go`
- Create: `internal/vast/account_test.go`

**Interfaces:**
- Consumes: existing `vast.Client` fields `baseURL`, `token`, and `http`.
- Produces: `func (client *Client) Balance(context.Context) (float64, error)`.

- [x] **Step 1: Write failing HTTP boundary tests**

Use an `httptest.Server` and a package-local `Client` to assert that `Balance` sends `GET /api/v0/users/current/` with `Authorization: Bearer test-token`, accepts both documented `balance` and legacy `credit`, rejects missing/non-finite/negative amounts, rejects an empty token, and returns a generic error for non-2xx responses.

```go
func TestBalanceReadsCurrentUser(t *testing.T) {
    // Server returns {"balance":42.125}; expect 42.125 and the exact auth contract.
}

func TestBalanceRejectsInvalidAmounts(t *testing.T) {
    // Table: {}, {"balance":-1}, {"balance":"NaN"}; every case must error.
}
```

- [x] **Step 2: Verify the new tests fail**

Run: `rtk go test ./internal/vast -run Balance -count=1`

Expected: build failure because `(*Client).Balance` does not exist.

- [x] **Step 3: Implement the narrow client method**

Decode a bounded JSON response from `/api/v0/users/current/` into pointer fields so missing and zero remain distinguishable:

```go
type accountResponse struct {
    Balance *float64 `json:"balance"`
    Credit  *float64 `json:"credit"`
}

func (client *Client) Balance(ctx context.Context) (float64, error)
```

Prefer `balance`, fall back to `credit`, and accept only `!math.IsNaN`, `!math.IsInf`, and `amount >= 0`. Reuse the client's authorization and bounded JSON conventions.

- [x] **Step 4: Run the Vast package tests**

Run: `rtk go test ./internal/vast -count=1`

Expected: all Vast tests pass.

---

### Task 2: Non-blocking global balance indicator

**Files:**
- Create: `internal/cli/application_balance.go`
- Create: `internal/cli/application_balance_test.go`
- Modify: `internal/cli/application.go`
- Modify: `internal/cli/application_view.go`
- Modify: `internal/cli/application_chrome_test.go`

**Interfaces:**
- Consumes: `vastAPIKey` sources without invoking a prompt, and `(*vast.Client).Balance` from Task 1.
- Produces: `ApplicationDependencies.Balance func(context.Context, string) (float64, error)`, balance messages, refresh scheduling, and `applicationHeader(title string) string`.

- [x] **Step 1: Write failing lifecycle and rendering tests**

Cover these observable behaviors with injected balance functions and real Bubble Tea messages:

```go
func TestBalanceAppearsRightAlignedOnEveryScreen(t *testing.T)
func TestBalanceRefreshKeepsLastSuccessfulValue(t *testing.T)
func TestBalanceIsHiddenWithoutCredentialOrSuccessfulRead(t *testing.T)
func TestBalanceHeaderDoesNotWrapAtNarrowWidths(t *testing.T)
```

Use literal expected values such as `VAST  $42.13`; verify a failed refresh leaves `$42.13` visible and a first-read failure leaves no dollar amount.

- [x] **Step 2: Verify the application tests fail**

Run: `rtk go test ./internal/cli -run 'Balance|ApplicationScreenRefresh' -count=1`

Expected: build or assertion failures because balance state and header composition are absent.

- [x] **Step 3: Implement credential discovery and polling**

Add `application_balance.go` with:

```go
const vastBalanceRefresh = 60 * time.Second
const vastBalanceTimeout = 3 * time.Second

type vastBalanceResult struct { amount float64; err error }
type vastBalanceTick struct{}

func (m *applicationModel) readAvailableVastToken() string
func (m *applicationModel) readVastBalance() tea.Cmd
func (m *applicationModel) waitVastBalance() tea.Cmd
func (m *applicationModel) applicationHeader(title string) string
```

`readAvailableVastToken` checks the captured `VAST_API_KEY`, then the owner-only credential store, but never calls `Prompter.VastAPIKey`. `readVastBalance` applies a three-second child context. Every result schedules one 60-second tick; successful values replace `m.vastBalance`, while errors preserve it.

- [x] **Step 4: Wire state and messages into the root model**

Add `Balance` to `ApplicationDependencies`, add `vastBalance *float64` and polling state to `applicationModel`, start the first read from `applicationBegin`, handle result/tick messages in `Update`, and replace the current header construction with `applicationHeader(title)`. Compose left title and right balance using visible ANSI widths; when they do not fit, truncate the title with an ellipsis and keep the balance on one line.

- [x] **Step 5: Run the CLI tests**

Run: `rtk go test ./internal/cli -count=1 -timeout=60s`

Expected: all CLI tests pass, including every screen-size preview.

---

### Task 3: Remove Vast host-fingerprint approval safely

**Files:**
- Modify: `internal/setup/orchestrator.go`
- Modify: `internal/setup/orchestrator_test.go`
- Modify: `internal/cli/application_prompts.go`
- Modify: `internal/cli/application.go`
- Modify: `internal/cli/application_view.go`
- Modify: `internal/cli/application_navigation.go`
- Modify: `internal/cli/prompts.go`
- Modify: `internal/cli/accessible.go`
- Modify: `internal/cli/prompts_test.go`
- Modify: `internal/cli/application_worker_test.go`
- Modify: `internal/cli/application_integration_test.go`
- Modify: `internal/cli/provisioning_loader_test.go`
- Modify: `internal/cli/accessible_test.go`

**Interfaces:**
- Consumes: `Checkpoint.trustMatches`, `hostKeysDigest`, `TrustStore.Save`, and complete `HostKeys` material.
- Produces: automatic first-use persistence and a hard `Vast host keys changed; refusing connection` error for pinned-key mismatch.

- [x] **Step 1: Replace confirmation-oriented tests with trust-invariant tests**

Add or update tests proving:

```go
func TestRunVastPinsFirstCompleteHostKeySetWithoutConfirmation(t *testing.T)
func TestResumeRejectsChangedPinnedHostKeys(t *testing.T)
func TestResumeAcceptsMatchingPinnedHostKeys(t *testing.T)
func TestRunVastRejectsIncompleteHostKeys(t *testing.T)
```

The first test must reach launch/save without delivering a confirmation. The mismatch test must assert that neither `TrustStore.Save` nor `ServerLauncher.Launch` runs. Keep the existing manual-route test that rejects an unreadable known-hosts file.

- [x] **Step 2: Verify trust tests fail for the expected prompt/mismatch behavior**

Run: `rtk go test ./internal/setup ./internal/cli -run 'HostKey|HostFingerprint|ManualRoute' -count=1`

Expected: first-use flow still requests confirmation or changed checkpoint keys can still be re-approved.

- [x] **Step 3: Implement automatic first-use pinning**

In `finishVast`, after validating non-empty raw keys and fingerprints:

```go
pinned := checkpoint != nil && checkpoint.HostKeysHash != ""
if pinned && !checkpoint.trustMatches(instance, knownHostsPath, keys.Raw) {
    return Result{}, paidInstanceError(instanceID, errors.New("Vast host keys changed; refusing connection"))
}
if !pinned {
    if err := deps.TrustStore.Save(knownHostsPath, keys.Raw); err != nil { /* wrap */ }
}
```

Persist the trusted host, port, file, and digest in the checkpoint exactly as today. Remove `ConfirmHostKeys` from `setup.Operator` and all implementations. Remove `promptHostKeys`, its editor branch, view copy, replay classification, and now-obsolete prompt tests. Keep the `ProgressHostKeys` stage and rename its completed label to `SSH host secured` so provisioning still gives feedback.

- [x] **Step 4: Run focused trust and UI tests**

Run: `rtk go test ./internal/setup ./internal/cli -count=1 -timeout=60s`

Expected: all tests pass and no screen waits for a Vast fingerprint decision.

---

### Task 4: Full verification and local installation

**Files:**
- Modify: `docs/superpowers/plans/2026-09-04-vast-balance-and-ssh-trust.md` to mark completed checkboxes and record results.

**Interfaces:**
- Consumes: Tasks 1-3.
- Produces: a tested locally installed CLI.

- [x] **Step 1: Run the full test suite**

Run: `rtk go test ./... -count=1 -timeout=90s`

Expected: all packages pass.

- [x] **Step 2: Run race tests on changed concurrency boundaries**

Run: `rtk go test -race ./internal/vast ./internal/setup ./internal/cli -count=1 -timeout=120s`

Expected: all packages pass without race reports.

- [x] **Step 3: Install locally**

Run: `rtk proxy sh install-macos.sh`

Expected: installation succeeds in the configured user binary directory and does not modify Pi/OpenCode profiles.

- [x] **Step 4: Record verification evidence**

Mark every completed checkbox and append the exact passing test counts. Record that no Vast instance was created, stopped, resumed, or destroyed during verification.

## Verification Evidence

- `rtk go test ./... -count=1 -timeout=90s`: 751 tests passed in 16 packages.
- `rtk go test -race ./internal/vast ./internal/setup ./internal/cli -count=1 -timeout=120s`: 552 tests passed in 3 packages, with no race report.
- `rtk git diff --check`: passed.
- `rtk proxy sh install-macos.sh`: installed successfully in `/Users/maximiliengilet/.local/bin`; existing Pi and OpenCode profiles were not changed.
- No Vast instance was created, stopped, resumed, or destroyed during verification.
