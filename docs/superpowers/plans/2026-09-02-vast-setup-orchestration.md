# Vast Setup Orchestration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a provider-aware `sovkit setup` that securely provisions the built-in Qwen recipe on Vast, plus a foreground `sovkit start` that owns the strict tunnel and dashboard lifecycle.

**Architecture:** Keep manual SSH setup and Vast provisioning as separate concrete workflows behind one Huh provider selector. A focused Vast orchestrator coordinates injected provider, operator, clock, host-key, remote-launch, and persistence interfaces; production adapters are the only code allowed to execute SSH tools or touch credentials. `start` composes the existing strict route with bounded health polling and dashboard lifetime without detached state.

**Tech Stack:** Go 1.22, Vast REST API, Charm Huh v0.6.0, Bubble Tea v1.1.0, TOML, OpenSSH command-line tools.

## Global Constraints

- Never persist `VAST_API_KEY`, Hugging Face credentials, private-key contents, or provider secrets.
- Never fall back to a public inference provider or a non-loopback endpoint.
- Vast REST search uses `type: "ondemand"`; `gpu_ram` is MB on the wire and GB inside Sovereign Kit.
- The inference service and tunnel bind only `127.0.0.1:30000`.
- Instance creation is impossible before explicit offer-cost confirmation.
- Remote execution is impossible before explicit host-fingerprint confirmation and isolated known-hosts persistence.
- Never silently replace a changed host key.
- Never accept user-provided Docker, shell, image, model, port, or SGLang flags.
- Post-create failures report the instance ID and that billing may remain active; never auto-destroy the instance.
- Performance remains `unknown/unmeasured`; health is not throughput.
- Tests never call live Vast, SSH, Docker, a network model endpoint, or a live TTY.
- Do not delete `docs/`, `profiles/`, or legacy scripts/tests in this change.
- Do not add code comments; the `//go:embed` compiler directive is the only required comment-like line.

---

### Task 1: Correct the Vast wire contract

**Files:**
- Modify: `internal/vast/search.go`
- Modify: `internal/vast/search_test.go`
- Modify: `internal/vast/client.go`
- Modify: `internal/vast/client_test.go`

**Interfaces:**
- Consumes: `SearchRequest{Limit int, MinimumVRAMGB int}` and existing `Client`.
- Produces: `Offer.GPUVRAMGB` normalized to GB and `CreateRequest{Image string, DiskGB int, Label string}`; `CreateInstance` always emits `runtype: "ssh_direct"` and no `onstart`.

- [ ] **Step 1: Replace the search test with a wire-unit contract**

Use a response containing `gpu_ram: 98304` and assert both the request and normalized result:

```go
func TestSearchOffersUsesVastWireUnitsAndNormalizesVRAM(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        body, _ := io.ReadAll(r.Body)
        for _, expected := range []string{
            `"limit":5`,
            `"type":"ondemand"`,
            `"verified":{"eq":true}`,
            `"rentable":{"eq":true}`,
            `"rented":{"eq":false}`,
            `"gpu_ram":{"gte":98304}`,
        } {
            if !strings.Contains(string(body), expected) {
                t.Fatalf("request body missing %s: %s", expected, body)
            }
        }
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write([]byte(`{"offers":[{"id":42,"gpu_name":"RTX PRO 6000","gpu_ram":98304,"dph_total":1.25,"geolocation":"FR","reliability":0.99}]}`))
    }))
    defer server.Close()

    offers, err := NewClient(server.URL, "test-token").SearchOffers(context.Background(), SearchRequest{Limit: 5, MinimumVRAMGB: 96})
    if err != nil {
        t.Fatal(err)
    }
    if len(offers) != 1 || offers[0].GPUVRAMGB != 96 {
        t.Fatalf("offers = %#v", offers)
    }
}
```

Add a table case for a non-integral response such as `24576 MB == 24 GB` so conversion is not integer-truncated.

- [ ] **Step 2: Run the search tests and confirm the current contract fails**

Run: `go test ./internal/vast -run 'TestSearchOffersUsesVastWireUnitsAndNormalizesVRAM' -count=1`

Expected: FAIL because the request contains `on-demand`, sends `96`, and exposes `98304` as GB.

- [ ] **Step 3: Normalize request and response units in `SearchOffers`**

Introduce a wire-only offer type and convert at the adapter boundary:

