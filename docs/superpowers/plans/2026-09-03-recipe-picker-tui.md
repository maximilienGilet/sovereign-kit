# Recipe Picker TUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the setup workload select with a responsive, Charmbracelet-native recipe picker that visualizes one active recipe without comparing it to other recipes.

**Architecture:** Extend `internal/catalogui.Model` with a picker mode while preserving its standalone browse mode. Use Bubbles list, progress, help, and viewport components inside the existing Bubble Tea/Lip Gloss model, then make `HuhPrompter.SelectWorkload` run that model only on the rich interactive path and retain Huh for accessible fallback.

**Tech Stack:** Go 1.22, Bubble Tea 1.1, Bubbles 0.20 (`list`, `progress`, `help`, `viewport`, `key`), Lip Gloss 0.13, Huh 0.6, TOML.

## Global Constraints

- The active detail screen contains no comparison, ranking, or score involving another recipe.
- Do not add a custom cursor, list, progress bar, terminal loop, help renderer, viewport, or ANSI renderer.
- A proportional chart is allowed only for configured context divided by verified native context from the same pinned model revision.
- Output, concurrency, VRAM, and disk remain exact absolute values.
- Unknown measurements render as `UNKNOWN` or `NOT MEASURED`, never as zero.
- Keep Huh for every setup step except the rich recipe picker.
- Preserve the existing custom Hugging Face sentinel value `huggingface`.
- Preserve unrelated working-tree changes.

---

## File Structure

- `internal/recipe/recipe.go`: own and validate native context and structured decision guidance.
- `internal/recipe/recipe_test.go`: validate optional native context and invalid ratios.
- `recipes/qwen-studio.toml`: declare Studio native context and use conditions.
- `recipes/qwen-solo-rtx5090.toml`: declare Solo native context and use conditions.
- `recipes/builtin_test.go`: require complete decision metadata on bundled recipes.
- `internal/catalogui/model.go`: keep shared entry, model, list, styles, and browse behavior.
- `internal/catalogui/picker.go`: own picker mode, key bindings, Bubbles progress/help/viewport composition, and selection state.
- `internal/catalogui/picker_test.go`: cover picker navigation, selection, cancellation, evidence viewport, and truthful rendering.
- `internal/catalogui/defaults.go`: map recipes into typed catalog entries; keep custom browse entry.
- `internal/catalogui/defaults_test.go`: cover typed recipe mapping.
- `internal/catalogui/sizing_test.go`: cover minimal, compact, and wide picker dimensions.
- `internal/cli/prompts.go`: run the rich picker and configure explicit Huh accessible fallback.
- `internal/cli/prompts_test.go`: cover picker entries, fallback accessibility, cancellation, and result mapping.

---

### Task 1: Add auditable recipe decision metadata

**Files:**
- Modify: `internal/recipe/recipe.go`
- Modify: `internal/recipe/recipe_test.go`
- Modify: `recipes/qwen-studio.toml`
- Modify: `recipes/qwen-solo-rtx5090.toml`
- Modify: `recipes/builtin_test.go`

**Interfaces:**
- Consumes: existing `recipe.Profile`, `recipe.Model`, `recipe.Serve.ContextWindow`, and TOML parsing.
- Produces: `Profile.UseWhen []string`, `Model.NativeContextWindow int`, and validation that later catalog rendering can trust.

- [ ] **Step 1: Write failing recipe validation tests**

Add tests that require native context to contain the configured context and require non-blank bundled guidance:

```go
func TestValidateRejectsNativeContextBelowConfiguredContext(t *testing.T) {
	r := Recipe{
		Version: 1, ID: "test", Name: "Test", Kind: "text-generation",
		Runtime: Runtime{Engine: "sglang", Image: "example/image@sha256:abc"},
		Model: Model{
			Repository: "org/model", Revision: "0123456789012345678901234567890123456789",
			NativeContextWindow: 16384,
		},
		Serve: Serve{ContextWindow: 32768, MaxOutputTokens: 1024, MaxRunningRequests: 1},
	}
	if err := r.Validate(); err == nil {
		t.Fatal("expected native context below configured context to be rejected")
	}
}

func TestCustomRecipeMayOmitNativeContextAndUseWhen(t *testing.T) {
	r, err := CustomHuggingFace("org/model", "319f741cce68d7914884900c138a1fbb70a42f30", false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Model.NativeContextWindow != 0 || len(r.Profile.UseWhen) != 0 {
		t.Fatalf("unexpected custom decision metadata: %#v", r)
	}
}
```

Update the bundled recipe tests to assert:

```go
if r.Model.NativeContextWindow != 262144 || len(r.Profile.UseWhen) == 0 {
	t.Fatalf("missing decision metadata: %#v", r)
}
for _, item := range r.Profile.UseWhen {
	if strings.TrimSpace(item) == "" {
		t.Fatalf("blank use_when item: %#v", r.Profile.UseWhen)
	}
}
```

- [ ] **Step 2: Run the focused recipe tests and confirm failure**

Run: `go test ./internal/recipe ./recipes -count=1`

Expected: FAIL because `UseWhen` and `NativeContextWindow` do not exist.

- [ ] **Step 3: Add the typed fields and structural validation**

Extend the existing types:

```go
type Profile struct {
	Status   string   `toml:"status"`
	Summary  string   `toml:"summary"`
	Evidence string   `toml:"evidence"`
	UseWhen  []string `toml:"use_when"`
}

type Model struct {
	Repository          string `toml:"repository"`
	Revision            string `toml:"revision"`
	Filename            string `toml:"filename"`
	TrustRemoteCode     bool   `toml:"trust_remote_code"`
	NativeContextWindow int    `toml:"native_context_window"`
}
```

Add validation after the positive server-limit check:

```go
if recipe.Model.NativeContextWindow < 0 ||
	(recipe.Model.NativeContextWindow > 0 && recipe.Model.NativeContextWindow < recipe.Serve.ContextWindow) {
	return fmt.Errorf("native context must contain the configured context window")
}
for _, item := range recipe.Profile.UseWhen {
	if strings.TrimSpace(item) == "" {
		return fmt.Errorf("recipe use_when entries cannot be blank")
	}
}
```

- [ ] **Step 4: Declare metadata in both bundled TOML recipes**

Use these values:

```toml
[profile]
use_when = [
  "One private developer session is the target workload",
  "The configured context and output ceilings cover the workload",
  "The exact required GPU is available",
]

[model]
native_context_window = 262144
```

Keep each recipe's existing status, summary, evidence, repository, and revision unchanged.

- [ ] **Step 5: Run focused tests and confirm success**

Run: `go test ./internal/recipe ./recipes -count=1`

Expected: PASS.

---

### Task 2: Give catalog entries one typed source of truth

**Files:**
- Modify: `internal/catalogui/model.go`
- Modify: `internal/catalogui/defaults.go`
- Modify: `internal/catalogui/defaults_test.go`
- Modify: `internal/catalogui/model_test.go`

**Interfaces:**
- Consumes: validated `recipe.Recipe` values.
- Produces: `catalogui.EntryFromRecipe(recipe.Recipe) Entry`, `Entry.Value`, and typed detail fields used by both browse and picker modes.

- [ ] **Step 1: Write failing adapter tests**

Require the adapter to preserve stable identity, typed limits, guidance, and source data:

```go
func TestEntryFromRecipePreservesTypedDecisionData(t *testing.T) {
	r, err := recipes.QwenSoloRTX5090()
	if err != nil {
		t.Fatal(err)
	}
	entry := EntryFromRecipe(r)
	if entry.Value != r.ID || entry.ContextWindow != 32768 || entry.NativeContextWindow != 262144 ||
		entry.MinimumVRAMGB != 32 || entry.GPUModel != "RTX 5090" || len(entry.UseWhen) != 3 {
		t.Fatalf("incomplete entry: %#v", entry)
	}
}
```

