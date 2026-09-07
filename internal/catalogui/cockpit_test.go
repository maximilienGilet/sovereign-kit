package catalogui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestCockpitDoesNotInferContextProvenanceFromPinnedRevision(t *testing.T) {
	model := NewPicker(pickerFixture())
	view := model.View()
	if !strings.Contains(view, "SOURCE UNKNOWN") || strings.Contains(view, "PINNED MODEL CONFIG") {
		t.Fatalf("a revision alone is not proof of context provenance:\n%s", view)
	}
}

func TestCockpitAccentFollowsSelectionWithoutChangingCatalog(t *testing.T) {
	entries := pickerFixture()
	browse := New(entries)
	beforeCatalog := browse.View()
	model := NewPicker(entries)
	first := model.progress.FullColor
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	if model.progress.FullColor == first {
		t.Fatal("recipe change did not update the animated bar's accent")
	}
	reordered := NewPicker([]Entry{entries[1], entries[0]})
	if reordered.progress.FullColor != model.progress.FullColor {
		t.Fatal("identity color depends on list order")
	}
	if browse.View() != beforeCatalog {
		t.Fatal("picker identity changed shared catalog styles")
	}
}

func TestContextMarkerAndTicksUseDisplayCellEndpoints(t *testing.T) {
	for _, test := range []struct {
		ratio           float64
		width, position int
	}{{0.125, 17, 2}, {1, 17, 16}, {0, 17, 0}, {1, 1, 0}} {
		marker := []rune(contextMarker(test.ratio, test.width))
		if len(marker) != test.width || marker[test.position] != '▼' {
			t.Fatalf("bad target marker: %q", string(marker))
		}
		ticks := []rune(contextTicks(test.width))
		if len(ticks) != test.width || ticks[0] != '┬' || ticks[len(ticks)-1] != '┬' {
			t.Fatalf("bad ruler bounds: %q", string(ticks))
		}
	}
}

func TestConfiguredSlotsAreExactBoundedAndNotLiveUsage(t *testing.T) {
	for _, count := range []int{1, 5, 10000} {
		view := ansi.Strip(concurrencySlots(count, 43, cyan))
		if !strings.Contains(view, requestCount(count)+" max") {
			t.Fatalf("missing configured total:\n%s", view)
		}
		boxes := strings.Count(view, "┌")
		if count <= 5 && boxes != count {
			t.Fatalf("%d requests produced %d slots", count, boxes)
		}
		if count > 5 && (boxes > 8 || !strings.Contains(view, fmt.Sprintf("+%s more", formatCatalogCount(count-boxes)))) {
			t.Fatalf("unbounded/inexact slots:\n%s", view)
		}
		if lipgloss.Width(view) > 43 {
			t.Fatalf("slots exceed available width:\n%s", view)
		}
	}
	unknown := concurrencySlots(0, 43, cyan)
	if !strings.Contains(unknown, "UNKNOWN") || strings.Contains(unknown, "┌") {
		t.Fatal("unknown count drew capacity")
	}
}

func TestSoloCapacityProfilesExposeKVCacheInCockpit(t *testing.T) {
	for _, entry := range DefaultEntries()[1:4] {
		for _, size := range []struct{ width, height int }{{72, 24}, {150, 34}} {
			model := NewPicker([]Entry{entry})
			updated, _ := model.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
			view := ansi.Strip(updated.(Model).View())
			want := fmt.Sprintf("%s K / %s V", entry.KVCacheTypeK, entry.KVCacheTypeV)
			if !strings.Contains(view, want) || !strings.Contains(view, requestCount(entry.MaxRunningRequests)) || !strings.Contains(view, formatCatalogCount(entry.ContextWindow)) {
				t.Fatalf("%s at %dx%d hides capacity contract %q:\n%s", entry.Name, size.width, size.height, want, view)
			}
			if lipgloss.Width(view) > size.width || lipgloss.Height(view) > size.height {
				t.Fatalf("%s overflows at %dx%d:\n%s", entry.Name, size.width, size.height, view)
			}
		}
	}
}

func TestDualMaxMinimalPickerExposesQ4Tradeoff(t *testing.T) {
	entry := DefaultEntries()[3]
	model := NewPicker([]Entry{entry})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	view := ansi.Strip(updated.(Model).View())
	if !strings.Contains(view, "KV") || !strings.Contains(view, "q4_0/q4_0") {
		t.Fatalf("minimal Dual Max view hides q4 cache tradeoff:\n%s", view)
	}
	if lipgloss.Width(view) > 30 || lipgloss.Height(view) > 10 {
		t.Fatalf("minimal Dual Max view overflows:\n%s", view)
	}
}

func TestCockpitResponsiveFallbackKeepsRequiredDataAndControls(t *testing.T) {
	entries := append(DefaultEntries(), pickerFixture()[0])
	last := &entries[len(entries)-1]
	last.Name = strings.Repeat("長い名前", 10)
	last.GPUModel = strings.Repeat("長いGPUモデル", 8)
	last.UseWhen = []string{strings.Repeat("long guidance\n", 10) + "TAIL"}
	for _, size := range []struct{ width, height int }{{30, 10}, {72, 24}, {108, 30}, {134, 30}, {150, 34}, {320, 80}} {
		for _, entry := range entries {
			model := NewPicker([]Entry{entry})
			updated, _ := model.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
			view := ansi.Strip(updated.(Model).View())
			lines := strings.Split(view, "\n")
			if lipgloss.Width(view) > size.width || len(lines) > size.height {
				t.Fatalf("overflow at %dx%d:\n%s", size.width, size.height, view)
			}
			if !strings.Contains(lines[len(lines)-1], "choose") || (!entry.Custom && !strings.Contains(view, "NOT MEASURED")) {
				t.Fatalf("required controls/data clipped at %dx%d:\n%s", size.width, size.height, view)
			}
			if size.width >= 72 && strings.Count(lines[len(lines)-2], "╯") != 2 {
				t.Fatalf("panel overflow at %dx%d:\n%s", size.width, size.height, view)
			}
			if size.width < 72 && !entry.Custom && !strings.Contains(view, "VRAM / GPU") {
				t.Fatalf("minimal view lost VRAM qualifier:\n%s", view)
			}
		}
	}
}