```go
type offerResponse struct {
    ID          int     `json:"id"`
    GPUName     string  `json:"gpu_name"`
    GPURAMMB    float64 `json:"gpu_ram"`
    HourlyUSD   float64 `json:"dph_total"`
    Location    string  `json:"geolocation"`
    Reliability float64 `json:"reliability"`
}

func (offer offerResponse) normalized() Offer {
    return Offer{
        ID: offer.ID,
        GPUName: offer.GPUName,
        GPUVRAMGB: offer.GPURAMMB / 1024,
        HourlyUSD: offer.HourlyUSD,
        Location: offer.Location,
        Reliability: offer.Reliability,
    }
}
```

Set `type` to `ondemand`, set the request threshold to `request.MinimumVRAMGB * 1024`, decode both array and single-object Vast response shapes into `offerResponse`, then normalize every result.

- [ ] **Step 4: Tighten the create-instance request test**

Construct only controlled public fields and assert the wire payload:

```go
instanceID, err := client.CreateInstance(context.Background(), 42, CreateRequest{
    Image: "lmsysorg/sglang@sha256:e21dd539b36ea7842101393ec3fe3b0d453626cd8251e4d4db33af0cd97d7f0b",
    DiskGB: 120,
    Label: "sovkit-qwen-studio",
})
```

Required assertions:

```go
for _, expected := range []string{
    `"disk":120`,
    `"runtype":"ssh_direct"`,
    `"label":"sovkit-qwen-studio"`,
} {
    if !strings.Contains(string(body), expected) {
        t.Fatalf("request body missing %s: %s", expected, body)
    }
}
if strings.Contains(string(body), "onstart") {
    t.Fatalf("create request must not start code before host trust: %s", body)
}
```

- [ ] **Step 5: Run the create test and confirm it fails**

Run: `go test ./internal/vast -run 'TestCreateInstance' -count=1`

Expected: FAIL because the current API accepts caller-controlled `Runtype`, `DirectSSH`, and `Onstart`.

- [ ] **Step 6: Make SSH-direct mode an adapter invariant**

Use these public and private request types:

```go
type CreateRequest struct {
    Image  string
    DiskGB int
    Label  string
}

type createPayload struct {
    Image   string `json:"image"`
    DiskGB  int    `json:"disk"`
    Runtype string `json:"runtype"`
    Label   string `json:"label,omitempty"`
}
```

Marshal `createPayload{Image: request.Image, DiskGB: request.DiskGB, Runtype: "ssh_direct", Label: request.Label}`. Reject blank image, non-positive disk, and non-digest image references before HTTP I/O.

- [ ] **Step 7: Verify and commit the adapter correction**

Run: `go test ./internal/vast -count=1`

Expected: PASS.

```bash
git add internal/vast/search.go internal/vast/search_test.go internal/vast/client.go internal/vast/client_test.go
git commit -m "fix: correct Vast offer and SSH contracts"
```

---

### Task 2: Bundle the typed launch recipe and Vast config

**Files:**
- Modify: `internal/recipe/recipe.go`
- Modify: `internal/recipe/recipe_test.go`
- Modify: `recipes/qwen-studio.toml`
- Create: `recipes/builtin.go`
- Create: `recipes/builtin_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Interfaces:**
- Consumes: existing recipe TOML and `config.Studio`.
- Produces: `recipe.Parse([]byte)`, `recipe.Requirements.MinimumDiskGB`, `recipes.QwenStudio() (recipe.Recipe, error)`, and `config.VastStudio(instanceID int, host string, port int, identityFile string, knownHostsFile string) Config`.

- [ ] **Step 1: Add failing recipe tests**

Extend the existing recipe test to require the disk floor:

```go
if r.Requirements.MinimumVRAMGB != 96 || r.Requirements.MinimumDiskGB != 120 {
    t.Fatalf("unexpected requirements: %#v", r.Requirements)
}
```

Add `recipes/builtin_test.go`:

```go
package recipes

import "testing"

func TestQwenStudioIsBundledAndValid(t *testing.T) {
    r, err := QwenStudio()
    if err != nil {
        t.Fatal(err)
    }
    if r.ID != "qwen-studio" || r.Runtime.Engine != "sglang" || r.Requirements.MinimumDiskGB != 120 {
        t.Fatalf("unexpected recipe: %#v", r)
    }
}
```

- [ ] **Step 2: Run recipe tests and confirm failure**

Run: `go test ./internal/recipe ./recipes -count=1`

Expected: FAIL because `MinimumDiskGB` and package `recipes` do not exist.

- [ ] **Step 3: Extract byte parsing and embed the existing TOML**

In `internal/recipe/recipe.go`, make file loading delegate to:

```go
func Parse(contents []byte) (Recipe, error) {
    var value Recipe
    if err := toml.Unmarshal(contents, &value); err != nil {
        return Recipe{}, fmt.Errorf("parse recipe: %w", err)
    }
    if err := value.Validate(); err != nil {
        return Recipe{}, err
    }
    return value, nil
}
```

Add `MinimumDiskGB int \`toml:"minimum_disk_gb"\`` to `Requirements`, reject negative values in `Validate`, and set `minimum_disk_gb = 120` in `recipes/qwen-studio.toml`.

