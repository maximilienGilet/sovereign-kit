package catalogui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func pickerFixture() []Entry {
	return []Entry{
		{
			Value: "solo", Name: "Qwen Solo", Kind: "text-generation", Status: "EXPERIMENTAL",
			Summary: "Private route for one developer", Evidence: "Historical candidate evidence only",
			UseWhen:         []string{"One private session is the target", "The configured ceilings cover the workload"},
			ModelRepository: "RadixArk/Qwen3.8-27B-NVFP4", ModelRevision: "319f741cce68d7914884900c138a1fbb70a42f30",
			Runtime: "sglang", GPUModel: "RTX 5090", GPUCount: 1, StrictGPU: true,
			ContextWindow: 32768, NativeContextWindow: 262144, MaxOutputTokens: 4096, MaxRunningRequests: 1,
			MinimumVRAMGB: 32, MinimumDiskGB: 100,
		},
		{
			Value: "studio", Name: "Qwen Studio", Kind: "text-generation", Status: "REFERENCE",
			Summary: "Long-context private route", Evidence: "Historical synthetic evidence",
			UseWhen:         []string{"Several private sessions must share one route"},
			ModelRepository: "RadixArk/Qwen3.8-27B-NVFP4", ModelRevision: "319f741cce68d7914884900c138a1fbb70a42f30",
			Runtime: "sglang", GPUModel: "RTX PRO 6000 S", GPUCount: 1, StrictGPU: true,
			ContextWindow: 262144, NativeContextWindow: 262144, MaxOutputTokens: 16384, MaxRunningRequests: 5,
			MinimumVRAMGB: 96, MinimumDiskGB: 120,
		},
	}
}

func TestPickerWideSidebarUsesAvailableSpaceWithoutWrappingTitles(t *testing.T) {
	model := NewPicker(DefaultEntries())
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 320, Height: 60})
	view := ansi.Strip(updated.(Model).View())
	lines := strings.Split(view, "\n")
	corner := strings.Index(lines[1], "╮")
	if corner < 0 || lipgloss.Width(lines[1][:corner])+1 < 48 {
		t.Fatalf("wide terminal leaves navigation too narrow: %s", lines[1])
	}
	for _, expected := range []string{"Qwen Studio  REFERENCE", "Qwen Solo — Full Context  EXPERIMENTAL", "Qwen Solo — Dual  EXPERIMENTAL", "Qwen Solo — Dual Max Lab  LAB", "Custom Hugging Face  CUSTOM"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("sidebar wraps or truncates title %q:\n%s", expected, view)
		}
	}
	if lipgloss.Width(lines[1]) != 320 {
		t.Fatalf("panel geometry loses terminal columns: got %d", lipgloss.Width(lines[1]))
	}
}

func TestPickerSelectedItemStaysWithinListWidth(t *testing.T) {
	entries := pickerFixture()
	entries[0].Summary = strings.Repeat("long description ", 10)
	model := NewPicker(entries)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 150, Height: 34})
	model = updated.(Model)
	for _, line := range strings.Split(model.list.View(), "\n") {
		if lipgloss.Width(line) > model.list.Width() {
			t.Fatalf("selected item exceeds its list width: %d > %d", lipgloss.Width(line), model.list.Width())
		}
	}
}

func TestPickerKeepsActionsAndPanelEdgesVisibleForEveryBuiltin(t *testing.T) {
	for _, size := range []struct{ width, height int }{{72, 24}, {134, 30}, {150, 34}, {320, 80}} {
		model := NewPicker(DefaultEntries())
		updated, _ := model.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
		model = updated.(Model)
		for index := range DefaultEntries() {
			view := ansi.Strip(model.View())
			lines := strings.Split(view, "\n")
			if !strings.Contains(lines[len(lines)-1], "choose") || !strings.Contains(view, "[ ") {
				t.Fatalf("%dx%d entry %d hides action/help:\n%s", size.width, size.height, index, view)
			}
			if strings.Count(lines[len(lines)-2], "╯") != 2 {
				t.Fatalf("%dx%d entry %d has uneven/clipped panels:\n%s", size.width, size.height, index, view)
			}
			updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
			model = updated.(Model)
		}
	}
}