func TestCockpitLabelsMultiGPURequirementsWithoutInventingATotal(t *testing.T) {
	entries := pickerFixture()[:1]
	entries[0].GPUCount = 2
	model := NewPicker(entries)
	view := model.View()
	for _, want := range []string{"2× RTX 5090", "32 GB minimum / GPU", "100 GB minimum / instance"} {
		if !strings.Contains(view, want) {
			t.Fatalf("hardware contract missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "64 GB") {
		t.Fatal("cockpit invented an aggregate VRAM figure")
	}
}

func TestCockpitLongNameKeepsStatusInDashboardAndSidebar(t *testing.T) {
	entry := pickerFixture()[0]
	entry.Name = strings.Repeat("長い名前", 20)
	entry.Status = "EXPERIMENTAL"
	for _, size := range []struct{ width, height int }{{30, 10}, {72, 24}, {150, 34}} {
		model := NewPicker([]Entry{entry})
		updated, _ := model.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
		model = updated.(Model)
		if size.width < 72 {
			if !strings.Contains(strings.Split(ansi.Strip(model.View()), "\n")[0], "EXPERIMENTAL") {
				t.Fatal("minimal header lost the status")
			}
			continue
		}
		width := size.width - model.pickerSidebarWidth() - 1 - panel.GetHorizontalFrameSize()
		header := strings.Split(ansi.Strip(model.recipeDashboard(entry, width, size.width >= 134)), "\n")[0]
		if !strings.Contains(header, "EXPERIMENTAL") {
			t.Errorf("dashboard header lost status at width %d: %s", size.width, header)
		}
		if !strings.Contains(ansi.Strip(model.list.View()), "EXPERIMENTAL") {
			t.Errorf("sidebar lost status at width %d", size.width)
		}
	}
}

func TestCockpitMultilineNameCannotDisplaceFooter(t *testing.T) {
	entry := pickerFixture()[0]
	entry.Name = "First\nSecond\nThird\nFourth"
	model := NewPicker([]Entry{entry})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 72, Height: 24})
	view := ansi.Strip(updated.(Model).View())
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[len(lines)-1], "choose") || !strings.Contains(view, "USE FIRST SECOND THIRD FOURTH") {
		t.Fatalf("multiline action displaced controls:\n%s", view)
	}
}

func TestEvidenceRetainsPreferredGPUContract(t *testing.T) {
	entry := pickerFixture()[0]
	entry.StrictGPU = false
	model := NewPicker([]Entry{entry})
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if !strings.Contains(ansi.Strip(updated.(Model).View()), "1× RTX 5090 · preferred") {
		t.Fatal("evidence lost the preferred GPU policy")
	}
}

func TestEvidenceRecoversAllGuidanceOmittedFromTheCockpit(t *testing.T) {
	entries := pickerFixture()[:1]
	entries[0].UseWhen = append(entries[0].UseWhen, strings.Repeat("long guidance ", 20)+"RECOVERABLE_TAIL")
	model := NewPicker(entries)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 72, Height: 24})
	updated, _ = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model = updated.(Model)
	var seen strings.Builder
	for {
		seen.WriteString(ansi.Strip(model.evidence.View()))
		if model.evidence.AtBottom() {
			break
		}
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(Model)
	}
	for _, want := range []string{"Private route for one developer", "CHOOSE THIS RECIPE IF", "RECOVERABLE_TAIL"} {
		if !strings.Contains(seen.String(), want) {
			t.Fatalf("evidence loses omitted detail %q", want)
		}
	}
}

func TestCockpitComposesCapacityWithoutOtherRecipeMetrics(t *testing.T) {
	model := NewPicker(pickerFixture())
	view := ansi.Strip(model.View())
	for _, want := range []string{"CONFIGURED CAPACITY", "ACCELERATOR CONTRACT", "▼", "32,768 / 262,144", "not live usage"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing cockpit element %q:\n%s", want, view)
		}
	}
	if strings.Count(view, "32,768") != 1 || strings.Count(view, "4,096") != 1 {
		t.Fatal("configured context and output must have a single placement")
	}
	for _, forbidden := range []string{"96 GB", "16,384", "CONFIGURATION AUDIT"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("cockpit contains unrelated inline content %q", forbidden)
		}
	}
}

func TestCockpitBoundsTheGraphicOnUltrawideTerminals(t *testing.T) {
	model := NewPicker(pickerFixture())
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 320, Height: 80})
	model = updated.(Model)
	content := model.recipeDashboard(pickerFixture()[0], 249, true)
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "▼") && lipgloss.Width(strings.TrimSpace(ansi.Strip(line))) > 160 {
			t.Fatal("ruler stretches beyond the reading width")
		}
	}
	if strings.Contains(content, strings.Repeat("─", 161)) {
		t.Fatal("context bar spans the ultrawide panel")
	}
}