Add `recipes/builtin.go`:

```go
package recipes

import (
    _ "embed"

    "github.com/maximilienGilet/sovereign-kit/internal/recipe"
)

//go:embed qwen-studio.toml
var qwenStudio []byte

func QwenStudio() (recipe.Recipe, error) {
    return recipe.Parse(qwenStudio)
}
```

- [ ] **Step 4: Add failing Vast config invariants**

Add tests that `VastStudio` sets provider kind and ID, and that validation rejects `vast` with ID zero and `manual` with a non-zero ID:

```go
func TestVastStudioRecordsInstance(t *testing.T) {
    cfg := VastStudio(987, "gpu.example", 22022, "/tmp/id", "/tmp/known_hosts")
    if cfg.Provider.Kind != "vast" || cfg.Provider.InstanceID != 987 || cfg.SSH.User != "root" {
        t.Fatalf("unexpected config: %#v", cfg)
    }
}
```

- [ ] **Step 5: Run config tests and confirm failure**

Run: `go test ./internal/config -count=1`

Expected: FAIL because `VastStudio` does not exist and provider-specific ID invariants are absent.

- [ ] **Step 6: Implement the Vast constructor and invariants**

Build on the existing secure route constructor:

```go
func VastStudio(instanceID int, host string, port int, identityFile, knownHostsFile string) Config {
    cfg := Studio(host, port, "root", identityFile, knownHostsFile)
    cfg.Provider = Provider{Kind: "vast", InstanceID: instanceID}
    return cfg
}
```

In `Validate`, require manual ID zero and Vast ID positive. Keep fixed route addresses and ports unchanged.

- [ ] **Step 7: Verify and commit the recipe/config contract**

Run: `go test ./internal/recipe ./recipes ./internal/config -count=1`

Expected: PASS.

```bash
git add internal/recipe/recipe.go internal/recipe/recipe_test.go recipes/qwen-studio.toml recipes/builtin.go recipes/builtin_test.go internal/config/config.go internal/config/config_test.go
git commit -m "feat: bundle the Vast launch recipe"
```

---

### Task 3: Build the test-first Vast setup orchestrator

**Files:**
- Create: `internal/setup/orchestrator.go`
- Create: `internal/setup/orchestrator_test.go`

**Interfaces:**
- Consumes: `vast.Client` behavior, validated `recipe.Recipe`, `config.VastStudio`.
- Produces:

```go
type VastAPI interface {
    SearchOffers(context.Context, vast.SearchRequest) ([]vast.Offer, error)
    CreateInstance(context.Context, int, vast.CreateRequest) (int, error)
    GetInstance(context.Context, int) (vast.Instance, error)
}

type Operator interface {
    SelectOffer(context.Context, []OfferView) (vast.Offer, error)
    ConfirmCost(context.Context, OfferView, int) (bool, error)
    ConfirmHostKeys(context.Context, []string) (bool, error)
}

type HostKeys struct {
    Raw          []byte
    Fingerprints []string
}

type HostKeyScanner interface {
    Scan(context.Context, string, int) (HostKeys, error)
}

type TrustStore interface {
    Save(string, []byte) error
}

type ServerLauncher interface {
    Launch(context.Context, config.SSH, recipe.Recipe) error
}

type Clock interface {
    Now() time.Time
    Sleep(context.Context, time.Duration) error
}

type Dependencies struct {
    NewAPI           func(string) VastAPI
    Operator         Operator
    HostKeyScanner   HostKeyScanner
    TrustStore       TrustStore
    ServerLauncher   ServerLauncher
    Clock            Clock
    SaveConfig       func(string, config.Config) error
    ValidateIdentity func(string) error
}

type Options struct {
    ConfigPath      string
    IdentityFile    string
    KnownHostsDir   string
    OfferLimit      int
    PollInterval    time.Duration
    PollTimeout     time.Duration
}

type OfferView struct {
    Offer      vast.Offer
    MonthlyUSD float64
    AnnualUSD  float64
}

type Result struct {
    InstanceID int
    ConfigPath string
}

func RunVast(context.Context, string, recipe.Recipe, Options, Dependencies) (Result, error)
```