Update existing catalog tests to construct entries with the typed fields instead of formatted `VRAM`, `Context`, and `Detail` values.

- [ ] **Step 2: Run catalog tests and confirm failure**

Run: `go test ./internal/catalogui -count=1`

Expected: FAIL because the typed entry fields and adapter do not exist.

- [ ] **Step 3: Replace duplicated formatted metrics with typed entry fields**

Define one view model:

```go
type Entry struct {
	Value               string
	Name                string
	Kind                string
	Status              string
	Summary             string
	Evidence            string
	UseWhen             []string
	ModelRepository     string
	ModelRevision       string
	Runtime             string
	GPUModel            string
	GPUCount            int
	StrictGPU            bool
	ContextWindow       int
	NativeContextWindow int
	MaxOutputTokens     int
	MaxRunningRequests  int
	MinimumVRAMGB       int
	MinimumDiskGB       int
	Custom              bool
}
```

Implement `EntryFromRecipe` by copying the validated domain fields and formatting only the runtime summary. Copy `UseWhen` with `append([]string(nil), selected.Profile.UseWhen...)` so entries do not alias mutable caller slices.

Export `CustomEntry()` and keep the standalone custom entry as `Custom: true`, with `Value: "custom"` for browse mode, `Status: "CUSTOM"`, and explicit unknown copy. Setup supplies its own custom entry value later.

- [ ] **Step 4: Adapt the existing browse list and detail renderer**

Make `entryItem.Description` format typed fields at view time:

```go
func (item entryItem) Description() string {
	vram := countOrUnknown(item.MinimumVRAMGB, "GB")
	context := countOrUnknown(item.ContextWindow, "context")
	return compact(item.Kind) + " · " + vram + " VRAM · " + context
}
```

Keep the existing `sovkit catalog` headings, filtering, and navigation semantics.

- [ ] **Step 5: Run catalog tests and confirm success**

Run: `go test ./internal/catalogui -count=1`

Expected: PASS.

---

### Task 3: Add picker mode with Bubbles components

**Files:**
- Create: `internal/catalogui/picker.go`
- Create: `internal/catalogui/picker_test.go`
- Modify: `internal/catalogui/model.go`
- Modify: `internal/catalogui/sizing_test.go`

**Interfaces:**
- Consumes: typed `[]catalogui.Entry` from Task 2.
- Produces: `catalogui.NewPicker([]Entry) Model`, `Model.SelectedValue() (string, bool)`, and `Model.Cancelled() bool`.

- [ ] **Step 1: Write failing picker state tests**

Cover selection and cancellation:

```go
func TestPickerReturnsActiveStableValue(t *testing.T) {
	model := NewPicker([]Entry{{Value: "studio", Name: "Studio"}, {Value: "solo", Name: "Solo"}})
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated, command := updated.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	selected, ok := updated.(Model).SelectedValue()
	if command == nil || !ok || selected != "solo" {
		t.Fatalf("selected=%q ok=%t command=%v", selected, ok, command)
	}
}

func TestPickerCancellationDoesNotSelect(t *testing.T) {
	model := NewPicker([]Entry{{Value: "studio", Name: "Studio"}})
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if command == nil || !updated.(Model).Cancelled() {
		t.Fatalf("picker did not cancel: %#v", updated)
	}
	if _, ok := updated.(Model).SelectedValue(); ok {
		t.Fatal("cancelled picker returned a selection")
	}
}
```

- [ ] **Step 2: Write failing rendering and evidence tests**

Assert that one selected entry renders its own data, no other recipe's metrics, a `NOT MEASURED` throughput state, and a context percentage only when native context is known. Assert `i` opens evidence, navigation scrolls the viewport instead of the list, and `esc` closes evidence before it cancels the picker.