func TestPickerAnimatesContextWithoutDelayingSelection(t *testing.T) {
	model := NewPicker(pickerFixture())
	command := model.Init()
	if command == nil {
		t.Fatal("picker does not start the context animation")
	}
	for frame := 0; command != nil && frame < 60; frame++ {
		updated, next := model.Update(command())
		model, command = updated.(Model), next
	}
	before := model.contextProgress(80)
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	if command == nil || model.progress.Percent() != 1 {
		t.Fatal("recipe change did not target the active recipe's context")
	}
	if model.contextProgress(80) != before {
		t.Fatal("bar jumped immediately instead of animating")
	}
	if !strings.Contains(model.View(), "262,144 / 262,144") {
		t.Fatal("numeric context was delayed by the animation")
	}
	for frame := 0; command != nil && frame < 60; frame++ {
		updated, next := model.Update(command())
		model, command = updated.(Model), next
	}
	if model.contextProgress(80) == before || model.progress.IsAnimating() {
		t.Fatal("animation frames were not applied or did not settle")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	updated, quit := updated.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	value, ok := updated.(Model).SelectedValue()
	if quit == nil || !ok || value != "solo" {
		t.Fatal("animation prevented immediate selection")
	}
}

func TestPickerReturnsActiveStableValue(t *testing.T) {
	model := NewPicker(pickerFixture())
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated, command := updated.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	selected, ok := updated.(Model).SelectedValue()
	if command == nil || !ok || selected != "studio" {
		t.Fatalf("selected=%q ok=%t command=%v", selected, ok, command)
	}
}

func TestPickerCancellationDoesNotSelect(t *testing.T) {
	model := NewPicker(pickerFixture())
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	final := updated.(Model)
	if command == nil || !final.Cancelled() {
		t.Fatalf("picker did not cancel: %#v", final)
	}
	if _, ok := final.SelectedValue(); ok {
		t.Fatal("cancelled picker returned a selection")
	}
}

func TestPickerRendersOnlyAuditableActiveRecipeMetrics(t *testing.T) {
	model := NewPicker(pickerFixture())
	model.width, model.height = 150, 34
	view := model.View()
	for _, expected := range []string{
		"CHOOSE A RECIPE", "Qwen Solo", "EXPERIMENTAL", "Private route for one developer",
		"32,768 / 262,144", "12.5%", "4,096", "1 request", "1× RTX 5090", "32 GB", "100 GB",
		"CHOOSE THIS RECIPE IF", "One private session is the target", "NOT MEASURED", "USE QWEN SOLO",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("picker missing %q:\n%s", expected, view)
		}
	}
	for _, forbidden := range []string{"DIRECT COMPARISON", "BEST RECIPE", "tok/s"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("picker contains unsupported comparison or claim %q:\n%s", forbidden, view)
		}
	}
}

func TestPickerNavigationRailDescribesPurposeWithoutComparingMetrics(t *testing.T) {
	model := NewPicker(pickerFixture())
	model.width, model.height = 150, 34
	view := model.View()
	if !strings.Contains(view, "Long-context private route") {
		t.Fatalf("navigation rail does not explain the recipe purpose:\n%s", view)
	}
	if strings.Contains(view, "96 GB VRAM") {
		t.Fatalf("navigation rail compares the inactive recipe's hardware:\n%s", view)
	}
}

func TestPickerKeepsFullConfigurationAuditInEvidence(t *testing.T) {
	model := NewPicker(pickerFixture())
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model = updated.(Model)
	view := model.View()
	for _, expected := range []string{
		"SOURCE UNKNOWN", "RadixArk/Qwen3.8-27B-NVFP4",
		"319f741cce68d7914884900c138a1fbb70a42f30", "sglang",
		"Historical candidate evidence only", "exact required",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("wide picker missing audit field %q:\n%s", expected, view)
		}
	}
}