- [ ] **Step 1: Write the pre-create behavior tests**

Use local fakes recording every call. Cover:

```go
func TestRunVastRejectsMissingAPIKey(t *testing.T)
func TestRunVastRejectsUnreadableIdentityBeforeSearching(t *testing.T)
func TestRunVastRejectsNoEligibleOffers(t *testing.T)
func TestRunVastSortsOffersAndBuildsCostScenarios(t *testing.T)
func TestRunVastDoesNotCreateWhenCostIsDeclined(t *testing.T)
```

For a `$1.25/h` offer, assert `MonthlyUSD == 912.50` and `AnnualUSD == 10950.00`. Supply one 24 GB offer from the fake API and assert it is omitted defensively for the 96 GB recipe even though the provider should already filter it. In the decline test assert `createCalls == 0`, `scanCalls == 0`, `launchCalls == 0`, and `saveCalls == 0`.

- [ ] **Step 2: Run the pre-create tests and confirm failure**

Run: `go test ./internal/setup -run 'TestRunVast(Rejects|Sorts|DoesNotCreate)' -count=1`

Expected: FAIL because the setup package does not exist.

- [ ] **Step 3: Implement validation, shortlist, and confirmation**

Use `strings.TrimSpace` for the token, call `recipe.Validate`, validate the identity before constructing the API, request `OfferLimit` with the recipe VRAM floor, defensively remove insufficient offers, and stable-sort by hourly cost then offer ID.

Build scenarios exactly as:

```go
func viewFor(offer vast.Offer) OfferView {
    return OfferView{
        Offer: offer,
        MonthlyUSD: offer.HourlyUSD * 730,
        AnnualUSD: offer.HourlyUSD * 8760,
    }
}
```

Return a cancellation error without creating when `ConfirmCost` returns false.

- [ ] **Step 4: Write failing paid-instance lifecycle tests**

Cover:

```go
func TestRunVastPollsUntilRunningSSHDetailsExist(t *testing.T)
func TestRunVastStopsOnTerminalInstanceStatus(t *testing.T)
func TestRunVastTimesOutWithInstanceAndBillingWarning(t *testing.T)
func TestRunVastDoesNothingTrustedWhenHostKeysAreDeclined(t *testing.T)
func TestRunVastLaunchesOnlyAfterTrustPersistence(t *testing.T)
func TestRunVastSavesVastConfigurationAfterLaunch(t *testing.T)
```

Use an event slice and assert successful order exactly:

```go
want := []string{
    "create",
    "get:loading",
    "sleep",
    "get:running",
    "scan",
    "confirm-host-keys",
    "save-known-hosts",
    "launch-server",
    "save-config",
}
```

For every error after `create`, assert the message contains both `instance 987` and `billing may still be active`. Host-key decline must leave trust, launch, and config call counts at zero.

- [ ] **Step 5: Run lifecycle tests and confirm failure**

Run: `go test ./internal/setup -run 'TestRunVast(Polls|Stops|TimesOut|DoesNothing|Launches|Saves)' -count=1`

Expected: FAIL because polling and the trusted post-create sequence are absent.

- [ ] **Step 6: Implement bounded polling and the trust boundary**

Compute `deadline := deps.Clock.Now().Add(options.PollTimeout)`. Call `GetInstance` immediately, accept only `running` plus host and valid port, fail immediately on `exited`, `unknown`, or `offline`, and call `Clock.Sleep` between attempts. Check the deadline before each sleep and honor context cancellation.

Derive the isolated path with no host-controlled path data:

```go
knownHostsPath := filepath.Join(options.KnownHostsDir, fmt.Sprintf("vast-%d_known_hosts", instanceID))
```

After polling: scan, require at least one raw key and fingerprint, confirm, save trust, launch with `config.SSH{Host: instance.SSHHost, Port: instance.SSHPort, User: "root", IdentityFile: options.IdentityFile, KnownHostsFile: knownHostsPath}`, then save `config.VastStudio`. Wrap every error from polling onward:

```go
func paidInstanceError(instanceID int, err error) error {
    return fmt.Errorf("Vast instance %d was created; billing may still be active: %w", instanceID, err)
}
```

- [ ] **Step 7: Verify and commit the orchestration state machine**

Run: `go test ./internal/setup -count=1`

Expected: PASS.

```bash
git add internal/setup/orchestrator.go internal/setup/orchestrator_test.go
git commit -m "feat: orchestrate confirmed Vast setup"
```