- [ ] **Step 3: Write failing size tests**

Render picker fixtures at `40x16`, `71x23`, `72x24`, `108x30`, `133x30`, `134x29`, and `134x30`. For every view, assert each line is at most the requested Lip Gloss width and total height is at most the requested height.

- [ ] **Step 4: Run picker tests and confirm failure**

Run: `go test ./internal/catalogui -run 'Picker|Catalog.*Fits' -count=1`

Expected: FAIL because picker mode does not exist.

- [ ] **Step 5: Add picker state and constructors to the shared model**

Add Bubbles component fields to `Model` and initialize them through a shared constructor:

```go
type modelMode int

const (
	browseMode modelMode = iota
	pickerMode
)

type Model struct {
	list       list.Model
	progress   progress.Model
	help       help.Model
	evidence   viewport.Model
	mode       modelMode
	width      int
	height     int
	selected   string
	cancelled  bool
	showDetail bool
}

func New(entries []Entry) Model { return newModel(entries, browseMode) }

func NewPicker(entries []Entry) Model {
	model := newModel(entries, pickerMode)
	model.list.SetFilteringEnabled(false)
	model.list.SetShowFilter(false)
	return model
}
```

Initialize `progress.New(progress.WithDefaultGradient())`, `help.New()`, and `viewport.New(1, 1)` from Bubbles. Keep the existing list delegate styling.

- [ ] **Step 6: Implement picker key bindings through `bubbles/key`**

Define bindings for choose, evidence, cancel, and quit. Pass list navigation to `list.Model.Update`; when the evidence viewport is open, pass scroll messages only to `viewport.Model.Update`. Return `tea.Quit` after setting selection or cancellation. `esc` closes the viewport first and cancels only from the main picker.

- [ ] **Step 7: Render minimal, compact, and wide picker views**

Dispatch with the established thresholds:

```go
func (model Model) pickerView() string {
	if model.width < 72 || model.height < 24 {
		return model.minimalPickerView()
	}
	if model.width >= 134 && model.height >= 30 {
		return model.widePickerView()
	}
	return model.compactPickerView()
}
```

Use `lipgloss.JoinHorizontal`, existing panel styles, and Bubbles help. Render the context bar with `model.progress.ViewAs(float64(entry.ContextWindow)/float64(entry.NativeContextWindow))` only when the denominator is positive. Render all other limits as absolute facts.

The decision section iterates `entry.UseWhen`; no recipe ID switch is allowed. The evidence screen uses Bubbles viewport content containing status, evidence, model, revision, runtime, and all exact limits.

- [ ] **Step 8: Run catalog tests and confirm success**

Run: `go test ./internal/catalogui -count=1`

Expected: PASS with browse and picker modes covered.

---

### Task 4: Integrate the picker into setup with a Huh fallback

**Files:**
- Modify: `internal/cli/prompts.go`
- Modify: `internal/cli/prompts_test.go`

**Interfaces:**
- Consumes: `catalogui.EntryFromRecipe`, `catalogui.NewPicker`, `Model.SelectedValue`, and existing `customHuggingFaceWorkload`.
- Produces: the unchanged `SelectWorkload(context.Context, []recipe.Recipe) (string, error)` behavior.

- [ ] **Step 1: Write failing picker-entry tests**

Add a helper test that builds setup entries and requires stable values:

```go
entries := workloadEntries([]recipe.Recipe{fixture})
if len(entries) != 2 || entries[0].Value != fixture.ID ||
	entries[1].Value != customHuggingFaceWorkload || !entries[1].Custom {
	t.Fatalf("unexpected entries: %#v", entries)
}
```

- [ ] **Step 2: Write failing rich-program result tests**

Extract a runner dependency so tests can return a final picker model without requiring a terminal:

```go
type recipePickerRunner func(context.Context, catalogui.Model, io.Reader, io.Writer) (catalogui.Model, error)
```

Test selected built-in value, custom sentinel, cancellation mapped to `setup cancelled`, and a runner error wrapped as `choose workload: ...`.

- [ ] **Step 3: Write failing accessible fallback tests**

Test `TERM=dumb` and `ACCESSIBLE=1` separately. Inject newline input and assert the Huh fallback returns the first option without alternate-screen escape sequences. Assert the `ACCESSIBLE` branch configures the form's accessible mode explicitly.

- [ ] **Step 4: Run focused CLI tests and confirm failure**

Run: `go test ./internal/cli -run 'Workload|RecipePicker|Accessible' -count=1`

Expected: FAIL because workload entries and the picker runner do not exist.

- [ ] **Step 5: Build setup entries without duplicating the custom sentinel**

Implement:

```go
func workloadEntries(available []recipe.Recipe) []catalogui.Entry {
	entries := make([]catalogui.Entry, 0, len(available)+1)
	for _, selected := range available {
		entries = append(entries, catalogui.EntryFromRecipe(selected))
	}
	custom := catalogui.CustomEntry()
	custom.Value = customHuggingFaceWorkload
	entries = append(entries, custom)
	return entries
}
```

`catalogui.CustomEntry` owns custom display data but accepts its setup value from the caller.

- [ ] **Step 6: Run the rich picker with injected streams**

The production runner uses:

```go
program := tea.NewProgram(
	model,
	tea.WithContext(ctx),
	tea.WithInput(input),
	tea.WithOutput(output),
	tea.WithAltScreen(),
)
result, err := program.Run()
```

Assert the result is `catalogui.Model`, map cancellation to `fmt.Errorf("setup cancelled")`, and return only a confirmed stable value.

- [ ] **Step 7: Preserve Huh and explicitly enable accessible mode**

Factor the existing select field into `workloadSelectField`. If `TERM == "dumb"` or `ACCESSIBLE != ""`, build the Huh form directly; call `WithAccessible(true)` for the `ACCESSIBLE` branch before `RunWithContext`. Keep existing labels and key map.

- [ ] **Step 8: Run focused CLI tests and confirm success**

Run: `go test ./internal/cli -run 'Workload|RecipePicker|Accessible' -count=1`

Expected: PASS.

---

### Task 5: Verify the complete setup surface

**Files:**
- Modify only if verification exposes a defect in files already listed above.

**Interfaces:**
- Consumes: all prior tasks.
- Produces: a verified recipe picker with unchanged setup orchestration semantics.

- [ ] **Step 1: Format modified Go files**

Run: `gofmt -w internal/recipe/recipe.go internal/recipe/recipe_test.go recipes/builtin_test.go internal/catalogui/model.go internal/catalogui/picker.go internal/catalogui/picker_test.go internal/catalogui/defaults.go internal/catalogui/defaults_test.go internal/catalogui/model_test.go internal/catalogui/sizing_test.go internal/cli/prompts.go internal/cli/prompts_test.go`

- [ ] **Step 2: Run focused package tests**

Run: `go test ./internal/recipe ./recipes ./internal/catalogui ./internal/cli -count=1`

Expected: PASS.

- [ ] **Step 3: Run the complete Go test suite**

Run: `go test ./... -count=1`

Expected: PASS. If an unrelated pre-existing dirty-worktree failure appears, record the exact package and failure without reverting unrelated changes.

- [ ] **Step 4: Run static analysis**

Run: `go vet ./...`

Expected: PASS.

- [ ] **Step 5: Inspect the final diff for scope and truthfulness**

Run: `git diff --check`

Run: `git diff -- internal/recipe recipes internal/catalogui internal/cli/prompts.go internal/cli/prompts_test.go`

Confirm that the diff contains no recipe-to-recipe comparison, no numeric throughput claim, no custom terminal control, and no unrelated reversal.