func TestPickerOmitsProgressWhenNativeContextIsUnknown(t *testing.T) {
	entries := pickerFixture()[:1]
	entries[0].NativeContextWindow = 0
	model := NewPicker(entries)
	model.width, model.height = 150, 34
	view := model.View()
	if !strings.Contains(view, "NATIVE CONTEXT UNKNOWN") || strings.Contains(view, "12.5%") {
		t.Fatalf("unknown native context rendered a ratio:\n%s", view)
	}
}

func TestPickerEvidenceUsesViewportWithoutMovingRecipeSelection(t *testing.T) {
	model := NewPicker(pickerFixture())
	model.width, model.height = 100, 26
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model = updated.(Model)
	view := model.View()
	for _, expected := range []string{"EVIDENCE & TECHNICAL DETAILS", "319f741cce68d7914884900c138a1fbb70a42f30", "Historical candidate evidence only"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("evidence view missing %q:\n%s", expected, view)
		}
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if model.showDetail {
		t.Fatal("escape did not close evidence viewport")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	selected, ok := updated.(Model).SelectedValue()
	if !ok || selected != "solo" {
		t.Fatalf("viewport navigation changed recipe selection: %q", selected)
	}
}

func TestPickerEvidenceWrapsLongValuesAndScrolls(t *testing.T) {
	entries := pickerFixture()[:1]
	entries[0].Evidence = strings.Repeat("auditable evidence remains readable ", 18) + "TAIL MARKER"
	if wrapped := wrappedFact("EVIDENCE", entries[0].Evidence, 52); !strings.Contains(wrapped, "TAIL") || !strings.Contains(wrapped, "MARKER") {
		t.Fatalf("wrapping discarded evidence content:\n%s", wrapped)
	}
	model := NewPicker(entries)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 60, Height: 16})
	updated, _ = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model = updated.(Model)
	if strings.Contains(model.evidence.View(), "TAIL MARKER") {
		t.Fatal("fixture is not long enough to require viewport scrolling")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = updated.(Model)
	if model.evidence.YOffset == 0 {
		t.Fatal("page down did not scroll wrapped evidence")
	}
	if model.evidence.TotalLineCount() <= model.evidence.Height {
		t.Fatalf("wrapped evidence did not produce scrollable content: %d lines", model.evidence.TotalLineCount())
	}
}

func TestPickerQuitKeysAndEvidenceCloseSemantics(t *testing.T) {
	for _, keyMessage := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
		{Type: tea.KeyCtrlC},
	} {
		model := NewPicker(pickerFixture())
		updated, command := model.Update(keyMessage)
		if command == nil || !updated.(Model).Cancelled() {
			t.Fatalf("key %#v did not cancel the picker", keyMessage)
		}
	}

	model := NewPicker(pickerFixture())
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	updated, command := updated.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	final := updated.(Model)
	if command != nil || final.showDetail || final.Cancelled() {
		t.Fatalf("q should close evidence before cancelling: %#v", final)
	}
}

func TestPickerFitsSupportedTerminalSizes(t *testing.T) {
	for _, size := range []struct{ width, height int }{
		{30, 10}, {40, 16}, {71, 23}, {72, 24}, {108, 30}, {133, 30}, {134, 29}, {134, 30}, {150, 34},
	} {
		model := NewPicker(pickerFixture())
		updated, _ := model.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
		view := updated.(Model).View()
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > size.width {
				t.Fatalf("%dx%d line width %d exceeds terminal: %q", size.width, size.height, lipgloss.Width(line), line)
			}
		}
		if lipgloss.Height(view) > size.height {
			t.Fatalf("%dx%d view height %d exceeds terminal:\n%s", size.width, size.height, lipgloss.Height(view), view)
		}
		if size.width == 72 && size.height == 24 && !strings.Contains(view, "choose") {
			t.Fatalf("72x24 picker clipped its keyboard help:\n%s", view)
		}
	}
}