---

### Task 4: Implement strict host-key and remote-launch adapters

**Files:**
- Create: `internal/setup/system.go`
- Create: `internal/setup/system_test.go`

**Interfaces:**
- Consumes: `HostKeyScanner`, `TrustStore`, and `ServerLauncher` from Task 3.
- Produces: `ExecRunner`, `SystemHostKeyScanner`, `FileTrustStore`, `StrictSSHLauncher`, `RealClock`, `ValidateIdentityFile`, and pure controlled-command construction.

```go
type Command struct {
    Name  string
    Args  []string
    Stdin []byte
}

type CommandRunner interface {
    Output(context.Context, Command) ([]byte, error)
    Run(context.Context, Command) error
}
```

- [ ] **Step 1: Write failing host-key scanner and trust-store tests**

Use a fake runner. Assert scan command shape:

```go
Command{Name: "ssh-keyscan", Args: []string{"-T", "10", "-p", "22022", "gpu.example"}}
```

Return raw key bytes, assert the next command is `ssh-keygen -lf - -E sha256` with those bytes as stdin, and return two fingerprint lines. Add tests rejecting blank scan output and blank fingerprint output. Add `ValidateIdentityFile` tests for a readable regular file, a missing path, and a directory.

For `FileTrustStore`, assert:

- new files are `0600` and parent directories are `0700`;
- identical existing bytes succeed and remain `0600`;
- different existing bytes return an error containing `changed` and leave original bytes untouched.

- [ ] **Step 2: Run host-key tests and confirm failure**

Run: `go test ./internal/setup -run 'Test(SystemHostKeyScanner|FileTrustStore)' -count=1`

Expected: FAIL because production adapters do not exist.

- [ ] **Step 3: Implement command execution and atomic trust persistence**

`ExecRunner.Output` uses `exec.CommandContext`, sets stdin when present, captures stderr, and includes sanitized command failure context without key contents. `ExecRunner.Run` streams neither secrets nor command output into the config.

`FileTrustStore.Save` reads an existing file before writing. On create, use a temporary file in the target directory, `Chmod(0600)`, write, close, rename, and final `Chmod(0600)`. Never overwrite different existing contents. `ValidateIdentityFile` accepts only readable regular files. `RealClock.Now` delegates to `time.Now`; `RealClock.Sleep` waits on a timer or returns the context error.

- [ ] **Step 4: Write failing controlled server-command tests**

Load `recipes.QwenStudio()` and assert `StrictSSHLauncher` emits strict SSH options and a remote command containing all reviewed values:

```go
for _, expected := range []string{
    "BatchMode=yes",
    "IdentitiesOnly=yes",
    "StrictHostKeyChecking=yes",
    "UserKnownHostsFile=/tmp/vast-987_known_hosts",
    "sglang serve",
    "'--model-path' 'RadixArk/Qwen3.8-27B-NVFP4'",
    "'--revision' '319f741cce68d7914884900c138a1fbb70a42f30'",
    "'--context-length' '262144'",
    "'--max-running-requests' '5'",
    "'--host' '127.0.0.1'",
    "'--port' '30000'",
} {
    if !strings.Contains(joined, expected) {
        t.Fatalf("command missing %q: %s", expected, joined)
    }
}
```

Assert the command contains fixed `nohup`, fixed `/workspace/sovkit-sglang.log`, redirected stdin/stdout/stderr, and no recipe image passed to a nested Docker invocation. Add a malicious custom recipe value such as `owner/model'; touch /tmp/pwned; echo '` and assert the shell encoder keeps it inside one safely quoted argument.

- [ ] **Step 5: Run remote-launch tests and confirm failure**

Run: `go test ./internal/setup -run 'TestStrictSSHLauncher' -count=1`

Expected: FAIL because the launcher does not exist.

- [ ] **Step 6: Implement safe reviewed argument generation**

Generate an argument slice from validated recipe fields, then shell-quote every argument with a single-quote encoder that converts `'` to `'"'"'`. The reviewed SGLang arguments are:

```go
[]string{
    "sglang", "serve",
    "--trust-remote-code",
    "--model-path", r.Model.Repository,
    "--revision", r.Model.Revision,
    "--context-length", strconv.Itoa(r.Serve.ContextWindow),
    "--kv-cache-dtype", "fp8_e4m3",
    "--mem-fraction-static", "0.85",
    "--attention-backend", "flashinfer",
    "--chunked-prefill-size", "2048",
    "--max-running-requests", strconv.Itoa(r.Serve.MaxRunningRequests),
    "--cuda-graph-max-bs", strconv.Itoa(r.Serve.MaxRunningRequests),
    "--reasoning-parser", "qwen3",
    "--tool-call-parser", "qwen3_coder",
    "--host", "127.0.0.1",
    "--port", "30000",
}
```

