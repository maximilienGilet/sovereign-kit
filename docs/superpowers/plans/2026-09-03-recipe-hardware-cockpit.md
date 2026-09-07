# Recipe Hardware Cockpit Implementation Plan

> **For agentic workers:** Use executing-plans to implement this approved refinement inline, task-by-task. Review the final renderer independently before delivery.

**Goal:** Implement the reviewed hardware cockpit without invented metrics or terminal overflow.

**Architecture:** Keep the Bubbles state and lifecycle in model.go/picker.go. Move recipe dashboard composition into cockpit.go. Use Lip Gloss for every enclosure, Bubbles progress for motion, and existing ANSI helpers for display-cell sizing.

**Tech Stack:** Existing Go, Bubble Tea, Bubbles, Lip Gloss and Charm ANSI packages; no new dependencies.

## Global Constraints

- One active recipe; no comparisons, synthetic scores, provider queries or live telemetry.
- Current native-context provenance is always SOURCE UNKNOWN; pinned revision alone is insufficient.
- VRAM is per GPU; disk is per instance.
- Preserve current dirty worktree and browse/accessibility paths. Do not commit unrelated changes.
- Work on the existing feat/recipe-picker-tui branch.

### Task 1: Review fixes and regression tests

**Files:** Modify picker.go/picker_test.go; create cockpit_test.go under internal/catalogui.

**Interfaces:** The existing NewPicker([]Entry), Update(tea.Msg), View(), SelectedValue() remain unchanged.

- [x] Write a known-native/unknown-source regression, replacing the obsolete inline-audit requirement with full evidence-view access:

```go
model := NewPicker(pickerFixture())
view := model.View()
if !strings.Contains(view, "SOURCE UNKNOWN") || strings.Contains(view, "PINNED MODEL CONFIG") {
    t.Fatal(view)
}
```

- [x] Add a two-GPU fixture and assert `32 GB minimum / GPU`, not an inferred `64 GB` total; verify disk qualifier and full UseWhen text through the evidence viewport.
- [x] Run `rtk go test ./internal/catalogui -count=1` and observe the expected failures.
- [x] Update the evidence composition to include NAME, PURPOSE, source wording and all guidance. Use measured viewport width and wrapped facts, not clipped technical values.

### Task 2: Cockpit renderer and identity

**Files:** Create internal/catalogui/cockpit.go; modify model.go and picker.go; test cockpit_test.go.

**Interfaces:**
- Move `func (model Model) recipeDashboard(entry Entry, width int, wide bool) string` into cockpit.go.
- Add `func contextMarker(ratio float64, width int) string` for a target marker clamped into `[0,width-1]`.
- Add `func concurrencySlots(count, width int, color lipgloss.Color) string` with a bounded displayed count and exact remainder.
- Add `func recipeAccent(entry Entry) lipgloss.Color` deriving an identity from stable Value, not list order/status.
- Extract `func entryDelegate(mode modelMode, color lipgloss.Color) list.DefaultDelegate` to preserve shared list styling without changing global styles.

- [x] Write marker tests with literal expected positions: 12.5% at width 17 gives index 2; 100% gives index 16. Verify unknown context produces no proportional graph.
- [x] Write slot tests for 1, 5, unknown, and a large count. Check the exact total and bounded dimensions.
- [x] Write composition tests for three hero values, GPU enclosure, configured slots, no other-recipe metrics, and bounded reading width.
- [x] Run targeted tests red.
- [x] Implement hero strip, context scale, slots and hardware enclosure with Lip Gloss. The core composition is:

```go
zones := lipgloss.JoinHorizontal(lipgloss.Top, capacity, "   ", hardware)
content := strings.Join([]string{header, hero, zones, footer}, "\n")
if lipgloss.Height(content) > bodyHeight {
    return model.compactDashboard(entry, width)
}
return content
```

- [x] Apply a copied, active-entry accent to the picker delegate, progress.FullColor, badge, enclosure and action. Keep SetPercent/frame handling and immediate selection unchanged.
- [x] Run catalog tests green, including existing navigation and animation tests.

### Task 3: Responsive verification and review

**Files:** internal/catalogui/cockpit_test.go and picker_test.go; only scoped fixes in renderer if needed.

- [x] Test every builtin including custom at 30x10, 72x24, 108x30, 134x30, 150x34, 320x80; also long/CJK names, guidance, multi-GPU contracts and resize during animation.
- [x] Assert outer bounds, matched panel bottoms and visible action/footer; verify full omitted guidance remains scrollable in `i`.
- [x] Inspect actual text renders at 150x34 and 134x30, not just passing size assertions.
- [x] Request independent read-only review against the revised spec while running global checks.
- [x] Run `rtk go test ./... -count=1`, `rtk go vet ./...`, `rtk git diff --check` and the contributor checks that are locally runnable.
- [x] Fix important review findings and re-run affected tests. Leave code uncommitted when its diff overlaps existing user work; report the tested working-tree result.