Reject non-SGLang recipes in this launcher. The remote command is `nohup <quoted args> >/workspace/sovkit-sglang.log 2>&1 </dev/null &`. Build SSH arguments with the confirmed identity and known-hosts paths, then `-p <port> -- root@host <remote command>`; run through the injected `CommandRunner`.

- [ ] **Step 7: Verify and commit secure system adapters**

Run: `go test ./internal/setup -count=1`

Expected: PASS without invoking real SSH.

```bash
git add internal/setup/system.go internal/setup/system_test.go
git commit -m "feat: add trusted SSH server launch"
```

---

### Task 5: Add Huh provider and confirmation UI

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Replace: `internal/cli/setup.go`
- Replace: `internal/cli/setup_test.go`
- Create: `internal/cli/prompts.go`
- Create: `internal/cli/prompts_test.go`

**Interfaces:**
- Consumes: `setup.Operator`, `setup.RunVast`, and existing manual config behavior.
- Produces:

```go
type SetupPrompter interface {
    SelectProvider(context.Context) (string, error)
    ManualRoute(context.Context, string) (ManualRoute, error)
    VastIdentity(context.Context, string) (string, error)
}

type SetupDependencies struct {
    Prompter SetupPrompter
    Getenv func(string) string
    HomeDir func() (string, error)
    RunVast func(context.Context, string, string, setup.Operator) (setup.Result, error)
}

func Setup(context.Context, io.Reader, io.Writer, string, string, SetupDependencies) error
```

`HuhPrompter` implements both `SetupPrompter` and `setup.Operator`.

- [ ] **Step 1: Add Huh v0.6.0 without upgrading the existing Charm stack**

Run: `go get github.com/charmbracelet/huh@v0.6.0`

Expected: `go.mod` adds Huh v0.6.0 while retaining Bubbles v0.20.0, Bubble Tea v1.1.0, and Lip Gloss v0.13.0.

- [ ] **Step 2: Write failing provider-selection tests with fake prompts**

Replace raw line-oriented tests with fakes. Cover:

```go
func TestSetupKeepsManualSecureRoute(t *testing.T)
func TestSetupVastRequiresAPIKeyBeforeRunner(t *testing.T)
func TestSetupVastDefaultsToExistingEd25519Identity(t *testing.T)
func TestSetupVastPrintsResultAndStartNextStep(t *testing.T)
```

Manual assertions remain: saved config uses provider `manual`, fixed loopback route, selected user/key/known-hosts, and validates both files. Vast assertions: the fake runner receives the exact environment token and expanded identity path, while output contains instance ID, billing warning, and `Next: sovkit start`.

- [ ] **Step 3: Run setup tests and confirm failure**

Run: `go test ./internal/cli -run 'TestSetup' -count=1`

Expected: FAIL because setup has no provider abstraction or Vast runner.

- [ ] **Step 4: Implement provider-aware setup dispatch**

`Setup` asks for provider first. Manual uses `ManualRoute` answers and the existing `config.Save(config.Studio(...))`. Vast reads `VAST_API_KEY`, fails if blank, obtains `HomeDir`, defaults identity to `filepath.Join(home, ".ssh", "id_ed25519")`, asks for identity, and calls the injected Vast runner. Do not read the API key before Vast is selected.

Keep all environment lookup injectable. Do not put the token into an error, result, config, or output.

- [ ] **Step 5: Write failing Huh view-model tests**

Test pure option and label builders rather than driving a terminal. Assert provider labels are `Vast` and `Manual SSH`; offer labels contain GPU, normalized VRAM, `$x.xx/h`, monthly and annual scenario labels, location, reliability, and `unknown/unmeasured`; cost and host-key confirmation titles contain the irreversible billing and first-use trust language.

- [ ] **Step 6: Implement `HuhPrompter` with injectable streams**

Construct forms with `huh.NewSelect`, `huh.NewInput`, and `huh.NewConfirm`. Each form uses `WithInput(input)` and `WithOutput(output)`. Map `huh.ErrUserAborted` to a concise cancellation error. Do not embed side effects in Huh callbacks; forms only collect values and confirmations.

Use `huh.NewOption(label, value)` for provider and offer choices. The cost confirmation affirmative label is `Create paid instance`; the host-key affirmative label is `Trust this fingerprint`.

- [ ] **Step 7: Verify and commit the interactive setup UI**

Run: `go test ./internal/cli -count=1`

Expected: PASS without a live TTY.

```bash
git add go.mod go.sum internal/cli/setup.go internal/cli/setup_test.go internal/cli/prompts.go internal/cli/prompts_test.go
git commit -m "feat: add provider-aware setup prompts"
```

---

### Task 6: Add the foreground start lifecycle

**Files:**
- Modify: `internal/route/route.go`
- Modify: `internal/route/route_test.go`
- Create: `internal/cli/start.go`
- Create: `internal/cli/start_test.go`

**Interfaces:**
- Consumes: validated `config.Config`, strict `route.Command`, `route.Healthcheck`, and dashboard runner.
- Produces:

```go
type Tunnel interface {
    Start() error
    Done() <-chan error
    Stop() error
}

type StartDependencies struct {
    NewTunnel func(context.Context, config.Config, io.Writer) (Tunnel, error)
    Healthcheck func(context.Context, string) error
    RunDashboard func(io.Writer) error
    Clock setup.Clock
    PollInterval time.Duration
    PollTimeout time.Duration
}

func Start(context.Context, io.Writer, string, StartDependencies) error
```

- [ ] **Step 1: Add a context-aware route command test**

Add `CommandContext(ctx, cfg)` and assert its argument list remains byte-for-byte equivalent to `Command(cfg)`: `-N`, `BatchMode=yes`, `ExitOnForwardFailure=yes`, `IdentitiesOnly=yes`, `StrictHostKeyChecking=yes`, isolated known-hosts, identity, and fixed loopback forward.

- [ ] **Step 2: Run route tests and confirm failure**

Run: `go test ./internal/route -run 'TestCommandContext' -count=1`

Expected: FAIL because `CommandContext` does not exist.

- [ ] **Step 3: Implement context-aware command construction**

Make `Command` delegate to `CommandContext(context.Background(), cfg)` and use `exec.CommandContext` in the latter. Keep all current credential-file validation and SSH arguments unchanged.

- [ ] **Step 4: Write failing start lifecycle tests**

Use fake tunnel, clock, health function, and dashboard. Cover:

```go
func TestStartWaitsForHealthBeforeDashboard(t *testing.T)
func TestStartStopsTunnelWhenDashboardExits(t *testing.T)
func TestStartStopsTunnelWhenHealthTimesOut(t *testing.T)
func TestStartFailsImmediatelyWhenTunnelExits(t *testing.T)
func TestStartNeverTreatsHealthAsMeasuredPerformance(t *testing.T)
```

Assert event order for success:

```go
[]string{"tunnel-start", "health-fail", "sleep", "health-pass", "dashboard", "tunnel-stop"}
```

The output may say `LIVE: endpoint healthy`; it must not contain `MEASURED`, tokens/second, latency, or throughput.

- [ ] **Step 5: Run start tests and confirm failure**

Run: `go test ./internal/cli -run 'TestStart' -count=1`

Expected: FAIL because `Start` does not exist.

- [ ] **Step 6: Implement bounded foreground ownership**

Load config first and derive `http://127.0.0.1:30000`. Start the tunnel, then loop until health succeeds, the tunnel’s `Done` channel yields, context is canceled, or the 30-minute clock deadline is reached. Sleep five seconds between health attempts. Defer `Stop` immediately after successful start.

The production tunnel wraps `route.CommandContext`; `Start` starts `cmd`, a goroutine sends exactly one `cmd.Wait()` result to a buffered done channel, and `Stop` cancels the command context then waits for the done result. Treat expected cancellation after dashboard exit as cleanup, not a user-facing failure.

- [ ] **Step 7: Verify and commit start lifecycle**

Run: `go test ./internal/route ./internal/cli -count=1`

Expected: PASS.

```bash
git add internal/route/route.go internal/route/route_test.go internal/cli/start.go internal/cli/start_test.go
git commit -m "feat: add foreground private route start"
```

---

### Task 7: Wire production dependencies and preserve commands

**Files:**
- Modify: `cmd/sovkit/main.go`
- Modify: `cmd/sovkit/main_test.go`
- Modify: `cmd/sovkit/setup_test.go`
- Modify: `cmd/sovkit/commands_test.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: all interfaces from Tasks 1–6.
- Produces: production `setup`, new `start`, unchanged `catalog`, `dashboard`, `tunnel`, and `doctor` command behavior.

- [ ] **Step 1: Write failing command-routing tests**

Introduce an internal `application` dependency struct used by `runWith`. Add tests asserting:

- `setup` delegates to provider-aware setup with the current streams/config path/user;
- `start` delegates to foreground lifecycle;
- help contains `setup`, `start`, `dashboard`, `tunnel`, and `doctor`;
- manual setup remains selectable through the command entrypoint;
- no-argument/help paths construct no Vast client and start no process.

Use only fakes; never set a real `VAST_API_KEY` or run SSH.

- [ ] **Step 2: Run command tests and confirm failure**

Run: `go test ./cmd/sovkit -count=1`

Expected: FAIL because `start` and injectable production wiring do not exist.

- [ ] **Step 3: Construct the production Vast workflow at the executable boundary**

Use these fixed production values:

```go
const vastBaseURL = "https://console.vast.ai"

setup.Options{
    ConfigPath: configPath,
    KnownHostsDir: filepath.Join(filepath.Dir(configPath), "known_hosts"),
    OfferLimit: 5,
    PollInterval: 5 * time.Second,
    PollTimeout: 10 * time.Minute,
}
```

Load `recipes.QwenStudio()`, create `vast.NewClient(vastBaseURL, token)` only inside `NewAPI`, use `setup.RealClock`, `setup.SystemHostKeyScanner`, `setup.FileTrustStore`, `setup.StrictSSHLauncher`, `setup.ValidateIdentityFile`, and `config.Save`. Pass `os.Getenv` and `os.UserHomeDir` through injectable function fields.

Production `start` uses five-second health polling and a 30-minute deadline, the strict route command, `route.Healthcheck`, and `tea.NewProgram(catalogui.New(catalogui.DefaultEntries()), tea.WithOutput(output)).Run()`.

- [ ] **Step 4: Keep explicit diagnostic commands unchanged**

`dashboard` still opens the recipe browser without implicitly creating resources. `tunnel` still blocks on the strict forward. `doctor` still performs its existing three-second one-shot health check. Add no public endpoint or provider fallback.

- [ ] **Step 5: Update user-facing command documentation**

Update README’s primary journey to:

```text
export VAST_API_KEY='***'
sovkit setup
sovkit start
```

State that setup offers Vast and Manual SSH, that `start` owns the tunnel only while the dashboard is open, that quitting does not stop Vast billing, and that the API key is never stored. Preserve the independent host-fingerprint and strict-route warnings. Do not remove legacy documentation in this task.

- [ ] **Step 6: Run focused command verification**

Run: `go test ./cmd/sovkit ./internal/cli ./internal/setup -count=1`

Expected: PASS.

- [ ] **Step 7: Run the repository verification gate**

Run:

```text
go test ./...
go vet ./...
git diff --check
go run ./cmd/sovkit help
```

Expected:

- every test package passes;
- `go vet` prints no findings;
- `git diff --check` prints nothing;
- help lists the provider-aware `setup` and foreground `start` commands.

- [ ] **Step 8: Commit the integrated CLI**

```bash
git add cmd/sovkit/main.go cmd/sovkit/main_test.go cmd/sovkit/setup_test.go cmd/sovkit/commands_test.go README.md
git commit -m "feat: wire simple Vast setup and start"
```

---

### Task 8: Final security and contract review

**Files:**
- Review only: all files changed in Tasks 1–7
- Modify only if a review finding requires a focused correction

**Interfaces:**
- Consumes: completed implementation and approved design.
- Produces: evidence that the implementation preserves the private-route, cost-confirmation, and host-trust invariants.

- [ ] **Step 1: Trace every consequential call**

Verify from tests and code that `CreateInstance` has exactly one callsite, and every path to it passes through `ConfirmCost == true`. Verify the server launcher has exactly one callsite, and every path to it passes through `ConfirmHostKeys == true` and successful `TrustStore.Save`.

Use LSP references for exported and package-visible symbols where available; do not rely on text search for callsite completeness.

- [ ] **Step 2: Scan persisted shapes and output paths**

Confirm `config.Config` has no API/HF secret fields, errors never interpolate the token, host-key contents are not printed as config, and only fingerprints are presented for approval. Confirm known-hosts and TOML final modes are `0600`.

- [ ] **Step 3: Re-run the full verification gate**

Run:

```text
go test ./...
go vet ./...
git diff --check
go run ./cmd/sovkit help
```

Expected: all checks pass with the same evidence as Task 7.

- [ ] **Step 4: Commit only if review changed code**

If the review required a correction, commit only those reviewed files with a message naming the invariant fixed. If no files changed, create no empty commit.
